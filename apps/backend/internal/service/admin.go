package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Admin panel (teamlead only, enforced by the handler's role middleware): accounts, LLM status relayed from the AI
// service, system health, notification catalog and a manual hint-worker tick. Nothing here returns password hashes,
// tokens or chat texts.

// HintTicker is what the admin panel needs from the hint worker (HintService.Tick).
type HintTicker interface {
	Tick(context.Context) error
}

type AdminService struct {
	db     *sql.DB
	aiURL  string
	client *http.Client
	ticker HintTicker
}

func NewAdminService(db *sql.DB, aiURL string, ticker HintTicker) *AdminService {
	return &AdminService{db: db, aiURL: strings.TrimRight(aiURL, "/"), client: &http.Client{Timeout: 2 * time.Minute}, ticker: ticker}
}

type AdminAccount struct {
	ID             string     `json:"id"`
	Email          string     `json:"email"`
	FirstName      string     `json:"first_name"`
	LastName       string     `json:"last_name"`
	Role           string     `json:"role"`
	Profession     string     `json:"profession"`
	Team           string     `json:"team"`
	Department     string     `json:"department"`
	BuddyName      string     `json:"buddy_name"`
	TeamleadName   string     `json:"teamlead_name"`
	CreatedAt      time.Time  `json:"created_at"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	ActiveSessions int        `json:"active_sessions"`
}

func (s *AdminService) Accounts(ctx context.Context) ([]AdminAccount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id::text, u.email, COALESCE(u.first_name,''), COALESCE(u.last_name,''), u.role, COALESCE(u.profession,''),
		       COALESCE(u.team,''), COALESCE(u.department,''),
		       COALESCE(NULLIF(trim(COALESCE(b.first_name,'') || ' ' || COALESCE(b.last_name,'')),''), b.email, ''),
		       COALESCE(NULLIF(trim(COALESCE(t.first_name,'') || ' ' || COALESCE(t.last_name,'')),''), t.email, ''),
		       u.created_at, u.last_login_at,
		       (SELECT count(*) FROM auth_sessions a WHERE a.user_id = u.id AND a.revoked_at IS NULL AND a.expires_at > now())
		FROM users u
		LEFT JOIN users b ON b.id = u.buddy_id
		LEFT JOIN users t ON t.id = u.teamlead_id
		ORDER BY CASE u.role WHEN 'teamlead' THEN 0 WHEN 'buddy' THEN 1 ELSE 2 END, u.created_at, u.email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AdminAccount, 0)
	for rows.Next() {
		var a AdminAccount
		if err := rows.Scan(&a.ID, &a.Email, &a.FirstName, &a.LastName, &a.Role, &a.Profession, &a.Team, &a.Department,
			&a.BuddyName, &a.TeamleadName, &a.CreatedAt, &a.LastLoginAt, &a.ActiveSessions); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *AdminService) aiCall(ctx context.Context, method, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.aiURL+path, bytes.NewReader(nil))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, ErrAIUnavailable
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK || !json.Valid(raw) {
		return nil, ErrAIUnavailable
	}
	return raw, nil
}

func (s *AdminService) LLM(ctx context.Context) (json.RawMessage, error) {
	return s.aiCall(ctx, http.MethodGet, "/v1/admin/llm")
}

func (s *AdminService) LLMPing(ctx context.Context) (json.RawMessage, error) {
	return s.aiCall(ctx, http.MethodPost, "/v1/admin/llm-ping")
}

func (s *AdminService) ReloadKB(ctx context.Context) (json.RawMessage, error) {
	return s.aiCall(ctx, http.MethodPost, "/v1/kb/reload")
}

type AdminCheck struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"` // ok | error
	Detail    string `json:"detail"`
	LatencyMS int64  `json:"latency_ms"`
}

type AdminSystem struct {
	Checks   []AdminCheck   `json:"checks"`
	Counters AdminCounters  `json:"counters"`
	Hints    map[string]int `json:"hints"`
}

type AdminCounters struct {
	UsersByRole     map[string]int `json:"users_by_role"`
	OpenQuestions   int            `json:"open_questions"`
	PendingRequests int            `json:"pending_requests"`
	MessagesToday   int            `json:"messages_today"`
	ActiveSessions  int            `json:"active_sessions"`
}

func (s *AdminService) hintCounts(ctx context.Context) (map[string]int, error) {
	out := map[string]int{"pending": 0, "processing": 0, "ready": 0, "unread": 0}
	rows, err := s.db.QueryContext(ctx, `SELECT status, count(*) FROM task_hints GROUP BY status`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return out, err
		}
		out[st] = n
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	var unread int
	err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM task_hints WHERE status='ready' AND read_at IS NULL`).Scan(&unread)
	out["unread"] = unread
	return out, err
}

func (s *AdminService) System(ctx context.Context) AdminSystem {
	sys := AdminSystem{Checks: []AdminCheck{}, Hints: map[string]int{}}
	timed := func(id, title string, fn func() (string, error)) {
		start := time.Now()
		detail, err := fn()
		c := AdminCheck{ID: id, Title: title, Status: "ok", Detail: detail, LatencyMS: time.Since(start).Milliseconds()}
		if err != nil {
			c.Status, c.Detail = "error", detail
		}
		sys.Checks = append(sys.Checks, c)
	}
	timed("database", "База данных", func() (string, error) {
		var one int
		if err := s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
			return "PostgreSQL не отвечает", err
		}
		return "PostgreSQL отвечает", nil
	})
	timed("ai", "AI-сервис", func() (string, error) {
		raw, err := s.aiCall(ctx, http.MethodGet, "/health")
		if err != nil {
			return "AI-сервис недоступен", err
		}
		var h struct{ Mode, Model string }
		_ = json.Unmarshal(raw, &h)
		d := "режим: " + h.Mode
		if h.Model != "" {
			d += ", модель: " + h.Model
		}
		return d, nil
	})
	timed("kb", "База знаний", func() (string, error) {
		raw, err := s.aiCall(ctx, http.MethodGet, "/v1/admin/kb")
		if err != nil {
			return "Не удалось получить данные базы знаний", err
		}
		var k struct{ Documents, Chunks int }
		_ = json.Unmarshal(raw, &k)
		return itoa(k.Documents) + " документов, " + itoa(k.Chunks) + " фрагментов", nil
	})
	timed("hint_worker", "Воркер подсказок", func() (string, error) {
		counts, err := s.hintCounts(ctx)
		sys.Hints = counts
		if err != nil {
			return "Не удалось прочитать очередь подсказок", err
		}
		return "в очереди: " + itoa(counts["pending"]+counts["processing"]) + ", готово: " + itoa(counts["ready"]) +
			", непрочитано: " + itoa(counts["unread"]), nil
	})

	sys.Counters.UsersByRole = map[string]int{"intern": 0, "buddy": 0, "teamlead": 0}
	if rows, err := s.db.QueryContext(ctx, `SELECT role, count(*) FROM users GROUP BY role`); err == nil {
		for rows.Next() {
			var role string
			var n int
			if rows.Scan(&role, &n) == nil {
				sys.Counters.UsersByRole[role] = n
			}
		}
		rows.Close()
	}
	_ = s.db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM mentor_questions WHERE status='open'),
		(SELECT count(*) FROM service_requests WHERE status='pending_approval'),
		(SELECT count(*) FROM chat_messages WHERE created_at >= date_trunc('day', now())),
		(SELECT count(*) FROM auth_sessions WHERE revoked_at IS NULL AND expires_at > now())`).
		Scan(&sys.Counters.OpenQuestions, &sys.Counters.PendingRequests, &sys.Counters.MessagesToday, &sys.Counters.ActiveSessions)
	return sys
}

func itoa(n int) string { return strconv.Itoa(n) }

type NotificationSample struct {
	Trigger string `json:"trigger,omitempty"`
	Text    string `json:"text"`
	Detail  string `json:"detail,omitempty"`
	Ask     string `json:"ask,omitempty"`
}

type LiveNumber struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

type NotificationInfo struct {
	ID      string               `json:"id"`
	Title   string               `json:"title"`
	Source  string               `json:"source"` // widget | site | worker
	When    string               `json:"when"`
	Cadence string               `json:"cadence"`
	Button  string               `json:"button,omitempty"`
	Check   string               `json:"check"` // preview (sample shown in own Бади) | backend (real worker tick)
	Samples []NotificationSample `json:"samples"`
	Live    []LiveNumber         `json:"live"`
}

// notificationCatalog is the single source of truth for Бади's pop-ups; the admin panel only renders it.
func notificationCatalog() []NotificationInfo {
	return []NotificationInfo{
		{ID: "greeting", Title: "Приветствие дня", Source: "widget", Check: "preview",
			When: "Первое открытие страницы за день: Бади здоровается и предлагает сводку.", Cadence: "Раз в день, на первой странице.",
			Button: "Показать", Samples: []NotificationSample{{Text: "Привет! У меня есть сводка на сегодня.", Ask: "Покажи сводку на сегодня"}}},
		{ID: "page_hint", Title: "Подсказка по странице", Source: "widget", Check: "preview",
			When:    "Виджет сопоставляет с шаблонами только заголовок вкладки и адрес страницы; текст страницы не читается.",
			Cadence: "Не чаще раза на страницу за сессию вкладки и только после приветствия дня.",
			Samples: []NotificationSample{
				{Trigger: "grafana, дашборд, dashboard", Text: "Это дашборды метрик. Подсказать, что смотреть в первую очередь?", Ask: "Что смотреть в первую очередь на дашбордах метрик?"},
				{Trigger: "jira, тикет, issue, board", Text: "Помочь взять задачу в Jira и оформить её по правилам?", Ask: "Как взять задачу в Jira и оформить её по правилам?"},
				{Trigger: "merge request, pull request, gitlab, github, code review", Text: "Напомнить правила код-ревью и мержа?", Ask: "Напомни правила код-ревью и мержа"},
				{Trigger: "wiki, confluence, вики, регламент, faq, памятка", Text: "Выдели текст — проверю, актуален ли он по базе знаний."},
			}},
		{ID: "selection", Title: "Вопрос про выделенный текст", Source: "widget", Check: "preview",
			When: "Пользователь выделил на странице не меньше 12 символов.", Cadence: "При каждом новом выделении.",
			Button: "Это актуально?", Samples: []NotificationSample{{Text: "Спросить меня про выделенный текст?", Ask: "Это актуально?"}}},
		{ID: "request_status", Title: "Статус заявки изменился", Source: "widget", Check: "preview",
			When:    "Виджет каждые 20 секунд сравнивает заявки пользователя (заявки ассистента и корпоративные тикеты доступа) с последними увиденными статусами.",
			Cadence: "Опрос раз в 20 секунд, всплывашка при смене статуса.",
			Samples: []NotificationSample{{Text: "🎉 Заявка ABS-000123: согласована"}}},
		{ID: "stuck_task", Title: "Задача идёт дольше плана", Source: "worker", Check: "backend",
			When:    "Воркер подсказок находит активную задачу, у которой activated_at + estimated_minutes меньше текущего времени, спрашивает AI-сервис (POST /hint, таймаут 8 с, до 2 попыток, затем запасной текст) и сохраняет готовую подсказку. Сайт показывает непрочитанные подсказки.",
			Cadence: "Воркер срабатывает раз в минуту; подсказка на задачу создаётся один раз.",
			Button:  "Обсудить с Бади", Samples: []NotificationSample{{Text: "Задача «Настроить окружение» идёт дольше плана. У меня есть подсказка:",
				Detail: "Разбейте задачу на шаги и начните с самого простого.", Ask: "Помоги с задачей: с чего начать?"}}},
		{ID: "mentor_answered", Title: "Наставник ответил", Source: "site", Check: "preview",
			When: "Открытый вопрос стажёра к наставнику получил ответ.", Cadence: "Один раз на ответ; сайт опрашивает раз в 20 секунд.",
			Samples: []NotificationSample{{Text: "Наставник ответил на твой вопрос — ответ уже в чате."}}},
		{ID: "staff_counts", Title: "Есть вопросы и заявки", Source: "site", Check: "preview",
			When: "Для наставника и руководителя: есть открытые вопросы стажёров или заявки на согласование.", Cadence: "При каждом изменении счётчиков; опрос раз в 20 секунд.",
			Samples: []NotificationSample{{Text: "От бота: вопросов стажёров — 2, заявок на согласование — 1."}}},
	}
}

func (s *AdminService) count(ctx context.Context, query string) int {
	var n int
	_ = s.db.QueryRowContext(ctx, query).Scan(&n)
	return n
}

const overdueWhere = `p.status='active' AND p.activated_at IS NOT NULL AND i.estimated_minutes IS NOT NULL
	AND p.activated_at + i.estimated_minutes * interval '1 minute' < now()`

func (s *AdminService) overdueCount(ctx context.Context) int {
	return s.count(ctx, `SELECT count(*) FROM user_checklist_progress p JOIN checklist_items i ON i.id=p.checklist_item_id WHERE `+overdueWhere)
}

func (s *AdminService) Notifications(ctx context.Context) []NotificationInfo {
	items := notificationCatalog()
	hints, _ := s.hintCounts(ctx)
	open := s.count(ctx, `SELECT count(*) FROM mentor_questions WHERE status='open'`)
	pending := s.count(ctx, `SELECT count(*) FROM service_requests WHERE status='pending_approval'`)
	for i := range items {
		switch items[i].ID {
		case "stuck_task":
			items[i].Live = []LiveNumber{{"Просроченных активных задач сейчас", s.overdueCount(ctx)},
				{"Подсказок в очереди", hints["pending"] + hints["processing"]}, {"Готовых подсказок", hints["ready"]},
				{"Непрочитанных подсказок", hints["unread"]}}
		case "request_status":
			items[i].Live = []LiveNumber{{"Заявок на согласовании", pending},
				{"Открытых тикетов доступа", s.count(ctx, `SELECT count(*) FROM corporate_access_requests WHERE status='open'`)}}
		case "mentor_answered":
			items[i].Live = []LiveNumber{{"Открытых вопросов стажёров", open},
				{"Вопросов с ответом", s.count(ctx, `SELECT count(*) FROM mentor_questions WHERE status='answered'`)}}
		case "staff_counts":
			items[i].Live = []LiveNumber{{"Вопросов стажёров", open}, {"Заявок на согласование", pending}}
		}
		if items[i].Live == nil {
			items[i].Live = []LiveNumber{}
		}
	}
	return items
}

type StuckTask struct {
	Task      string `json:"task"`
	UserEmail string `json:"user_email"`
	Hint      string `json:"hint_status"` // none | pending | processing | ready
	HintText  string `json:"hint_text"`
}

type StuckCheck struct {
	OverdueTasks  int         `json:"overdue_tasks"`
	PendingBefore int         `json:"pending_before"`
	ReadyBefore   int         `json:"ready_before"`
	PendingAfter  int         `json:"pending_after"`
	ReadyAfter    int         `json:"ready_after"`
	Tasks         []StuckTask `json:"tasks"`
	Error         string      `json:"error,omitempty"`
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// CheckStuckTask runs the real hint worker tick now and reports what the worker saw and did.
func (s *AdminService) CheckStuckTask(ctx context.Context) (StuckCheck, error) {
	res := StuckCheck{Tasks: []StuckTask{}}
	before, err := s.hintCounts(ctx)
	if err != nil {
		return res, err
	}
	res.PendingBefore, res.ReadyBefore = before["pending"]+before["processing"], before["ready"]
	if s.ticker != nil {
		if err := s.ticker.Tick(ctx); err != nil {
			res.Error = "Тик воркера завершился ошибкой"
		}
	}
	after, err := s.hintCounts(ctx)
	if err != nil {
		return res, err
	}
	res.PendingAfter, res.ReadyAfter = after["pending"]+after["processing"], after["ready"]
	res.OverdueTasks = s.overdueCount(ctx)
	rows, err := s.db.QueryContext(ctx, `SELECT i.title, u.email, COALESCE(h.status,'none'), COALESCE(h.text,'')
		FROM user_checklist_progress p
		JOIN checklist_items i ON i.id=p.checklist_item_id
		JOIN users u ON u.id=p.user_id
		LEFT JOIN task_hints h ON h.progress_id=p.id
		WHERE `+overdueWhere+` ORDER BY p.activated_at LIMIT 10`)
	if err != nil {
		return res, err
	}
	defer rows.Close()
	for rows.Next() {
		var t StuckTask
		if err := rows.Scan(&t.Task, &t.UserEmail, &t.Hint, &t.HintText); err != nil {
			return res, err
		}
		t.HintText = truncateRunes(t.HintText, 200)
		res.Tasks = append(res.Tasks, t)
	}
	return res, rows.Err()
}
