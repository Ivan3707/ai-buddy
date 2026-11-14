package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type countingTicker struct{ calls atomic.Int32 }

func (c *countingTicker) Tick(context.Context) error { c.calls.Add(1); return nil }

func TestAdminRoutesWithPostgres(t *testing.T) {
	db := openAssistantDB(t)
	ctx := context.Background()
	str := func(s string) *string { return &s }
	be := model.ProfessionBackend
	const secretHash = "SECRET-HASH-do-not-leak"

	lead := &model.User{ID: uuid.New(), Email: "adm-lead-" + uuid.NewString() + "@t.test", Role: model.RoleTeamlead, FirstName: str("Ирина"), LastName: str("Руководова")}
	buddy := &model.User{ID: uuid.New(), Email: "adm-buddy-" + uuid.NewString() + "@t.test", Role: model.RoleBuddy}
	intern := &model.User{ID: uuid.New(), Email: "adm-intern-" + uuid.NewString() + "@t.test", Role: model.RoleIntern, Profession: &be}
	users := mapUsers{lead.ID: lead, buddy.ID: buddy, intern.ID: intern}
	checklistID := uuid.New()
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.ExecContext(bg, `DELETE FROM user_checklist_progress WHERE user_id = $1`, intern.ID)
		_, _ = db.ExecContext(bg, `DELETE FROM checklists WHERE id = $1`, checklistID)
		_, _ = db.ExecContext(bg, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{intern.ID, buddy.ID, lead.ID})
	})
	for _, u := range []*model.User{lead, buddy, intern} {
		var prof any
		if u.Profession != nil {
			prof = string(*u.Profession)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, role, profession, first_name, last_name) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			u.ID, u.Email, secretHash, string(u.Role), prof, u.FirstName, u.LastName); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET buddy_id=$2, teamlead_id=$3 WHERE id=$1`, intern.ID, buddy.ID, lead.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO auth_sessions (user_id, token_hash, expires_at) VALUES ($1, $2, now() + interval '1 day'), ($1, $3, now() + interval '1 day')`,
		intern.ID, []byte(uuid.NewString()), []byte(uuid.NewString())); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at = now() WHERE id = (SELECT id FROM auth_sessions WHERE user_id=$1 LIMIT 1)`, intern.ID); err != nil {
		t.Fatal(err)
	}
	// An active task that is past its estimate.
	if _, err := db.ExecContext(ctx, `INSERT INTO checklists (id, name, type, profession) VALUES ($1, $2, 'professional', 'backend')`, checklistID, "Admin test "+checklistID.String()); err != nil {
		t.Fatal(err)
	}
	var stageID, itemID uuid.UUID
	if err := db.QueryRowContext(ctx, `INSERT INTO checklist_stages (checklist_id, title, week_start, week_end, position) VALUES ($1,'Этап',1,2,1) RETURNING id`, checklistID).Scan(&stageID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO checklist_items (checklist_id, stage_id, title, description, position, estimated_minutes) VALUES ($1,$2,'Долгая задача','d',1,10) RETURNING id`, checklistID, stageID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO user_checklist_progress (user_id, checklist_id, checklist_item_id, status, position, activated_at) VALUES ($1,$2,$3,'active',1, now() - interval '1 day')`, intern.ID, checklistID, itemID); err != nil {
		t.Fatal(err)
	}

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /health":
			_, _ = w.Write([]byte(`{"status":"ok","mode":"mock"}`))
		case "GET /v1/admin/llm":
			_, _ = w.Write([]byte(`{"provider":"mock","mode":"mock","model":"","fallback_model":"","balance":{"available":false,"note":"mock"},"budget":null}`))
		case "POST /v1/admin/llm-ping":
			_, _ = w.Write([]byte(`{"ok":true,"mode":"mock","latency_ms":0,"answer":"pong (mock)","input_tokens":0,"output_tokens":0,"error":""}`))
		case "GET /v1/admin/kb":
			_, _ = w.Write([]byte(`{"documents":3,"chunks":9,"source":"markdown"}`))
		case "GET /v1/kb/lint":
			_, _ = w.Write([]byte(`{"issues":[{"kind":"stale","doc_ids":["a"],"detail":"old"}]}`))
		case "POST /v1/analytics/gaps":
			_, _ = w.Write([]byte(`{"total_questions":0,"clusters":[]}`))
		case "POST /v1/kb/reload":
			_, _ = w.Write([]byte(`{"documents":3,"chunks":9,"source":"markdown"}`))
		default:
			t.Errorf("unexpected AI call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer aiServer.Close()

	jwtService, err := security.NewJWTService(testJWTSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ticker := &countingTicker{}
	mux := http.NewServeMux()
	handler.RegisterAdminRoutes(mux, middleware.Authenticate(jwtService, users), service.NewAdminService(db, aiServer.URL, ticker))
	api := httptest.NewServer(mux)
	defer api.Close()

	call := func(u *model.User, method, path string) (int, []byte) {
		t.Helper()
		req, _ := http.NewRequest(method, api.URL+path, nil)
		if u != nil {
			token, _ := jwtService.GenerateAccessToken(u.ID)
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, raw
	}
	routes := [][2]string{
		{"GET", "/api/admin/accounts"}, {"GET", "/api/admin/llm"}, {"POST", "/api/admin/llm/ping"},
		{"GET", "/api/admin/system"}, {"POST", "/api/admin/kb/reload"}, {"GET", "/api/admin/notifications"},
		{"POST", "/api/admin/notifications/stuck_task/check"}, {"GET", "/api/admin/kb/lint"}, {"GET", "/api/admin/kb/gaps"},
		{"GET", "/api/admin/feedback"},
	}
	for _, rt := range routes {
		if code, _ := call(nil, rt[0], rt[1]); code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: %d", rt[0], rt[1], code)
		}
		for _, u := range []*model.User{intern, buddy} {
			if code, _ := call(u, rt[0], rt[1]); code != http.StatusForbidden {
				t.Errorf("%s %s as %s: %d, want 403", rt[0], rt[1], u.Role, code)
			}
		}
	}
	if ticker.calls.Load() != 0 {
		t.Fatal("worker ticked for a non-teamlead")
	}

	code, raw := call(lead, "GET", "/api/admin/accounts")
	if code != 200 {
		t.Fatalf("accounts: %d %s", code, raw)
	}
	if strings.Contains(string(raw), secretHash) || strings.Contains(strings.ToLower(string(raw)), "password") || strings.Contains(string(raw), "token") {
		t.Fatalf("accounts leak secrets: %s", raw)
	}
	var accounts struct {
		Accounts []service.AdminAccount `json:"accounts"`
	}
	if err := json.Unmarshal(raw, &accounts); err != nil {
		t.Fatal(err)
	}
	var found *service.AdminAccount
	for i, a := range accounts.Accounts {
		if a.ID == intern.ID.String() {
			found = &accounts.Accounts[i]
		}
	}
	if found == nil || found.Role != "intern" || found.Profession != "backend" || found.BuddyName != buddy.Email || found.TeamleadName != "Ирина Руководова" || found.ActiveSessions != 1 {
		t.Fatalf("unexpected intern account: %+v", found)
	}

	code, raw = call(lead, "GET", "/api/admin/llm")
	var llm map[string]any
	if code != 200 || json.Unmarshal(raw, &llm) != nil || llm["mode"] != "mock" {
		t.Fatalf("llm: %d %s", code, raw)
	}
	if code, raw = call(lead, "POST", "/api/admin/llm/ping"); code != 200 || !strings.Contains(string(raw), `"ok":true`) {
		t.Fatalf("ping: %d %s", code, raw)
	}
	if code, raw = call(lead, "GET", "/api/admin/kb/lint"); code != 200 || !strings.Contains(string(raw), `"stale"`) {
		t.Fatalf("kb lint: %d %s", code, raw)
	}
	if code, raw = call(lead, "GET", "/api/admin/kb/gaps"); code != 200 || !strings.Contains(string(raw), `"clusters"`) {
		t.Fatalf("kb gaps: %d %s", code, raw)
	}
	if code, raw = call(lead, "POST", "/api/admin/kb/reload"); code != 200 || !strings.Contains(string(raw), `"chunks":9`) {
		t.Fatalf("reload: %d %s", code, raw)
	}

	code, raw = call(lead, "GET", "/api/admin/system")
	var sys service.AdminSystem
	if code != 200 || json.Unmarshal(raw, &sys) != nil {
		t.Fatalf("system: %d %s", code, raw)
	}
	ids := map[string]string{}
	for _, c := range sys.Checks {
		ids[c.ID] = c.Status
	}
	for _, id := range []string{"database", "ai", "kb", "hint_worker"} {
		if ids[id] != "ok" {
			t.Errorf("check %s = %q (%s)", id, ids[id], raw)
		}
	}
	if sys.Counters.UsersByRole["intern"] < 1 || sys.Counters.UsersByRole["teamlead"] < 1 {
		t.Errorf("counters: %+v", sys.Counters)
	}

	code, raw = call(lead, "GET", "/api/admin/notifications")
	var cat struct {
		Items []service.NotificationInfo `json:"items"`
	}
	if code != 200 || json.Unmarshal(raw, &cat) != nil || len(cat.Items) != 7 {
		t.Fatalf("notifications: %d %s", code, raw)
	}
	want := []string{"greeting", "page_hint", "selection", "request_status", "stuck_task", "mentor_answered", "staff_counts"}
	for i, id := range want {
		if cat.Items[i].ID != id || len(cat.Items[i].Samples) == 0 || cat.Items[i].When == "" {
			t.Errorf("catalog entry %d: %+v", i, cat.Items[i])
		}
	}
	if len(cat.Items[1].Samples) != 4 || cat.Items[4].Check != "backend" || len(cat.Items[4].Live) == 0 || cat.Items[4].Live[0].Value < 1 {
		t.Errorf("catalog details: %+v %+v", cat.Items[1], cat.Items[4])
	}

	code, raw = call(lead, "POST", "/api/admin/notifications/stuck_task/check")
	var check service.StuckCheck
	if code != 200 || json.Unmarshal(raw, &check) != nil {
		t.Fatalf("check: %d %s", code, raw)
	}
	if ticker.calls.Load() != 1 || check.OverdueTasks < 1 || len(check.Tasks) == 0 || len(check.Tasks) > 10 {
		t.Fatalf("check result: calls=%d %+v", ticker.calls.Load(), check)
	}
}
