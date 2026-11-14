package model

import (
	"time"

	"github.com/google/uuid"
)

type ChecklistStatus string

const (
	StatusUnavailable ChecklistStatus = "unavailable"
	StatusActive      ChecklistStatus = "active"
	StatusCompleted   ChecklistStatus = "completed"
	StatusVerified    ChecklistStatus = "verified"
)

type UserChecklistProgress struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	ChecklistID     uuid.UUID
	ChecklistItemID uuid.UUID
	Status          ChecklistStatus
	CompletedAt     *time.Time
	CreatedAt       time.Time
	Position        int
	ActivatedAt     *time.Time
}
