package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type chatRequest struct {
	Message string `json:"message"`
}
type source struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}
type chatResponse struct {
	Answer  string   `json:"answer"`
	Sources []source `json:"sources"`
	Mode    string   `json:"mode"`
}
type task struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type scenario struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Role  string `json:"role"`
	Tasks []task `json:"tasks"`
}

func send(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newHandler(
	aiURL string,
	onboarding []byte,
	deps handlerDependencies,
) http.Handler {
	mux := http.NewServeMux()
	client := &http.Client{Timeout: 10 * time.Second}

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		send(w, http.StatusOK, map[string]string{
			"status": "ok",
			"mode":   "mock",
		})
	})

	mux.HandleFunc("GET /api/onboarding", func(w http.ResponseWriter, r *http.Request) {
		role := r.URL.Query().Get("role")
		if role == "" {
			role = "ml"
		}
		if role != "ml" && role != "backend" && role != "frontend" {
			send(w, http.StatusBadRequest, map[string]string{"error": "unknown role"})
			return
		}

		var all []scenario
		if json.Unmarshal(onboarding, &all) != nil {
			send(w, http.StatusInternalServerError, map[string]string{"error": "invalid demo data"})
			return
		}

		selected := []scenario{}
		for _, s := range all {
			if s.Role == "all" || s.Role == role {
				selected = append(selected, s)
			}
		}

		send(w, http.StatusOK, selected)
	})

	mux.HandleFunc("POST /api/auth/login", deps.loginHandler())

	authenticated := func(handler http.Handler) http.Handler {
		return middleware.Authenticate(deps.jwt, deps.users)(handler)
	}

	mux.Handle(
		"GET /api/auth/me",
		authenticated(deps.meHandler()),
	)
	mux.Handle(
		"POST /api/users",
		authenticated(
			middleware.RequireRole(model.RoleBuddy)(
				http.HandlerFunc(deps.createInternHandler()),
			),
		),
	)
	mux.Handle(
		"POST /api/interns/{internID}/checklist-items/{itemID}/review",
		authenticated(
			middleware.RequireRole(model.RoleBuddy)(
				http.HandlerFunc(deps.reviewItemHandler()),
			),
		),
	)

	chatHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16384)

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var body chatRequest
		if err := decoder.Decode(&body); err != nil {
			send(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			send(w, http.StatusBadRequest, map[string]string{"error": "expected one JSON object"})
			return
		}
		if strings.TrimSpace(body.Message) == "" ||
			utf8.RuneCountInString(body.Message) > 2000 {
			send(w, http.StatusBadRequest, map[string]string{
				"error": "message must contain 1..2000 characters",
			})
			return
		}

		payload, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(
			r.Context(),
			http.MethodPost,
			aiURL+"/chat",
			bytes.NewReader(payload),
		)
		if err != nil {
			send(w, http.StatusBadGateway, map[string]string{"error": "AI unavailable"})
			return
		}

		req.Header.Set("Content-Type", "application/json")

		res, err := client.Do(req)
		if err != nil {
			send(w, http.StatusBadGateway, map[string]string{"error": "AI unavailable"})
			return
		}
		defer res.Body.Close()

		var reply chatResponse
		if res.StatusCode != http.StatusOK ||
			json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&reply) != nil ||
			reply.Answer == "" ||
			reply.Mode != "mock" ||
			reply.Sources == nil {
			send(w, http.StatusBadGateway, map[string]string{"error": "invalid AI response"})
			return
		}

		send(w, http.StatusOK, reply)
	})

	mux.Handle("POST /api/chat", authenticated(chatHandler))

	return mux
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	data, err := os.ReadFile(env("DATA_DIR", "../../data") + "/onboarding.json")
	if err != nil {
		return fmt.Errorf("read onboarding data: %w", err)
	}

	if !json.Valid(data) {
		return fmt.Errorf("invalid onboarding JSON")
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
	auth := service.NewAuthService(users, jwtService)

	deps := handlerDependencies{
		auth:     auth,
		users:    users,
		jwt:      jwtService,
		interns:  service.NewUserService(db),
		progress: service.NewProgressService(db),
	}
	defer db.Close()

	server := &http.Server{
		Addr: ":8080",
		Handler: newHandler(
			env("AI_URL", "http://127.0.0.1:8001"),
			data,
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
