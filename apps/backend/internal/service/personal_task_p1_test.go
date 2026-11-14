package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func TestPersonalTaskInsertConcurrentWithOrdinaryCompletion(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 2)
	lead := &model.User{Email: "lead-" + uuid.NewString() + "@example.test", PasswordHash: "hash", Role: model.RoleTeamlead}
	if err := repository.NewUserRepository(db).Create(ctx, lead); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, lead.ID) })
	intern := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	if _, err := db.ExecContext(ctx, `UPDATE users SET teamlead_id=$1 WHERE id=$2`, lead.ID, intern.ID); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	type result struct {
		id  uuid.UUID
		err error
	}
	created := make(chan result, 1)
	completed := make(chan error, 1)
	go func() {
		<-start
		id, err := NewPersonalTaskService(db).Create(ctx, lead.ID, intern.ID, 2, "Parallel personal", "", "Done", 30, false)
		created <- result{id, err}
	}()
	go func() {
		<-start
		completed <- NewProgressService(db).CompleteItem(ctx, intern.ID, fixture.profItems[0].ID)
	}()
	close(start)
	item := <-created
	if item.err != nil || item.id == uuid.Nil {
		t.Fatalf("insert personal item: %+v", item)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM checklist_items WHERE id=$1`, item.id)
	})
	if err := <-completed; err != nil {
		t.Fatalf("complete ordinary item: %v", err)
	}
	if got := getProgressForTest(t, db, ctx, intern.ID, fixture.profItems[0].ID).Status; got != model.StatusCompleted {
		t.Fatalf("first ordinary item: %s", got)
	}
	if got := getProgressForTest(t, db, ctx, intern.ID, fixture.profItems[1].ID).Status; got != model.StatusActive {
		t.Fatalf("second ordinary item: %s", got)
	}
	if got := getProgressForTest(t, db, ctx, intern.ID, item.id).Status; got != model.StatusActive {
		t.Fatalf("independent personal item: %s", got)
	}
	if got := getProgressForTest(t, db, ctx, intern.ID, fixture.commonItems[0].ID).Status; got != model.StatusActive {
		t.Fatalf("common checklist changed: %s", got)
	}
	var rows, positions, active int
	if err := db.QueryRowContext(ctx, `SELECT count(*),count(DISTINCT position),count(*) FILTER (WHERE status='active') FROM user_checklist_progress WHERE user_id=$1 AND checklist_id=$2`, intern.ID, fixture.professionalID).Scan(&rows, &positions, &active); err != nil || rows != 3 || positions != 3 || active != 2 {
		t.Fatalf("final progress: rows=%d positions=%d active=%d err=%v", rows, positions, active, err)
	}
}
