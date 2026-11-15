package service

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

// AI assistant: the backend is the only door to the Python AI service. It builds a verified UserContext from the
// database (the model never sees data the user may not see), relays the AI event stream, stores the conversation
// and turns an ActionProposal into a PendingAction that runs only after the user confirms it.
// Chat texts are never logged: only ids, statuses and latencies.

var (
	ErrActionCancelled   = errors.New("action cancelled")
	ErrActionExecuted    = errors.New("already executed")
	ErrActionInvalid     = errors.New("invalid action params")
	ErrAIUnavailable     = errors.New("AI unavailable")
	ErrNotBuddy          = errors.New("not a buddy")
	ErrNotTeamlead       = errors.New("not a teamlead")
	ErrQuestionAnswered  = errors.New("question already answered")
	ErrInvalidDecision   = errors.New("status must be approved or rejected")
	ErrRequestNotPending = errors.New("request already decided")
)

const (
	historyTurns    = 20
	maxContextTasks = 30
	minutesPerReply = 5 // mentor minutes a resolved question saves (assumption shown in metrics)
)

type AssistantService struct {
	db     *sql.DB
	aiURL  string
	stream *http.Client // no overall timeout: the request context bounds the stream
	short  *http.Client
}

func NewAssistantService(db *sql.DB, aiURL string) *AssistantService {
	return &AssistantService{
		db:     db,
		aiURL:  strings.TrimRight(aiURL, "/"),
		stream: &http.Client{},
		short:  &http.Client{Timeout: 3 * time.Minute},
	}
}
