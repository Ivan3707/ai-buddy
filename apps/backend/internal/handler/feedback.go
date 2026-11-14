package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type FeedbackManager interface {
	Get(ctx context.Context, userID, checklistID uuid.UUID) (*service.Feedback, error)
	Create(ctx context.Context, userID, checklistID uuid.UUID, rating int, comment string) (*service.Feedback, error)
}

func (d Dependencies) RegisterFeedbackRoutes(mux *http.ServeMux, authenticated func(http.Handler) http.Handler) {
	intern := middleware.RequireRole(model.RoleIntern)
	mux.Handle("GET /api/checklists/{checklistID}/feedback", authenticated(intern(http.HandlerFunc(d.getFeedback))))
	mux.Handle("POST /api/checklists/{checklistID}/feedback", authenticated(intern(http.HandlerFunc(d.postFeedback))))
}
func feedbackID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("checklistID"))
	if err != nil {
		send(w, 400, map[string]string{"error": "invalid checklist ID"})
		return uuid.Nil, false
	}
	return id, true
}
func feedbackError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrFeedbackNotFound):
		send(w, 404, map[string]string{"error": "checklist not found"})
	case errors.Is(err, service.ErrFeedbackIncomplete):
		send(w, 409, map[string]string{"error": "checklist is not fully completed and reviewed"})
	case errors.Is(err, service.ErrFeedbackExists):
		send(w, 409, map[string]string{"error": "feedback already submitted"})
	case errors.Is(err, service.ErrFeedbackInvalid):
		send(w, 400, map[string]string{"error": "rating must be 1..5 and comment at most 4000 characters"})
	default:
		send(w, 500, map[string]string{"error": "internal server error"})
	}
}
func (d Dependencies) getFeedback(w http.ResponseWriter, r *http.Request) {
	id, ok := feedbackID(w, r)
	if !ok {
		return
	}
	user, _ := middleware.CurrentUser(r.Context())
	item, err := d.Feedback.Get(r.Context(), user.ID, id)
	if err != nil {
		feedbackError(w, err)
		return
	}
	if item == nil {
		send(w, 200, map[string]any{"feedback": nil})
		return
	}
	send(w, 200, map[string]any{"feedback": item})
}
func (d Dependencies) postFeedback(w http.ResponseWriter, r *http.Request) {
	id, ok := feedbackID(w, r)
	if !ok {
		return
	}
	user, _ := middleware.CurrentUser(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var body struct {
		Rating  int    `json:"rating"`
		Comment string `json:"comment"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil || dec.Decode(&struct{}{}) != io.EOF {
		send(w, 400, map[string]string{"error": "invalid feedback JSON"})
		return
	}
	item, err := d.Feedback.Create(r.Context(), user.ID, id, body.Rating, body.Comment)
	if err != nil {
		feedbackError(w, err)
		return
	}
	send(w, 201, item)
}
