package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func TestPublishCredentialsExclusiveAndComplete(t *testing.T) {
	path := filepath.Join(os.TempDir(), "ai-buddy-credentials-"+uuid.NewString()+".json")
	t.Cleanup(func() { _ = os.Remove(path) })
	records := []service.InitialCredential{{CorporateID: "test-1", Email: "test@example.invalid", Password: uuid.NewString()}}
	if err := publishCredentials(path, records); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []service.InitialCredential
	if err := json.Unmarshal(data, &decoded); err != nil || len(decoded) != 1 {
		t.Fatal("credentials file is incomplete")
	}
	if err := publishCredentials(path, records); err == nil {
		t.Fatal("existing credentials file was replaceable")
	}
	again, err := os.ReadFile(path)
	if err != nil || string(again) != string(data) {
		t.Fatal("existing credentials file changed")
	}
	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".credentials-*.tmp")); err != nil || len(matches) != 0 {
		t.Fatal("temporary credentials file was left behind")
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatal("credentials file permissions are too broad")
		}
	}
}

func TestPrivateCredentialsPathRejectsRepository(t *testing.T) {
	repo, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	secretsDir := filepath.Join(repo, "secrets")
	_, statErr := os.Stat(secretsDir)
	createdSecretsDir := errors.Is(statErr, os.ErrNotExist)
	nestedDir := filepath.Join(secretsDir, "nested")
	_, nestedStatErr := os.Stat(nestedDir)
	createdNestedDir := errors.Is(nestedStatErr, os.ErrNotExist)
	t.Cleanup(func() {
		if createdNestedDir {
			_ = os.Remove(nestedDir)
		}
		if createdSecretsDir {
			_ = os.Remove(secretsDir)
		}
	})
	if _, err := privateCredentialsPath(filepath.Join(repo, "credential-test.json"), repo); err == nil {
		t.Fatal("accepted credentials path inside repository")
	}
	if _, err := privateCredentialsPath("relative-credentials.json", repo); err == nil {
		t.Fatal("accepted relative credentials path")
	}
	if _, err := privateCredentialsPath(filepath.Join(repo, "secrets", "hr-test-allowed.json"), repo); err != nil {
		t.Fatalf("rejected ignored secrets directory: %v", err)
	}
	if _, err := privateCredentialsPath(filepath.Join(nestedDir, "hr-test-rejected.json"), repo); err == nil {
		t.Fatal("accepted nested repository directory")
	}
}
