package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
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
		SELECT i.id, COALESCE(p.title_override, i.title), COALESCE(p.description_override, i.description), p.status, c.type, st.id, st.title, st.position, i.requires_review,
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
		WHERE p.user_id = $1 AND p.hidden_at IS NULL
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
