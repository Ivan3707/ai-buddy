package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

func TestHintWorkerRetryAndAttemptLimit(t *testing.T) {
	for _, tc := range []struct {
		name      string
		recoverAt int32
		wantText  string
		wantTry   int
	}{
		{"external service recovers", 2, "Recovered hint", 2},
		{"fallback after third failure", 0, fallbackHint, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, ctx := openChecklistTestDB(t)
			fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 1)
			intern := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
			item := fixture.profItems[0].ID
			if _, err := db.ExecContext(ctx, `UPDATE checklist_items SET estimated_minutes=1 WHERE id=$1`, item); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `UPDATE user_checklist_progress SET activated_at=now()-interval '2 minutes' WHERE user_id=$1 AND checklist_item_id=$2`, intern.ID, item); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/hint" {
					t.Errorf("unexpected hint request: %s %s", r.Method, r.URL.Path)
				}
				attempt := calls.Add(1)
				if tc.recoverAt > 0 && attempt >= tc.recoverAt {
					_, _ = w.Write([]byte(`{"hint":"Recovered hint"}`))
					return
				}
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()
			repo := repository.NewHintRepository(db)
			worker := NewHintService(repo, server.URL)
			for attempt := 1; attempt <= tc.wantTry; attempt++ {
				if err := worker.Tick(ctx); err != nil {
					t.Fatal(err)
				}
				var status string
				var storedAttempts int
				if err := db.QueryRowContext(ctx, `SELECT h.status,h.attempts FROM task_hints h JOIN user_checklist_progress p ON p.id=h.progress_id WHERE p.user_id=$1 AND p.checklist_item_id=$2`, intern.ID, item).Scan(&status, &storedAttempts); err != nil {
					t.Fatal(err)
				}
				if storedAttempts != attempt {
					t.Fatalf("attempt count=%d want=%d", storedAttempts, attempt)
				}
				if attempt < tc.wantTry {
					if status != "pending" {
						t.Fatalf("retry status=%s", status)
					}
					if err := worker.Tick(ctx); err != nil {
						t.Fatal(err)
					}
					if calls.Load() != int32(attempt) {
						t.Fatal("worker retried before next_attempt_at")
					}
					if _, err := db.ExecContext(ctx, `UPDATE task_hints h SET next_attempt_at=now()-interval '1 second' FROM user_checklist_progress p WHERE p.id=h.progress_id AND p.user_id=$1 AND p.checklist_item_id=$2`, intern.ID, item); err != nil {
						t.Fatal(err)
					}
				} else if status != "ready" {
					t.Fatalf("final status=%s", status)
				}
			}
			if err := worker.Tick(ctx); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != int32(tc.wantTry) {
				t.Fatalf("unexpected extra AI calls: %d", calls.Load())
			}
			hints, err := repo.List(ctx, intern.ID)
			if err != nil || len(hints) != 1 || hints[0].Text != tc.wantText {
				t.Fatalf("final hints: %+v err=%v", hints, err)
			}
		})
	}
}

func TestHintWorkerExternalTimeoutUsesFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	fake := &fakeHintStore{due: &repository.DueHint{Title: "Task", Minutes: 30}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := NewHintService(fake, server.URL).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.attempts != 1 || fake.success || fake.text != fallbackHint {
		t.Fatalf("timeout result: %+v", fake)
	}
}

func TestHintWorkersConcurrentClaimOnce(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 1)
	intern := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	item := fixture.profItems[0].ID
	if _, err := db.ExecContext(ctx, `UPDATE checklist_items SET estimated_minutes=1 WHERE id=$1`, item); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE user_checklist_progress SET activated_at=now()-interval '2 minutes' WHERE user_id=$1 AND checklist_item_id=$2`, intern.ID, item); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"hint":"One hint"}`))
	}))
	defer server.Close()
	worker := NewHintService(repository.NewHintRepository(db), server.URL)
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; results <- worker.Tick(ctx) }()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("concurrent hint worker: %v", err)
		}
	}
	var count, attempts int
	var status string
	if err := db.QueryRowContext(ctx, `SELECT count(*),max(h.attempts),max(h.status) FROM task_hints h JOIN user_checklist_progress p ON p.id=h.progress_id WHERE p.user_id=$1 AND p.checklist_item_id=$2`, intern.ID, item).Scan(&count, &attempts, &status); err != nil || count != 1 || attempts != 1 || status != "ready" || calls.Load() != 1 {
		t.Fatalf("hint claim result: rows=%d attempts=%d status=%s calls=%d err=%v", count, attempts, status, calls.Load(), err)
	}
}
