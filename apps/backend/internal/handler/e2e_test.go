package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type progressResponseDTO struct {
	TotalItems        int     `json:"total_items"`
	CompletedItems    int     `json:"completed_items"`
	CompletionPercent float64 `json:"completion_percent"`
	Common            *struct {
		TotalItems int `json:"total_items"`
	} `json:"common"`
	Professional struct {
		TotalItems int `json:"total_items"`
	} `json:"professional"`
}

// Run explicitly with RUN_E2E=1 and TEST_DATABASE_URL pointing to an isolated database.
// This test uses real HTTP requests, services, repositories and PostgreSQL.
// It creates and cleans up only its own rows. Do not run in parallel with
// other tests that create global common/backend checklists.
func TestE2E_InternBuddyWorkflow(t *testing.T) {
	if os.Getenv("RUN_E2E") != "1" {
		t.Skip("set RUN_E2E=1 to enable the end-to-end test")
	}

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL is required")
	}

	// Check the actual PostgreSQL database name, not a substring of the DSN.
	// This intentionally rejects app, postgres, and any unknown test database.
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close E2E database: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	var dbName string
	if err := db.QueryRowContext(ctx, "SELECT current_database()").Scan(&dbName); err != nil {
		t.Fatalf("read database name: %v", err)
	}
	if dbName != "ai_buddy_test" && !strings.HasPrefix(dbName, "ai_buddy_verify_") {
		t.Fatalf("unsafe E2E database %q", dbName)
	}
	var version int
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_id), 0)
		FROM goose_db_version WHERE is_applied
	`).Scan(&version); err != nil {
		t.Fatalf("read goose version: %v", err)
	}
	if version != 7 {
		t.Fatalf("test database must be at Goose v7, got v%d", version)
	}

	// Only one common and one backend professional checklist are permitted.
	// Never overwrite or delete checklists belonging to another test/seed.
	var conflicting int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM checklists
		WHERE type = 'common' OR (type = 'professional' AND profession = 'backend')
	`).Scan(&conflicting); err != nil {
		t.Fatalf("check fixture conflicts: %v", err)
	}
	if conflicting != 0 {
		t.Fatal("ai_buddy_test already contains common/backend checklists; use a clean dedicated test DB, do not delete existing data automatically")
	}

	commonID, professionalID := uuid.New(), uuid.New()
	commonItems := []uuid.UUID{uuid.New(), uuid.New()}
	professionalItems := []uuid.UUID{uuid.New(), uuid.New()}
	buddyID, otherBuddyID, teamleadID := uuid.New(), uuid.New(), uuid.New()
	var internID uuid.UUID

	// Cleanup is registered before inserts, so it also runs on assertion failure.
	// A separate context is essential because the test context may be canceled.
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if internID != uuid.Nil {
			if _, err := db.ExecContext(cleanupCtx, `DELETE FROM checklist_item_submissions
				WHERE progress_id IN (SELECT id FROM user_checklist_progress WHERE user_id = $1)`, internID); err != nil {
				t.Errorf("E2E cleanup submissions: %v", err)
			}
			if _, err := db.ExecContext(cleanupCtx, `DELETE FROM checklist_items WHERE assigned_user_id=$1 AND checklist_id=$2`, internID, professionalID); err != nil {
				t.Errorf("E2E cleanup personal items: %v", err)
			}
			if _, err := db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id = $1`, internID); err != nil {
				t.Errorf("E2E cleanup intern: %v", err)
			}
		}
		if _, err := db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id IN ($1, $2, $3)`, buddyID, otherBuddyID, teamleadID); err != nil {
			t.Errorf("E2E cleanup buddies: %v", err)
		}
		if _, err := db.ExecContext(cleanupCtx, `DELETE FROM checklists WHERE id IN ($1, $2)`, commonID, professionalID); err != nil {
			t.Errorf("E2E cleanup checklists: %v", err)
		}
	})

	// SQL is used only to seed content and buddy accounts (no public API exists
	// for either). All business operations below go through actual HTTP.
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("create E2E fixture: %v", err)
		}
	}
	mustExec(`INSERT INTO checklists (id, name, type) VALUES ($1, 'E2E common', 'common')`, commonID)
	mustExec(`INSERT INTO checklists (id, name, type, profession)
		VALUES ($1, 'E2E backend', 'professional', 'backend')`, professionalID)

	seedChecklist := func(checklistID uuid.UUID, items []uuid.UUID, reviewFirst bool) {
		t.Helper()
		stageID := uuid.New()
		mustExec(`INSERT INTO checklist_stages
			(id, checklist_id, title, week_start, week_end, goal, expected_result, position)
			VALUES ($1, $2, 'E2E stage', 1, 1, 'Testing', 'Working API', 1)`, stageID, checklistID)
		for i, itemID := range items {
			mustExec(`INSERT INTO checklist_items
				(id, checklist_id, stage_id, title, description, requires_review, estimated_minutes, position)
				VALUES ($1, $2, $3, $4, 'E2E test item', $5, 30, $6)`,
				itemID, checklistID, stageID, fmt.Sprintf("E2E task %d", i+1), reviewFirst && i == 0, i+1)
		}
	}
	seedChecklist(commonID, commonItems, false)
	seedChecklist(professionalID, professionalItems, true)

	buddyPassword := "e2e-buddy-password-123"
	internPassword := "e2e-intern-password-123"
	buddyHash, err := security.HashPassword(buddyPassword)
	if err != nil {
		t.Fatalf("hash buddy password: %v", err)
	}
	buddyEmail := "buddy-" + uuid.NewString() + "@example.com"
	otherEmail := "other-" + uuid.NewString() + "@example.com"
	mustExec(`INSERT INTO users (id, email, password_hash, role, first_name, last_name, telegram_username)
		VALUES ($1, $2, $3, 'buddy', 'Assigned', 'Buddy', 'assigned_buddy'), ($4, $5, $3, 'buddy', NULL, NULL, NULL)`,
		buddyID, buddyEmail, buddyHash, otherBuddyID, otherEmail)
	teamleadEmail := "lead-" + uuid.NewString() + "@example.com"
	mustExec(`INSERT INTO users(id,email,password_hash,role,first_name,last_name) VALUES($1,$2,$3,'teamlead','Assigned','Lead')`, teamleadID, teamleadEmail, buddyHash)

	jwtService, err := security.NewJWTService(testJWTSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("configure JWT: %v", err)
	}
	users := repository.NewUserRepository(db)
	progress := service.NewProgressService(db)
	userService := service.NewUserService(db)
	personalTasks := service.NewPersonalTaskService(db)
	server := httptest.NewServer(handler.NewRouter("", handler.Dependencies{
		Auth:             service.NewAuthService(users, jwtService),
		Users:            users,
		JWT:              jwtService,
		Team:             userService,
		PersonalTasks:    personalTasks,
		ChecklistEditor:  personalTasks,
		StaffSubmissions: progress,
		Checklists:       service.NewChecklistService(db),
		Items:            progress,
		Submissions:      progress,
	}))
	defer server.Close()
	client := &http.Client{Timeout: 12 * time.Second}

	// Helpers enforce HTTP status and decode actual JSON response bodies.
	request := func(method, path, token string, payload any, wantStatus int, out any) {
		t.Helper()
		var body io.Reader
		if payload != nil {
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("encode request: %v", err)
			}
			body = bytes.NewReader(data)
		}
		req, err := http.NewRequestWithContext(ctx, method, server.URL+path, body)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(io.LimitReader(res.Body, 64*1024))
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		if res.StatusCode != wantStatus {
			t.Fatalf("%s %s: status %d, want %d; body: %s", method, path, res.StatusCode, wantStatus, data)
		}
		if out != nil {
			if err := json.Unmarshal(data, out); err != nil {
				t.Fatalf("decode %s %s: %v (%s)", method, path, err, data)
			}
		}
	}
	login := func(email, password string, first bool) string {
		t.Helper()
		var response loginResponse
		request(http.MethodPost, "/api/auth/login", "", map[string]string{
			"email": email, "password": password,
		}, http.StatusOK, &response)
		if response.IsFirstLogin != first || response.AccessToken == "" || response.TokenType != "Bearer" {
			t.Fatalf("unexpected login result for %s: first=%v tokenPresent=%v", email, response.IsFirstLogin, response.AccessToken != "")
		}
		return response.AccessToken
	}
	getChecklists := func(token string) checklistsResponse {
		t.Helper()
		var value checklistsResponse
		request(http.MethodGet, "/api/checklists", token, nil, http.StatusOK, &value)
		return value
	}
	itemState := func(items []checklistStageDTO, id uuid.UUID) checklistItemDTO {
		t.Helper()
		for _, stage := range items {
			for _, item := range stage.Items {
				if item.ID == id.String() {
					return item
				}
			}
		}
		t.Fatalf("item %s missing from checklist response", id)
		return checklistItemDTO{}
	}
	assertStatus := func(item checklistItemDTO, want model.ChecklistStatus) {
		t.Helper()
		if item.Status != want {
			t.Fatalf("item %s status = %q, want %q", item.ID, item.Status, want)
		}
	}

	request(http.MethodGet, "/health", "", nil, http.StatusOK, nil)
	request(http.MethodGet, "/api/checklists", "", nil, http.StatusUnauthorized, nil)
	buddyToken := login(buddyEmail, buddyPassword, true)
	otherBuddyToken := login(otherEmail, buddyPassword, true)
	teamleadToken := login(teamleadEmail, buddyPassword, true)

	internEmail := "intern-" + uuid.NewString() + "@example.com"
	request(http.MethodPost, "/api/users", buddyToken, map[string]string{"email": internEmail}, http.StatusNotFound, nil)
	request(http.MethodPost, "/api/users", teamleadToken, map[string]string{"email": internEmail}, http.StatusNotFound, nil)
	internHash, err := security.HashPassword(internPassword)
	if err != nil {
		t.Fatal(err)
	}
	// Accounts are created in the product only by HR sync; the fixture inserts
	// the intern directly and reuses the shared progress initializer.
	internProfession := model.ProfessionBackend
	birthDate := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	employedSince := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	firstName, lastName, telegram, department, team := "Test", "Intern", "", "Engineering", "Backend"
	created := &model.User{
		Email: internEmail, PasswordHash: internHash, Role: model.RoleIntern, Profession: &internProfession,
		BuddyID: &buddyID, TeamleadID: &teamleadID, BirthDate: &birthDate, EmployedSince: &employedSince,
		FirstName: &firstName, LastName: &lastName, TelegramUsername: &telegram, Department: &department, Team: &team,
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin E2E intern fixture: %v", err)
	}
	if err := repository.NewUserRepository(tx).Create(ctx, created); err != nil {
		_ = tx.Rollback()
		t.Fatalf("create E2E intern fixture: %v", err)
	}
	if err := service.InitializeInternProgress(ctx, tx, created.ID, internProfession); err != nil {
		_ = tx.Rollback()
		t.Fatalf("initialize E2E intern progress: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit E2E intern fixture: %v", err)
	}
	internID = created.ID
	request(http.MethodPut, "/api/buddy/interns/"+internID.String()+"/teamlead", buddyToken, map[string]string{"teamlead_id": teamleadID.String()}, http.StatusNotFound, nil)

	request(http.MethodPost, "/api/auth/login", "", map[string]string{
		"email": internEmail, "password": "wrong-password",
	}, http.StatusUnauthorized, nil)
	var beforeLogin sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT last_login_at FROM users WHERE id = $1`, internID).Scan(&beforeLogin); err != nil || beforeLogin.Valid {
		t.Fatalf("failed login must not update last_login_at: %v %v", beforeLogin, err)
	}
	internToken := login(internEmail, internPassword, true)
	login(internEmail, internPassword, false)
	var lastLogin sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT last_login_at FROM users WHERE id = $1`, internID).Scan(&lastLogin); err != nil || !lastLogin.Valid {
		t.Fatalf("successful login must set last_login_at: %v %v", lastLogin, err)
	}
	var me publicUser
	request(http.MethodGet, "/api/auth/me", internToken, nil, http.StatusOK, &me)
	if me.ID != internID.String() || me.Role != model.RoleIntern {
		t.Fatalf("/api/auth/me returned wrong user: %+v", me)
	}
	if me.Buddy == nil || me.Buddy.ID != buddyID.String() || me.Buddy.TelegramUsername == nil || *me.Buddy.TelegramUsername != "assigned_buddy" {
		t.Fatalf("/api/auth/me missing assigned buddy: %+v", me)
	}
	if me.Teamlead == nil || me.Teamlead.ID != teamleadID.String() || me.AgeYears == nil || *me.AgeYears < 16 || me.TenureMonths == nil {
		t.Fatalf("/api/auth/me missing age, tenure or teamlead: %+v", me)
	}
	request(http.MethodGet, "/api/checklists", buddyToken, nil, http.StatusForbidden, nil)
	request(http.MethodGet, "/api/buddy/submissions", internToken, nil, http.StatusForbidden, nil)
	var buddyInterns []publicUser
	request(http.MethodGet, "/api/staff/interns", buddyToken, nil, http.StatusOK, &buddyInterns)
	if len(buddyInterns) != 1 || buddyInterns[0].ID != internID.String() {
		t.Fatalf("buddy assigned intern list is not isolated: %+v", buddyInterns)
	}
	var leadInterns []publicUser
	request(http.MethodGet, "/api/staff/interns", teamleadToken, nil, http.StatusOK, &leadInterns)
	if len(leadInterns) != 1 || leadInterns[0].ID != internID.String() {
		t.Fatalf("teamlead assigned intern list is not isolated: %+v", leadInterns)
	}
	var teamleads []publicUser
	request(http.MethodGet, "/api/staff/teamleads", buddyToken, nil, http.StatusOK, &teamleads)
	if len(teamleads) == 0 {
		t.Fatal("buddy teamlead lookup returned no users")
	}
	request(http.MethodGet, "/api/staff/teamleads", teamleadToken, nil, http.StatusForbidden, nil)
	request(http.MethodGet, "/api/staff/interns", internToken, nil, http.StatusForbidden, nil)

	initial := getChecklists(internToken)
	var ownProgress progressResponseDTO
	request(http.MethodGet, "/api/progress", internToken, nil, http.StatusOK, &ownProgress)
	if ownProgress.TotalItems != len(commonItems)+len(professionalItems) || ownProgress.CompletedItems != 0 || ownProgress.Common == nil {
		t.Fatalf("initial overall progress: %+v", ownProgress)
	}
	var leadProgress progressResponseDTO
	request(http.MethodGet, "/api/staff/interns/"+internID.String()+"/progress", teamleadToken, nil, http.StatusOK, &leadProgress)
	if leadProgress.Common == nil || leadProgress.TotalItems != len(commonItems)+len(professionalItems) || leadProgress.Professional.TotalItems != len(professionalItems) {
		t.Fatalf("teamlead progress must include both checklists: %+v", leadProgress)
	}
	request(http.MethodGet, "/api/staff/interns/"+internID.String()+"/progress", otherBuddyToken, nil, http.StatusNotFound, nil)
	request(http.MethodPost, "/api/teamlead/interns/"+internID.String()+"/checklist-items", buddyToken, map[string]any{"position": 1, "title": "denied", "expected_result": "x", "estimated_minutes": 30}, http.StatusForbidden, nil)
	if initial.Common.TotalItems != 2 || initial.Professional.TotalItems != 2 ||
		initial.Common.CompletedItems != 0 || initial.Professional.CompletedItems != 0 {
		t.Fatalf("unexpected initial checklists: %+v", initial)
	}
	assertStatus(itemState(initial.Common.Stages, commonItems[0]), model.StatusActive)
	assertStatus(itemState(initial.Common.Stages, commonItems[1]), model.StatusUnavailable)
	assertStatus(itemState(initial.Professional.Stages, professionalItems[0]), model.StatusActive)
	assertStatus(itemState(initial.Professional.Stages, professionalItems[1]), model.StatusUnavailable)
	if itemState(initial.Professional.Stages, professionalItems[0]).EstimatedMinutes == nil ||
		*itemState(initial.Professional.Stages, professionalItems[0]).EstimatedMinutes != 30 {
		t.Fatal("estimated_minutes missing from API")
	}

	commonPath := "/api/checklist-items/" + commonItems[0].String() + "/complete"
	request(http.MethodPost, commonPath, internToken, nil, http.StatusNoContent, nil)
	completed := getChecklists(internToken)
	assertStatus(itemState(completed.Common.Stages, commonItems[0]), model.StatusCompleted)
	assertStatus(itemState(completed.Common.Stages, commonItems[1]), model.StatusActive)
	if completed.Common.CompletedItems != 1 || completed.Common.CompletionPercent != 50 {
		t.Fatalf("unexpected common completion: %+v", completed.Common)
	}
	assertStatus(itemState(completed.Professional.Stages, professionalItems[0]), model.StatusActive)
	firstCompletedAt := itemState(completed.Common.Stages, commonItems[0]).CompletedAt
	if firstCompletedAt == nil {
		t.Fatal("self-completed item lacks completed_at")
	}
	request(http.MethodPost, commonPath, internToken, nil, http.StatusNoContent, nil)
	again := getChecklists(internToken)
	if x := itemState(again.Common.Stages, commonItems[0]).CompletedAt; x == nil || *x != *firstCompletedAt {
		t.Fatal("idempotent completion changed completed_at")
	}

	reviewItemPath := "/api/checklist-items/" + professionalItems[0].String()
	request(http.MethodPost, reviewItemPath+"/complete", internToken, nil, http.StatusBadRequest, nil)
	request(http.MethodPost, reviewItemPath+"/submissions", internToken,
		map[string]string{"result_text": "   "}, http.StatusBadRequest, nil)
	var attempt1 submissionDTO
	request(http.MethodPost, reviewItemPath+"/submissions", internToken,
		map[string]string{"result_text": "E2E first attempt"}, http.StatusCreated, &attempt1)
	if attempt1.ID == "" || attempt1.AttemptNumber != 1 || attempt1.Status != model.SubmissionPending {
		t.Fatalf("unexpected first submission: %+v", attempt1)
	}
	request(http.MethodPost, reviewItemPath+"/submissions", internToken,
		map[string]string{"result_text": "duplicate"}, http.StatusConflict, nil)
	afterSubmit := getChecklists(internToken)
	assertStatus(itemState(afterSubmit.Professional.Stages, professionalItems[0]), model.StatusCompleted)
	assertStatus(itemState(afterSubmit.Professional.Stages, professionalItems[1]), model.StatusActive)
	if afterSubmit.Professional.CompletedItems != 0 || afterSubmit.Professional.CompletionPercent != 0 {
		t.Fatalf("review-pending task should not count as confirmed: %+v", afterSubmit.Professional)
	}

	var pending []pendingSubmissionDTO
	request(http.MethodGet, "/api/buddy/submissions", buddyToken, nil, http.StatusOK, &pending)
	if len(pending) != 1 || pending[0].ID != attempt1.ID || pending[0].InternID != internID.String() {
		t.Fatalf("buddy pending queue should contain exactly this intern's submission: %+v", pending)
	}
	var foreignPending []pendingSubmissionDTO
	request(http.MethodGet, "/api/buddy/submissions", otherBuddyToken, nil, http.StatusOK, &foreignPending)
	if len(foreignPending) != 0 {
		t.Fatalf("foreign buddy sees private submissions: %+v", foreignPending)
	}
	reviewPath1 := "/api/buddy/submissions/" + attempt1.ID + "/review"
	request(http.MethodPost, reviewPath1, otherBuddyToken,
		map[string]any{"approved": true}, http.StatusNotFound, nil)
	request(http.MethodPost, reviewPath1, buddyToken,
		map[string]any{"approved": false}, http.StatusBadRequest, nil)
	request(http.MethodPost, reviewPath1, buddyToken,
		map[string]any{"approved": false, "comment": "Add tests"}, http.StatusNoContent, nil)
	request(http.MethodPost, reviewPath1, buddyToken,
		map[string]any{"approved": true}, http.StatusConflict, nil)
	afterReject := getChecklists(internToken)
	assertStatus(itemState(afterReject.Professional.Stages, professionalItems[0]), model.StatusActive)
	assertStatus(itemState(afterReject.Professional.Stages, professionalItems[1]), model.StatusActive)
	if afterReject.Professional.CompletedItems != 0 ||
		itemState(afterReject.Professional.Stages, professionalItems[0]).CompletedAt != nil {
		t.Fatal("rejected item must be active, with zero completed items and no completed_at")
	}
	if returned := itemState(afterReject.Professional.Stages, professionalItems[0]); !returned.ReturnedRework ||
		returned.ReworkComment == nil || *returned.ReworkComment != "Add tests" {
		t.Fatalf("rejected item must be marked returned_for_rework with the comment: %+v", returned)
	}
	if itemState(afterReject.Professional.Stages, professionalItems[1]).ReturnedRework {
		t.Fatal("untouched item marked as returned")
	}
	// Teamlead reads the history but never reviews.
	staffHistoryPath := "/api/staff/interns/" + internID.String() + "/checklist-items/" + professionalItems[0].String() + "/submissions"
	var staffHistory []submissionHistoryDTO
	request(http.MethodGet, staffHistoryPath, teamleadToken, nil, http.StatusOK, &staffHistory)
	if len(staffHistory) != 1 || staffHistory[0].Status != model.SubmissionRejected || staffHistory[0].ReviewedBy == nil || *staffHistory[0].ReviewedBy != buddyID.String() {
		t.Fatalf("teamlead history: %+v", staffHistory)
	}
	request(http.MethodGet, staffHistoryPath, buddyToken, nil, http.StatusOK, &staffHistory)
	request(http.MethodGet, staffHistoryPath, otherBuddyToken, nil, http.StatusNotFound, nil)
	request(http.MethodGet, staffHistoryPath, internToken, nil, http.StatusForbidden, nil)

	var attempt2 submissionDTO
	request(http.MethodPost, reviewItemPath+"/submissions", internToken,
		map[string]string{"result_url": "https://example.org/e2e/2"}, http.StatusCreated, &attempt2)
	if attempt2.AttemptNumber != 2 || attempt2.ID == attempt1.ID || attempt2.Status != model.SubmissionPending {
		t.Fatalf("retry should create pending attempt #2: %+v", attempt2)
	}
	if itemState(getChecklists(internToken).Professional.Stages, professionalItems[0]).ReturnedRework {
		t.Fatal("resubmitted item is still marked returned_for_rework")
	}
	request(http.MethodPost, "/api/buddy/submissions/"+attempt2.ID+"/review", teamleadToken,
		map[string]any{"approved": true}, http.StatusForbidden, nil)
	var history []submissionHistoryDTO
	request(http.MethodGet, reviewItemPath+"/submissions", internToken, nil, http.StatusOK, &history)
	if len(history) != 2 || history[0].AttemptNumber != 1 || history[0].Status != model.SubmissionRejected ||
		history[0].ReviewComment == nil || *history[0].ReviewComment != "Add tests" ||
		history[1].AttemptNumber != 2 || history[1].Status != model.SubmissionPending {
		t.Fatalf("incorrect submission history: %+v", history)
	}
	request(http.MethodPost, "/api/buddy/submissions/"+attempt2.ID+"/review", buddyToken,
		map[string]any{"approved": true}, http.StatusNoContent, nil)
	final := getChecklists(internToken)
	assertStatus(itemState(final.Professional.Stages, professionalItems[0]), model.StatusVerified)
	assertStatus(itemState(final.Professional.Stages, professionalItems[1]), model.StatusActive)
	if final.Professional.CompletedItems != 1 || final.Professional.CompletionPercent != 50 {
		t.Fatalf("unexpected verified progress: %+v", final.Professional)
	}
	request(http.MethodGet, reviewItemPath+"/submissions", internToken, nil, http.StatusOK, &history)
	if len(history) != 2 || history[0].Status != model.SubmissionRejected || history[1].Status != model.SubmissionApproved {
		t.Fatalf("review history changed incorrectly: %+v", history)
	}
	// Direct DB verification proves the HTTP scenario persisted its state.
	var progressStatus string
	var completedAt sql.NullTime
	if err := db.QueryRowContext(ctx, `
		SELECT status, completed_at FROM user_checklist_progress
		WHERE user_id = $1 AND checklist_item_id = $2
	`, internID, professionalItems[0]).Scan(&progressStatus, &completedAt); err != nil {
		t.Fatalf("read final progress from DB: %v", err)
	}
	if progressStatus != "verified" || !completedAt.Valid {
		t.Fatalf("DB final progress: status=%q, completed_at=%v", progressStatus, completedAt)
	}
	var savedAttempts int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM checklist_item_submissions WHERE progress_id = $1
	`, attempt2.ProgressID).Scan(&savedAttempts); err != nil || savedAttempts != 2 {
		t.Fatalf("DB submission history: count=%d, err=%v", savedAttempts, err)
	}
	request(http.MethodGet, "/api/buddy/submissions", buddyToken, nil, http.StatusOK, &pending)
	if len(pending) != 0 {
		t.Fatalf("buddy queue should be empty after approval: %+v", pending)
	}
	var inserted map[string]string
	request(http.MethodPost, "/api/teamlead/interns/"+internID.String()+"/checklist-items", teamleadToken, map[string]any{"position": 1, "title": "Lead personal task", "description": "Demo", "expected_result": "Documented", "estimated_minutes": 30, "requires_review": false}, http.StatusCreated, &inserted)
	if inserted["id"] == "" {
		t.Fatal("personal item ID missing")
	}
	request(http.MethodGet, "/api/progress", internToken, nil, http.StatusOK, &ownProgress)
	if ownProgress.TotalItems != 5 || ownProgress.CompletedItems != 2 || ownProgress.CompletionPercent != 40 {
		t.Fatalf("overall progress after insertion: %+v", ownProgress)
	}
	updated := getChecklists(internToken)
	if updated.Professional.Stages[0].Items[0].ID != inserted["id"] || updated.Professional.Stages[0].Items[1].Status != model.StatusVerified || updated.Professional.Stages[0].Items[2].Status != model.StatusActive {
		t.Fatalf("personal task changed existing progress: %+v", updated.Professional)
	}
	// Teamlead edits this intern's checklist: the change is per-intern, the catalog is not touched.
	editPath := "/api/teamlead/interns/" + internID.String() + "/checklist-items/"
	request(http.MethodPatch, editPath+inserted["id"], teamleadToken, map[string]any{"title": "Renamed for intern", "estimated_minutes": 45}, http.StatusNoContent, nil)
	request(http.MethodPatch, editPath+inserted["id"], teamleadToken, map[string]any{}, http.StatusBadRequest, nil)
	request(http.MethodPatch, editPath+inserted["id"], buddyToken, map[string]any{"title": "x"}, http.StatusForbidden, nil)
	request(http.MethodPatch, editPath+commonItems[0].String(), teamleadToken, map[string]any{"title": "Common, own wording"}, http.StatusNoContent, nil)
	edited := getChecklists(internToken)
	if item := itemState(edited.Professional.Stages, uuid.MustParse(inserted["id"])); item.Title != "Renamed for intern" || item.EstimatedMinutes == nil || *item.EstimatedMinutes != 45 || !item.Personal {
		t.Fatalf("personal task edit: %+v", item)
	}
	if item := itemState(edited.Common.Stages, commonItems[0]); item.Title != "Common, own wording" || item.Personal {
		t.Fatalf("catalog task override: %+v", item)
	}
	var catalogTitle string
	if err := db.QueryRowContext(ctx, `SELECT title FROM checklist_items WHERE id=$1`, commonItems[0]).Scan(&catalogTitle); err != nil || catalogTitle == "Common, own wording" {
		t.Fatalf("catalog changed by a per-intern edit: %q %v", catalogTitle, err)
	}
	// Removing the intern's active catalog task hides it and keeps the intern moving.
	request(http.MethodDelete, editPath+professionalItems[1].String(), teamleadToken, nil, http.StatusNoContent, nil)
	request(http.MethodDelete, editPath+professionalItems[1].String(), teamleadToken, nil, http.StatusNoContent, nil)
	request(http.MethodPatch, editPath+professionalItems[1].String(), teamleadToken, map[string]any{"title": "gone"}, http.StatusNotFound, nil)
	request(http.MethodDelete, editPath+uuid.NewString(), teamleadToken, nil, http.StatusNotFound, nil)
	afterRemove := getChecklists(internToken)
	for _, stage := range afterRemove.Professional.Stages {
		for _, item := range stage.Items {
			if item.ID == professionalItems[1].String() {
				t.Fatal("removed task is still in the intern's checklist")
			}
		}
	}
	if afterRemove.Professional.TotalItems != 2 || afterRemove.Professional.CompletedItems != 1 {
		t.Fatalf("progress after removal: %+v", afterRemove.Professional)
	}
	t.Log("E2E passed: auth, intern creation, checklists, completion, reject/retry, review, permissions and DB persistence")
}
