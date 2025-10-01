package middleware

import (
	"errors"
	"strconv"

	"github.com/SysTechSalihY/mini-s3-clone/auth"
	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/gofiber/fiber/v2"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func AuthMiddleware(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		log.WithFields(log.Fields{
			"method":       c.Method(),
			"original_url": c.OriginalURL(),
		}).Info("Incoming request for signature check")
		bucketName := c.Params("bucketName", "")
		var bucket db.Bucket
		if bucketName != "" {
			if err := DB.Where("bucket_name = ?", bucketName).First(&bucket).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				log.WithError(err).Error("DB error fetching bucket in AuthMiddleware")
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
			}
		}

		if bucket.ID != "" && bucket.ACL != nil && *bucket.ACL == "public-read" {
			c.Locals("user", nil)
			return c.Next()
		}

		accessKey := c.Get("X-Access-Key")
		signature := c.Get("X-Signature")
		expiresStr := c.Get("X-Expires")
		if accessKey == "" || signature == "" || expiresStr == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authentication headers"})
		}

		expires, err := strconv.ParseInt(expiresStr, 10, 64)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid expiration timestamp"})
		}

		if !auth.ValidateRequest(DB, accessKey, signature, c.Method(), c.OriginalURL(), expires) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired signature"})
		}

		user, err := auth.GetUserByAccessKey(DB, accessKey)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "user does not exist"})
		}
		if bucket.ID != "" && (bucket.ACL == nil || *bucket.ACL == "private") && user.ID != bucket.UserID {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}

		c.Locals("user", user)
		return c.Next()
	}
}
