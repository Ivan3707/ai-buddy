package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	snapshot := flag.String("snapshot", "../../data/corporate_employees.json", "HR snapshot JSON")
	credentialsPath := flag.String("credentials", "", "new account credential file (must not exist)")
	flag.Parse()
	if *credentialsPath == "" {
		return fmt.Errorf("-credentials is required; use a private path outside the repository")
	}
	if os.Getenv("DB_NAME") == "" {
		return fmt.Errorf("DB_NAME must be set explicitly")
	}
	if !filepath.IsAbs(*credentialsPath) {
		return fmt.Errorf("-credentials must be an absolute path")
	}
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(repoRoot, *credentialsPath)
	if err != nil {
		return err
	}
	if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
		return fmt.Errorf("credential file must be outside the repository")
	}
	data, err := os.ReadFile(*snapshot)
	if err != nil {
		return err
	}
	var employees []service.CorporateEmployee
	if err = json.Unmarshal(data, &employees); err != nil {
		return err
	}
	file, err := os.OpenFile(*credentialsPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	db, err := database.OpenPostgres()
	if err != nil {
		os.Remove(*credentialsPath)
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	credentials, err := service.NewHRSynchronizer(db).Sync(ctx, employees)
	if err != nil {
		file.Close()
		os.Remove(*credentialsPath)
		return err
	}
	if err = json.NewEncoder(file).Encode(credentials); err != nil {
		return fmt.Errorf("credentials file write failed after sync commit: %w", err)
	}
	if err = file.Sync(); err != nil {
		return err
	}
	fmt.Printf("synchronized %d HR records; credentials for %d new accounts saved\n", len(employees), len(credentials))
	return nil
}
