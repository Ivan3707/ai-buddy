package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
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
	repoRoot, err := findRepositoryRoot()
	if err != nil {
		return err
	}
	credentialFile, err := privateCredentialsPath(*credentialsPath, repoRoot)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*snapshot)
	if err != nil {
		return err
	}
	var employees []service.CorporateEmployee
	if err = json.Unmarshal(data, &employees); err != nil {
		return err
	}
	db, err := database.OpenPostgres()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	newAccounts, err := service.NewHRSynchronizer(db).Sync(ctx, employees, func(credentials []service.InitialCredential) error {
		return publishCredentials(credentialFile, credentials)
	})
	if err != nil {
		return err
	}
	fmt.Printf("synchronized %d HR records; credentials for %d new accounts saved\n", len(employees), newAccounts)
	return nil
}
