package model

import (
	"time"

	"github.com/google/uuid"
)

type ChecklistItem struct {
	ID          uuid.UUID
	ChecklistID uuid.UUID
	Title       string
	Description string
	Position    int
	CreatedAt   time.Time
}
