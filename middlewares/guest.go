package middlewares

import (
	"github.com/gofiber/fiber/v2"
	"github.com/umono-cms/umono/core"
)

func Guest() func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		tokenStr := c.Cookies("token")

		ju := &core.JWTUser{}

		ju.Token = tokenStr
		err := ju.Resolve()

		if err == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "already_logged_id",
			})
		}

		return c.Next()
	}
}
