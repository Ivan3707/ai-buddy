package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func TestCorporateAccessRequestIntegration(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	profession := model.ProfessionML
	first := &model.User{Email: "corp-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleIntern, Profession: &profession}
	second := &model.User{Email: "corp-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleIntern, Profession: &profession}
	for _, user := range []*model.User{first, second} {
		if err := repository.NewUserRepository(db).Create(ctx, user); err != nil {
			t.Fatal(err)
		}
		id := user.ID
		t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, id) })
	}
	data, err := json.Marshal([]CorporateEmployee{{ID: "corp-one", Email: first.Email, Role: model.RoleIntern, Profession: &profession, AllowedTools: []string{"ml-platform", "issue-tracker"}}, {ID: "corp-two", Email: second.Email, Role: model.RoleIntern, Profession: &profession, AllowedTools: []string{"issue-tracker"}}})
	if err != nil {
		t.Fatal(err)
	}
	directory, err := NewCorporateDirectory(data)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewCorporateService(db, directory)
	if profile, err := svc.Profile(first); err != nil || profile.ID != "corp-one" {
		t.Fatalf("profile=%+v err=%v", profile, err)
	}
	forged := *first
	forged.Role = model.RoleBuddy
	if _, err := svc.Profile(&forged); !errors.Is(err, ErrCorporateForbidden) {
		t.Fatalf("mismatched corporate role: %v", err)
	}
	if _, err := svc.ToolInfo(first, "ml-platform"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToolInfo(first, "payroll"); !errors.Is(err, ErrCorporateForbidden) {
		t.Fatalf("forbidden tool: %v", err)
	}
	key := "request-" + uuid.NewString()
	made, created, err := svc.CreateRequest(ctx, first, "ml-platform", key)
	if err != nil || !created || made.TicketNumber == "" {
		t.Fatalf("create=%+v %v %v", made, created, err)
	}
	again, created, err := svc.CreateRequest(ctx, first, "ml-platform", key)
	if err != nil || created || again.ID != made.ID {
		t.Fatalf("retry=%+v %v %v", again, created, err)
	}
	if _, _, err := svc.CreateRequest(ctx, first, "issue-tracker", key); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("key reuse: %v", err)
	}
	if _, _, err := svc.CreateRequest(ctx, first, "payroll", "request-"+uuid.NewString()); !errors.Is(err, ErrCorporateForbidden) {
		t.Fatalf("forbidden create: %v", err)
	}
	if _, err := svc.GetRequest(ctx, second, made.ID); !errors.Is(err, ErrCorporateNotFound) {
		t.Fatalf("foreign request: %v", err)
	}
	list, err := svc.ListRequests(ctx, first)
	if err != nil || len(list) != 1 || list[0].ID != made.ID {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	svc.Unavailable = true
	if _, _, err := svc.CreateRequest(ctx, first, "ml-platform", "request-"+uuid.NewString()); !errors.Is(err, ErrCorporateUnavailable) {
		t.Fatalf("unavailable create: %v", err)
	}
	if _, err := svc.GetRequest(ctx, first, made.ID); !errors.Is(err, ErrCorporateUnavailable) {
		t.Fatalf("unavailable read: %v", err)
	}
}
