package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// ---------------------------------------------------------------- requests (mock corporate services)

func (s *AssistantService) ListRequests(ctx context.Context, userID uuid.UUID, limit int) ([]ServiceRequest, error) {
	return repository.NewAssistantRepository(s.db).ListRequests(ctx, userID, limit)
}

// GetRequest returns a request to its owner or to the owner's teamlead (the approver, with the requester's name).
func (s *AssistantService) GetRequest(ctx context.Context, viewer *model.User, id uuid.UUID) (ServiceRequest, error) {
	repo := repository.NewAssistantRepository(s.db)
	r, err := repo.RequestForOwner(ctx, id, viewer.ID)
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return r, err
	}
	if viewer.Role != model.RoleTeamlead {
		return r, repository.ErrNotFound
	}
	r, err = repo.RequestForTeamlead(ctx, id, viewer.ID)
	if err != nil {
		return r, err
	}
	r.Requester, _ = repo.RequesterName(ctx, id)
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
	repo := repository.NewAssistantRepository(s.db)
	r, err := repo.DecideRequest(ctx, id, staff.ID, status, who, comment)
	if errors.Is(err, repository.ErrNotFound) {
		exists, checkErr := repo.TeamleadOwnsRequest(ctx, id, staff.ID)
		if checkErr != nil {
			return r, checkErr
		}
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
	rows, err := repository.NewAssistantRepository(s.db).StaffRequests(ctx, staff.ID)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, row := range rows {
		out = append(out, map[string]any{"request": row.Request, "intern_id": row.InternID, "intern_name": strings.TrimSpace(row.InternName)})
	}
	return out, nil
}
