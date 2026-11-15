package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func TestMLInternGetsCommonAndProfessionalProgress(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	var commonID, mlID uuid.UUID
	err := db.QueryRowContext(ctx, `SELECT id FROM checklists WHERE type='common'`).Scan(&commonID)
	if err == nil {
		t.Fatal("common checklist belongs to another fixture")
	}
	commonID = uuid.New()
	if _, err = db.ExecContext(ctx, `INSERT INTO checklists(id,name,type) VALUES($1,'ML test common','common')`, commonID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM checklists WHERE id=$1`, commonID) })
	commonStage := uuid.New()
	if _, err = db.ExecContext(ctx, `INSERT INTO checklist_stages(id,checklist_id,title,week_start,week_end,position) VALUES($1,$2,'Common',1,1,1)`, commonStage, commonID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO checklist_items(checklist_id,stage_id,title,position) VALUES($1,$2,'Common task',1)`, commonID, commonStage); err != nil {
		t.Fatal(err)
	}
	err = db.QueryRowContext(ctx, `SELECT id FROM checklists WHERE type='professional' AND profession='ml'`).Scan(&mlID)
	if err != nil {
		mlID = uuid.New()
		if _, err = db.ExecContext(ctx, `INSERT INTO checklists(id,name,type,profession) VALUES($1,'ML test','professional','ml')`, mlID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM checklists WHERE id=$1`, mlID) })
		stage := uuid.New()
		if _, err = db.ExecContext(ctx, `INSERT INTO checklist_stages(id,checklist_id,title,week_start,week_end,position) VALUES($1,$2,'ML',1,1,1)`, stage, mlID); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO checklist_items(checklist_id,stage_id,title,position) VALUES($1,$2,'ML task',1)`, mlID, stage); err != nil {
			t.Fatal(err)
		}
	}
	buddy := &model.User{Email: "ml-buddy-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleBuddy}
	if err = repository.NewUserRepository(db).Create(ctx, buddy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, buddy.ID) })
	intern, err := createTestIntern(ctx, db, "ml-intern-"+uuid.NewString()+"@example.com", "hash", model.ProfessionML, buddy.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, intern.ID) })
	lists, err := NewChecklistService(db).GetByUserID(ctx, intern.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lists.Common.TotalItems != 1 || lists.Professional.TotalItems < 1 || lists.Common.Items[0].Status != model.StatusActive || lists.Professional.Items[0].Status != model.StatusActive {
		t.Fatalf("ML assignment: %+v", lists)
	}
}
