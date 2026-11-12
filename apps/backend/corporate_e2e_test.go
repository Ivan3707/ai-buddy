package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func TestCorporateHTTPE2E(t *testing.T) {
	if os.Getenv("RUN_E2E") != "1" {
		t.Skip("set RUN_E2E=1")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var name string
	if err = db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "ai_buddy_test" {
		t.Fatalf("unsafe database %q: %v", name, err)
	}
	profession := model.ProfessionML
	buddy := &model.User{Email: "corp-http-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleBuddy}
	lead := &model.User{Email: "corp-http-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleTeamlead}
	for _, u := range []*model.User{buddy, lead} {
		if err = repository.NewUserRepository(db).Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		id := u.ID
		t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, id) })
	}
	own := &model.User{Email: "corp-http-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleIntern, Profession: &profession, BuddyID: &buddy.ID, TeamleadID: &lead.ID}
	other := &model.User{Email: "corp-http-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Role: model.RoleIntern, Profession: &profession}
	for _, u := range []*model.User{own, other} {
		if err = repository.NewUserRepository(db).Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		id := u.ID
		t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, id) })
	}
	data, err := json.Marshal([]service.CorporateEmployee{{ID: "own", Email: own.Email, Role: own.Role, Profession: &profession, BuddyID: "corp-buddy", ManagerID: "corp-lead", AllowedTools: []string{"ml-platform"}}, {ID: "other", Email: other.Email, Role: other.Role, Profession: &profession, AllowedTools: []string{"issue-tracker"}}, {ID: "corp-buddy", Email: buddy.Email, Role: buddy.Role, AllowedTools: []string{}}, {ID: "corp-lead", Email: lead.Email, Role: lead.Role, AllowedTools: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := service.NewCorporateDirectory(data)
	if err != nil {
		t.Fatal(err)
	}
	corp := service.NewCorporateService(db, dir)
	jwt, err := security.NewJWTService(testJWTSecret, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandler("", []byte(demo), handlerDependencies{users: repository.NewUserRepository(db), jwt: jwt, corporate: corp})
	request := func(method, path string, user *model.User, body any, want int) []byte {
		t.Helper()
		var payload []byte
		if body != nil {
			payload, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(payload))
		if user != nil {
			token, err := jwt.GenerateAccessToken(user.ID)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	request("GET", "/api/corporate/me", nil, nil, 401)
	request("GET", "/api/corporate/me", own, nil, 200)
	request("GET", "/api/corporate/employees/other", own, nil, 404)
	request("GET", "/api/corporate/employees/own", buddy, nil, 200)
	request("GET", "/api/corporate/employees/own", lead, nil, 200)
	request("GET", "/api/corporate/employees/other", buddy, nil, 404)
	request("GET", "/api/corporate/tools/ml-platform", own, nil, 200)
	request("GET", "/api/corporate/tools/payroll", own, nil, 403)
	path := "/api/corporate/access-requests"
	key := "request-" + uuid.NewString()
	request("POST", path, own, map[string]any{"tool": "ml-platform", "idempotency_key": key, "confirm": false}, 400)
	raw := request("POST", path, own, map[string]any{"tool": "ml-platform", "idempotency_key": key, "confirm": true}, 201)
	var created service.AccessRequest
	if err = json.Unmarshal(raw, &created); err != nil || created.ID == uuid.Nil || created.TicketNumber == "" {
		t.Fatalf("created ticket: %+v %v", created, err)
	}
	retry := request("POST", path, own, map[string]any{"tool": "ml-platform", "idempotency_key": key, "confirm": true}, 200)
	var same service.AccessRequest
	if err = json.Unmarshal(retry, &same); err != nil || same.ID != created.ID {
		t.Fatalf("idempotency retry: %+v %v", same, err)
	}
	request("POST", path, own, map[string]any{"tool": "payroll", "idempotency_key": "request-" + uuid.NewString(), "confirm": true}, 403)
	request("GET", path+"/"+created.ID.String(), other, nil, 404)
	request("GET", path+"/"+created.ID.String(), own, nil, 200)
	request("GET", path, own, nil, 200)
	corp.Unavailable = true
	request("GET", "/api/corporate/me", own, nil, 503)
	request("POST", path, own, map[string]any{"tool": "ml-platform", "idempotency_key": "request-" + uuid.NewString(), "confirm": true}, 503)
}
