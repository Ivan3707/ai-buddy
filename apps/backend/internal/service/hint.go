package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const fallbackHint = "Задание заняло больше запланированного времени. Разбейте его на небольшие шаги и обсудите затруднение со своим buddy."

type HintService struct {
	repo   hintStore
	client *http.Client
	url    string
}

type hintStore interface {
	EnqueueDue(context.Context) error
	Claim(context.Context) (*repository.DueHint, error)
	Resolve(context.Context, uuid.UUID, string, bool) error
	List(context.Context, uuid.UUID) ([]repository.Hint, error)
	GetByItem(context.Context, uuid.UUID, uuid.UUID) (*repository.Hint, error)
	MarkRead(context.Context, uuid.UUID, uuid.UUID) error
}

func NewHintService(repo hintStore, aiURL string) *HintService {
	return &HintService{repo: repo, client: &http.Client{Timeout: 8 * time.Second}, url: aiURL}
}
func (s *HintService) Tick(ctx context.Context) error {
	if err := s.repo.EnqueueDue(ctx); err != nil {
		return err
	}
	for i := 0; i < 10; i++ {
		due, err := s.repo.Claim(ctx)
		if err != nil {
			return err
		}
		if due == nil {
			return nil
		}
		body, _ := json.Marshal(map[string]any{"title": due.Title, "description": due.Description, "expected_result": due.Expected, "estimated_minutes": due.Minutes})
		req, err := http.NewRequestWithContext(ctx, "POST", s.url+"/hint", bytes.NewReader(body))
		text := ""
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			var response *http.Response
			response, err = s.client.Do(req)
			if err == nil {
				limited := io.LimitReader(response.Body, 8192)
				if response.StatusCode == 200 {
					var value struct {
						Hint string `json:"hint"`
					}
					if json.NewDecoder(limited).Decode(&value) == nil {
						text = value.Hint
					}
				}
				response.Body.Close()
			}
		}
		text = strings.TrimSpace(text)
		err = s.repo.Resolve(ctx, due.ID, chooseHint(text), text != "")
		if err != nil {
			return err
		}
	}
	return nil
}
func chooseHint(text string) string {
	if text == "" {
		return fallbackHint
	}
	return text
}
func (s *HintService) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("hint worker: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *HintService) List(ctx context.Context, userID uuid.UUID) ([]repository.Hint, error) {
	return s.repo.List(ctx, userID)
}
func (s *HintService) Get(ctx context.Context, userID, itemID uuid.UUID) (*repository.Hint, error) {
	return s.repo.GetByItem(ctx, userID, itemID)
}
func (s *HintService) Read(ctx context.Context, userID, hintID uuid.UUID) error {
	return s.repo.MarkRead(ctx, userID, hintID)
}
