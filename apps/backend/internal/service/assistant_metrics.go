package service

import (
	"context"

	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func (s *AssistantService) Metrics(ctx context.Context, staff *model.User) (ChatMetrics, error) {
	var m ChatMetrics
	if staff.Role == model.RoleIntern {
		return m, ErrNotBuddy
	}
	m, err := repository.NewAssistantRepository(s.db).Metrics(ctx, staff.ID)
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
