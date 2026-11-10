package model

import (
	"time"

	"github.com/google/uuid"
)

type ChecklistStage struct {
	ID             uuid.UUID
	ChecklistID    uuid.UUID
	Title          string
	WeekStart      int
	WeekEnd        int
	Goal           string
	ExpectedResult string
	Position       int
	CreatedAt      time.Time
}
