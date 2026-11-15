package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// ---------------------------------------------------------------- actions

type leaveParams struct {
	Kind     string `json:"kind"`
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
	Comment  string `json:"comment,omitempty"`
}

type accessParams struct {
	System        string `json:"system"`
	Resource      string `json:"resource"`
	AccessLevel   string `json:"access_level"`
	Justification string `json:"justification"`
}

type questionParams struct {
	Question string `json:"question"`
	Context  string `json:"context,omitempty"`
}

var accessSystems = map[string]bool{"jira": true, "confluence": true, "gitlab": true, "grafana": true, "redash": true, "kubernetes": true}

// accessLevels are the levels an access request may name at all (contract enum). Which of them a role may actually
// receive is decided by accessLevelAllowed when the request is confirmed.
var accessLevels = map[string]bool{"read": true, "write": true, "admin": true}

// Limits from contracts (AccessRequestParams, LeaveRequestParams), in characters.
const (
	maxAccessResource      = 200
	maxAccessJustification = 500
	maxLeaveComment        = 500
)

// accessLevelAllowed is the self-service policy: no role gets admin access through a request (demo; admin rights are
// granted by the resource owner outside the assistant). read and write are allowed for every role.
func accessLevelAllowed(role model.UserRole, level string) bool {
	switch level {
	case "read", "write":
		return role == model.RoleIntern || role == model.RoleBuddy || role == model.RoleTeamlead
	}
	return false
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// validateParams re-checks what the model proposed: the backend, not the LLM, decides what is valid.
func validateParams(kind string, raw json.RawMessage) (any, error) {
	switch kind {
	case "leave_request":
		var p leaveParams
		if json.Unmarshal(raw, &p) != nil || (p.Kind != "day_off" && p.Kind != "vacation") || runeLen(p.Comment) > maxLeaveComment {
			return nil, ErrActionInvalid
		}
		from, err1 := time.Parse("2006-01-02", p.DateFrom)
		to, err2 := time.Parse("2006-01-02", p.DateTo)
		if err1 != nil || err2 != nil || to.Before(from) || to.Sub(from) > 60*24*time.Hour {
			return nil, ErrActionInvalid
		}
		return p, nil
	case "access_request":
		var p accessParams
		if json.Unmarshal(raw, &p) != nil || !accessSystems[p.System] || !accessLevels[p.AccessLevel] {
			return nil, ErrActionInvalid
		}
		resource, justification := strings.TrimSpace(p.Resource), strings.TrimSpace(p.Justification)
		if resource == "" || runeLen(resource) > maxAccessResource || justification == "" || runeLen(justification) > maxAccessJustification {
			return nil, ErrActionInvalid
		}
		return p, nil
	case "mentor_question":
		var p questionParams
		if json.Unmarshal(raw, &p) != nil || strings.TrimSpace(p.Question) == "" {
			return nil, ErrActionInvalid
		}
		return p, nil
	}
	return nil, ErrActionInvalid
}

func (s *AssistantService) action(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, userID, actionID uuid.UUID, lock bool) (PendingAction, json.RawMessage, error) {
	query := `SELECT id, type, params, summary, status, error, created_at, result FROM assistant_actions WHERE id = $1 AND user_id = $2`
	if lock {
		query += ` FOR UPDATE`
	}
	var a PendingAction
	var result []byte
	err := q.QueryRowContext(ctx, query, actionID, userID).
		Scan(&a.ID, &a.Type, &a.Params, &a.Summary, &a.Status, &a.Error, &a.CreatedAt, &result)
	if errors.Is(err, sql.ErrNoRows) {
		return a, nil, repository.ErrNotFound
	}
	return a, result, err
}

// Confirm executes a pending action once (idempotent: a repeated confirm returns the stored result).
func (s *AssistantService) Confirm(ctx context.Context, user *model.User, actionID uuid.UUID) (ActionResult, error) {
	var out ActionResult
	err := withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		a, stored, err := s.action(ctx, tx, user.ID, actionID, true)
		if err != nil {
			return err
		}
		switch a.Status {
		case "executed":
			return json.Unmarshal(stored, &out)
		case "cancelled":
			return ErrActionCancelled
		case "failed":
			return ErrActionInvalid
		}
		params, err := validateParams(a.Type, a.Params)
		if err == nil && a.Type == "access_request" && !accessLevelAllowed(user.Role, params.(accessParams).AccessLevel) {
			// policy layer: the level is checked against the role, whatever the model proposed
			msg := "Доступ admin через заявку не выдаётся"
			if user.Role == model.RoleIntern {
				msg = "Стажёрам доступ admin не выдаётся"
			}
			if _, uerr := tx.ExecContext(ctx, `UPDATE assistant_actions SET status = 'failed', error = $2, updated_at = now() WHERE id = $1`, a.ID, msg); uerr != nil {
				return uerr
			}
			a.Status, a.Error = "failed", &msg
			out = ActionResult{Action: a}
			return nil
		}
		if err != nil {
			return err
		}
		a.Status = "executed"
		out = ActionResult{Action: a}
		if a.Type == "mentor_question" {
			q := params.(questionParams)
			mq := MentorQuestion{InternID: user.ID, InternName: fullName(user.FirstName, user.LastName), Question: q.Question,
				Context: q.Context, Status: "open", EscalationLevel: "mentor"}
			if err := tx.QueryRowContext(ctx, `
				INSERT INTO mentor_questions (intern_id, buddy_id, action_id, question, context)
				VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
				user.ID, user.BuddyID, a.ID, truncate(q.Question, 2000), truncate(q.Context, 2000)).Scan(&mq.ID, &mq.CreatedAt); err != nil {
				return fmt.Errorf("store question: %w", err)
			}
			mq.EscalatedAt = mq.CreatedAt
			out.MentorQuestion = &mq
		} else {
			prefix, approver := "ACC", "Владелец ресурса (демо)"
			if a.Type == "leave_request" {
				prefix, approver = "ABS", s.leaveApprover(ctx, tx, user)
			}
			var r ServiceRequest
			if err := tx.QueryRowContext(ctx, `
				INSERT INTO service_requests (user_id, action_id, number, type, approver, params)
				VALUES ($1, $2, $3 || '-' || lpad(nextval('service_request_number_seq')::text, 6, '0'), $4, $5, $6)
				RETURNING id, number, type, status, approver, params, created_at, updated_at`,
				user.ID, a.ID, prefix, a.Type, approver, []byte(a.Params)).
				Scan(&r.ID, &r.Number, &r.Type, &r.Status, &r.Approver, &r.Params, &r.CreatedAt, &r.UpdatedAt); err != nil {
				return fmt.Errorf("store request: %w", err)
			}
			out.Request = &r
		}
		result, _ := json.Marshal(out)
		_, err = tx.ExecContext(ctx, `UPDATE assistant_actions SET status = 'executed', result = $2, updated_at = now() WHERE id = $1`, a.ID, result)
		return err
	})
	return out, err
}

func (s *AssistantService) leaveApprover(ctx context.Context, tx *sql.Tx, user *model.User) string {
	if user.TeamleadID != nil {
		var first, last *string
		if tx.QueryRowContext(ctx, `SELECT first_name, last_name FROM users WHERE id = $1`, *user.TeamleadID).Scan(&first, &last) == nil {
			if name := fullName(first, last); name != "" {
				return name + " (руководитель)"
			}
		}
	}
	return "Руководитель команды (демо)"
}

// CreateRequest creates a leave/access request from the form in the chat window. The form submit is the explicit
// confirmation: the same validation, the same policy (no admin for interns) and the same numbering as a
// confirmed chat action. It goes through an assistant action so the request keeps one audit trail.
func (s *AssistantService) CreateRequest(ctx context.Context, user *model.User, kind string, params json.RawMessage) (ActionResult, error) {
	if kind != "leave_request" && kind != "access_request" {
		return ActionResult{}, ErrActionInvalid
	}
	if _, err := validateParams(kind, params); err != nil {
		return ActionResult{}, err
	}
	proposal, _ := json.Marshal(map[string]any{"proposal": map[string]any{"type": kind, "params": params, "summary": "Заявка из формы"}})
	a, err := s.storeProposal(ctx, user.ID, proposal)
	if err != nil {
		return ActionResult{}, err
	}
	return s.Confirm(ctx, user, a.ID)
}

// CreateQuestion sends a question to the user's buddy from the "Вопросы наставнику" tab (no chat needed): the same
// validation and storage as a mentor_question the user confirmed in the chat.
func (s *AssistantService) CreateQuestion(ctx context.Context, user *model.User, question string) (MentorQuestion, error) {
	params, _ := json.Marshal(map[string]string{"question": strings.TrimSpace(question), "context": "Вопрос из вкладки «Вопросы наставнику»"})
	if _, err := validateParams("mentor_question", params); err != nil {
		return MentorQuestion{}, err
	}
	proposal, _ := json.Marshal(map[string]any{"proposal": map[string]any{"type": "mentor_question", "params": json.RawMessage(params), "summary": "Вопрос наставнику"}})
	a, err := s.storeProposal(ctx, user.ID, proposal)
	if err != nil {
		return MentorQuestion{}, err
	}
	result, err := s.Confirm(ctx, user, a.ID)
	if err != nil {
		return MentorQuestion{}, err
	}
	if result.MentorQuestion == nil {
		return MentorQuestion{}, ErrActionInvalid
	}
	return *result.MentorQuestion, nil
}

func (s *AssistantService) Cancel(ctx context.Context, userID, actionID uuid.UUID) (PendingAction, error) {
	var a PendingAction
	err := withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		var err error
		a, _, err = s.action(ctx, tx, userID, actionID, true)
		if err != nil {
			return err
		}
		if a.Status == "executed" {
			return ErrActionExecuted
		}
		a.Status = "cancelled"
		_, err = tx.ExecContext(ctx, `UPDATE assistant_actions SET status = 'cancelled', updated_at = now() WHERE id = $1`, a.ID)
		return err
	})
	return a, err
}
