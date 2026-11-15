package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// ---------------------------------------------------------------- buddy side

const (
	buddyQuestionsLimit = 200
	ownQuestionsLimit   = 50
)

// Questions are the mentor's: escalated interns' questions go to the buddy only, the teamlead does not see them.
func (s *AssistantService) Questions(ctx context.Context, staff *model.User, status string) ([]MentorQuestion, error) {
	if staff.Role != model.RoleBuddy {
		return nil, ErrNotBuddy
	}
	return repository.NewMentorQuestionRepository(s.db).ListForBuddy(ctx, staff.ID, status, buddyQuestionsLimit)
}

// question loads one question by id with the same access rule as the list; the list limit does not apply to it.
func (s *AssistantService) question(ctx context.Context, staff *model.User, id uuid.UUID) (MentorQuestion, error) {
	if staff.Role != model.RoleBuddy {
		return MentorQuestion{}, ErrNotBuddy
	}
	return repository.NewMentorQuestionRepository(s.db).GetForBuddy(ctx, staff.ID, id)
}

// Answer stores the mentor's answer and delivers it to the intern's chat in one transaction. The open -> answered
// transition is atomic, so two simultaneous answers cannot both win. A question created from the questions tab has no
// chat conversation: its answer is shown in that tab, so no chat message is needed.
func (s *AssistantService) Answer(ctx context.Context, staff *model.User, id uuid.UUID, answer string) (MentorQuestion, error) {
	q, err := s.question(ctx, staff, id)
	if err != nil {
		return q, err
	}
	if q.Status == "answered" {
		return q, ErrQuestionAnswered
	}
	now := time.Now()
	who := strings.TrimSpace(fullName(staff.FirstName, staff.LastName))
	if who == "" {
		who = "наставник"
	}
	text := fmt.Sprintf("**Ответ наставника (%s)** на вопрос «%s»:\n\n%s", who, truncate(q.Question, 200), answer)
	err = withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		updated, err := repository.NewMentorQuestionRepository(tx).MarkAnswered(ctx, id, answer, now)
		if err != nil {
			return err
		}
		if !updated {
			return ErrQuestionAnswered
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chat_messages (conversation_id, role, content, finish_reason)
			SELECT m.conversation_id, 'assistant', $2, 'mentor_answer'
			FROM mentor_questions mq JOIN chat_messages m ON m.action_id = mq.action_id
			WHERE mq.id = $1 LIMIT 1`, id, text)
		return err
	})
	if err != nil {
		return q, err
	}
	q.Status, q.Answer, q.AnsweredAt = "answered", &answer, &now
	return q, nil
}

// OwnQuestions lists the intern's own escalated questions (to notify about mentor answers).
func (s *AssistantService) OwnQuestions(ctx context.Context, user *model.User) ([]MentorQuestion, error) {
	return repository.NewMentorQuestionRepository(s.db).ListByIntern(ctx, user.ID, ownQuestionsLimit)
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
