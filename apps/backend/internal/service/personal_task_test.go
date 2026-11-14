package service

import (
	"context"
	"database/sql"
	"errors"
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
	requireIsolatedTestDatabase(t, db, ctx)
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
	deadline := time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)
	first, err := personal.Create(ctx, teamlead.ID, interns[0].ID, 1, "Before completed", "Details", "Expected", 30, false, &deadline)
	if err != nil {
		t.Fatal(err)
	}
	var storedDueAt sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT due_at FROM checklist_items WHERE id=$1`, first).Scan(&storedDueAt); err != nil || !storedDueAt.Valid || !storedDueAt.Time.Equal(deadline) {
		t.Fatalf("deadline was not stored: %v %v", storedDueAt, err)
	}
	second, err := personal.Create(ctx, teamlead.ID, interns[0].ID, 3, "Before active", "", "Expected", 30, true)
	if err != nil {
		t.Fatal(err)
	}
	third, err := personal.Create(ctx, teamlead.ID, interns[0].ID, 6, "After unavailable", "", "Expected", 30, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM checklist_items WHERE id IN ($1,$2,$3)`, first, second, third)
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
	if len(got) != 6 {
		t.Fatalf("got %d professional progress rows", len(got))
	}
	wantIDs := []uuid.UUID{first, items[0].ID, second, items[1].ID, items[2].ID, third}
	wantStatus := []model.ChecklistStatus{model.StatusActive, model.StatusCompleted, model.StatusActive, model.StatusActive, model.StatusUnavailable, model.StatusActive}
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
	type createdTask struct {
		id  uuid.UUID
		err error
	}
	created := make(chan createdTask, 2)
	for i := 0; i < 2; i++ {
		go func() {
			id, createErr := personal.Create(ctx, teamlead.ID, interns[0].ID, 6, "Concurrent task", "", "Expected", 30, false)
			created <- createdTask{id, createErr}
		}()
	}
	for i := 0; i < 2; i++ {
		result := <-created
		if result.err != nil {
			t.Fatalf("concurrent insert: %v", result.err)
		}
		id := result.id
		t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM checklist_items WHERE id=$1`, id) })
	}
	var total, positions, active int
	if err := db.QueryRowContext(ctx, `SELECT count(*),count(DISTINCT position),count(*) FILTER (WHERE status='active')
		FROM user_checklist_progress WHERE user_id=$1 AND checklist_id=$2`, interns[0].ID, fixture.professionalID).
		Scan(&total, &positions, &active); err != nil {
		t.Fatal(err)
	}
	if total != 8 || positions != 8 || active != 6 {
		t.Fatalf("concurrent insert: total=%d distinct positions=%d active=%d", total, positions, active)
	}
	between, err := personal.Create(ctx, teamlead.ID, interns[0].ID, 5, "Between ordinary tasks", "", "Expected", 30, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM checklist_items WHERE id=$1`, between)
	})
	if err := NewProgressService(db).CompleteItem(ctx, interns[0].ID, items[1].ID); err != nil {
		t.Fatalf("complete ordinary item before personal item: %v", err)
	}
	ordinaryNext := getProgressForTest(t, db, ctx, interns[0].ID, items[2].ID)
	personalBetween := getProgressForTest(t, db, ctx, interns[0].ID, between)
	if ordinaryNext.Status != model.StatusActive || personalBetween.Status != model.StatusActive {
		t.Fatalf("ordinary sequence was interrupted: next=%s personal=%s", ordinaryNext.Status, personalBetween.Status)
	}
	if err := NewProgressService(db).CompleteItem(ctx, interns[0].ID, between); err != nil {
		t.Fatalf("complete independent personal item: %v", err)
	}
	if got := getProgressForTest(t, db, ctx, interns[0].ID, items[2].ID).Status; got != model.StatusActive {
		t.Fatalf("personal completion changed ordinary sequence: %s", got)
	}
}

func TestPersonalTaskAssignmentIsolation(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 1)
	users := repository.NewUserRepository(db)
	lead := &model.User{Email: "lead-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleTeamlead}
	otherLead := &model.User{Email: "lead-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleTeamlead}
	for _, user := range []*model.User{lead, otherLead} {
		if err := users.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
		id := user.ID
		t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, id) })
	}
	intern := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	if _, err := db.ExecContext(ctx, `UPDATE users SET teamlead_id=$1 WHERE id=$2`, lead.ID, intern.ID); err != nil {
		t.Fatal(err)
	}
	count := func() (int, int) {
		t.Helper()
		var items, progress int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklist_items WHERE assigned_user_id=$1`, intern.ID).Scan(&items); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM user_checklist_progress WHERE user_id=$1 AND checklist_id=$2`, intern.ID, fixture.professionalID).Scan(&progress); err != nil {
			t.Fatal(err)
		}
		return items, progress
	}
	itemsBefore, progressBefore := count()
	svc := NewPersonalTaskService(db)
	if _, err := svc.Create(ctx, otherLead.ID, intern.ID, 1, "Denied", "", "Result", 30, false); !errors.Is(err, ErrInternNotAssigned) {
		t.Fatalf("other teamlead: %v", err)
	}
	if items, progress := count(); items != itemsBefore || progress != progressBefore {
		t.Fatalf("denied request changed rows: items=%d progress=%d", items, progress)
	}
	if _, err := svc.Create(ctx, lead.ID, uuid.New(), 1, "Missing", "", "Result", 30, false); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("missing intern: %v", err)
	}
	if items, progress := count(); items != itemsBefore || progress != progressBefore {
		t.Fatalf("missing intern changed rows: items=%d progress=%d", items, progress)
	}
	id, err := svc.Create(ctx, lead.ID, intern.ID, 1, "Assigned", "", "Result", 30, false)
	if err != nil || id == uuid.Nil {
		t.Fatalf("assigned teamlead: id=%s err=%v", id, err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM checklist_items WHERE id=$1`, id) })
	if items, progress := count(); items != itemsBefore+1 || progress != progressBefore+1 {
		t.Fatalf("assigned request did not create rows: items=%d progress=%d", items, progress)
	}
	if got := getProgressForTest(t, db, ctx, intern.ID, id).Status; got != model.StatusActive {
		t.Fatalf("assigned personal task status: %s", got)
	}
}
