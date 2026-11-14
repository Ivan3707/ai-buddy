package handler

import (
	"testing"
	"time"
)

func TestFullMonths(t *testing.T) {
	from := time.Date(2000, 12, 15, 0, 0, 0, 0, time.UTC)
	if got := fullMonths(from, time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)); got != 310 {
		t.Fatalf("before anniversary: %d", got)
	}
	if got := fullMonths(from, time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)); got != 312 {
		t.Fatalf("on anniversary: %d", got)
	}
}
