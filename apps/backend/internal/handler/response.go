package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

// send отправляет JSON-ответ с указанным HTTP-статусом.
func send(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// decodeJSONBody декодирует один JSON-объект из тела запроса.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		send(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON",
		})
		return false
	}

	if decoder.Decode(&struct{}{}) != io.EOF {
		send(w, http.StatusBadRequest, map[string]string{
			"error": "expected one JSON object",
		})
		return false
	}

	return true
}

// parsePathUUID извлекает UUID из параметра URL.
func parsePathUUID(
	w http.ResponseWriter,
	r *http.Request,
	name, message string,
) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))

	if err != nil || id == uuid.Nil {
		send(w, http.StatusBadRequest, map[string]string{
			"error": message,
		})
		return uuid.Nil, false
	}

	return id, true
}

// writeServiceError преобразует ошибки бизнес-логики в HTTP-ответы.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		send(w, http.StatusNotFound, map[string]string{
			"error": "resource not found",
		})

	case errors.Is(err, service.ErrUserNotIntern),
		errors.Is(err, service.ErrUserNotBuddy),
		errors.Is(err, service.ErrCannotReviewSelf),
		errors.Is(err, service.ErrInternNotAssigned):
		send(w, http.StatusForbidden, map[string]string{
			"error": "forbidden",
		})

	case errors.Is(err, service.ErrInvalidSubmission),
		errors.Is(err, service.ErrSubmissionRequired):
		send(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})

	case errors.Is(err, service.ErrItemNotActive),
		errors.Is(err, service.ErrReviewNotPending),
		errors.Is(err, service.ErrSubmissionAlreadyReviewed),
		errors.Is(err, service.ErrProgressIncomplete),
		errors.Is(err, service.ErrProfessionNotSet):
		send(w, http.StatusConflict, map[string]string{
			"error": err.Error(),
		})

	default:
		log.Printf("checklist API error: %v", err)
		send(w, http.StatusInternalServerError, map[string]string{
			"error": "internal server error",
		})
	}
}
