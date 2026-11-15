package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	historyRows, err := repository.NewAssistantRepository(s.db).History(ctx, id, historyTurns)
	if err != nil {
		return uuid.Nil, nil, err
	}
	history := []ChatTurn{}
	for _, t := range historyRows {
		if strings.TrimSpace(t.Content) != "" {
			history = append(history, ChatTurn{Role: t.Role, Content: truncate(t.Content, 8000)})
		}
	}
	if err := repository.NewAssistantRepository(s.db).AddUserMessage(ctx, id, message); err != nil {
		return uuid.Nil, nil, fmt.Errorf("store message: %w", err)
	}
	return id, history, nil
}

// conversation returns the given conversation if it belongs to the user, or the user's latest/new one when nil.
func (s *AssistantService) conversation(ctx context.Context, userID uuid.UUID, requested *uuid.UUID) (uuid.UUID, error) {
	return repository.NewAssistantRepository(s.db).Conversation(ctx, userID, requested)
}

func (s *AssistantService) latestConversation(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error) {
	return repository.NewAssistantRepository(s.db).LatestConversation(ctx, userID)
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
		if err := repository.NewAssistantRepository(s.db).AddAssistantMessage(saveCtx, messageID, conversationID, truncate(text.String(), 16000), sources, actionID, finish); err != nil {
			log.Printf("assistant: store reply failed: %v", err)
		}
	}
	_ = repository.NewAssistantRepository(s.db).TouchConversation(saveCtx, conversationID)
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
	return repository.NewAssistantRepository(s.db).StoreProposal(ctx, userID, p.Type, p.Params, truncate(p.Summary, 500))
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
	out, err := repository.NewAssistantRepository(s.db).Messages(ctx, *conversationID)
	return conversationID, out, err
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
	return repository.NewAssistantRepository(s.db).SetFeedback(ctx, userID, messageID, rating, comment)
}
