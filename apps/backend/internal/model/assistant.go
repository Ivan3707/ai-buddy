package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PendingAction struct {
	ID        uuid.UUID       `json:"id"`
	Type      string          `json:"type"`
	Params    json.RawMessage `json:"params"`
	Summary   string          `json:"summary"`
	Status    string          `json:"status"`
	Error     *string         `json:"error,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type ServiceRequest struct {
	ID              uuid.UUID       `json:"id"`
	Number          string          `json:"number"`
	Type            string          `json:"type"`
	Status          string          `json:"status"`
	Approver        string          `json:"approver,omitempty"`
	DecisionComment string          `json:"decision_comment,omitempty"`
	Requester       string          `json:"requester,omitempty"`
	Params          json.RawMessage `json:"params"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type ChatMessage struct {
	ID           uuid.UUID       `json:"id"`
	Role         string          `json:"role"`
	Content      string          `json:"content"`
	Sources      json.RawMessage `json:"sources,omitempty"`
	Action       *PendingAction  `json:"action,omitempty"`
	FinishReason *string         `json:"finish_reason,omitempty"`
	Feedback     *string         `json:"feedback"`
	CreatedAt    time.Time       `json:"created_at"`
}

type ChatTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatMetrics struct {
	QuestionsTotal       int      `json:"questions_total"`
	AnsweredByBot        int      `json:"answered_by_bot"`
	EscalatedToMentor    int      `json:"escalated_to_mentor"`
	DeflectionRate       *float64 `json:"deflection_rate"`
	ActionsExecuted      int      `json:"actions_executed"`
	EstimatedMinutesSave int      `json:"estimated_mentor_minutes_saved"`
}

type AssistantTask struct {
	ID             string
	Title          string
	Description    string
	Status         string
	Checklist      string
	StageTitle     string
	StagePosition  int
	StageTotal     int
	RequiresResult bool
	ReviewComment  *string
	Submission     *string
}
