package service

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// AI assistant: the backend is the only door to the Python AI service. It builds a verified UserContext from the
// database (the model never sees data the user may not see), relays the AI event stream, stores the conversation
// and turns an ActionProposal into a PendingAction that runs only after the user confirms it.
// Chat texts are never logged: only ids, statuses and latencies.

var (
	ErrActionCancelled   = errors.New("action cancelled")
	ErrActionExecuted    = errors.New("already executed")
	ErrActionInvalid     = errors.New("invalid action params")
	ErrAIUnavailable     = errors.New("AI unavailable")
	ErrNotBuddy          = errors.New("not a buddy")
	ErrQuestionAnswered  = errors.New("question already answered")
	ErrInvalidDecision   = errors.New("status must be approved or rejected")
	ErrRequestNotPending = errors.New("request already decided")
)

const (
	historyTurns    = 20
	maxContextTasks = 30
	minutesPerReply = 5 // mentor minutes a resolved question saves (assumption shown in metrics)
)

type AssistantService struct {
	db     *sql.DB
	aiURL  string
	stream *http.Client // no overall timeout: the request context bounds the stream
	short  *http.Client
}

func NewAssistantService(db *sql.DB, aiURL string) *AssistantService {
	return &AssistantService{
		db:     db,
		aiURL:  strings.TrimRight(aiURL, "/"),
		stream: &http.Client{},
		short:  &http.Client{Timeout: 3 * time.Minute},
	}
}

// ---------------------------------------------------------------- DTOs (contracts/backend.openapi.json)

type PendingAction struct {
	ID        uuid.UUID       `json:"id"`
	Type      string          `json:"type"`
	Params    json.RawMessage `json:"params"`
	Summary   string          `json:"summary"`
	Status    string          `json:"status"`
	Error     *string         `json:"error,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type ServiceRequest struct {
	ID        uuid.UUID       `json:"id"`
	Number    string          `json:"number"`
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	Approver  string          `json:"approver,omitempty"`
	Params    json.RawMessage `json:"params"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type MentorQuestion struct {
	ID              uuid.UUID  `json:"id"`
	InternID        uuid.UUID  `json:"intern_id"`
	InternName      string     `json:"intern_name,omitempty"`
	Question        string     `json:"question"`
	Context         string     `json:"context,omitempty"`
	Status          string     `json:"status"`
	EscalationLevel string     `json:"escalation_level"`
	EscalatedAt     time.Time  `json:"escalated_at"`
	Answer          *string    `json:"answer,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	AnsweredAt      *time.Time `json:"answered_at,omitempty"`
}

type ActionResult struct {
	Action         PendingAction   `json:"action"`
	Request        *ServiceRequest `json:"request,omitempty"`
	MentorQuestion *MentorQuestion `json:"mentor_question,omitempty"`
}

type ChatMessage struct {
	ID           uuid.UUID       `json:"id"`
	Role         string          `json:"role"`
	Content      string          `json:"content"`
	Sources      json.RawMessage `json:"sources,omitempty"`
	Action       *PendingAction  `json:"action,omitempty"`
	FinishReason *string         `json:"finish_reason,omitempty"`
	Feedback     *string         `json:"feedback"`
	CreatedAt    time.Time       `json:"created_at"`
}

type ChatMetrics struct {
	QuestionsTotal       int      `json:"questions_total"`
	AnsweredByBot        int      `json:"answered_by_bot"`
	EscalatedToMentor    int      `json:"escalated_to_mentor"`
	DeflectionRate       *float64 `json:"deflection_rate"`
	ThumbsUp             int      `json:"thumbs_up"`
	ThumbsDown           int      `json:"thumbs_down"`
	ActionsExecuted      int      `json:"actions_executed"`
	EstimatedMinutesSave int      `json:"estimated_mentor_minutes_saved"`
}

type PageContext struct {
	URL       string `json:"url"`
	Title     string `json:"title,omitempty"`
	Selection string `json:"selection,omitempty"`
}

type taskContext struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	Status         string `json:"status"`
	Checklist      string `json:"checklist"`
	StageTitle     string `json:"stage_title,omitempty"`
	RequiresResult bool   `json:"requires_result,omitempty"`
	ReviewComment  string `json:"review_comment,omitempty"`
}

type stageContext struct {
	Title    string `json:"title"`
	Position int    `json:"position"`
	Total    int    `json:"total"`
}

type UserContext struct {
	UserID         string           `json:"user_id"`
	DisplayName    string           `json:"display_name,omitempty"`
	Role           string           `json:"role"`
	Profession     *string          `json:"profession"`
	Team           string           `json:"team,omitempty"`
	BuddyName      string           `json:"buddy_name,omitempty"`
	Today          string           `json:"today"`
	CurrentStage   *stageContext    `json:"current_stage,omitempty"`
	Tasks          []taskContext    `json:"tasks"`
	OpenRequests   []ServiceRequest `json:"open_requests"`
	AllowedActions []string         `json:"allowed_actions"`
}

type chatTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ---------------------------------------------------------------- user context

func fullName(first, last *string) string {
	var parts []string
	for _, p := range []*string{first, last} {
		if p != nil && strings.TrimSpace(*p) != "" {
			parts = append(parts, strings.TrimSpace(*p))
		}
	}
	return strings.Join(parts, " ")
}

// UserContext is built only from the user's own rows: RBAC lives here, not in the prompt.
func (s *AssistantService) UserContext(ctx context.Context, user *model.User) (UserContext, error) {
	uc := UserContext{
		UserID: user.ID.String(), Role: "intern", Today: time.Now().Format("2006-01-02"),
		Tasks: []taskContext{}, OpenRequests: []ServiceRequest{},
		AllowedActions: []string{"leave_request", "access_request", "mentor_question"},
	}
	if user.FirstName != nil {
		uc.DisplayName = strings.TrimSpace(*user.FirstName)
	}
	if user.Team != nil {
		uc.Team = *user.Team
	}
	if user.Profession != nil {
		p := string(*user.Profession)
		uc.Profession = &p
	}
	if user.Role != model.RoleIntern {
		uc.Role, uc.AllowedActions = "buddy", []string{"leave_request", "access_request"}
	}
	if user.BuddyID != nil {
		var first, last *string
		err := s.db.QueryRowContext(ctx, `SELECT first_name, last_name FROM users WHERE id = $1`, *user.BuddyID).Scan(&first, &last)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return uc, fmt.Errorf("load buddy: %w", err)
		}
		uc.BuddyName = fullName(first, last)
	}
	if err := s.loadTasks(ctx, user.ID, &uc); err != nil {
		return uc, err
	}
	requests, err := s.ListRequests(ctx, user.ID, 20)
	if err != nil {
		return uc, err
	}
	uc.OpenRequests = requests
	return uc, nil
}

func (s *AssistantService) loadTasks(ctx context.Context, userID uuid.UUID, uc *UserContext) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id, i.title, i.description, p.status, c.type, st.id, st.title, st.position, i.requires_review,
		       sub.status, sub.review_comment,
		       (SELECT count(*) FROM checklist_stages x WHERE x.checklist_id = c.id)
		FROM user_checklist_progress p
		JOIN checklist_items i ON i.id = p.checklist_item_id
		JOIN checklists c ON c.id = p.checklist_id
		JOIN checklist_stages st ON st.id = i.stage_id
		LEFT JOIN LATERAL (
			SELECT s.status, s.review_comment FROM checklist_item_submissions s
			WHERE s.progress_id = p.id ORDER BY s.attempt_number DESC LIMIT 1
		) sub ON true
		WHERE p.user_id = $1
		ORDER BY c.type DESC, st.position, p.position`, userID)
	if err != nil {
		return fmt.Errorf("load tasks: %w", err)
	}
	defer rows.Close()
	var current, later, done []taskContext
	for rows.Next() {
		var t taskContext
		var stageID uuid.UUID
		var stagePos, stageTotal int
		var subStatus, comment sql.NullString
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Checklist, &stageID, &t.StageTitle,
			&stagePos, &t.RequiresResult, &subStatus, &comment, &stageTotal); err != nil {
			return fmt.Errorf("scan task: %w", err)
		}
		if len(t.Description) > 600 {
			t.Description = string([]rune(t.Description)[:300]) + "…"
		}
		switch {
		case t.Status == "verified":
			t.Status = "completed"
		case t.Status == "active" && subStatus.String == "pending":
			t.Status = "on_review"
		case t.Status == "active" && subStatus.String == "rejected":
			t.Status, t.ReviewComment = "returned", comment.String
		}
		if t.Status == "completed" && comment.Valid {
			t.ReviewComment = comment.String
		}
		switch t.Status {
		case "active", "on_review", "returned":
			current = append(current, t)
			// the professional checklist (sorted first) defines the stage the intern is in
			if uc.CurrentStage == nil {
				uc.CurrentStage = &stageContext{Title: t.StageTitle, Position: stagePos, Total: stageTotal}
			}
		case "unavailable":
			later = append(later, t)
		default:
			done = append(done, t)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read tasks: %w", err)
	}
	// Current work first, then what comes next, then finished tasks (the brief mentions review comments on them).
	for _, group := range [][]taskContext{current, later, done} {
		for _, t := range group {
			if len(uc.Tasks) < maxContextTasks {
				uc.Tasks = append(uc.Tasks, t)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- conversations

// StartTurn checks the conversation owner, creates one when needed, returns prior turns and stores the user message.
func (s *AssistantService) StartTurn(ctx context.Context, userID uuid.UUID, conversationID *uuid.UUID, message string) (uuid.UUID, []chatTurn, error) {
	id, err := s.conversation(ctx, userID, conversationID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT role, content FROM (
			SELECT role, content, created_at FROM chat_messages WHERE conversation_id = $1
			ORDER BY created_at DESC LIMIT $2
		) last ORDER BY created_at`, id, historyTurns)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("load history: %w", err)
	}
	defer rows.Close()
	history := []chatTurn{}
	for rows.Next() {
		var t chatTurn
		if err := rows.Scan(&t.Role, &t.Content); err != nil {
			return uuid.Nil, nil, err
		}
		if strings.TrimSpace(t.Content) != "" {
			history = append(history, chatTurn{Role: t.Role, Content: truncate(t.Content, 8000)})
		}
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, nil, err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO chat_messages (conversation_id, role, content) VALUES ($1, 'user', $2)`, id, message); err != nil {
		return uuid.Nil, nil, fmt.Errorf("store message: %w", err)
	}
	return id, history, nil
}

// conversation returns the given conversation if it belongs to the user, or the user's latest/new one when nil.
func (s *AssistantService) conversation(ctx context.Context, userID uuid.UUID, requested *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	if requested != nil {
		err := s.db.QueryRowContext(ctx, `SELECT id FROM chat_conversations WHERE id = $1 AND user_id = $2`, *requested, userID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, repository.ErrNotFound
		}
		return id, err
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO chat_conversations (user_id) VALUES ($1) RETURNING id`, userID).Scan(&id)
	return id, err
}

func (s *AssistantService) latestConversation(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRowContext(ctx, `SELECT id FROM chat_conversations WHERE user_id = $1 ORDER BY updated_at DESC LIMIT 1`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// BriefConversation puts the proactive brief into the user's latest conversation (or a new one), so the chat
// that follows continues it.
func (s *AssistantService) BriefConversation(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	latest, err := s.latestConversation(ctx, userID)
	if err != nil {
		return uuid.Nil, err
	}
	if latest != nil {
		return *latest, nil
	}
	return s.conversation(ctx, userID, nil)
}

// Emit writes one SSE event to the client.
type Emit func(event string, data []byte) error

// Relay calls an AI streaming endpoint and re-emits its events. The first event is `meta`; an AI `action`
// (ActionProposal) is stored as a PendingAction and emitted as {"action": PendingAction}. The assistant message is
// stored when the stream ends, even if the client went away.
func (s *AssistantService) Relay(ctx context.Context, userID, conversationID uuid.UUID, path string, payload any, emit Emit) error {
	messageID := uuid.New()
	meta, _ := json.Marshal(map[string]string{"conversation_id": conversationID.String(), "message_id": messageID.String()})
	if err := emit("meta", meta); err != nil {
		return err
	}
	started := time.Now()
	var text strings.Builder
	var sources json.RawMessage
	var actionID *uuid.UUID
	var finish *string
	status := "ok"

	body, _ := json.Marshal(payload)
	streamCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodPost, s.aiURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := s.stream.Do(req)
	if err == nil && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		err = fmt.Errorf("AI HTTP %d", resp.StatusCode)
	}
	if err != nil {
		status = "ai_unavailable"
		errData, _ := json.Marshal(map[string]string{"code": "llm_unavailable", "message": "AI-сервис недоступен. Попробуй ещё раз чуть позже."})
		_ = emit("error", errData)
	} else {
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		event := ""
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(line[len("event:"):])
			case strings.HasPrefix(line, "data:"):
				data := []byte(strings.TrimSpace(line[len("data:"):]))
				switch event {
				case "delta":
					var d struct {
						Text string `json:"text"`
					}
					if json.Unmarshal(data, &d) == nil {
						text.WriteString(d.Text)
					}
				case "sources":
					var d struct {
						Sources json.RawMessage `json:"sources"`
					}
					if json.Unmarshal(data, &d) == nil {
						sources = d.Sources
					}
				case "done":
					var d struct {
						FinishReason string `json:"finish_reason"`
					}
					if json.Unmarshal(data, &d) == nil && d.FinishReason != "" {
						finish = &d.FinishReason
					}
				case "error":
					status = "ai_error"
				case "action":
					action, perr := s.storeProposal(ctx, userID, data)
					if perr != nil {
						log.Printf("assistant: store action proposal failed: %v", perr)
						status = "action_error"
						continue
					}
					actionID = &action.ID
					data, _ = json.Marshal(map[string]PendingAction{"action": action})
				}
				if err := emit(event, data); err != nil {
					status = "client_gone"
				}
			}
		}
		if err := scanner.Err(); err != nil && status == "ok" {
			status = "stream_broken"
			errData, _ := json.Marshal(map[string]string{"code": "timeout", "message": "Ответ прервался. Попробуй ещё раз."})
			_ = emit("error", errData)
		}
	}
	// Persist with a fresh context: the client may have disconnected mid-stream.
	saveCtx, cancelSave := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelSave()
	if text.Len() > 0 || actionID != nil {
		if _, err := s.db.ExecContext(saveCtx, `
			INSERT INTO chat_messages (id, conversation_id, role, content, sources, action_id, finish_reason)
			VALUES ($1, $2, 'assistant', $3, $4, $5, $6)`,
			messageID, conversationID, truncate(text.String(), 16000), nullJSON(sources), actionID, finish); err != nil {
			log.Printf("assistant: store reply failed: %v", err)
		}
	}
	_, _ = s.db.ExecContext(saveCtx, `UPDATE chat_conversations SET updated_at = now() WHERE id = $1`, conversationID)
	log.Printf("assistant: path=%s conversation=%s status=%s latency_ms=%d action=%t",
		path, conversationID, status, time.Since(started).Milliseconds(), actionID != nil)
	return nil
}

func nullJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

func truncate(s string, runes int) string {
	r := []rune(s)
	if len(r) <= runes {
		return s
	}
	return string(r[:runes])
}

func (s *AssistantService) storeProposal(ctx context.Context, userID uuid.UUID, data []byte) (PendingAction, error) {
	var in struct {
		Proposal struct {
			Type    string          `json:"type"`
			Params  json.RawMessage `json:"params"`
			Summary string          `json:"summary"`
		} `json:"proposal"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return PendingAction{}, err
	}
	p := in.Proposal
	if _, err := validateParams(p.Type, p.Params); err != nil {
		return PendingAction{}, err
	}
	var a PendingAction
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO assistant_actions (user_id, type, params, summary) VALUES ($1, $2, $3, $4)
		RETURNING id, type, params, summary, status, error, created_at`,
		userID, p.Type, []byte(p.Params), truncate(p.Summary, 500)).
		Scan(&a.ID, &a.Type, &a.Params, &a.Summary, &a.Status, &a.Error, &a.CreatedAt)
	return a, err
}

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

// validateParams re-checks what the model proposed: the backend, not the LLM, decides what is valid.
func validateParams(kind string, raw json.RawMessage) (any, error) {
	switch kind {
	case "leave_request":
		var p leaveParams
		if json.Unmarshal(raw, &p) != nil || (p.Kind != "day_off" && p.Kind != "vacation") {
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
		if json.Unmarshal(raw, &p) != nil || !accessSystems[p.System] || strings.TrimSpace(p.Justification) == "" ||
			(p.AccessLevel != "read" && p.AccessLevel != "write" && p.AccessLevel != "admin") {
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
		if err == nil && a.Type == "access_request" && params.(accessParams).AccessLevel == "admin" {
			// policy layer: interns never get admin access, whatever the model proposed
			msg := "Стажёрам доступ admin не выдаётся"
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

// ---------------------------------------------------------------- requests (mock corporate services)

const requestColumns = `id, number, type, status, COALESCE(approver, ''), params, created_at, updated_at`

func scanRequest(row interface{ Scan(...any) error }) (ServiceRequest, error) {
	var r ServiceRequest
	err := row.Scan(&r.ID, &r.Number, &r.Type, &r.Status, &r.Approver, &r.Params, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *AssistantService) ListRequests(ctx context.Context, userID uuid.UUID, limit int) ([]ServiceRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+requestColumns+` FROM service_requests WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list requests: %w", err)
	}
	defer rows.Close()
	out := []ServiceRequest{}
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *AssistantService) GetRequest(ctx context.Context, userID, id uuid.UUID) (ServiceRequest, error) {
	r, err := scanRequest(s.db.QueryRowContext(ctx, `SELECT `+requestColumns+` FROM service_requests WHERE id = $1 AND user_id = $2`, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return r, repository.ErrNotFound
	}
	return r, err
}

// DecideRequest is the mock corporate approval: the intern's buddy or teamlead approves or rejects a request.
func (s *AssistantService) DecideRequest(ctx context.Context, staff *model.User, id uuid.UUID, status string) (ServiceRequest, error) {
	if status != "approved" && status != "rejected" {
		return ServiceRequest{}, ErrInvalidDecision
	}
	if staff.Role == model.RoleIntern {
		return ServiceRequest{}, ErrNotBuddy
	}
	r, err := scanRequest(s.db.QueryRowContext(ctx, `
		UPDATE service_requests r SET status = $3, updated_at = now()
		FROM users u
		WHERE r.id = $1 AND u.id = r.user_id AND (u.buddy_id = $2 OR u.teamlead_id = $2) AND r.status = 'pending_approval'
		RETURNING r.id, r.number, r.type, r.status, COALESCE(r.approver, ''), r.params, r.created_at, r.updated_at`,
		id, staff.ID, status))
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		_ = s.db.QueryRowContext(ctx, `
			SELECT true FROM service_requests r JOIN users u ON u.id = r.user_id
			WHERE r.id = $1 AND (u.buddy_id = $2 OR u.teamlead_id = $2)`, id, staff.ID).Scan(&exists)
		if exists {
			return r, ErrRequestNotPending
		}
		return r, repository.ErrNotFound
	}
	return r, err
}

// StaffRequests lists requests of the interns assigned to a buddy/teamlead.
func (s *AssistantService) StaffRequests(ctx context.Context, staff *model.User) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.number, r.type, r.status, COALESCE(r.approver, ''), r.params, r.created_at, r.updated_at,
		       u.id, COALESCE(NULLIF(btrim(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.email), ''
		FROM service_requests r JOIN users u ON u.id = r.user_id
		WHERE u.buddy_id = $1 OR u.teamlead_id = $1 ORDER BY r.created_at DESC LIMIT 100`, staff.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var r ServiceRequest
		var internID uuid.UUID
		var first, last string
		if err := rows.Scan(&r.ID, &r.Number, &r.Type, &r.Status, &r.Approver, &r.Params, &r.CreatedAt, &r.UpdatedAt, &internID, &first, &last); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"request": r, "intern_id": internID, "intern_name": strings.TrimSpace(first + " " + last)})
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- history and feedback

func (s *AssistantService) Messages(ctx context.Context, userID uuid.UUID, conversationID *uuid.UUID) (*uuid.UUID, []ChatMessage, error) {
	if conversationID == nil {
		latest, err := s.latestConversation(ctx, userID)
		if err != nil || latest == nil {
			return nil, []ChatMessage{}, err
		}
		conversationID = latest
	} else if _, err := s.conversation(ctx, userID, conversationID); err != nil {
		return nil, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.role, m.content, m.sources, m.finish_reason, m.feedback, m.created_at,
		       a.id, a.type, a.params, a.summary, a.status, a.error, a.created_at
		FROM chat_messages m LEFT JOIN assistant_actions a ON a.id = m.action_id
		WHERE m.conversation_id = $1 ORDER BY m.created_at`, *conversationID)
	if err != nil {
		return nil, nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		var sources []byte
		var aID *uuid.UUID
		var aType, aSummary, aStatus *string
		var aParams []byte
		var aErr *string
		var aCreated *time.Time
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &sources, &m.FinishReason, &m.Feedback, &m.CreatedAt,
			&aID, &aType, &aParams, &aSummary, &aStatus, &aErr, &aCreated); err != nil {
			return nil, nil, err
		}
		if len(sources) > 0 {
			m.Sources = sources
		}
		if aID != nil {
			m.Action = &PendingAction{ID: *aID, Type: *aType, Params: aParams, Summary: *aSummary, Status: *aStatus, Error: aErr, CreatedAt: *aCreated}
		}
		out = append(out, m)
	}
	return conversationID, out, rows.Err()
}

func (s *AssistantService) Feedback(ctx context.Context, userID, messageID uuid.UUID, rating string) error {
	if rating != "up" && rating != "down" {
		return ErrActionInvalid
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE chat_messages m SET feedback = $3
		FROM chat_conversations c
		WHERE m.id = $1 AND c.id = m.conversation_id AND c.user_id = $2 AND m.role = 'assistant'`, messageID, userID, rating)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------- buddy side

func (s *AssistantService) Questions(ctx context.Context, staff *model.User, status string) ([]MentorQuestion, error) {
	if staff.Role == model.RoleIntern {
		return nil, ErrNotBuddy
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT q.id, q.intern_id, COALESCE(NULLIF(btrim(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.email), q.question, q.context,
		       q.status, q.escalation_level, q.answer, q.created_at, q.answered_at
		FROM mentor_questions q JOIN users u ON u.id = q.intern_id
		WHERE (q.buddy_id = $1 OR u.buddy_id = $1 OR u.teamlead_id = $1) AND ($2 = '' OR q.status = $2)
		ORDER BY q.created_at DESC LIMIT 200`, staff.ID, status)
	if err != nil {
		return nil, fmt.Errorf("list questions: %w", err)
	}
	defer rows.Close()
	out := []MentorQuestion{}
	for rows.Next() {
		var q MentorQuestion
		if err := rows.Scan(&q.ID, &q.InternID, &q.InternName, &q.Question, &q.Context, &q.Status, &q.EscalationLevel,
			&q.Answer, &q.CreatedAt, &q.AnsweredAt); err != nil {
			return nil, err
		}
		q.InternName, q.EscalatedAt = strings.TrimSpace(q.InternName), q.CreatedAt
		out = append(out, q)
	}
	return out, rows.Err()
}

func (s *AssistantService) question(ctx context.Context, staff *model.User, id uuid.UUID) (MentorQuestion, error) {
	qs, err := s.Questions(ctx, staff, "")
	if err != nil {
		return MentorQuestion{}, err
	}
	for _, q := range qs {
		if q.ID == id {
			return q, nil
		}
	}
	return MentorQuestion{}, repository.ErrNotFound
}

func (s *AssistantService) Answer(ctx context.Context, staff *model.User, id uuid.UUID, answer string) (MentorQuestion, error) {
	q, err := s.question(ctx, staff, id)
	if err != nil {
		return q, err
	}
	if q.Status == "answered" {
		return q, ErrQuestionAnswered
	}
	now := time.Now()
	if _, err := s.db.ExecContext(ctx, `UPDATE mentor_questions SET status = 'answered', answer = $2, answered_at = $3 WHERE id = $1`, id, answer, now); err != nil {
		return q, err
	}
	q.Status, q.Answer, q.AnsweredAt = "answered", &answer, &now
	// The answer also lands in the intern's chat, in the conversation the question was escalated from.
	who := strings.TrimSpace(fullName(staff.FirstName, staff.LastName))
	if who == "" {
		who = "наставник"
	}
	text := fmt.Sprintf("**Ответ наставника (%s)** на вопрос «%s»:\n\n%s", who, truncate(q.Question, 200), answer)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO chat_messages (conversation_id, role, content, finish_reason)
		SELECT m.conversation_id, 'assistant', $2, 'mentor_answer'
		FROM mentor_questions mq JOIN chat_messages m ON m.action_id = mq.action_id
		WHERE mq.id = $1 LIMIT 1`, id, text); err != nil {
		log.Printf("assistant: answer to chat failed: %v", err)
	}
	return q, nil
}

// OwnQuestions lists the intern's own escalated questions (to notify about mentor answers).
func (s *AssistantService) OwnQuestions(ctx context.Context, user *model.User) ([]MentorQuestion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, intern_id, question, context, status, escalation_level, answer, created_at, answered_at
		FROM mentor_questions WHERE intern_id = $1 ORDER BY created_at DESC LIMIT 50`, user.ID)
	if err != nil {
		return nil, fmt.Errorf("own questions: %w", err)
	}
	defer rows.Close()
	out := []MentorQuestion{}
	for rows.Next() {
		var q MentorQuestion
		if err := rows.Scan(&q.ID, &q.InternID, &q.Question, &q.Context, &q.Status, &q.EscalationLevel, &q.Answer, &q.CreatedAt, &q.AnsweredAt); err != nil {
			return nil, err
		}
		q.EscalatedAt = q.CreatedAt
		out = append(out, q)
	}
	return out, rows.Err()
}

func (s *AssistantService) Draft(ctx context.Context, staff *model.User, id uuid.UUID) (json.RawMessage, error) {
	q, err := s.question(ctx, staff, id)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"question": q.Question, "context": q.Context}
	var intern model.User
	if err := s.db.QueryRowContext(ctx, `SELECT id, role, profession, buddy_id, first_name, team FROM users WHERE id = $1`, q.InternID).
		Scan(&intern.ID, &intern.Role, &intern.Profession, &intern.BuddyID, &intern.FirstName, &intern.Team); err == nil {
		if uc, err := s.UserContext(ctx, &intern); err == nil {
			payload["user_context"] = uc
		}
	}
	return s.callAI(ctx, http.MethodPost, "/v1/mentor/draft", payload)
}

func (s *AssistantService) Gaps(ctx context.Context, staff *model.User) (json.RawMessage, error) {
	qs, err := s.Questions(ctx, staff, "")
	if err != nil {
		return nil, err
	}
	items := make([]map[string]string, 0, len(qs))
	for _, q := range qs {
		items = append(items, map[string]string{"id": q.ID.String(), "text": q.Question})
	}
	return s.callAI(ctx, http.MethodPost, "/v1/analytics/gaps", map[string]any{"questions": items})
}

func (s *AssistantService) Lint(ctx context.Context, staff *model.User) (json.RawMessage, error) {
	if staff.Role == model.RoleIntern {
		return nil, ErrNotBuddy
	}
	return s.callAI(ctx, http.MethodGet, "/v1/kb/lint", nil)
}

var kbDocID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// KBDoc returns one knowledge-base document (sections in order) for the reader in the chat.
// The demo KB holds no per-role documents, so every signed-in user may read it.
func (s *AssistantService) KBDoc(ctx context.Context, id string) (json.RawMessage, error) {
	if !kbDocID.MatchString(id) {
		return nil, repository.ErrNotFound
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.aiURL+"/v1/kb/docs/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.short.Do(req)
	if err != nil {
		return nil, ErrAIUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, repository.ErrNotFound
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || resp.StatusCode != http.StatusOK || !json.Valid(raw) {
		return nil, ErrAIUnavailable
	}
	return raw, nil
}

func (s *AssistantService) callAI(ctx context.Context, method, path string, payload any) (json.RawMessage, error) {
	var body io.Reader
	if payload != nil {
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.aiURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.short.Do(req)
	if err != nil {
		return nil, ErrAIUnavailable
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || resp.StatusCode != http.StatusOK || !json.Valid(raw) {
		return nil, ErrAIUnavailable
	}
	return raw, nil
}

func (s *AssistantService) Metrics(ctx context.Context, staff *model.User) (ChatMetrics, error) {
	var m ChatMetrics
	if staff.Role == model.RoleIntern {
		return m, ErrNotBuddy
	}
	err := s.db.QueryRowContext(ctx, `
		WITH interns AS (SELECT id FROM users WHERE buddy_id = $1 OR teamlead_id = $1),
		     msgs AS (SELECT m.* FROM chat_messages m JOIN chat_conversations c ON c.id = m.conversation_id
		              WHERE c.user_id IN (SELECT id FROM interns))
		SELECT (SELECT count(*) FROM msgs WHERE role = 'user'),
		       (SELECT count(*) FROM mentor_questions WHERE intern_id IN (SELECT id FROM interns)),
		       (SELECT count(*) FROM msgs WHERE feedback = 'up'),
		       (SELECT count(*) FROM msgs WHERE feedback = 'down'),
		       (SELECT count(*) FROM assistant_actions WHERE status = 'executed' AND user_id IN (SELECT id FROM interns))`,
		staff.ID).Scan(&m.QuestionsTotal, &m.EscalatedToMentor, &m.ThumbsUp, &m.ThumbsDown, &m.ActionsExecuted)
	if err != nil {
		return m, err
	}
	m.AnsweredByBot = max(m.QuestionsTotal-m.EscalatedToMentor, 0)
	if m.QuestionsTotal > 0 {
		rate := float64(int(100*(1-float64(m.EscalatedToMentor)/float64(m.QuestionsTotal)))) / 100
		m.DeflectionRate = &rate
	}
	m.EstimatedMinutesSave = m.AnsweredByBot * minutesPerReply
	return m, nil
}
