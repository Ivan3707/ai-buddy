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
	if user.Role == model.RoleBuddy || user.Role == model.RoleTeamlead {
		if err := s.loadTeam(ctx, user, &uc); err != nil {
			return uc, err
		}
	}
	return uc, nil
}

const (
	maxContextInterns      = 30
	maxContextInternTasks  = 3
	maxContextOpenItems    = 20
	maxContextQuestionText = 300
)

// loadTeam gives a mentor or team lead the same view of their interns as their cabinet: the buddy sees interns
// assigned to them (buddy_id), the team lead sees their team (teamlead_id). Nobody else's interns get into the prompt.
func (s *AssistantService) loadTeam(ctx context.Context, user *model.User, uc *UserContext) error {
	interns, err := repository.NewUserRepository(s.db).ListAssignedInterns(ctx, user.ID, user.Role)
	if err != nil {
		return err
	}
	names := repository.NewAssistantRepository(s.db)
	uc.MyInterns = []teamInternContext{}
	for i := range interns {
		if i == maxContextInterns {
			break
		}
		in := &interns[i]
		item := teamInternContext{Name: fullName(in.FirstName, in.LastName), Checklists: []checklistProgress{}}
		if item.Name == "" {
			item.Name = in.Email
		}
		if in.Profession != nil {
			p := string(*in.Profession)
			item.Profession = &p
		}
		if in.Team != nil {
			item.Team = *in.Team
		}
		if in.LastLoginAt != nil {
			item.LastLogin = in.LastLoginAt.Format("2006-01-02")
		}
		if user.Role == model.RoleTeamlead && in.BuddyID != nil {
			first, last, err := names.UserName(ctx, *in.BuddyID)
			if err != nil && err != repository.ErrNotFound {
				return err
			}
			item.Buddy = fullName(first, last)
		}
		rows, err := names.Tasks(ctx, in.ID)
		if err != nil {
			return err
		}
		summarizeTasks(rows, &item)
		uc.MyInterns = append(uc.MyInterns, item)
	}
	if user.Role == model.RoleBuddy {
		questions, err := repository.NewMentorQuestionRepository(s.db).ListForBuddy(ctx, user.ID, "open", maxContextOpenItems)
		if err != nil {
			return err
		}
		for _, q := range questions {
			text := q.Question
			if r := []rune(text); len(r) > maxContextQuestionText {
				text = string(r[:maxContextQuestionText]) + "…"
			}
			uc.OpenQuestions = append(uc.OpenQuestions, openQuestionInfo{Intern: q.InternName, Question: text, Since: q.CreatedAt.Format("2006-01-02")})
		}
	}
	if user.Role == model.RoleTeamlead {
		requests, err := names.StaffRequests(ctx, user.ID)
		if err != nil {
			return err
		}
		for _, r := range requests {
			if r.Request.Status != "pending_approval" || len(uc.PendingApprovals) == maxContextOpenItems {
				continue
			}
			uc.PendingApprovals = append(uc.PendingApprovals, pendingApproval{Intern: strings.TrimSpace(r.InternName), Number: r.Request.Number, Type: r.Request.Type, Status: r.Request.Status})
		}
	}
	return nil
}

// summarizeTasks folds an intern's checklist rows into done/total per checklist, the current stage and
// up to three current tasks with their review state.
func summarizeTasks(rows []model.AssistantTask, item *teamInternContext) {
	index := map[string]int{}
	for _, row := range rows {
		i, ok := index[row.Checklist]
		if !ok {
			i = len(item.Checklists)
			index[row.Checklist] = i
			item.Checklists = append(item.Checklists, checklistProgress{Checklist: row.Checklist})
		}
		item.Checklists[i].Total++
		pending := row.Submission != nil && *row.Submission == "pending"
		rejected := row.Submission != nil && *row.Submission == "rejected"
		state := row.Status
		switch {
		case pending:
			// submitted for review: not done until the mentor accepts it
			state = "on_review"
			item.OnReview++
		case rejected && row.Status != "verified":
			state = "returned"
			item.Returned++
		case row.Status == "verified" || row.Status == "completed":
			item.Checklists[i].Done++
			continue
		case row.Status != "active":
			continue
		}
		if item.CurrentStage == "" {
			item.CurrentStage = row.StageTitle
		}
		if len(item.CurrentTasks) < maxContextInternTasks {
			item.CurrentTasks = append(item.CurrentTasks, row.Title+" ("+state+")")
		}
	}
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
