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

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// Admin panel (teamlead only, enforced by the handler's role middleware): accounts, LLM status relayed from the AI
// service, system health, notification catalog and a manual hint-worker tick. Nothing here returns password hashes,
// tokens or chat texts.

// HintTicker is what the admin panel needs from the hint worker (HintService.Tick).
type HintTicker interface {
	Tick(context.Context) error
}

type AdminService struct {
	// OpenForAll lets every signed-in role into the admin panel (demo recording). Off by default: RBAC stays with the backend.
	OpenForAll bool
	db         *sql.DB
	aiURL      string
	client     *http.Client
	ticker     HintTicker
}

func NewAdminService(db *sql.DB, aiURL string, ticker HintTicker) *AdminService {
	return &AdminService{db: db, aiURL: strings.TrimRight(aiURL, "/"), client: &http.Client{Timeout: 2 * time.Minute}, ticker: ticker}
}

// IsOpenForAll reports the demo flag for the handler, which only sees the service through an interface.
func (s *AdminService) IsOpenForAll() bool { return s.OpenForAll }

type AdminAccount = model.AdminAccount
type AnswerFeedbackItem = model.AnswerFeedbackItem
type AnswerFeedback = model.AnswerFeedback

// AnswerFeedback counts thumbs under bot answers and lists the comments left under thumbs-down. It is a product-quality
// signal for the admin panel; the mentor desk does not show it.
func (s *AdminService) AnswerFeedback(ctx context.Context) (AnswerFeedback, error) {
	return repository.NewAdminRepository(s.db).AnswerFeedback(ctx)
}

// RestartOnboarding makes the intern's next login a "first" one again (last_login_at = NULL), so the welcome tour starts.
// Returns sql.ErrNoRows when there is no such intern. Staff accounts have no tour.
func (s *AdminService) RestartOnboarding(ctx context.Context, userID string) error {
	return repository.NewAdminRepository(s.db).RestartOnboarding(ctx, userID)
}

func (s *AdminService) Accounts(ctx context.Context) ([]AdminAccount, error) {
	return repository.NewAdminRepository(s.db).Accounts(ctx)
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

// KBLint asks the AI service for contradictions and stale documents of the knowledge base.
func (s *AdminService) KBLint(ctx context.Context) (json.RawMessage, error) {
	return s.aiCall(ctx, http.MethodGet, "/v1/kb/lint")
}

// KBGaps clusters the questions the bot had to escalate (all of them): topics the knowledge base does not cover.
func (s *AdminService) KBGaps(ctx context.Context) (json.RawMessage, error) {
	items, err := repository.NewAdminRepository(s.db).GapQuestions(ctx)
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]any{"questions": items})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.aiURL+"/v1/analytics/gaps", bytes.NewReader(payload))
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

type AdminCounters = model.AdminCounters

func (s *AdminService) hintCounts(ctx context.Context) (map[string]int, error) {
	return repository.NewAdminRepository(s.db).HintCounts(ctx)
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
		if err := repository.NewAdminRepository(s.db).Ping(ctx); err != nil {
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

	sys.Counters, _ = repository.NewAdminRepository(s.db).Counters(ctx)
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

func (s *AdminService) overdueCount(ctx context.Context) int {
	counts, _ := repository.NewAdminRepository(s.db).NotificationCounts(ctx)
	return counts.Overdue
}

func (s *AdminService) Notifications(ctx context.Context) []NotificationInfo {
	items := notificationCatalog()
	hints, _ := s.hintCounts(ctx)
	counts, _ := repository.NewAdminRepository(s.db).NotificationCounts(ctx)
	open, pending := counts.OpenQuestions, counts.PendingRequests
	for i := range items {
		switch items[i].ID {
		case "stuck_task":
			items[i].Live = []LiveNumber{{"Просроченных активных задач сейчас", s.overdueCount(ctx)},
				{"Подсказок в очереди", hints["pending"] + hints["processing"]}, {"Готовых подсказок", hints["ready"]},
				{"Непрочитанных подсказок", hints["unread"]}}
		case "request_status":
			items[i].Live = []LiveNumber{{"Заявок на согласовании", pending},
				{"Открытых тикетов доступа", counts.OpenAccess}}
		case "mentor_answered":
			items[i].Live = []LiveNumber{{"Открытых вопросов стажёров", open},
				{"Вопросов с ответом", counts.AnsweredQuestions}}
		case "staff_counts":
			items[i].Live = []LiveNumber{{"Вопросов стажёров", open}, {"Заявок на согласование", pending}}
		}
		if items[i].Live == nil {
			items[i].Live = []LiveNumber{}
		}
	}
	return items
}

type StuckTask = model.StuckTask

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
	rows, err := repository.NewAdminRepository(s.db).StuckTasks(ctx)
	if err != nil {
		return res, err
	}
	for _, t := range rows {
		t.HintText = truncateRunes(t.HintText, 200)
		res.Tasks = append(res.Tasks, t)
	}
	return res, nil
}

// DemoCompletion is the result of CompleteChecklists.
type DemoCompletion struct {
	InternID            string `json:"intern_id"`
	ItemsCompleted      int64  `json:"items_completed"`
	SubmissionsApproved int64  `json:"submissions_approved"`
}

// CompleteChecklists is a DEMO operation of the admin panel: it marks every visible task of the intern's current
// checklists done (review tasks verified, pending submissions approved on behalf of the intern's buddy), so both
// checklists reach 100% and the web app offers the checklist feedback form. Idempotent: a second call changes
// nothing. repository.ErrNotFound when the user does not exist or is not an intern.
func (s *AdminService) CompleteChecklists(ctx context.Context, actorID, internID uuid.UUID) (DemoCompletion, error) {
	out := DemoCompletion{InternID: internID.String()}
	err := withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := repository.NewPersonalTaskRepository(tx).LockUser(ctx, internID); err != nil {
			return err
		}
		user, err := repository.NewUserRepository(tx).GetByID(ctx, internID)
		if err != nil {
			return err
		}
		if user.Role != model.RoleIntern {
			return repository.ErrNotFound
		}
		res, err := repository.NewInternChecklistRepository(tx).CompleteAll(ctx, internID, actorID)
		if err != nil {
			return err
		}
		out.ItemsCompleted, out.SubmissionsApproved = res.ItemsCompleted, res.SubmissionsApproved
		return nil
	})
	return out, err
}
