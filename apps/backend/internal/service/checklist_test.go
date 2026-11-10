package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

type checklistTestFixture struct {
	commonID       uuid.UUID
	professionalID uuid.UUID
	commonItems    []model.ChecklistItem
	profItems      []model.ChecklistItem
}

func openChecklistTestDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()

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
	t.Cleanup(cancel)

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	return db, ctx
}

func createChecklistTestFixture(
	t *testing.T,
	db *sql.DB,
	ctx context.Context,
	profession model.Profession,
	commonCount int,
	professionalCount int,
) checklistTestFixture {
	t.Helper()

	fixture := checklistTestFixture{
		commonID:       uuid.New(),
		professionalID: uuid.New(),
	}

	// Создаём чек-лист и сразу регистрируем его очистку.
	// Это гарантирует cleanup даже при ошибке создания этапа или заданий.
	createChecklist := func(
		id uuid.UUID,
		name string,
		checklistType string,
		prof *model.Profession,
	) {
		t.Helper()

		var err error

		if prof == nil {
			_, err = db.ExecContext(ctx, `
				INSERT INTO checklists (id, name, type)
				VALUES ($1, $2, $3)
			`, id, name, checklistType)
		} else {
			_, err = db.ExecContext(ctx, `
				INSERT INTO checklists (id, name, type, profession)
				VALUES ($1, $2, $3, $4)
			`, id, name, checklistType, *prof)
		}

		if err != nil {
			t.Fatalf("create %s checklist: %v", checklistType, err)
		}

		t.Cleanup(func() {
			_, err := db.ExecContext(
				context.Background(),
				"DELETE FROM checklists WHERE id = $1",
				id,
			)
			if err != nil {
				t.Errorf("cleanup checklist %s: %v", id, err)
			}
		})
	}

	createChecklist(
		fixture.commonID,
		"Test common "+fixture.commonID.String(),
		"common",
		nil,
	)

	createChecklist(
		fixture.professionalID,
		"Test professional "+fixture.professionalID.String(),
		"professional",
		&profession,
	)

	// Создаём этап, затем привязываем к нему задания.
	insertItems := func(checklistID uuid.UUID, count int) []model.ChecklistItem {
		t.Helper()

		var stageID uuid.UUID

		err := db.QueryRowContext(ctx, `
			INSERT INTO checklist_stages (
				checklist_id,
				title,
				week_start,
				week_end,
				position
			)
			VALUES ($1, $2, 1, 1, 1)
			RETURNING id
		`, checklistID, "Test stage").Scan(&stageID)
		if err != nil {
			t.Fatalf("create checklist stage: %v", err)
		}

		for i := 1; i <= count; i++ {
			_, err := db.ExecContext(ctx, `
				INSERT INTO checklist_items (
					checklist_id,
					stage_id,
					title,
					position
				)
				VALUES ($1, $2, $3, $4)
			`,
				checklistID,
				stageID,
				fmt.Sprintf("Test item %d", i),
				i,
			)
			if err != nil {
				t.Fatalf("create checklist item: %v", err)
			}
		}

		items, err := repository.NewChecklistRepository(db).GetItems(
			ctx,
			checklistID,
		)
		if err != nil {
			t.Fatalf("get fixture checklist items: %v", err)
		}

		return items
	}

	fixture.commonItems = insertItems(fixture.commonID, commonCount)
	fixture.profItems = insertItems(fixture.professionalID, professionalCount)

	return fixture
}

func createChecklistTestIntern(
	t *testing.T,
	db *sql.DB,
	ctx context.Context,
	profession model.Profession,
) *model.User {
	t.Helper()

	email := "checklist-" + uuid.NewString() + "@example.com"
	buddy := &model.User{
		Email:        "buddy-" + uuid.NewString() + "@example.com",
		PasswordHash: "test-password-hash",
		Role:         model.RoleBuddy,
	}

	if err := repository.NewUserRepository(db).Create(ctx, buddy); err != nil {
		t.Fatalf("create buddy: %v", err)
	}

	user, err := NewUserService(db).CreateIntern(
		ctx,
		email,
		"test-password-hash",
		profession,
		buddy.ID,
	)
	if err != nil {
		t.Fatalf("create intern: %v", err)
	}
	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			"DELETE FROM users WHERE id = $1",
			user.ID,
		)
		if err != nil {
			t.Errorf("cleanup test intern: %v", err)
		}
	})

	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			"DELETE FROM buddy WHERE id = $1",
			buddy.ID,
		)
		if err != nil {
			t.Errorf("cleanup test user: %v", err)
		}
	})

	return user
}

func TestChecklistService_GetByUserID_Success(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	professions := []model.Profession{
		model.ProfessionBackend,
		model.ProfessionFrontend,
		model.ProfessionML,
	}

	for _, profession := range professions {
		t.Run(string(profession), func(t *testing.T) {
			fixture := createChecklistTestFixture(
				t,
				db,
				ctx,
				profession,
				3,
				2,
			)
			user := createChecklistTestIntern(t, db, ctx, profession)

			result, err := NewChecklistService(db).GetByUserID(ctx, user.ID)
			if err != nil {
				t.Fatalf("get user checklists: %v", err)
			}

			if result.Common.Checklist.ID != fixture.commonID {
				t.Errorf(
					"unexpected common checklist ID: got %s, want %s",
					result.Common.Checklist.ID,
					fixture.commonID,
				)
			}

			if result.Professional.Checklist.ID != fixture.professionalID {
				t.Errorf(
					"unexpected professional checklist ID: got %s, want %s",
					result.Professional.Checklist.ID,
					fixture.professionalID,
				)
			}

			checkItems := func(
				name string,
				got []ChecklistItemProgress,
				want []model.ChecklistItem,
			) {
				t.Helper()

				if len(got) != len(want) {
					t.Fatalf(
						"%s: expected %d items, got %d",
						name,
						len(want),
						len(got),
					)
				}

				for i := range want {
					if got[i].Item.ID != want[i].ID {
						t.Errorf(
							"%s item %d: unexpected ID: got %s, want %s",
							name,
							i,
							got[i].Item.ID,
							want[i].ID,
						)
					}

					expectedStatus := model.StatusUnavailable
					if i == 0 {
						expectedStatus = model.StatusActive
					}

					if got[i].Status != expectedStatus {
						t.Errorf(
							"%s item %d: expected status %q, got %q",
							name,
							i,
							expectedStatus,
							got[i].Status,
						)
					}

					if got[i].CompletedAt != nil {
						t.Errorf("%s item %d: expected nil CompletedAt", name, i)
					}
				}
			}

			checkItems("common", result.Common.Items, fixture.commonItems)
			checkItems(
				"professional",
				result.Professional.Items,
				fixture.profItems,
			)
		})
	}
}

func TestChecklistService_GetByUserID_UserNotFound(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	_, err := NewChecklistService(db).GetByUserID(ctx, uuid.New())
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected repository.ErrNotFound, got %v", err)
	}
}

func TestChecklistService_GetByUserID_UserNotIntern(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	user := &model.User{
		Email:        "buddy-" + uuid.NewString() + "@example.com",
		PasswordHash: "test-password-hash",
		Role:         model.RoleBuddy,
	}

	if err := repository.NewUserRepository(db).Create(ctx, user); err != nil {
		t.Fatalf("create buddy: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			"DELETE FROM users WHERE id = $1",
			user.ID,
		)
		if err != nil {
			t.Errorf("cleanup buddy: %v", err)
		}
	})

	_, err := NewChecklistService(db).GetByUserID(ctx, user.ID)
	if !errors.Is(err, ErrUserNotIntern) {
		t.Errorf("expected ErrUserNotIntern, got %v", err)
	}
}

func TestChecklistService_GetByUserID_ProfessionalChecklistNotFound(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	commonID := uuid.New()
	_, err := db.ExecContext(ctx, `
		INSERT INTO checklists (id, name, type)
		VALUES ($1, $2, 'common')
	`, commonID, "Test common "+commonID.String())
	if err != nil {
		t.Fatalf("create common checklist: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			"DELETE FROM checklists WHERE id = $1",
			commonID,
		)
		if err != nil {
			t.Errorf("cleanup common checklist: %v", err)
		}
	})

	profession := model.ProfessionBackend
	user := &model.User{
		Email:        "missing-checklist-" + uuid.NewString() + "@example.com",
		PasswordHash: "test-password-hash",
		Role:         model.RoleIntern,
		Profession:   &profession,
	}

	if err := repository.NewUserRepository(db).Create(ctx, user); err != nil {
		t.Fatalf("create intern: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			"DELETE FROM users WHERE id = $1",
			user.ID,
		)
		if err != nil {
			t.Errorf("cleanup intern: %v", err)
		}
	})

	_, err = NewChecklistService(db).GetByUserID(ctx, user.ID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected repository.ErrNotFound, got %v", err)
	}
}

func TestChecklistService_GetByUserID_MissingProgress(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	fixture := createChecklistTestFixture(
		t,
		db,
		ctx,
		model.ProfessionBackend,
		2,
		2,
	)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)

	itemID := fixture.commonItems[0].ID

	if _, err := db.ExecContext(ctx, `
		DELETE FROM user_checklist_progress
		WHERE user_id = $1 AND checklist_item_id = $2
	`, user.ID, itemID); err != nil {
		t.Fatalf("delete progress record: %v", err)
	}

	_, err := NewChecklistService(db).GetByUserID(ctx, user.ID)
	if !errors.Is(err, ErrProgressIncomplete) {
		t.Errorf("expected ErrProgressIncomplete, got %v", err)
	}
}

func TestChecklistService_GetByUserID_UnexpectedProgress(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	fixture := createChecklistTestFixture(
		t,
		db,
		ctx,
		model.ProfessionBackend,
		2,
		2,
	)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)

	// Создаём задание в чужом профессиональном чек-листе.
	foreignChecklistID := uuid.New()
	_, err := db.ExecContext(ctx, `
		INSERT INTO checklists (id, name, type, profession)
		VALUES ($1, $2, 'professional', 'frontend')
	`, foreignChecklistID, "Foreign checklist "+foreignChecklistID.String())
	if err != nil {
		t.Fatalf("create foreign checklist: %v", err)
	}

	t.Cleanup(func() {
		_, err := db.ExecContext(
			context.Background(),
			"DELETE FROM checklists WHERE id = $1",
			foreignChecklistID,
		)
		if err != nil {
			t.Errorf("cleanup foreign checklist: %v", err)
		}
	})

	foreignItemID := uuid.New()
	_, err = db.ExecContext(ctx, `
		INSERT INTO checklist_items (id, checklist_id, title, position)
		VALUES ($1, $2, $3, 1)
	`, foreignItemID, foreignChecklistID, "Foreign item")
	if err != nil {
		t.Fatalf("create foreign item: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO user_checklist_progress (
			id, user_id, checklist_id, checklist_item_id, status
		)
		VALUES ($1, $2, $3, $4, 'unavailable')
	`, uuid.New(), user.ID, foreignChecklistID, foreignItemID)
	if err != nil {
		t.Fatalf("create unexpected progress record: %v", err)
	}

	_, err = NewChecklistService(db).GetByUserID(ctx, user.ID)
	if !errors.Is(err, ErrProgressIncomplete) {
		t.Errorf("expected ErrProgressIncomplete, got %v", err)
	}

	_ = fixture
}

func TestChecklistService_GetByUserID_EmptyChecklists(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	createChecklistTestFixture(
		t,
		db,
		ctx,
		model.ProfessionBackend,
		0,
		0,
	)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)

	result, err := NewChecklistService(db).GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user checklists: %v", err)
	}

	if result.Common.Items == nil {
		t.Error("expected non-nil empty common items")
	}
	if len(result.Common.Items) != 0 {
		t.Errorf("expected no common items, got %d", len(result.Common.Items))
	}

	if result.Professional.Items == nil {
		t.Error("expected non-nil empty professional items")
	}
	if len(result.Professional.Items) != 0 {
		t.Errorf(
			"expected no professional items, got %d",
			len(result.Professional.Items),
		)
	}
	if result.Common.TotalItems != 0 {
		t.Errorf("expected 0 common total items, got %d", result.Common.TotalItems)
	}
	if result.Common.CompletedItems != 0 {
		t.Errorf(
			"expected 0 completed common items, got %d",
			result.Common.CompletedItems,
		)
	}
	if result.Common.CompletionPercent != 0 {
		t.Errorf(
			"expected 0 common completion percent, got %f",
			result.Common.CompletionPercent,
		)
	}

	if result.Professional.TotalItems != 0 {
		t.Errorf(
			"expected 0 professional total items, got %d",
			result.Professional.TotalItems,
		)
	}
	if result.Professional.CompletedItems != 0 {
		t.Errorf(
			"expected 0 completed professional items, got %d",
			result.Professional.CompletedItems,
		)
	}
	if result.Professional.CompletionPercent != 0 {
		t.Errorf(
			"expected 0 professional completion percent, got %f",
			result.Professional.CompletionPercent,
		)
	}
}
func TestChecklistService_GetByUserID_ProgressSummary(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	fixture := createChecklistTestFixture(
		t,
		db,
		ctx,
		model.ProfessionBackend,
		3,
		2,
	)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)

	// Общий чек-лист: 2 из 3 заданий завершены.
	for _, item := range fixture.commonItems[:2] {
		_, err := db.ExecContext(ctx, `
			UPDATE user_checklist_progress
			SET status = 'completed', completed_at = NOW()
			WHERE user_id = $1 AND checklist_item_id = $2
		`, user.ID, item.ID)
		if err != nil {
			t.Fatalf("complete common item: %v", err)
		}
	}

	// Профессиональный чек-лист: 1 из 2 заданий завершено.
	_, err := db.ExecContext(ctx, `
		UPDATE user_checklist_progress
		SET status = 'completed', completed_at = NOW()
		WHERE user_id = $1 AND checklist_item_id = $2
	`, user.ID, fixture.profItems[0].ID)
	if err != nil {
		t.Fatalf("complete professional item: %v", err)
	}

	result, err := NewChecklistService(db).GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user checklists: %v", err)
	}

	tests := []struct {
		name          string
		gotTotal      int
		gotCompleted  int
		gotPercent    float64
		wantTotal     int
		wantCompleted int
		wantPercent   float64
	}{
		{
			name:          "common",
			gotTotal:      result.Common.TotalItems,
			gotCompleted:  result.Common.CompletedItems,
			gotPercent:    result.Common.CompletionPercent,
			wantTotal:     3,
			wantCompleted: 2,
			wantPercent:   200.0 / 3.0,
		},
		{
			name:          "professional",
			gotTotal:      result.Professional.TotalItems,
			gotCompleted:  result.Professional.CompletedItems,
			gotPercent:    result.Professional.CompletionPercent,
			wantTotal:     2,
			wantCompleted: 1,
			wantPercent:   50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.gotTotal != tt.wantTotal {
				t.Errorf("TotalItems: got %d, want %d", tt.gotTotal, tt.wantTotal)
			}

			if tt.gotCompleted != tt.wantCompleted {
				t.Errorf(
					"CompletedItems: got %d, want %d",
					tt.gotCompleted,
					tt.wantCompleted,
				)
			}

			if math.Abs(tt.gotPercent-tt.wantPercent) > 0.000001 {
				t.Errorf(
					"CompletionPercent: got %f, want %f",
					tt.gotPercent,
					tt.wantPercent,
				)
			}
		})
	}
}

func TestChecklistService_GetByUserID_FullyCompletedChecklist(t *testing.T) {
	db, ctx := openChecklistTestDB(t)

	fixture := createChecklistTestFixture(
		t,
		db,
		ctx,
		model.ProfessionBackend,
		3,
		2,
	)
	user := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)

	// Полностью завершаем общий checklist.
	for _, item := range fixture.commonItems {
		_, err := db.ExecContext(ctx, `
			UPDATE user_checklist_progress
			SET status = 'completed', completed_at = NOW()
			WHERE user_id = $1 AND checklist_item_id = $2
		`, user.ID, item.ID)
		if err != nil {
			t.Fatalf("complete common item: %v", err)
		}
	}

	// В профессиональном checklist завершаем только первое задание.
	_, err := db.ExecContext(ctx, `
		UPDATE user_checklist_progress
		SET status = 'completed', completed_at = NOW()
		WHERE user_id = $1 AND checklist_item_id = $2
	`, user.ID, fixture.profItems[0].ID)
	if err != nil {
		t.Fatalf("complete professional item: %v", err)
	}

	result, err := NewChecklistService(db).GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user checklists: %v", err)
	}

	tests := []struct {
		name          string
		gotTotal      int
		gotCompleted  int
		gotPercent    float64
		wantTotal     int
		wantCompleted int
		wantPercent   float64
	}{
		{
			name:          "common",
			gotTotal:      result.Common.TotalItems,
			gotCompleted:  result.Common.CompletedItems,
			gotPercent:    result.Common.CompletionPercent,
			wantTotal:     3,
			wantCompleted: 3,
			wantPercent:   100,
		},
		{
			name:          "professional",
			gotTotal:      result.Professional.TotalItems,
			gotCompleted:  result.Professional.CompletedItems,
			gotPercent:    result.Professional.CompletionPercent,
			wantTotal:     2,
			wantCompleted: 1,
			wantPercent:   50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.gotTotal != tt.wantTotal {
				t.Errorf("TotalItems: got %d, want %d", tt.gotTotal, tt.wantTotal)
			}

			if tt.gotCompleted != tt.wantCompleted {
				t.Errorf(
					"CompletedItems: got %d, want %d",
					tt.gotCompleted,
					tt.wantCompleted,
				)
			}

			if math.Abs(tt.gotPercent-tt.wantPercent) > 0.000001 {
				t.Errorf(
					"CompletionPercent: got %f, want %f",
					tt.gotPercent,
					tt.wantPercent,
				)
			}
		})
	}

	// Дополнительно проверяем статусы заданий.
	for i, item := range result.Common.Items {
		if item.Status != model.StatusCompleted {
			t.Errorf(
				"common item %d: expected completed, got %q",
				i,
				item.Status,
			)
		}
	}

	if result.Professional.Items[0].Status != model.StatusCompleted {
		t.Errorf(
			"professional first item: expected completed, got %q",
			result.Professional.Items[0].Status,
		)
	}

	if result.Professional.Items[1].Status != model.StatusUnavailable {
		t.Errorf(
			"professional second item: expected unavailable, got %q",
			result.Professional.Items[1].Status,
		)
	}
}
