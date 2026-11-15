package model

import (
	"time"

	"github.com/google/uuid"
)

type Feedback struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"user_id"`
	ChecklistID    uuid.UUID  `json:"checklist_id"`
	Rating         int        `json:"rating"`
	Comment        string     `json:"comment"`
	DeliveryStatus string     `json:"delivery_status"`
	CreatedAt      time.Time  `json:"created_at"`
	DeliveredAt    *time.Time `json:"delivered_at"`
}

type AnalyticsEvent struct {
	FeedbackID    uuid.UUID
	CorporateID   string
	ChecklistID   uuid.UUID
	ChecklistType string
	Rating        int
	Comment       string
	CreatedAt     time.Time
}
