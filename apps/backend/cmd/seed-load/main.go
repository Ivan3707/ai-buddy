package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

const userCount = 100

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	password := os.Getenv("LOAD_USER_PASSWORD")
	if len([]rune(password)) < 12 || len(password) > 256 {
		return errors.New("LOAD_USER_PASSWORD must contain 12..256 characters")
	}
	db, err := database.OpenPostgres()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var name, version string
	if err := db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name); err != nil {
		return err
	}
	if name != "ai_buddy_load" {
		return fmt.Errorf("refusing to seed database %q", name)
	}
	var applied int
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied").Scan(&applied); err != nil {
		return err
	}
	head, err := migrationHead("/migrations")
	if err != nil {
		return err
	}
	version = strconv.Itoa(applied)
	if applied != head && applied != 9 {
		return fmt.Errorf("Goose version %s differs from migration head %d", version, head)
	}
	if applied == 9 {
		var compatible bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.checklist_feedback') IS NOT NULL
			AND to_regclass('public.corporate_access_requests') IS NOT NULL
			AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='users' AND column_name='corporate_id')`).Scan(&compatible); err != nil {
			return err
		}
		if !compatible {
			return errors.New("legacy Goose v9 schema fingerprint mismatch")
		}
	}
	if err := ensureChecklists(ctx, db); err != nil {
		return err
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	users := repository.NewUserRepository(db)
	buddy, err := ensureStaff(ctx, users, "load.buddy@example.invalid", model.RoleBuddy, hash)
	if err != nil {
		return err
	}
	lead, err := ensureStaff(ctx, users, "load.teamlead@example.invalid", model.RoleTeamlead, hash)
	if err != nil {
		return err
	}
	creator := service.NewUserService(db)
	birth := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	employed := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	created := 0
	for i := 1; i <= userCount; i++ {
		email := fmt.Sprintf("load.intern.%03d@example.invalid", i)
		user, err := users.GetByEmail(ctx, email)
		if errors.Is(err, repository.ErrNotFound) {
			user, err = creator.CreateInternWithAssignments(ctx, email, hash, model.ProfessionBackend, buddy.ID, lead.ID, birth, employed, "Load", fmt.Sprintf("Intern%03d", i), "", "Synthetic", "Load")
			if err == nil {
				created++
			}
		}
		if err != nil {
			return fmt.Errorf("ensure %s: %w", email, err)
		}
		if user.Role != model.RoleIntern || user.Profession == nil || *user.Profession != model.ProfessionBackend || user.BuddyID == nil || *user.BuddyID != buddy.ID || user.TeamleadID == nil || *user.TeamleadID != lead.ID {
			return fmt.Errorf("existing user %s has unexpected assignments", email)
		}
		valid, err := security.VerifyPassword(password, user.PasswordHash)
		if err != nil || !valid {
			return fmt.Errorf("password for existing user %s differs; use original LOAD_USER_PASSWORD", email)
		}
	}
	log.Printf("database=%s goose_head=%d load_users=%d created=%d", name, head, userCount, created)
	return nil
}

func migrationHead(dir string) (int, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return 0, err
	}
	head := 0
	for _, file := range files {
		prefix, _, ok := strings.Cut(filepath.Base(file), "_")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(prefix)
		if err == nil && n > head {
			head = n
		}
	}
	if head == 0 {
		return 0, errors.New("no migration files found")
	}
	return head, nil
}

func ensureStaff(ctx context.Context, users *repository.UserRepository, email string, role model.UserRole, hash string) (*model.User, error) {
	user, err := users.GetByEmail(ctx, email)
	if errors.Is(err, repository.ErrNotFound) {
		user = &model.User{Email: email, PasswordHash: hash, Role: role}
		err = users.Create(ctx, user)
	}
	if err != nil {
		return nil, err
	}
	if user.Role != role {
		return nil, fmt.Errorf("unexpected role for %s", email)
	}
	return user, nil
}

func ensureChecklists(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, spec := range []struct {
		typeName, profession, name string
	}{
		{"common", "", "Load common"},
		{"professional", "backend", "Load backend"},
	} {
		var id uuid.UUID
		var existingName string
		var row *sql.Row
		if spec.typeName == "common" {
			row = tx.QueryRowContext(ctx, "SELECT id,name FROM checklists WHERE type='common'")
		} else {
			row = tx.QueryRowContext(ctx, "SELECT id,name FROM checklists WHERE type='professional' AND profession='backend'")
		}
		err := row.Scan(&id, &existingName)
		if errors.Is(err, sql.ErrNoRows) {
			var profession any
			if spec.profession != "" {
				profession = spec.profession
			}
			err = tx.QueryRowContext(ctx, "INSERT INTO checklists(name,type,profession) VALUES($1,$2,$3) RETURNING id", spec.name, spec.typeName, profession).Scan(&id)
		} else if err == nil && existingName != spec.name {
			return fmt.Errorf("conflicting %s checklist %q", spec.typeName, existingName)
		}
		if err != nil {
			return err
		}
		var stageID uuid.UUID
		err = tx.QueryRowContext(ctx, "SELECT id FROM checklist_stages WHERE checklist_id=$1 AND position=1", id).Scan(&stageID)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, "INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,position) VALUES($1,'Load stage',1,1,1) RETURNING id", id).Scan(&stageID)
		}
		if err != nil {
			return err
		}
		var itemID uuid.UUID
		err = tx.QueryRowContext(ctx, "SELECT id FROM checklist_items WHERE checklist_id=$1 AND position=1", id).Scan(&itemID)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, "INSERT INTO checklist_items(checklist_id,stage_id,title,position) VALUES($1,$2,'Load item',1) RETURNING id", id, stageID).Scan(&itemID)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
