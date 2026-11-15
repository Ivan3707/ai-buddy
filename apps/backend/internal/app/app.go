package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
	"github.com/nvkmaksim/ai-buddy/backend/internal/handler"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func Run() error {
	corporateData, err := os.ReadFile(env("DATA_DIR", "../../data") + "/corporate_employees.json")
	if err != nil {
		return fmt.Errorf("read corporate mock: %w", err)
	}
	directory, err := service.NewCorporateDirectory(corporateData)
	if err != nil {
		return fmt.Errorf("parse corporate mock: %w", err)
	}

	db, err := database.OpenPostgres()
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	jwtService, err := security.NewJWTService(
		os.Getenv("JWT_SECRET"),
		15*time.Minute,
	)
	if err != nil {
		return fmt.Errorf("configure JWT: %w", err)
	}

	users := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	auth := service.NewAuthServiceWithSessions(users, jwtService, sessionRepo)
	hints := service.NewHintService(repository.NewHintRepository(db), env("AI_URL", "http://127.0.0.1:8001"))

	progress := service.NewProgressService(db)
	personalTasks := service.NewPersonalTaskService(db)
	userService := service.NewUserService(db)
	admin := service.NewAdminService(db, env("AI_URL", "http://127.0.0.1:8001"), hints)
	admin.OpenForAll = os.Getenv("ADMIN_OPEN_FOR_ALL") == "1"
	deps := handler.Dependencies{
		Ready:            func(ctx context.Context) error { return checkReady(ctx, db) },
		Feedback:         service.NewFeedbackService(db, service.MockAnalytics{DB: db, Unavailable: os.Getenv("ANALYTICS_MOCK_UNAVAILABLE") == "1"}),
		Auth:             auth,
		Sessions:         auth,
		SessionCheck:     sessionRepo,
		PersonalTasks:    personalTasks,
		ChecklistEditor:  personalTasks,
		StaffSubmissions: progress,
		Hints:            hints,
		Personal:         service.NewPersonalService(repository.NewPersonalRepository(db)),
		Corporate:        service.NewCorporateService(db, directory),
		Users:            users,
		JWT:              jwtService,
		Team:             userService,
		Checklists:       service.NewChecklistService(db),
		Items:            progress,
		Submissions:      progress,
		Assistant:        service.NewAssistantService(db, env("AI_URL", "http://127.0.0.1:8001")),
		TaskComments:     service.NewTaskCommentService(db),
		Admin:            admin,
	}
	deps.Corporate.Unavailable = os.Getenv("CORPORATE_MOCK_UNAVAILABLE") == "1"
	defer db.Close()
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	go hints.Run(workerCtx)

	server := &http.Server{
		Addr: ":8080",
		Handler: handler.NewRouter(
			env("AI_URL", "http://127.0.0.1:8001"),
			deps,
		),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Print("AI-buddy backend listening on :8080")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server: %w", err)
	}

	return nil
}

// checkReady: без миграций и загруженного каталога пользовательские API бесполезны, поэтому сервис не должен считаться готовым.
func checkReady(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var items int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklist_items`).Scan(&items); err != nil {
		return errors.New("database is not migrated")
	}
	if items == 0 {
		return errors.New("checklist catalog is not loaded")
	}
	return nil
}
