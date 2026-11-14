package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
)

var catalogScripts = []string{
	"catalog_common_backend.sql",
	"catalog_frontend.sql",
	"catalog_ml.sql",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("DB_NAME") == "" {
		return errors.New("DB_NAME must be set explicitly")
	}
	db, err := database.OpenPostgres()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, name := range catalogScripts {
		data, err := os.ReadFile(filepath.Join("scripts", name))
		if err != nil {
			return err
		}
		body, err := scriptBody(string(data))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, body); err != nil {
			return fmt.Errorf("initialize %s: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Println("catalog initialized")
	return nil
}

func scriptBody(script string) (string, error) {
	start := strings.Index(script, "\nBEGIN;")
	end := strings.LastIndex(script, "\nCOMMIT;")
	if start < 0 || end <= start {
		return "", errors.New("catalog script transaction markers missing")
	}
	return script[start+len("\nBEGIN;") : end], nil
}
