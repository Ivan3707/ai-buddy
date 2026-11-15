package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// Per-intern checklist edits (migration 00007): a teamlead changes or removes a task of one intern; the catalog
// and other interns are not affected, and removing the current task opens the next one.
func TestTeamleadEditsOnlyThisInternsChecklist(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 3)
	lead := &model.User{Email: "lead-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleTeamlead}
	otherLead := &model.User{Email: "lead-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleTeamlead}
	for _, user := range []*model.User{lead, otherLead} {
		if err := repository.NewUserRepository(db).Create(ctx, user); err != nil {
			t.Fatal(err)
		}
		id := user.ID
		t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, id) })
	}
	intern := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	neighbour := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	for _, id := range []uuid.UUID{intern.ID, neighbour.ID} {
		if _, err := db.ExecContext(ctx, `UPDATE users SET teamlead_id=$1 WHERE id=$2`, lead.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewPersonalTaskService(db)
	first, second, third := fixture.profItems[0].ID, fixture.profItems[1].ID, fixture.profItems[2].ID
	title := "  Own wording  "
	minutes := 15

	if err := svc.UpdateTask(ctx, otherLead.ID, intern.ID, first, TaskEdit{Title: &title}); !errors.Is(err, ErrInternNotAssigned) {
		t.Fatalf("other teamlead edit: %v", err)
	}
	if err := svc.RemoveTask(ctx, otherLead.ID, intern.ID, first); !errors.Is(err, ErrInternNotAssigned) {
		t.Fatalf("other teamlead remove: %v", err)
	}
	if err := svc.UpdateTask(ctx, lead.ID, intern.ID, first, TaskEdit{}); !errors.Is(err, ErrInvalidTask) {
		t.Fatalf("empty edit: %v", err)
	}
	if err := svc.UpdateTask(ctx, lead.ID, intern.ID, uuid.New(), TaskEdit{Title: &title}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("missing item: %v", err)
	}
	if err := svc.UpdateTask(ctx, lead.ID, intern.ID, first, TaskEdit{Title: &title, EstimatedMinutes: &minutes}); err != nil {
		t.Fatal(err)
	}

	lists, err := NewChecklistService(db).GetByUserID(ctx, intern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := lists.Professional.Items[0].Item; got.Title != "Own wording" || got.EstimatedMinutes == nil || *got.EstimatedMinutes != 15 {
		t.Fatalf("override not applied: %+v", got)
	}
	neighbourLists, err := NewChecklistService(db).GetByUserID(ctx, neighbour.ID)
	if err != nil {
		t.Fatal(err)
	}
	if neighbourLists.Professional.Items[0].Item.Title == "Own wording" {
		t.Fatal("edit leaked to another intern")
	}

	// The active first task is removed: the second one becomes active, totals shrink, history stays.
	if err := svc.RemoveTask(ctx, lead.ID, intern.ID, first); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveTask(ctx, lead.ID, intern.ID, first); err != nil {
		t.Fatalf("second remove must be a no-op: %v", err)
	}
	if err := svc.UpdateTask(ctx, lead.ID, intern.ID, first, TaskEdit{Title: &title}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("edit of a removed task: %v", err)
	}
	if got := getProgressForTest(t, db, ctx, intern.ID, second).Status; got != model.StatusActive {
		t.Fatalf("next task after removal: %s", got)
	}
	if got := getProgressForTest(t, db, ctx, intern.ID, third).Status; got != model.StatusUnavailable {
		t.Fatalf("later task opened too early: %s", got)
	}
	lists, err = NewChecklistService(db).GetByUserID(ctx, intern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lists.Professional.TotalItems != 2 || lists.Professional.Items[0].Item.ID != second {
		t.Fatalf("removed task still visible: %+v", lists.Professional.Items)
	}
	if err := NewProgressService(db).CompleteItem(ctx, intern.ID, second); err != nil {
		t.Fatal(err)
	}
	if got := getProgressForTest(t, db, ctx, intern.ID, third).Status; got != model.StatusActive {
		t.Fatalf("sequence after removal: %s", got)
	}

	// A personal task can be added to the common checklist too.
	id, err := svc.CreateInChecklist(ctx, lead.ID, intern.ID, model.ChecklistTypeCommon, 1, "Common personal", "", "Done", 10, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM user_checklist_progress WHERE checklist_item_id=$1`, id)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM checklist_items WHERE id=$1`, id)
	})
	lists, err = NewChecklistService(db).GetByUserID(ctx, intern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lists.Common.TotalItems != 2 || lists.Common.Items[0].Item.ID != id {
		t.Fatalf("common personal task: %+v", lists.Common.Items)
	}
	if _, err := svc.CreateInChecklist(ctx, lead.ID, intern.ID, "other", 1, "x", "", "y", 10, false, nil); !errors.Is(err, ErrInvalidChecklistType) {
		t.Fatalf("invalid checklist type: %v", err)
	}
}

// The demo action finishes both checklists (100%), approves pending work and is idempotent.
func TestCompleteChecklistsDemo(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 2)
	intern := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	reviewed := fixture.profItems[0].ID
	setRequiresReview(t, db, ctx, reviewed)
	text := "work"
	if _, err := NewProgressService(db).SubmitItem(ctx, intern.ID, reviewed, &text, nil); err != nil {
		t.Fatal(err)
	}
	admin := NewAdminService(db, "http://127.0.0.1:1", nil)
	if _, err := admin.CompleteChecklists(ctx, *intern.BuddyID, *intern.BuddyID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("staff account: %v", err)
	}
	res, err := admin.CompleteChecklists(ctx, *intern.BuddyID, intern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.SubmissionsApproved != 1 || res.ItemsCompleted != 4 {
		t.Fatalf("first run: %+v", res)
	}
	lists, err := NewChecklistService(db).GetByUserID(ctx, intern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lists.Common.CompletionPercent != 100 || lists.Professional.CompletionPercent != 100 {
		t.Fatalf("not 100%%: common=%v professional=%v", lists.Common.CompletionPercent, lists.Professional.CompletionPercent)
	}
	if got := getProgressForTest(t, db, ctx, intern.ID, reviewed).Status; got != model.StatusVerified {
		t.Fatalf("review task status: %s", got)
	}
	again, err := admin.CompleteChecklists(ctx, *intern.BuddyID, intern.ID)
	if err != nil || again.ItemsCompleted != 0 || again.SubmissionsApproved != 0 {
		t.Fatalf("second run must change nothing: %+v %v", again, err)
	}
}

// Staff read the review history of their intern's task; others do not.
func TestListInternSubmissionsAccess(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 1)
	intern := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	lead := &model.User{Email: "lead-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleTeamlead}
	if err := repository.NewUserRepository(db).Create(ctx, lead); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, lead.ID) })
	if _, err := db.ExecContext(ctx, `UPDATE users SET teamlead_id=$1 WHERE id=$2`, lead.ID, intern.ID); err != nil {
		t.Fatal(err)
	}
	itemID := fixture.profItems[0].ID
	setRequiresReview(t, db, ctx, itemID)
	progress := NewProgressService(db)
	text := "attempt"
	submission, err := progress.SubmitItem(ctx, intern.ID, itemID, &text, nil)
	if err != nil {
		t.Fatal(err)
	}
	comment := "redo"
	if err := progress.ReviewSubmission(ctx, *intern.BuddyID, submission.ID, false, &comment); err != nil {
		t.Fatal(err)
	}
	if err := progress.ReviewSubmission(ctx, lead.ID, submission.ID, true, nil); err == nil {
		t.Fatal("teamlead must not review")
	}
	lists, err := NewChecklistService(db).GetByUserID(ctx, intern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item := lists.Professional.Items[0]; !item.ReturnedForRework || item.ReworkComment == nil || *item.ReworkComment != "redo" {
		t.Fatalf("rework marker: %+v", item)
	}
	buddy := &model.User{ID: *intern.BuddyID, Role: model.RoleBuddy}
	for _, actor := range []*model.User{buddy, lead} {
		history, err := progress.ListInternSubmissions(ctx, actor, intern.ID, itemID)
		if err != nil || len(history) != 1 || history[0].Status != model.SubmissionRejected {
			t.Fatalf("%s history: %+v %v", actor.Role, history, err)
		}
	}
	stranger := &model.User{ID: uuid.New(), Role: model.RoleTeamlead}
	if _, err := progress.ListInternSubmissions(ctx, stranger, intern.ID, itemID); !errors.Is(err, ErrInternNotAssigned) {
		t.Fatalf("stranger: %v", err)
	}
	if _, err := progress.ListInternSubmissions(ctx, lead, uuid.New(), itemID); !errors.Is(err, ErrInternNotAssigned) {
		t.Fatalf("missing intern: %v", err)
	}
	if _, err := progress.ListInternSubmissions(ctx, lead, intern.ID, uuid.New()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("missing item: %v", err)
	}
}
