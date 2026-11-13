package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

func TestChecklistRepository(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}
	requireIsolatedTestDatabase(t, db, ctx)

	repo := NewChecklistRepository(db)

	// Создаём тестовые чек-листы.
	var commonID uuid.UUID
	err = db.QueryRowContext(ctx, `
		INSERT INTO checklists (name, type)
		VALUES ('Test common checklist', 'common')
		RETURNING id
	`).Scan(&commonID)
	if err != nil {
		t.Fatalf("create common checklist: %v", err)
	}

	var professionalID uuid.UUID
	err = db.QueryRowContext(ctx, `
		INSERT INTO checklists (name, type, profession)
		VALUES ('Test backend checklist', 'professional', 'backend')
		RETURNING id
	`).Scan(&professionalID)
	if err != nil {
		t.Fatalf("create professional checklist: %v", err)
	}

	// Удаляем тестовые чек-листы и связанные задания.
	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			"DELETE FROM checklists WHERE id IN ($1, $2)",
			commonID,
			professionalID,
		)
		if err != nil {
			t.Errorf("cleanup test checklists: %v", err)
		}
	})

	// GetCommon
	common, err := repo.GetCommon(ctx)
	if err != nil {
		t.Fatalf("get common checklist: %v", err)
	}

	if common.ID != commonID {
		t.Errorf("expected common ID %s, got %s", commonID, common.ID)
	}

	if common.Profession != nil {
		t.Errorf("expected nil profession, got %v", *common.Profession)
	}

	// GetProfessional
	professional, err := repo.GetProfessional(
		ctx,
		model.ProfessionBackend,
	)
	if err != nil {
		t.Fatalf("get professional checklist: %v", err)
	}

	if professional.ID != professionalID {
		t.Errorf(
			"expected professional ID %s, got %s",
			professionalID,
			professional.ID,
		)
	}

	if professional.Profession == nil ||
		*professional.Profession != model.ProfessionBackend {
		t.Errorf("unexpected profession: %v", professional.Profession)
	}

	// Создаём задания в обратном порядке.
	var stageID uuid.UUID
	err = db.QueryRowContext(ctx, `
		INSERT INTO checklist_stages (
			checklist_id, title, week_start, week_end, position
		)
		VALUES ($1, 'Test stage', 1, 1, 1)
		RETURNING id
	`, professionalID).Scan(&stageID)
	if err != nil {
		t.Fatalf("create checklist stage: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO checklist_items (
			checklist_id, stage_id, title, estimated_minutes, position
		)
		VALUES
			($1, $2, 'Second item', NULL, 2),
			($1, $2, 'First item', 30, 1)
	`, professionalID, stageID)
	if err != nil {
		t.Fatalf("create checklist items: %v", err)
	}

	// GetItems
	items, err := repo.GetItems(ctx, professionalID)
	if err != nil {
		t.Fatalf("get checklist items: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].Position != 1 || items[1].Position != 2 {
		t.Errorf(
			"items are not ordered: positions %d, %d",
			items[0].Position,
			items[1].Position,
		)
	}

	if items[0].EstimatedMinutes == nil ||
		*items[0].EstimatedMinutes != 30 {
		t.Errorf(
			"expected first item estimated minutes 30, got %v",
			items[0].EstimatedMinutes,
		)
	}
	if items[1].EstimatedMinutes != nil {
		t.Errorf(
			"expected second item estimated minutes to be nil, got %d",
			*items[1].EstimatedMinutes,
		)
	}

	// Empty checklist
	emptyItems, err := repo.GetItems(ctx, commonID)
	if err != nil {
		t.Fatalf("get empty checklist items: %v", err)
	}

	if len(emptyItems) != 0 {
		t.Errorf("expected empty items, got %d", len(emptyItems))
	}

	// Not found
	_, err = repo.GetProfessional(ctx, model.ProfessionML)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
