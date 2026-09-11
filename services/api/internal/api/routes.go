package api

import (
	"github.com/gofiber/fiber/v2"

	"github.com/berzz26/recall/services/api/internal/api/handlers"
)

func RegisterRoutes(app *fiber.App) {
	app.Get("/health", handlers.Health)

	v1 := app.Group("/api/v1")
	_ = v1
}
