package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

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
		// сотрудники не задают вопросов наставнику: им доступны только заявки; роль уходит в AI как есть (buddy / teamlead)
		uc.Role, uc.AllowedActions = string(user.Role), []string{"leave_request", "access_request"}
	}
	if user.BuddyID != nil {
		first, last, err := repository.NewAssistantRepository(s.db).UserName(ctx, *user.BuddyID)
		if err != nil && err != repository.ErrNotFound {
			return uc, err
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
	rows, err := repository.NewAssistantRepository(s.db).Tasks(ctx, userID)
	if err != nil {
		return err
	}
	var current, later, done []taskContext
	for _, row := range rows {
		t := taskContext{ID: row.ID, Title: row.Title, Description: row.Description, Status: row.Status, Checklist: row.Checklist, StageTitle: row.StageTitle, RequiresResult: row.RequiresResult}
		if len(t.Description) > 600 {
			t.Description = string([]rune(t.Description)[:300]) + "…"
		}
		switch {
		case t.Status == "verified":
			t.Status = "completed"
		case t.Status == "active" && row.Submission != nil && *row.Submission == "pending":
			t.Status = "on_review"
		case t.Status == "active" && row.Submission != nil && *row.Submission == "rejected":
			t.Status = "returned"
			if row.ReviewComment != nil {
				t.ReviewComment = *row.ReviewComment
			}
		}
		if t.Status == "completed" && row.ReviewComment != nil {
			t.ReviewComment = *row.ReviewComment
		}
		switch t.Status {
		case "active", "on_review", "returned":
			current = append(current, t)
			// the professional checklist (sorted first) defines the stage the intern is in
			if uc.CurrentStage == nil {
				uc.CurrentStage = &stageContext{Title: t.StageTitle, Position: row.StagePosition, Total: row.StageTotal}
			}
		case "unavailable":
			later = append(later, t)
		default:
			done = append(done, t)
		}
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
