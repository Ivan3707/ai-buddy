package model

import (
	"time"

	"github.com/google/uuid"
)

// MentorQuestion is an intern's question escalated to their buddy (from the
// chat or the "Вопросы наставнику" tab). It is serialized as-is by the API.
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
