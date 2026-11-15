// Command seed-demo fills a fresh demo database with a believable onboarding picture for reviewers:
// interns at different stages, a task waiting for the buddy's review, a task returned for rework,
// a checklist finished for the feedback form, a pending request for the team lead, an open question
// for the buddy, notes and calendar events.
//
// It goes through the same services as the HTTP API, so statuses, the next open task and review
// history follow the real business rules. Data is synthetic. The command refuses to run twice: if a demo
// intern already has a finished task, the database is considered in use and nothing is changed.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
	_ "time/tzdata" // demo events are placed in Moscow time even in a container without tzdata

	"github.com/google/uuid"

	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

const (
	buddyEmail    = "buddy@demo.example"
	teamleadEmail = "teamlead@demo.example"
	mlEmail       = "ml.intern@demo.example"
	backendEmail  = "backend.intern@demo.example"
	frontendEmail = "frontend.intern@demo.example"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("DB_NAME") == "" {
		return errors.New("DB_NAME must be set explicitly")
	}
	db, err := database.OpenPostgres()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return err
	}
	seeded, err := Seed(ctx, db, time.Now().In(loc))
	if err != nil {
		return err
	}
	if !seeded {
		fmt.Println("demo seed skipped: demo interns already have progress")
		return nil
	}
	fmt.Println("demo seed applied")
	return nil
}

// seeder carries the services and users the scenario needs.
type seeder struct {
	db        *sql.DB
	now       time.Time
	progress  *service.ProgressService
	checklist *service.ChecklistService
	assistant *service.AssistantService
	personal  *service.PersonalService
	users     map[string]*model.User
}

// Seed applies the demo scenario once. It reports false when the demo interns already have progress.
func Seed(ctx context.Context, db *sql.DB, now time.Time) (bool, error) {
	var used bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM user_checklist_progress p JOIN users u ON u.id = p.user_id
		WHERE u.email = ANY($1) AND p.status IN ('completed', 'verified'))`,
		[]string{mlEmail, backendEmail, frontendEmail}).Scan(&used); err != nil {
		return false, fmt.Errorf("check existing progress: %w", err)
	}
	if used {
		return false, nil
	}
	s := &seeder{
		db:        db,
		now:       now,
		progress:  service.NewProgressService(db),
		checklist: service.NewChecklistService(db),
		assistant: service.NewAssistantService(db, ""),
		personal:  service.NewPersonalService(repository.NewPersonalRepository(db)),
		users:     map[string]*model.User{},
	}
	users := repository.NewUserRepository(db)
	for _, email := range []string{buddyEmail, teamleadEmail, mlEmail, backendEmail, frontendEmail} {
		u, err := users.GetByEmail(ctx, email)
		if err != nil {
			return false, fmt.Errorf("demo user %s (run sync-hr first): %w", email, err)
		}
		s.users[email] = u
	}
	for _, step := range []func(context.Context) error{s.mlIntern, s.backendIntern, s.frontendIntern, s.staff} {
		if err := step(ctx); err != nil {
			return false, err
		}
	}
	return true, nil
}

// mlIntern is the main persona of the demo video: second week, a task waiting for review, a request
// for the team lead, a question for the buddy, a task that runs over its estimate (proactive hint).
func (s *seeder) mlIntern(ctx context.Context) error {
	u := s.users[mlEmail]
	if err := s.advance(ctx, u, common, 3, nil); err != nil {
		return err
	}
	if err := s.advance(ctx, u, professional, 1, nil); err != nil {
		return err
	}
	// The data review task is submitted and waits for the buddy.
	if _, err := s.submitNext(ctx, u, professional, "Описал 14 полей выборки, нашёл пропуски в price и дубли по item_id. Заметки и графики в ноутбуке.", "https://example.com/demo/ml/data-notes"); err != nil {
		return err
	}
	// The current common task was started three days ago: the hint worker will notice it is taking long.
	if err := s.startNext(ctx, u, common, 72*time.Hour); err != nil {
		return err
	}
	if err := s.request(ctx, u, "access_request", map[string]any{
		"system": "grafana", "resource": "Рекомендации — онлайн-метрики", "access_level": "read",
		"justification": "Нужен для задачи по оценке модели: смотреть CTR и латентность",
	}); err != nil {
		return err
	}
	if _, err := s.assistant.CreateQuestion(ctx, u, "Можно ли забирать рабочий ноутбук домой на выходные?"); err != nil {
		return fmt.Errorf("mentor question: %w", err)
	}
	if err := s.note(ctx, u, "Встреча с наставником 1:1", "Договорились: к пятнице разобрать выборку, потом бейзлайн. Вопросы копить в чате Бади, срочное писать Ольге в телеграм."); err != nil {
		return err
	}
	if err := s.note(ctx, u, "Полезные ссылки", "Grafana: дашборд «Рекомендации — онлайн-метрики». Вики: правила доступа к данным, код-ревью."); err != nil {
		return err
	}
	if err := s.event(ctx, u, "1:1 с наставником", "Разбор выборки и план по бейзлайну", 1, 11, 30); err != nil {
		return err
	}
	if err := s.event(ctx, u, "Стендап команды", "Ежедневная встреча, 15 минут", 1, 10, 15); err != nil {
		return err
	}
	return s.event(ctx, u, "Демо результатов команды", "Послушать, как команда показывает результаты спринта", 4, 16, 60)
}

// backendIntern has a submission the buddy returned for rework.
func (s *seeder) backendIntern(ctx context.Context) error {
	u := s.users[backendEmail]
	if err := s.advance(ctx, u, common, 2, nil); err != nil {
		return err
	}
	if err := s.advance(ctx, u, professional, 1, nil); err != nil {
		return err
	}
	submission, err := s.submitNext(ctx, u, professional, "Поднял сервис локально через docker compose, тесты проходят.", "")
	if err != nil {
		return err
	}
	comment := "Не вижу, как ты проверил миграции. Приложи вывод goose status и скрин /health."
	if err := s.progress.ReviewSubmission(ctx, s.users[buddyEmail].ID, submission, false, &comment); err != nil {
		return fmt.Errorf("return backend submission for rework: %w", err)
	}
	return s.request(ctx, u, "leave_request", map[string]any{
		"kind": "day_off", "date_from": s.day(9).Format(time.DateOnly), "date_to": s.day(9).Format(time.DateOnly),
		"comment": "Сессия в университете",
	})
}

// frontendIntern has finished the common checklist (the feedback form is available) and is close to
// the end of the professional one.
func (s *seeder) frontendIntern(ctx context.Context) error {
	u := s.users[frontendEmail]
	approve := "Хорошая работа, принято."
	if err := s.advance(ctx, u, common, -1, &approve); err != nil {
		return err
	}
	if err := s.advance(ctx, u, professional, 5, &approve); err != nil {
		return err
	}
	_, err := s.submitNext(ctx, u, professional, "Демо записано, в описании PR перечислил риски и метрики.", "https://example.com/demo/frontend/pr-42")
	return err
}

// staff gives the buddy and the team lead their own calendar.
func (s *seeder) staff(ctx context.Context) error {
	if err := s.event(ctx, s.users[buddyEmail], "1:1 с Алексеем", "Разбор выборки и план по бейзлайну", 1, 11, 30); err != nil {
		return err
	}
	return s.event(ctx, s.users[teamleadEmail], "Синк по адаптации стажёров", "Прогресс, риски, заявки", 2, 15, 30)
}

type track int

const (
	common track = iota
	professional
)

func (s *seeder) items(ctx context.Context, u *model.User, t track) ([]service.ChecklistItemProgress, error) {
	lists, err := s.checklist.GetByUserID(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("checklists of %s: %w", u.Email, err)
	}
	if t == common {
		return lists.Common.Items, nil
	}
	return lists.Professional.Items, nil
}

// nextActive returns the first active item of the track, or nil when nothing is open.
func (s *seeder) nextActive(ctx context.Context, u *model.User, t track) (*service.ChecklistItemProgress, error) {
	items, err := s.items(ctx, u, t)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Status == model.StatusActive {
			return &items[i], nil
		}
	}
	return nil, nil
}

// advance finishes n items of a track in order (n < 0: all). Items that need a review are submitted and,
// with approve set, approved by the buddy; without it the loop stops at the first such item.
func (s *seeder) advance(ctx context.Context, u *model.User, t track, n int, approve *string) error {
	for done := 0; n < 0 || done < n; done++ {
		item, err := s.nextActive(ctx, u, t)
		if err != nil {
			return err
		}
		if item == nil {
			return nil
		}
		if err := s.progress.StartItem(ctx, u.ID, item.Item.ID); err != nil {
			return fmt.Errorf("start %q for %s: %w", item.Item.Title, u.Email, err)
		}
		if !item.Item.RequiresReview {
			if err := s.progress.CompleteItem(ctx, u.ID, item.Item.ID); err != nil {
				return fmt.Errorf("complete %q for %s: %w", item.Item.Title, u.Email, err)
			}
			continue
		}
		if approve == nil {
			return nil
		}
		text := "Результат по задаче «" + item.Item.Title + "» приложен."
		submission, err := s.progress.SubmitItem(ctx, u.ID, item.Item.ID, &text, nil)
		if err != nil {
			return fmt.Errorf("submit %q for %s: %w", item.Item.Title, u.Email, err)
		}
		if err := s.progress.ReviewSubmission(ctx, s.users[buddyEmail].ID, submission.ID, true, approve); err != nil {
			return fmt.Errorf("approve %q for %s: %w", item.Item.Title, u.Email, err)
		}
	}
	return nil
}

// submitNext starts the next active item (it must require a review) and submits a result.
func (s *seeder) submitNext(ctx context.Context, u *model.User, t track, text, url string) (uuid.UUID, error) {
	item, err := s.nextActive(ctx, u, t)
	if err != nil {
		return uuid.Nil, err
	}
	if item == nil || !item.Item.RequiresReview {
		return uuid.Nil, fmt.Errorf("demo seed for %s expects an open task with review", u.Email)
	}
	if err := s.progress.StartItem(ctx, u.ID, item.Item.ID); err != nil {
		return uuid.Nil, err
	}
	var link *string
	if url != "" {
		link = &url
	}
	submission, err := s.progress.SubmitItem(ctx, u.ID, item.Item.ID, &text, link)
	if err != nil {
		return uuid.Nil, fmt.Errorf("submit %q for %s: %w", item.Item.Title, u.Email, err)
	}
	return submission.ID, nil
}

// startNext takes the next active item into work and back-dates the start, so the task looks overdue.
func (s *seeder) startNext(ctx context.Context, u *model.User, t track, ago time.Duration) error {
	item, err := s.nextActive(ctx, u, t)
	if err != nil || item == nil {
		return err
	}
	if err := s.progress.StartItem(ctx, u.ID, item.Item.ID); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE user_checklist_progress SET activated_at = $3
		WHERE user_id = $1 AND checklist_item_id = $2`, u.ID, item.Item.ID, s.now.Add(-ago))
	return err
}

func (s *seeder) request(ctx context.Context, u *model.User, kind string, params map[string]any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	if _, err := s.assistant.CreateRequest(ctx, u, kind, raw); err != nil {
		return fmt.Errorf("%s for %s: %w", kind, u.Email, err)
	}
	return nil
}

func (s *seeder) note(ctx context.Context, u *model.User, title, content string) error {
	return s.personal.CreateNote(ctx, u.ID, &repository.Note{Title: title, Content: content})
}

// day returns local midnight plus the given number of days.
func (s *seeder) day(offset int) time.Time {
	y, m, d := s.now.Date()
	return time.Date(y, m, d+offset, 0, 0, 0, 0, s.now.Location())
}

func (s *seeder) event(ctx context.Context, u *model.User, title, description string, dayOffset, hour, minutes int) error {
	start := s.day(dayOffset).Add(time.Duration(hour) * time.Hour)
	return s.personal.CreateEvent(ctx, u.ID, &repository.CalendarEvent{
		Title: title, Description: description, StartsAt: start, EndsAt: start.Add(time.Duration(minutes) * time.Minute),
	})
}
