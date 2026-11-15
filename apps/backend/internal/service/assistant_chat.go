package service

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// ---------------------------------------------------------------- conversations

// StartTurn checks the conversation owner, creates one when needed, returns prior turns and stores the user message.
func (s *AssistantService) StartTurn(ctx context.Context, userID uuid.UUID, conversationID *uuid.UUID, message string) (uuid.UUID, []ChatTurn, error) {
	id, err := s.conversation(ctx, userID, conversationID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT role, content FROM (
			SELECT role, content, created_at FROM chat_messages WHERE conversation_id = $1
			ORDER BY created_at DESC LIMIT $2
		) last ORDER BY created_at`, id, historyTurns)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("load history: %w", err)
	}
	defer rows.Close()
	history := []ChatTurn{}
	for rows.Next() {
		var t ChatTurn
		if err := rows.Scan(&t.Role, &t.Content); err != nil {
			return uuid.Nil, nil, err
		}
		if strings.TrimSpace(t.Content) != "" {
			history = append(history, ChatTurn{Role: t.Role, Content: truncate(t.Content, 8000)})
		}
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, nil, err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO chat_messages (conversation_id, role, content, created_at) VALUES ($1, 'user', $2, clock_timestamp())`, id, message); err != nil {
		return uuid.Nil, nil, fmt.Errorf("store message: %w", err)
	}
	return id, history, nil
}

// conversation returns the given conversation if it belongs to the user, or the user's latest/new one when nil.
func (s *AssistantService) conversation(ctx context.Context, userID uuid.UUID, requested *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	if requested != nil {
		err := s.db.QueryRowContext(ctx, `SELECT id FROM chat_conversations WHERE id = $1 AND user_id = $2`, *requested, userID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, repository.ErrNotFound
		}
		return id, err
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO chat_conversations (user_id) VALUES ($1) RETURNING id`, userID).Scan(&id)
	return id, err
}

func (s *AssistantService) latestConversation(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRowContext(ctx, `SELECT id FROM chat_conversations WHERE user_id = $1 ORDER BY updated_at DESC LIMIT 1`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// BriefConversation puts the proactive brief into the user's latest conversation (or a new one), so the chat
// that follows continues it.
func (s *AssistantService) BriefConversation(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	latest, err := s.latestConversation(ctx, userID)
	if err != nil {
		return uuid.Nil, err
	}
	if latest != nil {
		return *latest, nil
	}
	return s.conversation(ctx, userID, nil)
}

// Emit writes one SSE event to the client.
type Emit func(event string, data []byte) error

// Relay calls an AI streaming endpoint and re-emits its events. The first event is `meta`; an AI `action`
// (ActionProposal) is stored as a PendingAction and emitted as {"action": PendingAction}. The assistant message is
// stored when the stream ends, even if the client went away.
func (s *AssistantService) Relay(ctx context.Context, userID, conversationID uuid.UUID, path string, payload any, emit Emit) error {
	messageID := uuid.New()
	meta, _ := json.Marshal(map[string]string{"conversation_id": conversationID.String(), "message_id": messageID.String()})
	if err := emit("meta", meta); err != nil {
		return err
	}
	started := time.Now()
	var text strings.Builder
	var sources json.RawMessage
	var actionID *uuid.UUID
	var finish *string
	status := "ok"

	body, _ := json.Marshal(payload)
	streamCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodPost, s.aiURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := s.stream.Do(req)
	if err == nil && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		err = fmt.Errorf("AI HTTP %d", resp.StatusCode)
	}
	if err != nil {
		status = "ai_unavailable"
		errData, _ := json.Marshal(map[string]string{"code": "llm_unavailable", "message": "AI-сервис недоступен. Попробуй ещё раз чуть позже."})
		_ = emit("error", errData)
	} else {
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		event := ""
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(line[len("event:"):])
			case strings.HasPrefix(line, "data:"):
				data := []byte(strings.TrimSpace(line[len("data:"):]))
				switch event {
				case "delta":
					var d struct {
						Text string `json:"text"`
					}
					if json.Unmarshal(data, &d) == nil {
						text.WriteString(d.Text)
					}
				case "sources":
					var d struct {
						Sources json.RawMessage `json:"sources"`
					}
					if json.Unmarshal(data, &d) == nil {
						sources = d.Sources
					}
				case "done":
					var d struct {
						FinishReason string `json:"finish_reason"`
					}
					if json.Unmarshal(data, &d) == nil && d.FinishReason != "" {
						finish = &d.FinishReason
					}
				case "error":
					status = "ai_error"
				case "action":
					action, perr := s.storeProposal(ctx, userID, data)
					if perr != nil {
						log.Printf("assistant: store action proposal failed: %v", perr)
						status = "action_error"
						continue
					}
					actionID = &action.ID
					data, _ = json.Marshal(map[string]PendingAction{"action": action})
				}
				if err := emit(event, data); err != nil {
					status = "client_gone"
				}
			}
		}
		if err := scanner.Err(); err != nil && status == "ok" {
			status = "stream_broken"
			errData, _ := json.Marshal(map[string]string{"code": "timeout", "message": "Ответ прервался. Попробуй ещё раз."})
			_ = emit("error", errData)
		}
	}
	// Persist with a fresh context: the client may have disconnected mid-stream.
	saveCtx, cancelSave := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelSave()
	if text.Len() > 0 || actionID != nil {
		if _, err := s.db.ExecContext(saveCtx, `
			INSERT INTO chat_messages (id, conversation_id, role, content, sources, action_id, finish_reason)
			VALUES ($1, $2, 'assistant', $3, $4, $5, $6)`,
			messageID, conversationID, truncate(text.String(), 16000), nullJSON(sources), actionID, finish); err != nil {
			log.Printf("assistant: store reply failed: %v", err)
		}
	}
	_, _ = s.db.ExecContext(saveCtx, `UPDATE chat_conversations SET updated_at = now() WHERE id = $1`, conversationID)
	log.Printf("assistant: path=%s conversation=%s status=%s latency_ms=%d action=%t",
		path, conversationID, status, time.Since(started).Milliseconds(), actionID != nil)
	return nil
}

func nullJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

func truncate(s string, runes int) string {
	r := []rune(s)
	if len(r) <= runes {
		return s
	}
	return string(r[:runes])
}

func (s *AssistantService) storeProposal(ctx context.Context, userID uuid.UUID, data []byte) (PendingAction, error) {
	var in struct {
		Proposal struct {
			Type    string          `json:"type"`
			Params  json.RawMessage `json:"params"`
			Summary string          `json:"summary"`
		} `json:"proposal"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return PendingAction{}, err
	}
	p := in.Proposal
	if _, err := validateParams(p.Type, p.Params); err != nil {
		return PendingAction{}, err
	}
	var a PendingAction
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO assistant_actions (user_id, type, params, summary) VALUES ($1, $2, $3, $4)
		RETURNING id, type, params, summary, status, error, created_at`,
		userID, p.Type, []byte(p.Params), truncate(p.Summary, 500)).
		Scan(&a.ID, &a.Type, &a.Params, &a.Summary, &a.Status, &a.Error, &a.CreatedAt)
	return a, err
}

// ---------------------------------------------------------------- history and feedback

func (s *AssistantService) Messages(ctx context.Context, userID uuid.UUID, conversationID *uuid.UUID) (*uuid.UUID, []ChatMessage, error) {
	if conversationID == nil {
		latest, err := s.latestConversation(ctx, userID)
		if err != nil || latest == nil {
			return nil, []ChatMessage{}, err
		}
		conversationID = latest
	} else if _, err := s.conversation(ctx, userID, conversationID); err != nil {
		return nil, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.role, m.content, m.sources, m.finish_reason, m.feedback, m.created_at,
		       a.id, a.type, a.params, a.summary, a.status, a.error, a.created_at
		FROM chat_messages m LEFT JOIN assistant_actions a ON a.id = m.action_id
		WHERE m.conversation_id = $1 ORDER BY m.created_at`, *conversationID)
	if err != nil {
		return nil, nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		var sources []byte
		var aID *uuid.UUID
		var aType, aSummary, aStatus *string
		var aParams []byte
		var aErr *string
		var aCreated *time.Time
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &sources, &m.FinishReason, &m.Feedback, &m.CreatedAt,
			&aID, &aType, &aParams, &aSummary, &aStatus, &aErr, &aCreated); err != nil {
			return nil, nil, err
		}
		if len(sources) > 0 {
			m.Sources = sources
		}
		if aID != nil {
			m.Action = &PendingAction{ID: *aID, Type: *aType, Params: aParams, Summary: *aSummary, Status: *aStatus, Error: aErr, CreatedAt: *aCreated}
		}
		out = append(out, m)
	}
	return conversationID, out, rows.Err()
}

func (s *AssistantService) Feedback(ctx context.Context, userID, messageID uuid.UUID, rating, comment string) error {
	if rating != "up" && rating != "down" {
		return ErrActionInvalid
	}
	comment = strings.TrimSpace(comment)
	if r := []rune(comment); len(r) > 1000 {
		comment = string(r[:1000])
	}
	if rating != "down" {
		comment = ""
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE chat_messages m SET
		  feedback_comment = CASE WHEN $4 <> '' THEN $4 WHEN m.feedback IS DISTINCT FROM $3 THEN NULL ELSE m.feedback_comment END,
		  feedback = $3
		FROM chat_conversations c
		WHERE m.id = $1 AND c.id = m.conversation_id AND c.user_id = $2 AND m.role = 'assistant'`, messageID, userID, rating, comment)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return repository.ErrNotFound
	}
	return nil
}
