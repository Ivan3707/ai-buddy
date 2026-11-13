package model

import (
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	RoleIntern   UserRole = "intern"
	RoleBuddy    UserRole = "buddy"
	RoleTeamlead UserRole = "teamlead"
)

type Profession string

const (
	ProfessionBackend  Profession = "backend"
	ProfessionFrontend Profession = "frontend"
	ProfessionML       Profession = "ml"
)

type User struct {
	ID               uuid.UUID
	CorporateID      *string
	Email            string
	PasswordHash     string
	Role             UserRole
	Profession       *Profession
	BuddyID          *uuid.UUID
	TeamleadID       *uuid.UUID
	BirthDate        *time.Time
	EmployedSince    *time.Time
	CreatedAt        time.Time
	LastLoginAt      *time.Time
	FirstName        *string
	LastName         *string
	TelegramUsername *string
	Department       *string
	Team             *string
}
