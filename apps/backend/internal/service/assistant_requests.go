package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// ---------------------------------------------------------------- requests (mock corporate services)

const requestColumns = `id, number, type, status, COALESCE(approver, ''), COALESCE(decision_comment, ''), params, created_at, updated_at`

func scanRequest(row interface{ Scan(...any) error }) (ServiceRequest, error) {
	var r ServiceRequest
	err := row.Scan(&r.ID, &r.Number, &r.Type, &r.Status, &r.Approver, &r.DecisionComment, &r.Params, &r.CreatedAt, &r.UpdatedAt)
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

// GetRequest returns a request to its owner or to the owner's teamlead (the approver, with the requester's name).
func (s *AssistantService) GetRequest(ctx context.Context, viewer *model.User, id uuid.UUID) (ServiceRequest, error) {
	r, err := scanRequest(s.db.QueryRowContext(ctx, `SELECT `+requestColumns+` FROM service_requests WHERE id = $1 AND user_id = $2`, id, viewer.ID))
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return r, err
	}
	if viewer.Role != model.RoleTeamlead {
		return r, repository.ErrNotFound
	}
	var name string
	r, err = scanRequest(s.db.QueryRowContext(ctx, `
		SELECT r.id, r.number, r.type, r.status, COALESCE(r.approver, ''), COALESCE(r.decision_comment, ''), r.params, r.created_at, r.updated_at
		FROM service_requests r JOIN users u ON u.id = r.user_id WHERE r.id = $1 AND u.teamlead_id = $2`, id, viewer.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return r, repository.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	_ = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(NULLIF(btrim(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.email)
		FROM service_requests r JOIN users u ON u.id = r.user_id WHERE r.id = $1`, id).Scan(&name)
	r.Requester = name
	return r, nil
}

// DecideRequest is the mock corporate approval: only the intern's teamlead approves or rejects a request, with a comment.
func (s *AssistantService) DecideRequest(ctx context.Context, staff *model.User, id uuid.UUID, status, comment string) (ServiceRequest, error) {
	if status != "approved" && status != "rejected" {
		return ServiceRequest{}, ErrInvalidDecision
	}
	if staff.Role != model.RoleTeamlead {
		return ServiceRequest{}, ErrNotTeamlead
	}
	comment = strings.TrimSpace(comment)
	if r := []rune(comment); len(r) > 500 {
		comment = string(r[:500])
	}
	who := strings.TrimSpace(fullName(staff.FirstName, staff.LastName))
	if who == "" {
		who = staff.Email
	}
	r, err := scanRequest(s.db.QueryRowContext(ctx, `
		UPDATE service_requests r SET status = $3, approver = $4, decision_comment = NULLIF($5, ''), updated_at = now()
		FROM users u
		WHERE r.id = $1 AND u.id = r.user_id AND u.teamlead_id = $2 AND r.status = 'pending_approval'
		RETURNING r.id, r.number, r.type, r.status, COALESCE(r.approver, ''), COALESCE(r.decision_comment, ''), r.params, r.created_at, r.updated_at`,
		id, staff.ID, status, who, comment))
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		_ = s.db.QueryRowContext(ctx, `
			SELECT true FROM service_requests r JOIN users u ON u.id = r.user_id
			WHERE r.id = $1 AND u.teamlead_id = $2`, id, staff.ID).Scan(&exists)
		if exists {
			return r, ErrRequestNotPending
		}
		return r, repository.ErrNotFound
	}
	return r, err
}

// StaffRequests lists requests of the teamlead's interns for approval (buddies do not approve requests).
func (s *AssistantService) StaffRequests(ctx context.Context, staff *model.User) ([]map[string]any, error) {
	if staff.Role != model.RoleTeamlead {
		return nil, ErrNotTeamlead
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.number, r.type, r.status, COALESCE(r.approver, ''), COALESCE(r.decision_comment, ''), r.params, r.created_at, r.updated_at,
		       u.id, COALESCE(NULLIF(btrim(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.email), ''
		FROM service_requests r JOIN users u ON u.id = r.user_id
		WHERE u.teamlead_id = $1 ORDER BY r.created_at DESC LIMIT 100`, staff.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var r ServiceRequest
		var internID uuid.UUID
		var first, last string
		if err := rows.Scan(&r.ID, &r.Number, &r.Type, &r.Status, &r.Approver, &r.DecisionComment, &r.Params, &r.CreatedAt, &r.UpdatedAt, &internID, &first, &last); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"request": r, "intern_id": internID, "intern_name": strings.TrimSpace(first + " " + last)})
	}
	return out, rows.Err()
}
