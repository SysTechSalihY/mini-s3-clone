package handlers

import (
	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func GetTaskProgress(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		taskID := c.Params("taskID")
		user, ok := c.Locals("user").(*db.User)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthenticated"})
		}
		var task db.Task
		if err := DB.First(&task, "id = ?", taskID).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "task not found"})
		}
		if task.UserID != user.ID {
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}
		return c.JSON(fiber.Map{
			"status":   task.Status,
			"progress": task.Progress,
			"message":  task.Message,
		})
	}
}
