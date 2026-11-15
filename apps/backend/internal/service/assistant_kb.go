package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// ---------------------------------------------------------------- suggestions (ready-made hints, no LLM)

type suggestion struct {
	Question string `json:"question"`
	DocID    string `json:"doc_id"`
	Title    string `json:"title"`
	Heading  string `json:"heading"`
	URL      string `json:"url"`
	Excerpt  string `json:"excerpt"`
}

// Suggestions returns the ready-made questions the chat offers in the empty input (AI /v1/suggestions).
func (s *AssistantService) Suggestions(ctx context.Context) (json.RawMessage, error) {
	return s.callAI(ctx, http.MethodGet, "/v1/suggestions", nil)
}

type HintTurnResult struct {
	ConversationID uuid.UUID       `json:"conversation_id"`
	MessageID      uuid.UUID       `json:"message_id"`
	Text           string          `json:"text"`
	Sources        json.RawMessage `json:"sources"`
}

// HintTurn answers one of the offered suggestions WITHOUT the model: a short excerpt of the document section and a
// link to it. The question must be one of the suggestions and the text is built here from the knowledge base, so the
// client cannot write an "assistant" message into the dialog. Both messages join the conversation: the next message
// of the user goes through the model with this exchange as its history.
func (s *AssistantService) HintTurn(ctx context.Context, userID uuid.UUID, conversationID *uuid.UUID, question string) (HintTurnResult, error) {
	var out HintTurnResult
	raw, err := s.Suggestions(ctx)
	if err != nil {
		return out, err
	}
	var list struct {
		Suggestions []suggestion `json:"suggestions"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return out, ErrAIUnavailable
	}
	var hit *suggestion
	for i := range list.Suggestions {
		if strings.EqualFold(strings.TrimSpace(list.Suggestions[i].Question), strings.TrimSpace(question)) {
			hit = &list.Suggestions[i]
			break
		}
	}
	if hit == nil {
		return out, repository.ErrNotFound
	}
	snippet := strings.Join(strings.Fields(hit.Excerpt), " ")
	if r := []rune(snippet); len(r) > 100 {
		snippet = strings.TrimSpace(string(r[:100]))
	}
	out.Sources, _ = json.Marshal([]map[string]string{{"id": hit.DocID, "title": hit.Title + " — " + hit.Heading, "url": hit.URL, "snippet": snippet}})
	out.Text = fmt.Sprintf("Об этом написано в документе «%s», раздел «%s»:\n\n%s\n\nНужно подробнее или по-другому? Напиши вопрос, отвечу сам.", hit.Title, hit.Heading, hit.Excerpt)
	out.ConversationID, err = s.conversation(ctx, userID, conversationID)
	if err != nil {
		return out, err
	}
	err = withTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO chat_messages (conversation_id, role, content) VALUES ($1, 'user', $2)`, out.ConversationID, hit.Question); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO chat_messages (conversation_id, role, content, sources, finish_reason, created_at)
			VALUES ($1, 'assistant', $2, $3, 'hint', clock_timestamp()) RETURNING id`, out.ConversationID, out.Text, []byte(out.Sources)).Scan(&out.MessageID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE chat_conversations SET updated_at = now() WHERE id = $1`, out.ConversationID)
		return err
	})
	return out, err
}

var kbDocID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// KBDoc returns one knowledge-base document (sections in order) for the reader in the chat.
// The demo KB holds no per-role documents, so every signed-in user may read it.
func (s *AssistantService) KBDoc(ctx context.Context, id string) (json.RawMessage, error) {
	if !kbDocID.MatchString(id) {
		return nil, repository.ErrNotFound
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.aiURL+"/v1/kb/docs/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.short.Do(req)
	if err != nil {
		return nil, ErrAIUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, repository.ErrNotFound
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || resp.StatusCode != http.StatusOK || !json.Valid(raw) {
		return nil, ErrAIUnavailable
	}
	return raw, nil
}

func (s *AssistantService) callAI(ctx context.Context, method, path string, payload any) (json.RawMessage, error) {
	var body io.Reader
	if payload != nil {
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.aiURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.short.Do(req)
	if err != nil {
		return nil, ErrAIUnavailable
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || resp.StatusCode != http.StatusOK || !json.Valid(raw) {
		return nil, ErrAIUnavailable
	}
	return raw, nil
}
