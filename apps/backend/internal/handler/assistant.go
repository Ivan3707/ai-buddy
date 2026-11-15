package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

// RegisterAssistantRoutes adds the AI assistant API (contracts/backend.openapi.json): streaming chat and brief,
// history, feedback, confirmed actions, mock corporate requests and the buddy side (escalated questions,
// AI draft, knowledge gaps, KB lint, metrics).
func RegisterAssistantRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler, s Assistant) {
	h := assistantHandler{s: s}
	route := func(pattern string, fn func(http.ResponseWriter, *http.Request, *model.User)) {
		mux.Handle(pattern, auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := middleware.CurrentUser(r.Context())
			if !ok {
				send(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			fn(w, r, user)
		})))
	}
	route("POST /api/chat/stream", h.chatStream)
	route("POST /api/chat/brief/stream", h.brief)
	route("GET /api/chat/messages", h.messages)
	route("POST /api/chat/messages/{id}/feedback", h.feedback)
	route("POST /api/actions/{id}/confirm", h.confirm)
	route("POST /api/actions/{id}/cancel", h.cancel)
	route("GET /api/requests", h.requests)
	route("POST /api/requests", h.createRequest)
	route("GET /api/requests/{id}", h.request)
	route("GET /api/buddy/requests", h.staffRequests)
	route("POST /api/buddy/requests/{id}/decision", h.decide)
	route("GET /api/questions", h.ownQuestions)
	route("POST /api/questions", h.createQuestion)
	route("GET /api/buddy/questions", h.questions)
	route("POST /api/buddy/questions/{id}/answer", h.answer)
	route("GET /api/buddy/questions/{id}/draft", h.draft)
	route("GET /api/kb/docs/{id}", h.kbDoc)
	route("GET /api/suggestions", h.suggestions)
	route("POST /api/chat/hint", h.hintTurn)
	route("GET /api/metrics/chat", h.metrics)
}

type assistantHandler struct{ s Assistant }

func assistantError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		send(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, service.ErrNotBuddy), errors.Is(err, service.ErrNotTeamlead):
		send(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
	case errors.Is(err, service.ErrActionCancelled), errors.Is(err, service.ErrActionExecuted),
		errors.Is(err, service.ErrQuestionAnswered), errors.Is(err, service.ErrRequestNotPending):
		send(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrActionInvalid):
		send(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidDecision):
		send(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrAIUnavailable):
		send(w, http.StatusBadGateway, map[string]string{"error": "AI unavailable"})
	default:
		log.Printf("assistant API error: %v", err)
		send(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}

// decodeBody is decodeJSONBody with a larger limit (page context and selection can be a few KB).
func decodeBody(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		send(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		send(w, http.StatusBadRequest, map[string]string{"error": "expected one JSON object"})
		return false
	}
	return true
}

// sseWriter streams events. The server's WriteTimeout (15 s) is extended for this response only.
func sseWriter(w http.ResponseWriter) (service.Emit, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		send(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return nil, false
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(3 * time.Minute))
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	return func(event string, data []byte) error {
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}, true
}

type streamChatRequest struct {
	Message        string               `json:"message"`
	ConversationID *uuid.UUID           `json:"conversation_id,omitempty"`
	PageContext    *service.PageContext `json:"page_context,omitempty"`
}

func (h assistantHandler) chatStream(w http.ResponseWriter, r *http.Request, user *model.User) {
	var body streamChatRequest
	if !decodeBody(w, r, &body, 16384) {
		return
	}
	body.Message = strings.TrimSpace(body.Message)
	if body.Message == "" || utf8.RuneCountInString(body.Message) > 2000 {
		send(w, http.StatusBadRequest, map[string]string{"error": "message must contain 1..2000 characters"})
		return
	}
	if pc := body.PageContext; pc != nil && (pc.URL == "" || utf8.RuneCountInString(pc.URL) > 2000 ||
		utf8.RuneCountInString(pc.Title) > 300 || utf8.RuneCountInString(pc.Selection) > 2000) {
		send(w, http.StatusBadRequest, map[string]string{"error": "invalid page_context"})
		return
	}
	uc, err := h.s.UserContext(r.Context(), user)
	if err != nil {
		assistantError(w, err)
		return
	}
	conversationID, history, err := h.s.StartTurn(r.Context(), user.ID, body.ConversationID, body.Message)
	if err != nil {
		assistantError(w, err)
		return
	}
	payload := map[string]any{"message": body.Message, "history": history, "user_context": uc}
	if body.PageContext != nil {
		payload["page_context"] = body.PageContext
	}
	emit, ok := sseWriter(w)
	if !ok {
		return
	}
	_ = h.s.Relay(r.Context(), user.ID, conversationID, "/v1/chat/stream", payload, emit)
}

func (h assistantHandler) brief(w http.ResponseWriter, r *http.Request, user *model.User) {
	uc, err := h.s.UserContext(r.Context(), user)
	if err != nil {
		assistantError(w, err)
		return
	}
	conversationID, err := h.s.BriefConversation(r.Context(), user.ID)
	if err != nil {
		assistantError(w, err)
		return
	}
	emit, ok := sseWriter(w)
	if !ok {
		return
	}
	_ = h.s.Relay(r.Context(), user.ID, conversationID, "/v1/brief/stream", map[string]any{"user_context": uc}, emit)
}

func (h assistantHandler) messages(w http.ResponseWriter, r *http.Request, user *model.User) {
	var conversationID *uuid.UUID
	if raw := r.URL.Query().Get("conversation_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			send(w, http.StatusBadRequest, map[string]string{"error": "invalid conversation_id"})
			return
		}
		conversationID = &id
	}
	id, messages, err := h.s.Messages(r.Context(), user.ID, conversationID)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusOK, map[string]any{"conversation_id": id, "messages": messages})
}

func (h assistantHandler) feedback(w http.ResponseWriter, r *http.Request, user *model.User) {
	id, ok := parsePathUUID(w, r, "id", "invalid message id")
	if !ok {
		return
	}
	var body struct {
		Rating  string `json:"rating"`
		Comment string `json:"comment"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body.Rating != "up" && body.Rating != "down" {
		send(w, http.StatusBadRequest, map[string]string{"error": "rating must be up or down"})
		return
	}
	if err := h.s.Feedback(r.Context(), user.ID, id, body.Rating, body.Comment); err != nil {
		assistantError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h assistantHandler) confirm(w http.ResponseWriter, r *http.Request, user *model.User) {
	id, ok := parsePathUUID(w, r, "id", "invalid action id")
	if !ok {
		return
	}
	result, err := h.s.Confirm(r.Context(), user, id)
	if err != nil {
		assistantError(w, err)
		return
	}
	if result.Action.Status == "failed" {
		msg := "invalid params"
		if result.Action.Error != nil {
			msg = *result.Action.Error
		}
		send(w, http.StatusUnprocessableEntity, map[string]string{"error": msg})
		return
	}
	send(w, http.StatusOK, result)
}

func (h assistantHandler) cancel(w http.ResponseWriter, r *http.Request, user *model.User) {
	id, ok := parsePathUUID(w, r, "id", "invalid action id")
	if !ok {
		return
	}
	action, err := h.s.Cancel(r.Context(), user.ID, id)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusOK, action)
}

func (h assistantHandler) requests(w http.ResponseWriter, r *http.Request, user *model.User) {
	list, err := h.s.ListRequests(r.Context(), user.ID, 100)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusOK, list)
}

func (h assistantHandler) createRequest(w http.ResponseWriter, r *http.Request, user *model.User) {
	var in struct {
		Type   string          `json:"type"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		send(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	result, err := h.s.CreateRequest(r.Context(), user, in.Type, in.Params)
	if err != nil {
		assistantError(w, err)
		return
	}
	if result.Action.Status == "failed" {
		msg := "invalid params"
		if result.Action.Error != nil {
			msg = *result.Action.Error
		}
		send(w, http.StatusUnprocessableEntity, map[string]string{"error": msg})
		return
	}
	send(w, http.StatusCreated, result.Request)
}

func (h assistantHandler) request(w http.ResponseWriter, r *http.Request, user *model.User) {
	id, ok := parsePathUUID(w, r, "id", "invalid request id")
	if !ok {
		return
	}
	req, err := h.s.GetRequest(r.Context(), user, id)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusOK, req)
}

func (h assistantHandler) staffRequests(w http.ResponseWriter, r *http.Request, user *model.User) {
	list, err := h.s.StaffRequests(r.Context(), user)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusOK, list)
}

func (h assistantHandler) decide(w http.ResponseWriter, r *http.Request, user *model.User) {
	id, ok := parsePathUUID(w, r, "id", "invalid request id")
	if !ok {
		return
	}
	var body struct {
		Status  string `json:"status"`
		Comment string `json:"comment"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	req, err := h.s.DecideRequest(r.Context(), user, id, body.Status, body.Comment)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusOK, req)
}

func (h assistantHandler) questions(w http.ResponseWriter, r *http.Request, user *model.User) {
	status := r.URL.Query().Get("status")
	if status != "" && status != "open" && status != "answered" {
		send(w, http.StatusBadRequest, map[string]string{"error": "status must be open or answered"})
		return
	}
	list, err := h.s.Questions(r.Context(), user, status)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusOK, list)
}

func (h assistantHandler) createQuestion(w http.ResponseWriter, r *http.Request, user *model.User) {
	var in struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		send(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	q, err := h.s.CreateQuestion(r.Context(), user, in.Question)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusCreated, q)
}

func (h assistantHandler) ownQuestions(w http.ResponseWriter, r *http.Request, user *model.User) {
	list, err := h.s.OwnQuestions(r.Context(), user)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusOK, list)
}

func (h assistantHandler) answer(w http.ResponseWriter, r *http.Request, user *model.User) {
	id, ok := parsePathUUID(w, r, "id", "invalid question id")
	if !ok {
		return
	}
	var body struct {
		Answer string `json:"answer"`
	}
	if !decodeBody(w, r, &body, 32768) {
		return
	}
	body.Answer = strings.TrimSpace(body.Answer)
	if body.Answer == "" || utf8.RuneCountInString(body.Answer) > 4000 {
		send(w, http.StatusBadRequest, map[string]string{"error": "answer must contain 1..4000 characters"})
		return
	}
	q, err := h.s.Answer(r.Context(), user, id, body.Answer)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusOK, q)
}

func sendRaw(w http.ResponseWriter, raw json.RawMessage, err error) {
	if err != nil {
		assistantError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (h assistantHandler) draft(w http.ResponseWriter, r *http.Request, user *model.User) {
	id, ok := parsePathUUID(w, r, "id", "invalid question id")
	if !ok {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(3 * time.Minute))
	raw, err := h.s.Draft(r.Context(), user, id)
	sendRaw(w, raw, err)
}

func (h assistantHandler) suggestions(w http.ResponseWriter, r *http.Request, _ *model.User) {
	raw, err := h.s.Suggestions(r.Context())
	sendRaw(w, raw, err)
}

func (h assistantHandler) hintTurn(w http.ResponseWriter, r *http.Request, user *model.User) {
	var body struct {
		Question       string     `json:"question"`
		ConversationID *uuid.UUID `json:"conversation_id"`
	}
	if !decodeBody(w, r, &body, 4096) {
		return
	}
	res, err := h.s.HintTurn(r.Context(), user.ID, body.ConversationID, body.Question)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusOK, res)
}

func (h assistantHandler) kbDoc(w http.ResponseWriter, r *http.Request, _ *model.User) {
	raw, err := h.s.KBDoc(r.Context(), r.PathValue("id"))
	sendRaw(w, raw, err)
}

func (h assistantHandler) metrics(w http.ResponseWriter, r *http.Request, user *model.User) {
	m, err := h.s.Metrics(r.Context(), user)
	if err != nil {
		assistantError(w, err)
		return
	}
	send(w, http.StatusOK, m)
}
