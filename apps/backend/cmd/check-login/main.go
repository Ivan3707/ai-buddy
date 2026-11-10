package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nvkmaksim/ai-buddy/backend/internal/database"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func main() {
	db, err := database.OpenPostgres()
	if err != nil {
		panic(err)
	}
	defer db.Close()

	jwtService, err := security.NewJWTService(
		os.Getenv("JWT_SECRET"),
		15*time.Minute,
	)
	if err != nil {
		panic(err)
	}

	users := repository.NewUserRepository(db)
	auth := service.NewAuthService(users, jwtService)

	result, err := auth.Login(
		context.Background(),
		"admin@example.com",
		os.Getenv("BOOTSTRAP_TEAMLEAD_PASSWORD"),
	)
	if err != nil {
		fmt.Println("Login failed:", err)
		return
	}

	fmt.Println("Login successful")
	fmt.Println("Token generated:", result.AccessToken != "")
	fmt.Println("User role:", result.User.Role)
}