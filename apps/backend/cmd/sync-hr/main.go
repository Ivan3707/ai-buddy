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
	demoPassword := flag.String("demo-password", os.Getenv("DEMO_PASSWORD"), "shared password for new demo accounts (env DEMO_PASSWORD); synthetic data only")
	flag.Parse()
	if *credentialsPath == "" && *demoPassword == "" {
		return fmt.Errorf("-credentials is required unless -demo-password (DEMO_PASSWORD) is set; use a private path outside the repository")
	}
	if os.Getenv("DB_NAME") == "" {
		return fmt.Errorf("DB_NAME must be set explicitly")
	}
	credentialFile := ""
	if *credentialsPath != "" {
		repoRoot, err := findRepositoryRoot()
		if err != nil {
			return err
		}
		if credentialFile, err = privateCredentialsPath(*credentialsPath, repoRoot); err != nil {
			return err
		}
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
	synchronizer := service.NewHRSynchronizer(db)
	if *demoPassword != "" {
		if synchronizer, err = synchronizer.WithDemoPassword(*demoPassword); err != nil {
			return err
		}
	}
	newAccounts, err := synchronizer.Sync(ctx, employees, func(credentials []service.InitialCredential) error {
		return publish(credentialFile, credentials)
	})
	if err != nil {
		return err
	}
	if credentialFile == "" {
		fmt.Printf("synchronized %d HR records; %d new accounts use the demo password\n", len(employees), newAccounts)
	} else {
		fmt.Printf("synchronized %d HR records; credentials for %d new accounts saved\n", len(employees), newAccounts)
	}
	return nil
}

// publish stores new credentials in a private file. With a demo password and no file, only the e-mails of
// new accounts are printed: the shared password is already known to the operator.
func publish(path string, credentials []service.InitialCredential) error {
	if path != "" {
		return publishCredentials(path, credentials)
	}
	for _, c := range credentials {
		fmt.Printf("new demo account: %s\n", c.Email)
	}
	return nil
}
