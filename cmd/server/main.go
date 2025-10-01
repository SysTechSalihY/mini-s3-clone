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
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Load env
	if err := godotenv.Load(".env"); err != nil {
		log.Warn("No .env file found, using environment variables")
	} else {
		log.Info(".env file loaded")
	}

	// Logger setup
	log.SetFormatter(&log.JSONFormatter{TimestampFormat: time.RFC3339})
	log.SetLevel(log.DebugLevel)
	log.Info("Logger initialized")

	// Fiber app
	app := fiber.New()
	log.Info("Fiber app initialized")

	// DB connection
	log.Info("Connecting to database...")
	if err := db.ConnectDb(); err != nil {
		log.Fatal("Failed to connect to database:", err)
	} else {
		log.Info("Database connected successfully")
	}

	// AWS SES client
	log.Info("Loading AWS config...")
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(os.Getenv("AWS_REGION")))
	if err != nil {
		log.Fatal("Failed to load AWS config:", err)
	}
	sesClient := sesv2.NewFromConfig(cfg)
	log.Info("AWS SES client initialized")

	// Redis + Asynq
	redisAddr := os.Getenv("REDIS_ADDR")
	log.WithField("redis_addr", redisAddr).Info("Connecting to Redis...")
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	if _, err := redisClient.Ping(context.Background()).Result(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	} else {
		log.Info("Redis connected successfully")
	}

	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer redisClient.Close()
	defer asynqClient.Close()
	log.Info("Asynq client initialized")

	// Rate limiter middleware
	app.Use(middleware.RateLimit(redisClient, 20, time.Minute))
	log.Info("RateLimit middleware added")

	// Public routes
	log.Info("Registering public routes...")
	app.Post("/api/auth/signup", handlers.SignUp(db.DB))
	app.Get("/api/auth/verify-email", handlers.VerifyEmail(db.DB))
	app.Post("/api/auth/secret-key", handlers.CreateSecretKey(db.DB))
	app.Post("/api/presigned/upload", middleware.ValidatePresignedURL(db.DB), handlers.UploadFilePresignedURL(db.DB))
	app.Get("/api/presigned/download", middleware.ValidatePresignedURL(db.DB), handlers.DownloadFilePresignedURL(db.DB))
	log.Info("Public routes registered")

	// Auth middleware
	log.Info("Registering auth middleware...")
	app.Use(middleware.AuthMiddleware(db.DB))
	log.Info("Auth middleware registered")

	// Authenticated routes
	log.Info("Registering authenticated routes...")
	app.Get("/api/auth/verification-link",
		handlers.CreateVerificationLink(db.DB, sesClient, os.Getenv("AWS_EMAIL"), os.Getenv("APP_URL")))

	app.Post("/api/buckets", handlers.CreateBucket(db.DB))
	app.Get("/api/buckets", handlers.ListBuckets(db.DB))
	app.Get("/api/buckets/:bucketName", handlers.GetBucketInfo(db.DB))
	app.Delete("/api/buckets/:bucketName", handlers.DeleteBucket(db.DB))

	// Presigned URL generation routes (bucket owner only)
	app.Post("/api/presigned/url/download", handlers.CreateDownloadPresignedURL(db.DB))
	app.Post("/api/presigned/url/upload", handlers.CreateUploadPresignedURL(db.DB))

	app.Post("/api/buckets/:bucketName/files/:fileName", handlers.UploadFile(db.DB))
	app.Get("/api/buckets/:bucketName/files/:fileName", handlers.DownloadFile(db.DB))
	app.Delete("/api/buckets/:bucketName/files/:fileName", handlers.DeleteFile(db.DB))

	app.Post("/api/tasks/empty-bucket/:bucketName", handlers.EnqueueEmptyBucketTask(asynqClient, db.DB))
	app.Post("/api/tasks/copy-bucket/:bucketSrc/:bucketDest", handlers.EnqueueCopyBucketTask(asynqClient, db.DB))
	app.Get("/api/tasks/:taskID", handlers.GetTaskProgress(db.DB))
	log.Info("Authenticated routes registered")

	// Start server
	port := ":8080"
	log.WithField("port", port).Info("Starting server...")
	if err := app.Listen(port); err != nil {
		log.Fatal("Server failed:", err)
	}
}
