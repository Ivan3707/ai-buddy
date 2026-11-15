package service

import (
	"context"

	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

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
		       (SELECT count(*) FROM assistant_actions WHERE status = 'executed' AND user_id IN (SELECT id FROM interns))`,
		staff.ID).Scan(&m.QuestionsTotal, &m.EscalatedToMentor, &m.ActionsExecuted)
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
