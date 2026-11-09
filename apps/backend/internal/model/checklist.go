package model

import (
	"time"

	"github.com/google/uuid"
)

type ChecklistType string

const (
	ChecklistTypeCommon       ChecklistType = "common"
	ChecklistTypeProfessional ChecklistType = "professional"
)

type Checklist struct {
	ID         uuid.UUID
	Name       string
	Type       ChecklistType
	Profession *Profession
	CreatedAt  time.Time
}
