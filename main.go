package main

import (
	"context"
	"os"
	"time"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/SysTechSalihY/mini-s3-clone/handlers"
	"github.com/SysTechSalihY/mini-s3-clone/middleware"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/gofiber/fiber/v2"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Logger setup
	log.SetFormatter(&log.JSONFormatter{TimestampFormat: time.RFC3339})
	log.SetLevel(log.DebugLevel)

	// Fiber app
	app := fiber.New()

	// DB connection
	if err := db.ConnectDb(); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// AWS SES client
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(os.Getenv("AWS_REGION")))
	if err != nil {
		log.Fatal("Failed to load AWS config:", err)
	}
	sesClient := sesv2.NewFromConfig(cfg)

	// Asynq client
	redisAddr := os.Getenv("REDIS_ADDR")
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	defer redisClient.Close()
	defer asynqClient.Close()
	app.Use(middleware.RateLimit(redisClient, 20, time.Minute))
	// Public routes (no auth)
	app.Post("/api/auth/signup", handlers.SignUp(db.DB))
	app.Get("/api/auth/verify-email", handlers.VerifyEmail(db.DB))
	app.Post("/api/auth/secret-key", handlers.CreateSecretKey(db.DB))

	// Presigned URL file routes
	app.Post("/api/presigned/upload", handlers.UploadFilePresignedURL(db.DB))
	app.Get("/api/presigned/download", handlers.DownloadFilePresignedURL(db.DB))

	// Middleware for auth + optional presigned URL validation
	app.Use(middleware.AuthMiddleware(db.DB))
	app.Use(middleware.ValidatePresignedURL(db.DB))

	// Authenticated routes
	app.Get("/api/auth/verification-link",
		handlers.CreateVerificationLink(db.DB, sesClient, os.Getenv("AWS_EMAIL"), os.Getenv("APP_URL")))

	// Bucket routes
	app.Post("/api/buckets", handlers.CreateBucket(db.DB))
	app.Get("/api/buckets", handlers.ListBuckets(db.DB))
	app.Get("/api/buckets/:bucketName", handlers.GetBucketInfo(db.DB))
	app.Delete("/api/buckets/:bucketName", handlers.DeleteBucket(db.DB))

	// File routes
	app.Post("/api/buckets/:bucketName/files/:fileName", handlers.UploadFile(db.DB))
	app.Get("/api/buckets/:bucketName/files/:fileName", handlers.DownloadFile(db.DB))
	app.Delete("/api/buckets/:bucketName/files/:fileName", handlers.DeleteFile(db.DB))

	// Async tasks routes
	app.Post("/api/tasks/empty-bucket/:bucketName", handlers.EnqueueEmptyBucketTask(asynqClient, db.DB))
	app.Post("/api/tasks/copy-bucket/:bucketSrc/:bucketDest", handlers.EnqueueCopyBucketTask(asynqClient, db.DB))
	app.Get("/api/tasks/:taskID", handlers.GetTaskProgress(db.DB))

	// Start server
	log.Fatal(app.Listen(":3000"))
}
