package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func uniqueP1HRSnapshot() []CorporateEmployee {
	employees := validHRFixture()
	suffix := uuid.NewString()
	for i := range employees {
		employees[i].ID += "-" + suffix
		employees[i].Email = strings.Replace(employees[i].Email, "@", "-"+suffix+"@", 1)
	}
	employees[2].BuddyID = employees[1].ID
	employees[2].ManagerID = employees[0].ID
	return employees
}

func TestHRSynchronizerProfessionChangePreservesHistory(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	backend := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 2)
	frontID, stageID, firstID, secondID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := db.ExecContext(ctx, `INSERT INTO checklists(id,name,type,profession) VALUES($1,'P1 frontend','professional','frontend')`, frontID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM checklists WHERE id=$1`, frontID); err != nil {
			t.Errorf("cleanup frontend checklist: %v", err)
		}
	})
	if _, err := db.ExecContext(ctx, `INSERT INTO checklist_stages(id,checklist_id,title,week_start,week_end,position) VALUES($1,$2,'Frontend',1,1,1)`, stageID, frontID); err != nil {
		t.Fatal(err)
	}
	for position, id := range []uuid.UUID{firstID, secondID} {
		if _, err := db.ExecContext(ctx, `INSERT INTO checklist_items(id,checklist_id,stage_id,title,position) VALUES($1,$2,$3,'Frontend item',$4)`, id, frontID, stageID, position+1); err != nil {
			t.Fatal(err)
		}
	}
	employees := uniqueP1HRSnapshot()
	t.Cleanup(func() {
		for _, index := range []int{2, 0, 1} {
			if _, err := db.ExecContext(context.Background(), `DELETE FROM users WHERE corporate_id=$1`, employees[index].ID); err != nil {
				t.Errorf("cleanup HR user: %v", err)
			}
		}
	})
	sync := NewHRSynchronizer(db)
	if created, err := sync.Sync(ctx, employees, func([]InitialCredential) error { return nil }); err != nil || created != 3 {
		t.Fatalf("initial sync: created=%d err=%v", created, err)
	}
	var internID uuid.UUID
	var hash string
	if err := db.QueryRowContext(ctx, `SELECT id,password_hash FROM users WHERE corporate_id=$1`, employees[2].ID).Scan(&internID, &hash); err != nil {
		t.Fatal(err)
	}
	progress := NewProgressService(db)
	if err := progress.CompleteItem(ctx, internID, backend.profItems[0].ID); err != nil {
		t.Fatal(err)
	}
	note := &repository.Note{Title: "Keep my note", Content: "private"}
	if err := repository.NewPersonalRepository(db).CreateNote(ctx, internID, note); err != nil {
		t.Fatal(err)
	}
	frontend := model.ProfessionFrontend
	employees[2].Profession = &frontend
	if created, err := sync.Sync(ctx, employees, func([]InitialCredential) error { return nil }); err != nil || created != 0 {
		t.Fatalf("profession switch: created=%d err=%v", created, err)
	}
	if created, err := sync.Sync(ctx, employees, func([]InitialCredential) error { return nil }); err != nil || created != 0 {
		t.Fatalf("repeat sync: created=%d err=%v", created, err)
	}
	var currentID uuid.UUID
	var currentHash string
	var count, oldCount, newCount int
	if err := db.QueryRowContext(ctx, `SELECT id,password_hash FROM users WHERE corporate_id=$1`, employees[2].ID).Scan(&currentID, &currentHash); err != nil {
		t.Fatal(err)
	}
	if currentID != internID || currentHash != hash {
		t.Fatal("HR sync replaced user identity or password hash")
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE corporate_id=$1`, employees[2].ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate HR user: count=%d err=%v", count, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE checklist_id=$2),count(*) FILTER (WHERE checklist_id=$3) FROM user_checklist_progress WHERE user_id=$1`, internID, backend.professionalID, frontID).Scan(&oldCount, &newCount); err != nil || oldCount != 2 || newCount != 2 {
		t.Fatalf("profession progress: old=%d new=%d err=%v", oldCount, newCount, err)
	}
	if got := getProgressForTest(t, db, ctx, internID, backend.profItems[0].ID).Status; got != model.StatusCompleted {
		t.Fatalf("historical completion changed: %s", got)
	}
	if got := getProgressForTest(t, db, ctx, internID, firstID).Status; got != model.StatusActive {
		t.Fatalf("new track first item: %s", got)
	}
	if got := getProgressForTest(t, db, ctx, internID, secondID).Status; got != model.StatusUnavailable {
		t.Fatalf("new track second item: %s", got)
	}
	if err := progress.CompleteItem(ctx, internID, backend.profItems[1].ID); !errors.Is(err, ErrOutdatedProfessionTrack) {
		t.Fatalf("old track mutation: %v", err)
	}
	if saved, err := repository.NewPersonalRepository(db).GetNote(ctx, internID, note.ID); err != nil || saved.Content != "private" {
		t.Fatalf("user note lost: %+v %v", saved, err)
	}
	if err := progress.CompleteItem(ctx, internID, firstID); err != nil {
		t.Fatalf("new track unavailable: %v", err)
	}
}

func TestHRSynchronizerConcurrentSameSnapshot(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 1)
	employees := uniqueP1HRSnapshot()
	t.Cleanup(func() {
		for _, index := range []int{2, 0, 1} {
			if _, err := db.ExecContext(context.Background(), `DELETE FROM users WHERE corporate_id=$1`, employees[index].ID); err != nil {
				t.Errorf("cleanup HR user: %v", err)
			}
		}
	})
	sync := NewHRSynchronizer(db)
	type result struct {
		created, published int
		err                error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			published := 0
			created, err := sync.Sync(ctx, employees, func(credentials []InitialCredential) error {
				published = len(credentials)
				return nil
			})
			results <- result{created, published, err}
		}()
	}
	close(start)
	createdTotal, publishedTotal := 0, 0
	for i := 0; i < 2; i++ {
		got := <-results
		if got.err != nil || (got.created != 0 && got.created != 3) || got.published != got.created {
			t.Fatalf("concurrent sync: %+v", got)
		}
		createdTotal += got.created
		publishedTotal += got.published
	}
	if createdTotal != 3 || publishedTotal != 3 {
		t.Fatalf("duplicate creation: created=%d published=%d", createdTotal, publishedTotal)
	}
	for _, employee := range employees {
		var users int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE corporate_id=$1`, employee.ID).Scan(&users); err != nil || users != 1 {
			t.Fatalf("HR identity %s: count=%d err=%v", employee.ID, users, err)
		}
	}
	var progressCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM user_checklist_progress p JOIN users u ON u.id=p.user_id WHERE u.corporate_id=$1`, employees[2].ID).Scan(&progressCount); err != nil || progressCount != 2 {
		t.Fatalf("duplicate progress: count=%d err=%v", progressCount, err)
	}
	if created, err := sync.Sync(ctx, employees, func(credentials []InitialCredential) error {
		if len(credentials) != 0 {
			t.Error("repeat published credentials")
		}
		return nil
	}); err != nil || created != 0 {
		t.Fatalf("repeat sync: created=%d err=%v", created, err)
	}
}

func TestHRSynchronizerExistingEmailConflictRollsBack(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 1)
	employees := uniqueP1HRSnapshot()
	conflicting := &model.User{Email: employees[0].Email, PasswordHash: "existing-hash", Role: model.RoleBuddy}
	if err := repository.NewUserRepository(db).Create(ctx, conflicting); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, conflicting.ID) })
	if _, err := NewHRSynchronizer(db).Sync(ctx, employees, func([]InitialCredential) error {
		t.Error("credentials should not be published after identity conflict")
		return nil
	}); err == nil {
		t.Fatal("accepted existing email with incompatible role")
	}
	for _, employee := range employees {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE corporate_id=$1`, employee.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial HR user for %s: count=%d err=%v", employee.ID, count, err)
		}
	}
	var hash string
	if err := db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id=$1`, conflicting.ID).Scan(&hash); err != nil || hash != "existing-hash" {
		t.Fatalf("conflicting account changed: hash preserved=%v err=%v", hash == "existing-hash", err)
	}
}
