package service

import (
	"context"
	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeHintStore struct {
	due      *repository.DueHint
	text     string
	success  bool
	attempts int
}

func (f *fakeHintStore) EnqueueDue(context.Context) error { return nil }
func (f *fakeHintStore) Claim(context.Context) (*repository.DueHint, error) {
	if f.due == nil {
		return nil, nil
	}
	x := f.due
	f.due = nil
	return x, nil
}
func (f *fakeHintStore) Resolve(_ context.Context, _ uuid.UUID, text string, success bool) error {
	f.text = text
	f.success = success
	f.attempts++
	return nil
}
func (f *fakeHintStore) List(context.Context, uuid.UUID) ([]repository.Hint, error) { return nil, nil }
func (f *fakeHintStore) GetByItem(context.Context, uuid.UUID, uuid.UUID) (*repository.Hint, error) {
	return nil, nil
}
func (f *fakeHintStore) MarkRead(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func TestHintTickAIAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response int
		want     string
		success  bool
	}{{"ai", 200, "Try a small step", true}, {"fallback", 503, fallbackHint, false}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/hint" || r.Method != "POST" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tc.response)
				if tc.response == 200 {
					_, _ = w.Write([]byte(`{"hint":"Try a small step"}`))
				}
			}))
			defer server.Close()
			fake := &fakeHintStore{due: &repository.DueHint{ID: uuid.New(), Title: "Task", Minutes: 30}}
			if err := NewHintService(fake, server.URL).Tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			if fake.text != tc.want || fake.success != tc.success || fake.attempts != 1 {
				t.Fatalf("got %+v", fake)
			}
		})
	}
}

func TestHintTickUnavailableAI(t *testing.T) {
	fake := &fakeHintStore{due: &repository.DueHint{ID: uuid.New(), Title: "Task", Minutes: 30}}
	if err := NewHintService(fake, "http://127.0.0.1:0").Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.text != fallbackHint || fake.success || fake.attempts != 1 {
		t.Fatalf("unexpected unavailable AI result: %+v", fake)
	}
}
