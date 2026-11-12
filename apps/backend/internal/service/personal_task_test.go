package service

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"os"
	"testing"
	"time"
)

func TestPersonalTaskInsertAroundActiveAndCompleted(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 3)
	buddy := &model.User{Email: "buddy-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleBuddy}
	if err = repository.NewUserRepository(db).Create(ctx, buddy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, buddy.ID) })
	teamlead := &model.User{Email: "lead-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleTeamlead}
	if err = repository.NewUserRepository(db).Create(ctx, teamlead); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, teamlead.ID) })
	svc := NewUserService(db)
	interns := make([]*model.User, 2)
	for i := range interns {
		interns[i], err = svc.CreateIntern(ctx, "intern-"+uuid.NewString()+"@example.com", "hash", model.ProfessionBackend, buddy.ID)
		if err != nil {
			t.Fatal(err)
		}
		id := interns[i].ID
		t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, id) })
		if _, err = db.ExecContext(ctx, `UPDATE users SET teamlead_id=$1 WHERE id=$2`, teamlead.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	before, err := repository.NewProgressRepository(db).GetByUserID(ctx, interns[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	items, err := repository.NewChecklistRepository(db).GetItems(ctx, fixture.professionalID)
	if err != nil {
		t.Fatal(err)
	}
	if err = NewProgressService(db).CompleteItem(ctx, interns[0].ID, items[0].ID); err != nil {
		t.Fatal(err)
	}
	personal := NewPersonalTaskService(db)
	first, err := personal.Create(ctx, teamlead.ID, interns[0].ID, 1, "Before completed", "", "Expected", 30, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := personal.Create(ctx, teamlead.ID, interns[0].ID, 3, "Before active", "", "Expected", 30, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM checklist_items WHERE id IN ($1,$2)`, first, second)
	})
	after, err := repository.NewProgressRepository(db).GetByUserID(ctx, interns[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []model.UserChecklistProgress
	for _, p := range after {
		if p.ChecklistID == fixture.professionalID {
			got = append(got, p)
		}
	}
	if len(got) != 5 {
		t.Fatalf("got %d professional progress rows", len(got))
	}
	wantIDs := []uuid.UUID{first, items[0].ID, second, items[1].ID, items[2].ID}
	wantStatus := []model.ChecklistStatus{model.StatusActive, model.StatusCompleted, model.StatusActive, model.StatusActive, model.StatusUnavailable}
	for i, p := range got {
		if p.ChecklistItemID != wantIDs[i] || p.Status != wantStatus[i] || p.Position != i+1 {
			t.Fatalf("position %d got %+v", i+1, p)
		}
	}
	other, err := repository.NewProgressRepository(db).GetByUserID(ctx, interns[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != len(before) {
		t.Fatalf("other intern progress changed: %d -> %d", len(before), len(other))
	}
	for i := range before {
		if before[i].ChecklistItemID != other[i].ChecklistItemID || before[i].Status != other[i].Status || before[i].Position != other[i].Position {
			t.Fatalf("other intern progress changed at %d", i)
		}
	}
}
