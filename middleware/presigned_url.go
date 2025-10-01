package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func ValidatePresignedURL(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		bucket := c.Query("bucket")
		key := c.Query("key")
		sig := c.Query("sig")
		expiresStr := c.Query("expires")
		versionID := c.Query("versionID", "")

		if bucket == "" || key == "" || sig == "" || expiresStr == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "missing required query params"})
		}

		// Ensure HTTP method matches operation
		expectedOp := ""
		switch c.Method() {
		case http.MethodGet:
			expectedOp = "download"
		case http.MethodPost:
			expectedOp = "upload"
		default:
			return c.Status(http.StatusMethodNotAllowed).JSON(fiber.Map{"error": "unsupported HTTP method"})
		}

		expires, err := strconv.ParseInt(expiresStr, 10, 64)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid expiration"})
		}

		if time.Now().Unix() > expires {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "URL expired"})
		}

		// Fetch bucket
		var bucketData db.Bucket
		if err := DB.Where("bucket_name = ?", bucket).First(&bucketData).Error; err != nil {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "bucket not found"})
		}

		// Fetch user
		var user db.User
		if err := DB.Where("id = ?", bucketData.UserID).First(&user).Error; err != nil {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "user not found"})
		}

		// Construct HMAC message exactly like GeneratePresignedURL
		message := fmt.Sprintf("%s:%s:%s:%d:%s", bucket, key, expectedOp, expires, versionID)
		h := hmac.New(sha256.New, []byte(user.SecretKey))
		h.Write([]byte(message))
		expectedSig := base64.URLEncoding.EncodeToString(h.Sum(nil))

		if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "invalid signature"})
		}

		c.Locals("bucket", bucket)
		c.Locals("key", key)
		c.Locals("user", user)
		c.Locals("operation", expectedOp)
		c.Locals("versionID", versionID)

		return c.Next()
	}
}
