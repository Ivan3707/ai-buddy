package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

func main() {
	password := os.Getenv("BOOTSTRAP_TEAMLEAD_PASSWORD")
	if password == "" {
		fmt.Println("password environment variable is empty")
		return
	}

	db, err := database.OpenPostgres()
	if err != nil {
		fmt.Println("database connection error:", err)
		return
	}
	defer db.Close()

	users := repository.NewUserRepository(db)

	user, err := users.GetByEmail(
		context.Background(),
		"admin@example.com",
	)
	if err != nil {
		fmt.Println("get user error:", err)
		return
	}

	valid, err := security.VerifyPassword(password, user.PasswordHash)
	if err != nil {
		fmt.Println("verification error:", err)
		return
	}

	fmt.Printf("User found: true\n")
	fmt.Printf("Hash length: %d\n", len(user.PasswordHash))
	fmt.Printf("Password matches: %t\n", valid)
}