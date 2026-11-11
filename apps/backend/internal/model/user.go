package model

import (
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	RoleIntern UserRole = "intern"
	RoleBuddy  UserRole = "buddy"
)

type Profession string

const (
	ProfessionBackend  Profession = "backend"
	ProfessionFrontend Profession = "frontend"
	ProfessionML       Profession = "ml"
)

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	Role         UserRole
	Profession   *Profession
	BuddyID      *uuid.UUID
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}
