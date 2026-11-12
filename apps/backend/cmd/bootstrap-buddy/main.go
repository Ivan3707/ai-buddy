package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	email := strings.TrimSpace(os.Getenv("BOOTSTRAP_BUDDY_EMAIL"))
	password := os.Getenv("BOOTSTRAP_BUDDY_PASSWORD")
	role := model.RoleBuddy
	if os.Getenv("BOOTSTRAP_ROLE") == string(model.RoleTeamlead) {
		role = model.RoleTeamlead
	}
	if value := os.Getenv("BOOTSTRAP_ROLE"); value != "" && value != string(model.RoleBuddy) && value != string(model.RoleTeamlead) {
		return errors.New("BOOTSTRAP_ROLE must be buddy or teamlead")
	}

	if email == "" {
		return errors.New("BOOTSTRAP_BUDDY_EMAIL is required")
	}

	parsedEmail, err := mail.ParseAddress(email)
	if err != nil || parsedEmail.Address != email {
		return errors.New("BOOTSTRAP_BUDDY_EMAIL must be a valid email address")
	}

	if utf8.RuneCountInString(password) < 12 {
		return errors.New("BOOTSTRAP_BUDDY_PASSWORD must contain at least 12 characters")
	}

	if len(password) > 1024 {
		return errors.New("BOOTSTRAP_BUDDY_PASSWORD is too long")
	}

	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	db, err := database.OpenPostgres()
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer db.Close()

	user := &model.User{
		Email:        email,
		PasswordHash: passwordHash,
		Role:         role,
		Profession:   nil,
	}

	userRepo := repository.NewUserRepository(db)

	if err := userRepo.Create(context.Background(), user); err != nil {
		if errors.Is(err, repository.ErrEmailAlreadyExists) {
			return fmt.Errorf("user with email %q already exists; existing account was not changed", email)
		}

		return fmt.Errorf("create buddy: %w", err)
	}

	fmt.Printf("%s created successfully: %s (id: %s)\n", role, user.Email, user.ID)

	return nil
}
