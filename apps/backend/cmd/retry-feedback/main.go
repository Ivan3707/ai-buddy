package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func main() {
	if os.Getenv("DB_NAME") == "" {
		fmt.Fprintln(os.Stderr, "DB_NAME must be set explicitly")
		os.Exit(1)
	}
	db, err := database.OpenPostgres()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sender := service.NewFeedbackService(db, service.MockAnalytics{DB: db, Unavailable: os.Getenv("ANALYTICS_MOCK_UNAVAILABLE") == "1"})
	sent, err := sender.Retry(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("delivered %d feedback records\n", sent)
}
