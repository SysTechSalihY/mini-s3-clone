package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/SysTechSalihY/mini-s3-clone/auth"
	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/aws/aws-sdk-go-v2/aws"
	sesv2 "github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"gorm.io/gorm"
)

type SignUpRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SignUpResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
}

type CreateAccessRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func SignUp(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req SignUpRequest
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to hash password"})
		}

		accessKey, secretKey, err := auth.GenerateKeys()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to generate keys"})
		}

		user := db.User{
			ID:           uuid.New().String(),
			Email:        req.Email,
			SecretKey:    secretKey,
			AccessKey:    accessKey,
			UserRole:     "user",
			PasswordHash: string(hashedPassword),
		}
		if err := DB.Create(&user).Error; err != nil {
			if strings.Contains(err.Error(), "Duplicate entry") {
				return c.Status(400).JSON(fiber.Map{"error": "email already exists"})
			}
			return c.Status(500).JSON(fiber.Map{"error": "failed to create user"})
		}

		resp := SignUpResponse{
			ID:        user.ID,
			Email:     user.Email,
			AccessKey: accessKey,
			SecretKey: secretKey,
		}
		return c.Status(201).JSON(resp)
	}
}

func CreateVerificationLink(DB *gorm.DB, sesClient *sesv2.Client, senderEmail string, appUrl string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var user *db.User
		user, ok := c.Locals("user").(*db.User)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"error": "User does not exist"})
		}
		tokenBytes := make([]byte, 32)
		_, err := rand.Read(tokenBytes)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to generate token"})
		}
		token := hex.EncodeToString(tokenBytes)
		verification := db.EmailVerification{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     token,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		if err := DB.Create(&verification).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to save verification token"})
		}
		link := appUrl + "/verify-email?token=" + token
		input := &sesv2.SendEmailInput{
			FromEmailAddress: aws.String(senderEmail),
			ReplyToAddresses: []string{},
			Destination: &types.Destination{
				ToAddresses: []string{user.Email},
			},
			Content: &types.EmailContent{
				Simple: &types.Message{
					Subject: &types.Content{
						Data: aws.String("Verify your email for AwesomeApp"),
					},
					Body: &types.Body{
						Html: &types.Content{
							Data: aws.String(fmt.Sprintf(`
                        <html>
                        <body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
                            <h2 style="color: #4CAF50;">Welcome to AwesomeApp!</h2>
                            <p>Hi there,</p>
                            <p>Thanks for signing up. Please verify your email by clicking the button below:</p>
                            <a href="%s" style="display:inline-block; padding:10px 20px; margin:20px 0; 
                               background-color:#4CAF50; color:white; text-decoration:none; border-radius:5px;">
                               Verify Email
                            </a>
                            <p>If you did not sign up, you can ignore this email.</p>
                            <hr/>
                            <p style="font-size:12px; color:#888;">AwesomeApp Inc.</p>
                        </body>
                        </html>
                    `, link)),
						},
					},
				},
			},
		}
		if _, err := sesClient.SendEmail(context.Background(), input); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to send verification email"})
		}

		return c.Status(201).JSON(fiber.Map{"message": "verification email sent"})
	}
}

func VerifyEmail(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Query("token")
		if token == "" {
			return c.Status(400).JSON(fiber.Map{"error": "missing token"})
		}
		var verification db.EmailVerification
		if err := DB.Where("token = ?", token).First(&verification).Error; err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid or expired token"})
		}

		if time.Now().After(verification.ExpiresAt) {
			return c.Status(400).JSON(fiber.Map{"error": "token expired"})
		}
		if err := DB.Model(&db.User{}).Where("id = ?", verification.UserID).Update("is_verified", true).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to verify user"})
		}
		DB.Delete(&verification)

		return c.Status(200).JSON(fiber.Map{"message": "email verified successfully"})
	}
}

func CreateSecretKey(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req CreateAccessRequest
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
		}

		var user db.User
		if err := DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return c.Status(404).JSON(fiber.Map{"error": "user not found"})
			}
			return c.Status(500).JSON(fiber.Map{"error": "database error"})
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "invalid credentials"})
		}

		accessKey, secretKey, err := auth.GenerateKeys()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to generate keys"})
		}

		user.AccessKey = accessKey
		user.SecretKey = secretKey
		if err := DB.Save(&user).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to update keys"})
		}

		return c.Status(200).JSON(fiber.Map{
			"access_key": accessKey,
			"secret_key": secretKey,
		})
	}
}
