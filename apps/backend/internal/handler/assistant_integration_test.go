package handler_test

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

// fakeAI imitates the Python AI service: SSE for chat/brief (the proposal is chosen by the message text),
// JSON for draft/gaps/lint, and records what the backend sent.
type fakeAI struct {
	mu       sync.Mutex
	requests map[string][]map[string]any
	down     bool
}

func (f *fakeAI) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.requests[r.URL.Path] = append(f.requests[r.URL.Path], body)
		down := f.down
		f.mu.Unlock()
		if down {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/v1/chat/stream", "/v1/brief/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			msg, _ := body["message"].(string)
			events := []string{
				`event: status` + "\n" + `data: {"stage":"retrieving"}`,
				`event: sources` + "\n" + `data: {"sources":[{"id":"leave","title":"Отпуска","url":"https://kb/leave"}]}`,
				`event: delta` + "\n" + `data: {"text":"Ответ по базе [1]."}`,
			}
			switch {
			case strings.Contains(msg, "отгул"):
				events = append(events, `event: action`+"\n"+`data: {"proposal":{"type":"leave_request","params":{"kind":"day_off","date_from":"2026-11-20","date_to":"2026-11-20"},"summary":"Отгул 20.11"}}`)
			case strings.Contains(msg, "admin"):
				events = append(events, `event: action`+"\n"+`data: {"proposal":{"type":"access_request","params":{"system":"grafana","resource":"prod","access_level":"admin","justification":"нужно"},"summary":"Админ"}}`)
			case strings.Contains(msg, "наставник"):
				events = append(events, `event: action`+"\n"+`data: {"proposal":{"type":"mentor_question","params":{"question":"Где взять датасет?","context":"нет в базе"},"summary":"Вопрос наставнику"}}`)
			case strings.Contains(msg, "плохо"):
				events = append(events, `event: action`+"\n"+`data: {"proposal":{"type":"leave_request","params":{"kind":"party"},"summary":"?"}}`)
			}
			events = append(events, `event: done`+"\n"+`data: {"finish_reason":"answered","mode":"llm"}`)
			for _, e := range events {
				_, _ = fmt.Fprintf(w, "%s\n\n", e)
			}
		case "/v1/mentor/draft":
			_, _ = w.Write([]byte(`{"draft":"Черновик","sources":[],"confidence":"none"}`))
		case "/v1/analytics/gaps":
			_, _ = w.Write([]byte(`{"total_questions":1,"clusters":[]}`))
		case "/v1/kb/lint":
			_, _ = w.Write([]byte(`{"issues":[]}`))
		case "/v1/kb/docs/leave":
			_, _ = w.Write([]byte(`{"id":"leave","title":"Отпуска","url":"u","updated":"","sections":[]}`))
		case "/v1/kb/docs/nope":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected AI path %s", r.URL.Path)
		}
	})
}

type mapUsers map[uuid.UUID]*model.User

func (m mapUsers) GetByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	if u, ok := m[id]; ok {
		return u, nil
	}
	return nil, repository.ErrNotFound
}

func openAssistantDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var name string
	if err := db.QueryRow(`SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "ai_buddy_test" && !strings.HasPrefix(name, "ai_buddy_verify_") {
		t.Fatalf("unsafe test database %q", name)
	}
	return db
}

type sseEvent struct {
	name string
	data map[string]any
}

func readSSE(t *testing.T, body io.Reader) []sseEvent {
	t.Helper()
	var out []sseEvent
	scanner := bufio.NewScanner(body)
	name := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			name = line[7:]
		} else if strings.HasPrefix(line, "data: ") {
			var data map[string]any
			if err := json.Unmarshal([]byte(line[6:]), &data); err != nil {
				t.Fatalf("bad SSE data %q: %v", line, err)
			}
			out = append(out, sseEvent{name, data})
		}
	}
	return out
}

func TestAssistantFlowWithPostgres(t *testing.T) {
	db := openAssistantDB(t)
	ctx := context.Background()
	str := func(s string) *string { return &s }
	ml := model.ProfessionML

	buddy := &model.User{ID: uuid.New(), Email: "buddy-" + uuid.NewString() + "@t.test", Role: model.RoleBuddy, FirstName: str("Олег"), LastName: str("Наставников")}
	lead := &model.User{ID: uuid.New(), Email: "lead-" + uuid.NewString() + "@t.test", Role: model.RoleTeamlead, FirstName: str("Ирина"), LastName: str("Руководова")}
	intern := &model.User{ID: uuid.New(), Email: "intern-" + uuid.NewString() + "@t.test", Role: model.RoleIntern, Profession: &ml,
		BuddyID: &buddy.ID, TeamleadID: &lead.ID, FirstName: str("Аня"), LastName: str("Стажёрова"), Team: str("Рекомендации")}
	other := &model.User{ID: uuid.New(), Email: "other-" + uuid.NewString() + "@t.test", Role: model.RoleIntern, Profession: &ml}
	users := mapUsers{buddy.ID: buddy, lead.ID: lead, intern.ID: intern, other.ID: other}
	for _, u := range []*model.User{buddy, lead, intern, other} {
		var prof any
		if u.Profession != nil {
			prof = string(*u.Profession)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, role, profession, buddy_id, teamlead_id, first_name, last_name, team)
			VALUES ($1,$2,'x',$3,$4,NULL,NULL,$5,$6,$7)`, u.ID, u.Email, string(u.Role), prof, u.FirstName, u.LastName, u.Team); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET buddy_id = $2, teamlead_id = $3 WHERE id = $1`, intern.ID, buddy.ID, lead.ID); err != nil {
		t.Fatal(err)
	}
	// One professional checklist: active task with a rejected submission, a pending-review task, a locked and a done one.
	checklistID := uuid.New()
	var progress []uuid.UUID
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.ExecContext(bg, `DELETE FROM checklist_item_submissions WHERE progress_id = ANY($1)`, progress)
		_, _ = db.ExecContext(bg, `DELETE FROM user_checklist_progress WHERE user_id = ANY($1)`, []uuid.UUID{intern.ID, other.ID})
		_, _ = db.ExecContext(bg, `DELETE FROM checklists WHERE id = $1`, checklistID)
		_, _ = db.ExecContext(bg, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{intern.ID, other.ID, buddy.ID, lead.ID})
	})
	if _, err := db.ExecContext(ctx, `INSERT INTO checklists (id, name, type, profession) VALUES ($1, $2, 'professional', 'ml')`, checklistID, "Assistant test "+checklistID.String()); err != nil {
		t.Fatal(err)
	}
	var stageID uuid.UUID
	if err := db.QueryRowContext(ctx, `INSERT INTO checklist_stages (checklist_id, title, week_start, week_end, position) VALUES ($1, 'Погружение', 1, 2, 1) RETURNING id`, checklistID).Scan(&stageID); err != nil {
		t.Fatal(err)
	}
	statuses := []string{"active", "active", "unavailable", "verified"}
	progress = make([]uuid.UUID, len(statuses))
	for i, st := range statuses {
		var itemID uuid.UUID
		if err := db.QueryRowContext(ctx, `INSERT INTO checklist_items (checklist_id, stage_id, title, description, position, requires_review)
			VALUES ($1, $2, $3, $4, $5, true) RETURNING id`, checklistID, stageID, fmt.Sprintf("Задача %d", i+1), strings.Repeat("д", 700*(i%2)), i+1).Scan(&itemID); err != nil {
			t.Fatal(err)
		}
		var completed any
		if st == "verified" {
			completed = time.Now()
		}
		if err := db.QueryRowContext(ctx, `INSERT INTO user_checklist_progress (user_id, checklist_id, checklist_item_id, status, position, completed_at)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, intern.ID, checklistID, itemID, st, i+1, completed).Scan(&progress[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO checklist_item_submissions (progress_id, attempt_number, result_text, status, submitted_at, reviewed_at, reviewed_by, review_comment)
		VALUES ($1, 1, 'r', 'rejected', now(), now(), $2, 'Добавь графики'), ($3, 1, 'r', 'pending', now(), NULL, NULL, NULL),
		       ($4, 1, 'r', 'approved', now(), now(), $2, 'Отлично')`, progress[0], buddy.ID, progress[1], progress[3]); err != nil {
		t.Fatal(err)
	}

	ai := &fakeAI{requests: map[string][]map[string]any{}}
	aiServer := httptest.NewServer(ai.handler(t))
	defer aiServer.Close()
	jwtService, err := security.NewJWTService(testJWTSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterAssistantRoutes(mux, middleware.Authenticate(jwtService, users), service.NewAssistantService(db, aiServer.URL))
	api := httptest.NewServer(mux)
	defer api.Close()

	call := func(u *model.User, method, path string, body any) (*http.Response, []byte) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = strings.NewReader(string(raw))
		}
		req, _ := http.NewRequest(method, api.URL+path, reader)
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
		return res, raw
	}
	expect := func(u *model.User, method, path string, body any, status int) []byte {
		t.Helper()
		res, raw := call(u, method, path, body)
		if res.StatusCode != status {
			t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, status, raw)
		}
		return raw
	}
	stream := func(u *model.User, path string, body any) []sseEvent {
		t.Helper()
		res, raw := call(u, http.MethodPost, path, body)
		if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatalf("%s: %d %s", path, res.StatusCode, raw)
		}
		return readSSE(t, strings.NewReader(string(raw)))
	}
	find := func(events []sseEvent, name string) map[string]any {
		for _, e := range events {
			if e.name == name {
				return e.data
			}
		}
		t.Fatalf("no %s event in %v", name, events)
		return nil
	}

	// ---- auth and validation
	expect(nil, http.MethodPost, "/api/chat/stream", map[string]string{"message": "x"}, http.StatusUnauthorized)
	expect(intern, http.MethodPost, "/api/chat/stream", map[string]string{"message": "  "}, http.StatusBadRequest)
	expect(intern, http.MethodPost, "/api/chat/stream", map[string]any{"message": "x", "page_context": map[string]string{"url": ""}}, http.StatusBadRequest)
	expect(intern, http.MethodPost, "/api/chat/stream", map[string]any{"message": "x", "extra": 1}, http.StatusBadRequest)
	expect(intern, http.MethodPost, "/api/chat/stream", map[string]any{"message": "x", "conversation_id": uuid.NewString()}, http.StatusNotFound)

	// ---- chat: meta first, verified user context, stored action
	events := stream(intern, "/api/chat/stream", map[string]any{"message": "Хочу отгул в пятницу",
		"page_context": map[string]string{"url": "https://wiki/x", "title": "Вики", "selection": "2 одобрения"}})
	if events[0].name != "meta" {
		t.Fatalf("first event %s", events[0].name)
	}
	conversationID := events[0].data["conversation_id"].(string)
	leaveAction := find(events, "action")["action"].(map[string]any)
	if leaveAction["status"] != "pending" || leaveAction["type"] != "leave_request" {
		t.Fatalf("action %v", leaveAction)
	}
	sent := ai.requests["/v1/chat/stream"][0]
	uc := sent["user_context"].(map[string]any)
	if uc["display_name"] != "Аня" || uc["buddy_name"] != "Олег Наставников" || uc["team"] != "Рекомендации" || uc["role"] != "intern" {
		t.Fatalf("user context %v", uc)
	}
	tasks := uc["tasks"].([]any)
	gotStatus := map[string]string{}
	for _, raw := range tasks {
		task := raw.(map[string]any)
		gotStatus[task["title"].(string)] = task["status"].(string)
	}
	if gotStatus["Задача 1"] != "returned" || gotStatus["Задача 2"] != "on_review" || gotStatus["Задача 3"] != "unavailable" || gotStatus["Задача 4"] != "completed" {
		t.Fatalf("task statuses %v", gotStatus)
	}
	if stage := uc["current_stage"].(map[string]any); stage["title"] != "Погружение" || stage["total"].(float64) != 1 {
		t.Fatalf("stage %v", stage)
	}
	if sent["page_context"] == nil || len(sent["history"].([]any)) != 0 {
		t.Fatalf("payload %v", sent)
	}

	// second turn in the same conversation carries history
	stream(intern, "/api/chat/stream", map[string]any{"message": "а за сколько дней?", "conversation_id": conversationID})
	if h := ai.requests["/v1/chat/stream"][1]["history"].([]any); len(h) != 2 {
		t.Fatalf("history %v", h)
	}
	// another intern cannot continue it
	expect(other, http.MethodPost, "/api/chat/stream", map[string]any{"message": "x", "conversation_id": conversationID}, http.StatusNotFound)

	// ---- actions: confirm is idempotent, cancel after execute conflicts, other users get 404
	actionID := leaveAction["id"].(string)
	expect(other, http.MethodPost, "/api/actions/"+actionID+"/confirm", nil, http.StatusNotFound)
	expect(intern, http.MethodPost, "/api/actions/not-a-uuid/confirm", nil, http.StatusBadRequest)
	var result map[string]any
	_ = json.Unmarshal(expect(intern, http.MethodPost, "/api/actions/"+actionID+"/confirm", nil, http.StatusOK), &result)
	request := result["request"].(map[string]any)
	if !strings.HasPrefix(request["number"].(string), "ABS-") || request["status"] != "pending_approval" || request["approver"] != "Ирина Руководова (руководитель)" {
		t.Fatalf("request %v", request)
	}
	var again map[string]any
	_ = json.Unmarshal(expect(intern, http.MethodPost, "/api/actions/"+actionID+"/confirm", nil, http.StatusOK), &again)
	if again["request"].(map[string]any)["number"] != request["number"] {
		t.Fatalf("confirm not idempotent: %v", again)
	}
	expect(intern, http.MethodPost, "/api/actions/"+actionID+"/cancel", nil, http.StatusConflict)

	// admin access is refused by the backend policy whatever the model proposed
	adminAction := find(stream(intern, "/api/chat/stream", map[string]any{"message": "дай admin"}), "action")["action"].(map[string]any)
	expect(intern, http.MethodPost, "/api/actions/"+adminAction["id"].(string)+"/confirm", nil, http.StatusUnprocessableEntity)
	expect(intern, http.MethodPost, "/api/actions/"+adminAction["id"].(string)+"/confirm", nil, http.StatusUnprocessableEntity)

	// invalid proposal is not stored; the answer still streams
	if evs := stream(intern, "/api/chat/stream", map[string]any{"message": "плохо"}); len(evs) == 0 || evs[len(evs)-1].name != "done" {
		t.Fatalf("events %v", evs)
	}

	// cancel a pending action, then confirming it conflicts
	pending := find(stream(intern, "/api/chat/stream", map[string]any{"message": "ещё отгул"}), "action")["action"].(map[string]any)
	var cancelled map[string]any
	_ = json.Unmarshal(expect(intern, http.MethodPost, "/api/actions/"+pending["id"].(string)+"/cancel", nil, http.StatusOK), &cancelled)
	if cancelled["status"] != "cancelled" {
		t.Fatalf("cancel %v", cancelled)
	}
	expect(intern, http.MethodPost, "/api/actions/"+pending["id"].(string)+"/confirm", nil, http.StatusConflict)
	expect(intern, http.MethodPost, "/api/actions/"+uuid.NewString()+"/cancel", nil, http.StatusNotFound)

	// ---- requests: list, get, decision by staff only
	var list []map[string]any
	_ = json.Unmarshal(expect(intern, http.MethodGet, "/api/requests", nil, http.StatusOK), &list)
	if len(list) != 1 {
		t.Fatalf("requests %v", list)
	}
	requestID := request["id"].(string)
	expect(intern, http.MethodGet, "/api/requests/"+requestID, nil, http.StatusOK)
	expect(other, http.MethodGet, "/api/requests/"+requestID, nil, http.StatusNotFound)
	expect(intern, http.MethodGet, "/api/buddy/requests", nil, http.StatusForbidden)
	expect(intern, http.MethodPost, "/api/buddy/requests/"+requestID+"/decision", map[string]string{"status": "approved"}, http.StatusForbidden)
	expect(buddy, http.MethodPost, "/api/buddy/requests/"+requestID+"/decision", map[string]string{"status": "maybe"}, http.StatusBadRequest)
	var staffList []map[string]any
	_ = json.Unmarshal(expect(lead, http.MethodGet, "/api/buddy/requests", nil, http.StatusOK), &staffList)
	if len(staffList) != 1 || staffList[0]["intern_name"] != "Аня Стажёрова" {
		t.Fatalf("staff requests %v", staffList)
	}
	var decided map[string]any
	_ = json.Unmarshal(expect(lead, http.MethodPost, "/api/buddy/requests/"+requestID+"/decision", map[string]string{"status": "approved"}, http.StatusOK), &decided)
	if decided["status"] != "approved" {
		t.Fatalf("decision %v", decided)
	}
	expect(buddy, http.MethodPost, "/api/buddy/requests/"+requestID+"/decision", map[string]string{"status": "rejected"}, http.StatusConflict)
	expect(buddy, http.MethodPost, "/api/buddy/requests/"+uuid.NewString()+"/decision", map[string]string{"status": "rejected"}, http.StatusNotFound)

	// ---- escalation to the buddy, draft, answer
	question := find(stream(intern, "/api/chat/stream", map[string]any{"message": "спроси наставника"}), "action")["action"].(map[string]any)
	var qResult map[string]any
	_ = json.Unmarshal(expect(intern, http.MethodPost, "/api/actions/"+question["id"].(string)+"/confirm", nil, http.StatusOK), &qResult)
	qid := qResult["mentor_question"].(map[string]any)["id"].(string)
	expect(intern, http.MethodGet, "/api/buddy/questions", nil, http.StatusForbidden)
	expect(buddy, http.MethodGet, "/api/buddy/questions?status=maybe", nil, http.StatusBadRequest)
	var questions []map[string]any
	_ = json.Unmarshal(expect(buddy, http.MethodGet, "/api/buddy/questions?status=open", nil, http.StatusOK), &questions)
	if len(questions) != 1 || questions[0]["intern_name"] != "Аня Стажёрова" {
		t.Fatalf("questions %v", questions)
	}
	expect(buddy, http.MethodGet, "/api/buddy/questions/"+qid+"/draft", nil, http.StatusOK)
	if ai.requests["/v1/mentor/draft"][0]["user_context"] == nil {
		t.Fatalf("draft without user context")
	}
	expect(buddy, http.MethodGet, "/api/buddy/questions/"+uuid.NewString()+"/draft", nil, http.StatusNotFound)
	expect(buddy, http.MethodPost, "/api/buddy/questions/"+qid+"/answer", map[string]string{"answer": " "}, http.StatusBadRequest)
	expect(buddy, http.MethodPost, "/api/buddy/questions/"+qid+"/answer", map[string]string{"answer": "Датасет в S3"}, http.StatusOK)
	expect(lead, http.MethodPost, "/api/buddy/questions/"+qid+"/answer", map[string]string{"answer": "ещё"}, http.StatusConflict)
	expect(buddy, http.MethodGet, "/api/analytics/gaps", nil, http.StatusOK)
	if q := ai.requests["/v1/analytics/gaps"][0]["questions"].([]any); len(q) != 1 {
		t.Fatalf("gaps payload %v", q)
	}
	expect(lead, http.MethodGet, "/api/kb/lint", nil, http.StatusOK)
	expect(intern, http.MethodGet, "/api/kb/lint", nil, http.StatusForbidden)
	expect(intern, http.MethodGet, "/api/kb/docs/leave", nil, http.StatusOK)
	// ---- request form in the chat window: same validation and policy as a confirmed action
	form := func(typ string, params map[string]string) map[string]any {
		return map[string]any{"type": typ, "params": params}
	}
	var created map[string]any
	if err := json.Unmarshal(expect(intern, http.MethodPost, "/api/requests", form("leave_request",
		map[string]string{"kind": "day_off", "date_from": "2026-12-05", "date_to": "2026-12-05"}), http.StatusCreated), &created); err != nil {
		t.Fatal(err)
	}
	if num, _ := created["number"].(string); !strings.HasPrefix(num, "ABS-") {
		t.Fatalf("form request number %v", created)
	}
	expect(intern, http.MethodPost, "/api/requests", form("access_request",
		map[string]string{"system": "grafana", "resource": "ML", "access_level": "admin", "justification": "надо"}), http.StatusUnprocessableEntity)
	expect(intern, http.MethodPost, "/api/requests", form("leave_request", map[string]string{"kind": "party"}), http.StatusUnprocessableEntity)
	expect(intern, http.MethodPost, "/api/requests", form("mentor_question", map[string]string{"question": "x"}), http.StatusUnprocessableEntity)
	expect(intern, http.MethodGet, "/api/kb/docs/nope", nil, http.StatusNotFound)
	expect(intern, http.MethodGet, "/api/kb/docs/bad%20id", nil, http.StatusNotFound)
	expect(intern, http.MethodGet, "/api/analytics/gaps", nil, http.StatusForbidden)

	// ---- history and feedback
	var history struct {
		ConversationID string           `json:"conversation_id"`
		Messages       []map[string]any `json:"messages"`
	}
	_ = json.Unmarshal(expect(intern, http.MethodGet, "/api/chat/messages?conversation_id="+conversationID, nil, http.StatusOK), &history)
	if len(history.Messages) != 4 || history.Messages[1]["action"] == nil || history.Messages[1]["sources"] == nil {
		t.Fatalf("history %+v", history)
	}
	assistantMsg := history.Messages[1]["id"].(string)
	expect(intern, http.MethodGet, "/api/chat/messages", nil, http.StatusOK)
	expect(intern, http.MethodGet, "/api/chat/messages?conversation_id=bad", nil, http.StatusBadRequest)
	expect(other, http.MethodGet, "/api/chat/messages?conversation_id="+conversationID, nil, http.StatusNotFound)
	var empty struct {
		Messages []any `json:"messages"`
	}
	_ = json.Unmarshal(expect(other, http.MethodGet, "/api/chat/messages", nil, http.StatusOK), &empty)
	if empty.Messages == nil || len(empty.Messages) != 0 {
		t.Fatalf("empty history %v", empty)
	}
	expect(intern, http.MethodPost, "/api/chat/messages/"+assistantMsg+"/feedback", map[string]string{"rating": "meh"}, http.StatusBadRequest)
	expect(other, http.MethodPost, "/api/chat/messages/"+assistantMsg+"/feedback", map[string]string{"rating": "up"}, http.StatusNotFound)
	expect(intern, http.MethodPost, "/api/chat/messages/"+assistantMsg+"/feedback", map[string]string{"rating": "up"}, http.StatusNoContent)

	// ---- brief continues the latest conversation; buddy gets a buddy context
	brief := stream(intern, "/api/chat/brief/stream", nil)
	if brief[0].data["conversation_id"] == "" {
		t.Fatalf("brief meta %v", brief[0])
	}
	stream(other, "/api/chat/brief/stream", nil)
	stream(buddy, "/api/chat/stream", map[string]any{"message": "привет"})
	last := ai.requests["/v1/chat/stream"]
	if bc := last[len(last)-1]["user_context"].(map[string]any); bc["role"] != "buddy" || len(bc["allowed_actions"].([]any)) != 2 {
		t.Fatalf("buddy context %v", bc)
	}

	// ---- metrics for staff only
	var metrics map[string]any
	_ = json.Unmarshal(expect(buddy, http.MethodGet, "/api/metrics/chat", nil, http.StatusOK), &metrics)
	if metrics["escalated_to_mentor"].(float64) != 1 || metrics["thumbs_up"].(float64) != 1 || metrics["actions_executed"].(float64) != 3 ||
		metrics["questions_total"].(float64) != 6 || metrics["deflection_rate"] == nil {
		t.Fatalf("metrics %v", metrics)
	}
	expect(intern, http.MethodGet, "/api/metrics/chat", nil, http.StatusForbidden)

	// ---- AI down: stream reports an error event, JSON endpoints 502
	ai.mu.Lock()
	ai.down = true
	ai.mu.Unlock()
	if e := find(stream(intern, "/api/chat/stream", map[string]any{"message": "что-нибудь"}), "error"); e["code"] != "llm_unavailable" {
		t.Fatalf("error event %v", e)
	}
	expect(buddy, http.MethodGet, "/api/kb/lint", nil, http.StatusBadGateway)
}
