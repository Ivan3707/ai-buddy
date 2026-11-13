package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

func TestHRSynchronizerPostgres(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 2, 2)
	employees := validHRFixture()
	suffix := uuid.NewString()
	for i := range employees {
		employees[i].ID += "-" + suffix
		employees[i].Email = strings.Replace(employees[i].Email, "@", "-"+suffix+"@", 1)
	}
	employees[2].BuddyID = employees[1].ID
	employees[2].ManagerID = employees[0].ID
	ids := []string{employees[0].ID, employees[1].ID, employees[2].ID}
	t.Cleanup(func() {
		cleanupIDs := []string{ids[2], ids[0], ids[1]}
		if len(ids) > 3 {
			cleanupIDs = append(cleanupIDs, ids[3:]...)
		}
		for _, id := range cleanupIDs {
			if _, err := db.ExecContext(context.Background(), `DELETE FROM users WHERE corporate_id=$1`, id); err != nil {
				t.Errorf("cleanup HR user: %v", err)
			}
		}
	})
	sync := NewHRSynchronizer(db)
	credentials, err := sync.Sync(ctx, employees)
	if err != nil {
		t.Fatal(err)
	}
	if len(credentials) != 3 {
		t.Fatalf("new credentials=%d", len(credentials))
	}
	var id uuid.UUID
	var hash string
	if err = db.QueryRowContext(ctx, `SELECT id,password_hash FROM users WHERE corporate_id=$1`, ids[2]).Scan(&id, &hash); err != nil {
		t.Fatal(err)
	}
	valid, err := security.VerifyPassword(credentials[2].Password, hash)
	if err != nil || !valid {
		t.Fatalf("new login credential invalid: %v", err)
	}
	var count int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_checklist_progress WHERE user_id=$1 AND checklist_id IN ($2,$3)`, id, fixture.commonID, fixture.professionalID).Scan(&count); err != nil || count != 4 {
		t.Fatalf("progress=%d: %v", count, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE user_checklist_progress SET status='completed',completed_at=now(),activated_at=NULL WHERE user_id=$1 AND checklist_item_id=$2`, id, fixture.commonItems[0].ID); err != nil {
		t.Fatal(err)
	}
	second, err := sync.Sync(ctx, employees)
	if err != nil || len(second) != 0 {
		t.Fatalf("repeat: credentials=%d err=%v", len(second), err)
	}
	employees[2].Email = "renamed-" + suffix + "@example.test"
	// A new Buddy and lead replace the assignments without changing local ID.
	employees = append(employees, CorporateEmployee{ID: "buddy-2-" + suffix, Email: "buddy2-" + suffix + "@example.test", Role: model.RoleBuddy}, CorporateEmployee{ID: "lead-2-" + suffix, Email: "lead2-" + suffix + "@example.test", Role: model.RoleTeamlead})
	ids = append(ids, employees[3].ID, employees[4].ID)
	employees[2].BuddyID = employees[3].ID
	employees[2].ManagerID = employees[4].ID
	if _, err = sync.Sync(ctx, employees); err != nil {
		t.Fatal(err)
	}
	var current uuid.UUID
	var email, storedHash, status string
	var buddy, lead uuid.UUID
	if err = db.QueryRowContext(ctx, `SELECT id,email,password_hash,buddy_id,teamlead_id FROM users WHERE corporate_id=$1`, ids[2]).Scan(&current, &email, &storedHash, &buddy, &lead); err != nil {
		t.Fatal(err)
	}
	if current != id || email != employees[2].Email || storedHash != hash {
		t.Fatal("HR changed local ID, email or password hash unexpectedly")
	}
	if err = db.QueryRowContext(ctx, `SELECT status FROM user_checklist_progress WHERE user_id=$1 AND checklist_item_id=$2`, id, fixture.commonItems[0].ID).Scan(&status); err != nil || status != "completed" {
		t.Fatalf("progress reset: %s %v", status, err)
	}
	var expectedBuddy, expectedLead uuid.UUID
	if err = db.QueryRowContext(ctx, `SELECT id FROM users WHERE corporate_id=$1`, employees[3].ID).Scan(&expectedBuddy); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT id FROM users WHERE corporate_id=$1`, employees[4].ID).Scan(&expectedLead); err != nil {
		t.Fatal(err)
	}
	if buddy != expectedBuddy || lead != expectedLead {
		t.Fatal("HR assignments not updated")
	}
	changed := append([]CorporateEmployee(nil), employees...)
	changed = append(changed[:2], changed[3:]...)
	if _, err = sync.Sync(ctx, changed); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE corporate_id=$1`, ids[2]).Scan(&count); err != nil || count != 1 {
		t.Fatal("missing HR record deleted user")
	}
	duplicate := append([]CorporateEmployee(nil), employees...)
	duplicate[2].Email = duplicate[1].Email
	if _, err = sync.Sync(ctx, duplicate); err == nil {
		t.Fatal("accepted conflicting email")
	}
}
