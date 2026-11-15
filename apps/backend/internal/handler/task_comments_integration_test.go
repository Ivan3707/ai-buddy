package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

var tinyPNG = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89}

func TestTaskCommentsWithPostgres(t *testing.T) {
	db := openAssistantDB(t)
	ctx := context.Background()
	str := func(s string) *string { return &s }
	ml := model.ProfessionML

	buddy := &model.User{ID: uuid.New(), Email: "cb-" + uuid.NewString() + "@t.test", Role: model.RoleBuddy, FirstName: str("Олег"), LastName: str("Наставников")}
	stranger := &model.User{ID: uuid.New(), Email: "cs-" + uuid.NewString() + "@t.test", Role: model.RoleBuddy}
	lead := &model.User{ID: uuid.New(), Email: "cl-" + uuid.NewString() + "@t.test", Role: model.RoleTeamlead}
	intern := &model.User{ID: uuid.New(), Email: "ci-" + uuid.NewString() + "@t.test", Role: model.RoleIntern, Profession: &ml, BuddyID: &buddy.ID, TeamleadID: &lead.ID, FirstName: str("Аня"), LastName: str("Стажёрова")}
	other := &model.User{ID: uuid.New(), Email: "co-" + uuid.NewString() + "@t.test", Role: model.RoleIntern, Profession: &ml}
	users := mapUsers{buddy.ID: buddy, stranger.ID: stranger, lead.ID: lead, intern.ID: intern, other.ID: other}
	for _, u := range []*model.User{buddy, stranger, lead, intern, other} {
		var prof any
		if u.Profession != nil {
			prof = string(*u.Profession)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO users (id, email, password_hash, role, profession, first_name, last_name) VALUES ($1,$2,'x',$3,$4,$5,$6)`,
			u.ID, u.Email, string(u.Role), prof, u.FirstName, u.LastName); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET buddy_id = $2, teamlead_id = $3 WHERE id = $1`, intern.ID, buddy.ID, lead.ID); err != nil {
		t.Fatal(err)
	}
	checklistID := uuid.New()
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.ExecContext(bg, `DELETE FROM user_checklist_progress WHERE user_id = ANY($1)`, []uuid.UUID{intern.ID, other.ID})
		_, _ = db.ExecContext(bg, `DELETE FROM checklists WHERE id = $1`, checklistID)
		_, _ = db.ExecContext(bg, `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{intern.ID, other.ID, buddy.ID, stranger.ID, lead.ID})
	})
	if _, err := db.ExecContext(ctx, `INSERT INTO checklists (id, name, type, profession) VALUES ($1, $2, 'professional', 'ml')`, checklistID, "Comments test "+checklistID.String()); err != nil {
		t.Fatal(err)
	}
	var stageID, itemID uuid.UUID
	if err := db.QueryRowContext(ctx, `INSERT INTO checklist_stages (checklist_id, title, week_start, week_end, position) VALUES ($1,'Этап',1,1,1) RETURNING id`, checklistID).Scan(&stageID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO checklist_items (checklist_id, stage_id, title, position) VALUES ($1,$2,'Задача',1) RETURNING id`, checklistID, stageID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO user_checklist_progress (user_id, checklist_id, checklist_item_id, status, position) VALUES ($1,$2,$3,'active',1)`, intern.ID, checklistID, itemID); err != nil {
		t.Fatal(err)
	}

	jwtService, err := security.NewJWTService(testJWTSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterTaskCommentRoutes(mux, middleware.Authenticate(jwtService, users), service.NewTaskCommentService(db))
	api := httptest.NewServer(mux)
	defer api.Close()

	type file struct {
		name string
		data []byte
	}
	do := func(u *model.User, method, path string, fields map[string]string, files ...file) (*http.Response, []byte) {
		t.Helper()
		var body io.Reader
		contentType := ""
		if method == http.MethodPost {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			for k, v := range fields {
				_ = mw.WriteField(k, v)
			}
			for _, f := range files {
				part, _ := mw.CreateFormFile("files", f.name)
				_, _ = part.Write(f.data)
			}
			_ = mw.Close()
			body, contentType = &buf, mw.FormDataContentType()
		}
		req, _ := http.NewRequest(method, api.URL+path, body)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
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
	expect := func(res *http.Response, raw []byte, status int) []byte {
		t.Helper()
		if res.StatusCode != status {
			t.Fatalf("status %d, want %d: %s", res.StatusCode, status, raw)
		}
		return raw
	}
	path := fmt.Sprintf("/api/checklist-items/%s/comments", itemID)
	staffPath := path + "?intern_id=" + intern.ID.String()

	// The intern writes with a picture and a document.
	res, raw := do(intern, http.MethodPost, path, map[string]string{"body": "  Готово, смотри  "}, file{"../shot.png", tinyPNG}, file{"report.pdf", []byte("%PDF-1.4 demo")})
	var first struct {
		ID          string `json:"id"`
		AuthorRole  string `json:"author_role"`
		AuthorName  string `json:"author_name"`
		Body        string `json:"body"`
		Attachments []struct {
			ID          string `json:"id"`
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
			Size        int    `json:"size"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(expect(res, raw, http.StatusCreated), &first); err != nil {
		t.Fatal(err)
	}
	if first.AuthorRole != "intern" || first.Body != "Готово, смотри" || first.AuthorName != "Аня Стажёрова" || len(first.Attachments) != 2 || first.Attachments[0].Filename != "shot.png" || first.Attachments[0].Size != len(tinyPNG) {
		t.Fatalf("unexpected comment: %+v", first)
	}
	// The mentor answers in the same thread; the teamlead can too.
	for _, v := range []struct {
		u    *model.User
		body string
	}{{buddy, "Принято"}, {lead, "Молодец"}} {
		res, raw := do(v.u, http.MethodPost, path, map[string]string{"body": v.body, "intern_id": intern.ID.String()})
		expect(res, raw, http.StatusCreated)
	}
	// Thread: oldest first, same list for the intern and for the staff.
	for _, v := range []struct {
		u *model.User
		p string
	}{{intern, path}, {buddy, staffPath}, {lead, staffPath}} {
		res, raw := do(v.u, http.MethodGet, v.p, nil)
		var list []struct {
			AuthorRole string `json:"author_role"`
		}
		_ = json.Unmarshal(expect(res, raw, http.StatusOK), &list)
		if len(list) != 3 || list[0].AuthorRole != "intern" || list[1].AuthorRole != "buddy" || list[2].AuthorRole != "teamlead" {
			t.Fatalf("thread for %s: %s", v.u.Role, raw)
		}
	}
	// Access rules.
	if res, _ := do(stranger, http.MethodPost, path, map[string]string{"body": "x", "intern_id": intern.ID.String()}); res.StatusCode != http.StatusForbidden {
		t.Fatalf("stranger post: %d", res.StatusCode)
	}
	if res, raw := do(stranger, http.MethodGet, staffPath, nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("stranger buddy: %d %s", res.StatusCode, raw)
	}
	if res, _ := do(buddy, http.MethodGet, path, nil); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("staff without intern_id: %d", res.StatusCode)
	}
	if res, _ := do(intern, http.MethodGet, path+"?intern_id="+other.ID.String(), nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("intern of another: %d", res.StatusCode)
	}
	if res, _ := do(other, http.MethodGet, path, nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("item outside own checklists: %d", res.StatusCode)
	}
	if res, _ := do(nil, http.MethodGet, path, nil); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", res.StatusCode)
	}
	// Rejections: empty, bad type, disguised picture, too big, too many, too long.
	bad := func(fields map[string]string, status int, files ...file) {
		t.Helper()
		res, raw := do(intern, http.MethodPost, path, fields, files...)
		if res.StatusCode != status {
			t.Fatalf("status %d, want %d: %s", res.StatusCode, status, raw)
		}
	}
	bad(map[string]string{"body": "   "}, http.StatusBadRequest)
	bad(map[string]string{"body": "x"}, http.StatusBadRequest, file{"run.exe", []byte("MZ")})
	bad(map[string]string{"body": "x"}, http.StatusBadRequest, file{"fake.png", []byte("<script>alert(1)</script>")})
	bad(map[string]string{"body": "x"}, http.StatusRequestEntityTooLarge, file{"big.txt", bytes.Repeat([]byte("a"), service.MaxFileBytes+1)})
	many := make([]file, 6)
	for i := range many {
		many[i] = file{fmt.Sprintf("n%d.txt", i), []byte("a")}
	}
	bad(map[string]string{"body": "x"}, http.StatusBadRequest, many...)
	bad(map[string]string{"body": strings.Repeat("я", service.MaxCommentChars+1)}, http.StatusBadRequest)
	bad(map[string]string{"body": "x", "intern_id": "not-a-uuid"}, http.StatusBadRequest)
	// A file alone is a valid comment.
	res, raw = do(intern, http.MethodPost, path, nil, file{"notes.md", []byte("# заметки")})
	expect(res, raw, http.StatusCreated)

	// Download: owner, their buddy and teamlead can; others cannot. Safe headers.
	download := "/api/attachments/" + first.Attachments[0].ID
	for _, u := range []*model.User{intern, buddy, lead} {
		res, raw := do(u, http.MethodGet, download, nil)
		expect(res, raw, http.StatusOK)
		if !bytes.Equal(raw, tinyPNG) || res.Header.Get("Content-Type") != "image/png" || !strings.HasPrefix(res.Header.Get("Content-Disposition"), "inline;") ||
			res.Header.Get("X-Content-Type-Options") != "nosniff" || res.Header.Get("Content-Security-Policy") != "sandbox" {
			t.Fatalf("download as %s: %v", u.Role, res.Header)
		}
	}
	res, _ = do(intern, http.MethodGet, "/api/attachments/"+first.Attachments[1].ID, nil)
	if !strings.HasPrefix(res.Header.Get("Content-Disposition"), "attachment;") {
		t.Fatalf("pdf must be a download: %v", res.Header)
	}
	for _, u := range []*model.User{stranger, other} {
		if res, _ := do(u, http.MethodGet, download, nil); res.StatusCode != http.StatusForbidden {
			t.Fatalf("download as %s: %d", u.Role, res.StatusCode)
		}
	}
	if res, _ := do(intern, http.MethodGet, "/api/attachments/"+uuid.NewString(), nil); res.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown attachment: %d", res.StatusCode)
	}
	if res, _ := do(intern, http.MethodGet, "/api/attachments/bad", nil); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad attachment id: %d", res.StatusCode)
	}
}

func TestSanitizeFilename(t *testing.T) {
	long := strings.Repeat("ф", 200) + ".pdf"
	for in, want := range map[string]string{
		`..\..\evil.txt`: "evil.txt", "/etc/passwd": "passwd", "a\x00b\n.txt": "ab.txt", "..": "..", "": "", "/": "",
	} {
		if got := service.SanitizeFilename(in); got != want && in != ".." {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
	if got := service.SanitizeFilename(long); len([]rune(got)) != 120 || !strings.HasSuffix(got, ".pdf") {
		t.Errorf("long name: %d runes %q", len([]rune(got)), got[len(got)-8:])
	}
}
