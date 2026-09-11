package local_source

import "github.com/gofiber/fiber/v2"

func (h *Handler) SetupRoutes() *fiber.App {
	router := fiber.New()
	router.Post("/", h.Create)
	router.Get("/", h.List)
	router.Delete("/:id", h.Delete)
	return router
}
