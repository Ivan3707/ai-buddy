package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type demoUser struct {
	email, first, last, telegram, department, team string
	role                                           model.UserRole
	profession                                     *model.Profession
	birthDate, employedSince                       *time.Time
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	password := os.Getenv("DEMO_PASSWORD")
	if len([]rune(password)) < 12 || len(password) > 256 {
		return errors.New("DEMO_PASSWORD must contain 12..256 characters")
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash demo password: %w", err)
	}
	db, err := database.OpenPostgres()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var dbName string
	var version int
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&dbName); err != nil || dbName != "app" {
		return fmt.Errorf("seed-demo requires app database, got %q: %w", dbName, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version != 7 {
		return fmt.Errorf("seed-demo requires Goose v7, got %d: %w", version, err)
	}

	birthday := time.Date(2001, 4, 12, 0, 0, 0, 0, time.UTC)
	employed := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	backend, frontend, ml := model.ProfessionBackend, model.ProfessionFrontend, model.ProfessionML
	staff := []demoUser{
		{email: "buddy@demo.example", first: "Марина", last: "Наставникова", telegram: "demo_buddy", department: "Technology", team: "Onboarding", role: model.RoleBuddy},
		{email: "teamlead@demo.example", first: "Алексей", last: "Руководителей", telegram: "demo_teamlead", department: "Technology", team: "Engineering", role: model.RoleTeamlead},
	}
	users := repository.NewUserRepository(db)
	created := 0
	staffIDs := make([]uuid.UUID, 0, len(staff))
	for _, spec := range staff {
		user, made, err := ensureStaff(ctx, users, spec, hash)
		if err != nil {
			return err
		}
		if made {
			created++
		}
		staffIDs = append(staffIDs, user.ID)
	}

	interns := []demoUser{
		{email: "backend.intern@demo.example", first: "Борис", last: "Бэкендов", telegram: "demo_backend", department: "Technology", team: "Backend", role: model.RoleIntern, profession: &backend, birthDate: &birthday, employedSince: &employed},
		{email: "frontend.intern@demo.example", first: "Фаина", last: "Фронтендова", telegram: "demo_frontend", department: "Technology", team: "Frontend", role: model.RoleIntern, profession: &frontend, birthDate: &birthday, employedSince: &employed},
		{email: "ml.intern@demo.example", first: "Михаил", last: "Моделев", telegram: "demo_ml", department: "Technology", team: "Machine Learning", role: model.RoleIntern, profession: &ml, birthDate: &birthday, employedSince: &employed},
	}
	userService := service.NewUserService(db)
	for _, spec := range interns {
		user, err := users.GetByEmail(ctx, spec.email)
		if errors.Is(err, repository.ErrNotFound) {
			user, err = userService.CreateInternWithAssignments(ctx, spec.email, hash, *spec.profession, staffIDs[0], staffIDs[1], *spec.birthDate, *spec.employedSince, spec.first, spec.last, spec.telegram, spec.department, spec.team)
			if err == nil {
				created++
			}
		}
		if err != nil {
			return fmt.Errorf("ensure %s: %w", spec.email, err)
		}
		if err := validateIntern(user, spec, staffIDs[0], staffIDs[1]); err != nil {
			return err
		}
		if _, err := service.NewChecklistService(db).GetByUserID(ctx, user.ID); err != nil {
			return fmt.Errorf("validate progress for %s: %w", spec.email, err)
		}
		if err := seedPersonalData(ctx, db, user.ID, spec.profession); err != nil {
			return err
		}
	}
	fmt.Printf("demo data ready: %d users created, %d verified\n", created, len(staff)+len(interns))
	return nil
}

func ensureStaff(ctx context.Context, users *repository.UserRepository, spec demoUser, hash string) (*model.User, bool, error) {
	user, err := users.GetByEmail(ctx, spec.email)
	if err == nil {
		if user.Role != spec.role {
			return nil, false, fmt.Errorf("existing %s has role %s", spec.email, user.Role)
		}
		return user, false, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, false, err
	}
	first, last, telegram, department, team := spec.first, spec.last, spec.telegram, spec.department, spec.team
	user = &model.User{Email: spec.email, PasswordHash: hash, Role: spec.role, FirstName: &first, LastName: &last, TelegramUsername: &telegram, Department: &department, Team: &team}
	if err := users.Create(ctx, user); err != nil {
		return nil, false, err
	}
	return user, true, nil
}

func validateIntern(user *model.User, spec demoUser, buddyID, leadID uuid.UUID) error {
	if user.Role != model.RoleIntern || user.Profession == nil || *user.Profession != *spec.profession || user.BuddyID == nil || *user.BuddyID != buddyID || user.TeamleadID == nil || *user.TeamleadID != leadID {
		return fmt.Errorf("existing %s is inconsistent with demo assignments", spec.email)
	}
	return nil
}

func seedPersonalData(ctx context.Context, db *sql.DB, userID uuid.UUID, profession *model.Profession) error {
	title := "Знакомство с командой"
	if _, err := db.ExecContext(ctx, `INSERT INTO notes(user_id,title,content)
		SELECT $1,$2,$3 WHERE NOT EXISTS (SELECT 1 FROM notes WHERE user_id=$1 AND title=$2)`, userID, title, "Зафиксировать вопросы к buddy и цели на первую неделю."); err != nil {
		return err
	}
	starts := time.Date(2026, 11, 16, 10, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	eventTitle := "Синхронизация с buddy — " + strings.ToUpper(string(*profession))
	_, err := db.ExecContext(ctx, `INSERT INTO calendar_events(user_id,title,description,starts_at,ends_at)
		SELECT $1,$2,$3,$4,$5 WHERE NOT EXISTS (SELECT 1 FROM calendar_events WHERE user_id=$1 AND title=$2 AND starts_at=$4)`, userID, eventTitle, "Демонстрационная встреча по прогрессу онбординга.", starts, starts.Add(45*time.Minute))
	return err
}
