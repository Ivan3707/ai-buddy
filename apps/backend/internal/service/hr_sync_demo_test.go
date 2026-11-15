package service

import (
	"strings"
	"testing"
)

func TestWithDemoPasswordRejectsShortPassword(t *testing.T) {
	if _, err := NewHRSynchronizer(nil).WithDemoPassword("short"); err == nil || !strings.Contains(err.Error(), "at least") {
		t.Fatalf("short demo password accepted: %v", err)
	}
}

func TestWithDemoPasswordStoresPassword(t *testing.T) {
	s, err := NewHRSynchronizer(nil).WithDemoPassword("long-enough-password")
	if err != nil {
		t.Fatal(err)
	}
	if s.demoPassword != "long-enough-password" {
		t.Fatalf("demo password not stored")
	}
}
