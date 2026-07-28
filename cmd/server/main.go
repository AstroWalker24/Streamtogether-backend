package main

import (
	"log"

	"github.com/AstroWalker24/Streamtogether-backend/internal/app"
)

func main() {
	a, err := app.New()
	if err != nil {
		log.Fatalf("failed to initialize application: %v", err)
	}

	if err := a.Run(); err != nil {
		log.Fatalf("application error: %v", err)
	}
}
