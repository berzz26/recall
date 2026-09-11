package video

import "github.com/gofiber/fiber/v2"

func (h *Handler) SetupRoutes() *fiber.App {
	router := fiber.New()

	router.Post("/", h.Create)
	router.Get("/", h.List)
	router.Get("/:id", h.Get)
	router.Delete("/:id", h.Delete)

	return router
}
