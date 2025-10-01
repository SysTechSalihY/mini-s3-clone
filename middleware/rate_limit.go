package middleware

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/context"
)

func RateLimit(client *redis.Client, limit int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := context.Background()
		identifier := c.IP()

		key := fmt.Sprintf("rate_limit:%s", identifier)
		count, err := client.Incr(ctx, key).Result()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "rate limit error"})
		}

		if count == 1 {
			client.Expire(ctx, key, window)
		}

		if count > int64(limit) {
			ttl, _ := client.TTL(ctx, key).Result()
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       "rate limit exceeded",
				"retry_after": int(ttl.Seconds()),
			})
		}

		return c.Next()
	}
}
