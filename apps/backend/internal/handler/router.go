package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
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

func NewRouter(
	aiURL string,
	onboarding []byte,
	deps Dependencies,
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

	authenticated := func(handler http.Handler) http.Handler {
		if deps.SessionCheck != nil {
			return middleware.Authenticate(deps.JWT, deps.Users, deps.SessionCheck)(handler)
		}
		return middleware.Authenticate(deps.JWT, deps.Users)(handler)
	}
	Dependencies{
		Auth:     deps.Auth,
		Users:    deps.Users,
		Interns:  deps.Interns,
		Sessions: deps.Sessions,
	}.RegisterAuthRoutes(mux, authenticated)

	if deps.Corporate != nil {
		Dependencies{
			Corporate: deps.Corporate,
			Users:     deps.Users,
		}.RegisterCorporateRoutes(mux, authenticated)
	}
	if deps.Checklists != nil {
		Dependencies{
			Users:      deps.Users,
			Checklists: deps.Checklists,
		}.RegisterProgressRoutes(mux, authenticated)
	}
	if deps.PersonalTasks != nil {
		Dependencies{
			PersonalTasks: deps.PersonalTasks,
		}.RegisterPersonalTaskRoutes(mux, authenticated)
	}
	if deps.Hints != nil {
		hintDeps := Dependencies{
			Hints: deps.Hints,
		}

		hintDeps.RegisterHintRoutes(mux, authenticated)
	}
	if deps.Personal != nil {
		Dependencies{
			Personal: deps.Personal,
		}.RegisterPersonalRoutes(mux, authenticated)
	}

	if deps.Interns != nil {
		Dependencies{
			Interns: deps.Interns,
		}.RegisterAssignmentRoutes(mux, authenticated)
	}
	Dependencies{
		Checklists:  deps.Checklists,
		Items:       deps.Items,
		Submissions: deps.Submissions,
	}.RegisterChecklistRoutes(mux, authenticated)

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
