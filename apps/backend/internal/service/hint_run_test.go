package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type failingHintRunStore struct {
	*fakeHintStore
	called chan struct{}
}

func (s *failingHintRunStore) EnqueueDue(context.Context) error {
	close(s.called)
	return errors.New("test enqueue failure")
}

func TestHintWorkerRunStopsAfterStoreFailureAndCancellation(t *testing.T) {
	store := &failingHintRunStore{fakeHintStore: &fakeHintStore{}, called: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { NewHintService(store, "http://unused.invalid").Run(ctx); close(done) }()
	select {
	case <-store.called:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not poll the store")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
}
