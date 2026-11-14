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

func TestHintRoutesWithRepositoryEnforceOwnerAndReadState(t *testing.T) {
	db, ctx := openP0TestDB(t)
	checklistID, stageID, itemID := uuid.New(), uuid.New(), uuid.New()
	if _, err := db.ExecContext(ctx, `INSERT INTO checklists(id,name,type) VALUES($1,$2,'common')`, checklistID, "Hint route "+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{`DELETE FROM checklist_items WHERE checklist_id=$1`, `DELETE FROM checklist_stages WHERE checklist_id=$1`, `DELETE FROM checklists WHERE id=$1`} {
			if _, err := db.ExecContext(context.Background(), query, checklistID); err != nil {
				t.Errorf("cleanup hint fixture: %v", err)
			}
		}
	})
	if _, err := db.ExecContext(ctx, `INSERT INTO checklist_stages(id,checklist_id,title,week_start,week_end,position) VALUES($1,$2,'Hints',1,1,1)`, stageID, checklistID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO checklist_items(id,checklist_id,stage_id,title,position) VALUES($1,$2,$3,'Hint item',1)`, itemID, checklistID, stageID); err != nil {
		t.Fatal(err)
	}
	owner := createP0User(t, db, ctx, model.RoleIntern)
	foreign := createP0User(t, db, ctx, model.RoleIntern)
	var progressID uuid.UUID
	if err := db.QueryRowContext(ctx, `INSERT INTO user_checklist_progress(user_id,checklist_id,checklist_item_id,status,position,activated_at) VALUES($1,$2,$3,'active',1,now()) RETURNING id`, owner.ID, checklistID, itemID).Scan(&progressID); err != nil {
		t.Fatal(err)
	}
	var hintID uuid.UUID
	if err := db.QueryRowContext(ctx, `INSERT INTO task_hints(progress_id,text,status) VALUES($1,'Use a small first step','ready') RETURNING id`, progressID).Scan(&hintID); err != nil {
		t.Fatal(err)
	}
	jwt, err := security.NewJWTService(testJWTSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	router := handler.NewRouter("", handler.Dependencies{Users: repository.NewUserRepository(db), JWT: jwt, Hints: service.NewHintService(repository.NewHintRepository(db), "http://unused.invalid")})
	ownerToken, foreignToken := p0Token(t, jwt, owner), p0Token(t, jwt, foreign)
	var owned, hidden []repository.Hint
	p0Call(t, router, http.MethodGet, "/api/hints", ownerToken, nil, 200, &owned)
	p0Call(t, router, http.MethodGet, "/api/hints", foreignToken, nil, 200, &hidden)
	if len(owned) != 1 || owned[0].ID != hintID || len(hidden) != 0 {
		t.Fatalf("list isolation: owned=%+v foreign=%+v", owned, hidden)
	}
	path := "/api/checklist-items/" + itemID.String() + "/hint"
	var detail repository.Hint
	p0Call(t, router, http.MethodGet, path, ownerToken, nil, 200, &detail)
	if detail.ID != hintID || detail.ReadAt != nil {
		t.Fatalf("initial hint: %+v", detail)
	}
	p0Call(t, router, http.MethodGet, path, foreignToken, nil, 404, nil)
	readPath := "/api/hints/" + hintID.String() + "/read"
	p0Call(t, router, http.MethodPost, readPath, foreignToken, nil, 404, nil)
	p0Call(t, router, http.MethodPost, readPath, ownerToken, nil, 204, nil)
	p0Call(t, router, http.MethodGet, path, ownerToken, nil, 200, &detail)
	if detail.ReadAt == nil {
		t.Fatal("read marker was not persisted")
	}
	firstRead := *detail.ReadAt
	p0Call(t, router, http.MethodPost, readPath, ownerToken, nil, 204, nil)
	p0Call(t, router, http.MethodGet, path, ownerToken, nil, 200, &detail)
	if detail.ReadAt == nil || !detail.ReadAt.Equal(firstRead) {
		t.Fatalf("repeated read changed timestamp: %+v", detail.ReadAt)
	}
}
