package repository

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

// MentorQuestionRepository is the SQL access to mentor_questions. Access rules
// (which buddy sees which question) are part of the queries.
type MentorQuestionRepository struct {
	db DBTX
}

func NewMentorQuestionRepository(db DBTX) *MentorQuestionRepository {
	return &MentorQuestionRepository{db: db}
}

// mentorQuestionSelect lists a mentor's questions: asked to them directly or by their interns.
const mentorQuestionSelect = `
		SELECT q.id, q.intern_id, COALESCE(NULLIF(btrim(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.email), q.question, q.context,
		       q.status, q.escalation_level, q.answer, q.created_at, q.answered_at
		FROM mentor_questions q JOIN users u ON u.id = q.intern_id
		WHERE (q.buddy_id = $1 OR u.buddy_id = $1)`

func scanMentorQuestion(row interface{ Scan(dest ...any) error }) (model.MentorQuestion, error) {
	var q model.MentorQuestion
	if err := row.Scan(&q.ID, &q.InternID, &q.InternName, &q.Question, &q.Context, &q.Status, &q.EscalationLevel,
		&q.Answer, &q.CreatedAt, &q.AnsweredAt); err != nil {
		return q, err
	}
	q.InternName, q.EscalatedAt = strings.TrimSpace(q.InternName), q.CreatedAt
	return q, nil
}

// ListForBuddy returns the buddy's questions, newest first; an empty status means any status.
func (r *MentorQuestionRepository) ListForBuddy(ctx context.Context, buddyID uuid.UUID, status string, limit int) ([]model.MentorQuestion, error) {
	rows, err := r.db.QueryContext(ctx, mentorQuestionSelect+` AND ($2 = '' OR q.status = $2) ORDER BY q.created_at DESC LIMIT $3`, buddyID, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list questions: %w", err)
	}
	defer rows.Close()
	out := []model.MentorQuestion{}
	for rows.Next() {
		q, err := scanMentorQuestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// GetForBuddy loads one question by id with the same access rule as ListForBuddy (no list limit applies).
func (r *MentorQuestionRepository) GetForBuddy(ctx context.Context, buddyID, id uuid.UUID) (model.MentorQuestion, error) {
	q, err := scanMentorQuestion(r.db.QueryRowContext(ctx, mentorQuestionSelect+` AND q.id = $2`, buddyID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.MentorQuestion{}, ErrNotFound
	}
	return q, err
}

// MarkAnswered atomically moves a question to "answered". It returns false when
// the question was already answered (a concurrent answer won).
func (r *MentorQuestionRepository) MarkAnswered(ctx context.Context, id uuid.UUID, answer string, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE mentor_questions SET status = 'answered', answer = $2, answered_at = $3 WHERE id = $1 AND status <> 'answered'`, id, answer, at)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ListByIntern returns the intern's own questions, newest first.
func (r *MentorQuestionRepository) ListByIntern(ctx context.Context, internID uuid.UUID, limit int) ([]model.MentorQuestion, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, intern_id, question, context, status, escalation_level, answer, created_at, answered_at
		FROM mentor_questions WHERE intern_id = $1 ORDER BY created_at DESC LIMIT $2`, internID, limit)
	if err != nil {
		return nil, fmt.Errorf("own questions: %w", err)
	}
	defer rows.Close()
	out := []model.MentorQuestion{}
	for rows.Next() {
		var q model.MentorQuestion
		if err := rows.Scan(&q.ID, &q.InternID, &q.Question, &q.Context, &q.Status, &q.EscalationLevel, &q.Answer, &q.CreatedAt, &q.AnsweredAt); err != nil {
			return nil, err
		}
		q.EscalatedAt = q.CreatedAt
		out = append(out, q)
	}
	return out, rows.Err()
}
