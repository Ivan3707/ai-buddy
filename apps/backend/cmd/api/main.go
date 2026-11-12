package main

import (
	"log"

	"github.com/nvkmaksim/ai-buddy/backend/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
