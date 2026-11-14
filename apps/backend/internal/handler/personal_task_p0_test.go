package handler_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func TestPersonalTaskHTTPAssignmentIsolation(t *testing.T) {
	db, ctx := openP0TestDB(t)
	checklistID, stageID, ordinaryID := uuid.New(), uuid.New(), uuid.New()
	if _, err := db.ExecContext(ctx, `INSERT INTO checklists(id,name,type,profession) VALUES($1,'P0 professional','professional','backend')`, checklistID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM checklists WHERE id=$1`, checklistID); err != nil {
			t.Errorf("cleanup P0 checklist: %v", err)
		}
	})
	if _, err := db.ExecContext(ctx, `INSERT INTO checklist_stages(id,checklist_id,title,week_start,week_end,position) VALUES($1,$2,'P0 stage',1,1,1)`, stageID, checklistID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO checklist_items(id,checklist_id,stage_id,title,position) VALUES($1,$2,$3,'Ordinary',1)`, ordinaryID, checklistID, stageID); err != nil {
		t.Fatal(err)
	}
	assignedLead := createP0User(t, db, ctx, model.RoleTeamlead)
	otherLead := createP0User(t, db, ctx, model.RoleTeamlead)
	profession := model.ProfessionBackend
	intern := &model.User{Email: "p0-" + uuid.NewString() + "@example.test", PasswordHash: "test-hash", Role: model.RoleIntern, Profession: &profession, TeamleadID: &assignedLead.ID}
	if err := repository.NewUserRepository(db).Create(ctx, intern); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM checklist_items WHERE assigned_user_id=$1`, intern.ID); err != nil {
			t.Errorf("cleanup P0 personal items: %v", err)
		}
		if _, err := db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, intern.ID); err != nil {
			t.Errorf("cleanup P0 intern: %v", err)
		}
	})
	if _, err := db.ExecContext(ctx, `INSERT INTO user_checklist_progress(user_id,checklist_id,checklist_item_id,status,position,activated_at) VALUES($1,$2,$3,'active',1,now())`, intern.ID, checklistID, ordinaryID); err != nil {
		t.Fatal(err)
	}
	jwt, err := security.NewJWTService(testJWTSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	router := handler.NewRouter("", handler.Dependencies{
		Users: repository.NewUserRepository(db), JWT: jwt, PersonalTasks: service.NewPersonalTaskService(db),
	})
	path := "/api/teamlead/interns/" + intern.ID.String() + "/checklist-items"
	newTask := map[string]any{"position": 1, "title": "Assigned task", "expected_result": "Done", "estimated_minutes": 30}
	count := func() (int, int) {
		t.Helper()
		var items, progress int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklist_items WHERE assigned_user_id=$1`, intern.ID).Scan(&items); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM user_checklist_progress WHERE user_id=$1 AND checklist_id=$2`, intern.ID, checklistID).Scan(&progress); err != nil {
			t.Fatal(err)
		}
		return items, progress
	}
	itemsBefore, progressBefore := count()
	p0Call(t, router, http.MethodPost, path, p0Token(t, jwt, otherLead), newTask, 404, nil)
	if items, progress := count(); items != itemsBefore || progress != progressBefore {
		t.Fatalf("foreign teamlead changed rows: items=%d progress=%d", items, progress)
	}
	p0Call(t, router, http.MethodPost, "/api/teamlead/interns/"+uuid.NewString()+"/checklist-items", p0Token(t, jwt, assignedLead), newTask, 404, nil)
	if items, progress := count(); items != itemsBefore || progress != progressBefore {
		t.Fatalf("missing intern changed rows: items=%d progress=%d", items, progress)
	}
	p0Call(t, router, http.MethodPost, path, p0Token(t, jwt, intern), newTask, 403, nil)
	if items, progress := count(); items != itemsBefore || progress != progressBefore {
		t.Fatalf("wrong role changed rows: items=%d progress=%d", items, progress)
	}
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	p0Call(t, router, http.MethodPost, path, p0Token(t, jwt, assignedLead), newTask, 201, &created)
	if created.ID == uuid.Nil {
		t.Fatal("assigned teamlead did not receive task ID")
	}
	if items, progress := count(); items != itemsBefore+1 || progress != progressBefore+1 {
		t.Fatalf("assigned request rows: items=%d progress=%d", items, progress)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM user_checklist_progress WHERE user_id=$1 AND checklist_item_id=$2`, intern.ID, created.ID).Scan(&status); err != nil || status != "active" {
		t.Fatalf("created task status=%q err=%v", status, err)
	}
}
