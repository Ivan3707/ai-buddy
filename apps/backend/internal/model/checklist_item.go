package model

import (
	"time"

	"github.com/google/uuid"
)

type ChecklistItem struct {
	ID               uuid.UUID
	ChecklistID      uuid.UUID
	StageID          uuid.UUID
	Title            string
	Description      string
	ExpectedResult   *string
	RequiresReview   bool
	EstimatedMinutes *int
	DueAt            *time.Time
	AssignedUserID   *uuid.UUID
	Position         int
	CreatedAt        time.Time
}
