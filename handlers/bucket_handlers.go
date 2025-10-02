package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/SysTechSalihY/mini-s3-clone/tasks"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type CreateBucketRequest struct {
	BucketName string  `json:"bucketName"`
	Region     string  `json:"region"`
	ACL        *string `json:"acl,omitempty"`        // optional string
	Versioning *bool   `json:"versioning,omitempty"` // optional bool
	Quota      *int64  `json:"quota,omitempty"`      // optional int64
}

// fake regions
var allowedRegions = map[string]bool{
	"USA":   true,
	"TR":    true,
	"CHINA": true,
	"JP":    true,
}

func ListBuckets(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user, ok := c.Locals("user").(*db.User)
		if !ok {
			log.Error("ListBuckets: missing user in context")
			return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}

		var buckets []db.Bucket
		if err := DB.Where("user_id = ?", user.ID).Find(&buckets).Error; err != nil {
			log.WithError(err).WithField("user_id", user.ID).Error("Failed to fetch buckets")
			return c.Status(500).JSON(fiber.Map{"error": "failed to fetch buckets"})
		}

		log.WithField("user_id", user.ID).Info("Buckets listed successfully")
		return c.Status(200).JSON(fiber.Map{"buckets": buckets})
	}
}

func CreateBucketDir(bucketName string) error {
	dirPath := filepath.Join("/storage", bucketName)
	err := os.MkdirAll(dirPath, 0755)
	if err != nil {
		log.WithError(err).WithField("bucket", bucketName).Error("Failed to create bucket directory")
		return fmt.Errorf("failed to create bucket directory: %w", err)
	}

	log.WithField("bucket", bucketName).Info("Bucket directory created")
	return nil
}

func CreateBucket(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req CreateBucketRequest
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			log.WithError(err).Error("Invalid bucket creation request")
			return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
		}

		if err := validateBucketName(req.BucketName); err != nil {
			log.WithError(err).WithField("bucket", req.BucketName).Warn("Invalid bucket name")
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		if !allowedRegions[strings.ToUpper(req.Region)] {
			log.WithField("region", req.Region).Warn("Invalid region provided")
			return c.Status(400).JSON(fiber.Map{"error": "invalid region"})
		}

		user, ok := c.Locals("user").(*db.User)
		if !ok {
			log.Error("CreateBucket: missing user in context")
			return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}

		// Check if bucket already exists
		var existing db.Bucket
		if err := DB.Where("bucket_name = ?", req.BucketName).First(&existing).Error; err == nil {
			log.WithField("bucket", req.BucketName).Warn("Bucket name already exists")
			return c.Status(409).JSON(fiber.Map{"error": "bucket name already exists"})
		}

		newBucket := &db.Bucket{
			ID:         uuid.NewString(),
			BucketName: req.BucketName,
			UserID:     user.ID,
			Region:     strings.ToUpper(req.Region),
		}

		// Optional fields
		if req.ACL != nil {
			newBucket.ACL = req.ACL
		} else {
			defaultACL := "private"
			newBucket.ACL = &defaultACL
		}

		if req.Versioning != nil {
			newBucket.Versioning = *req.Versioning
		} else {
			newBucket.Versioning = false
		}

		if req.Quota != nil {
			newBucket.Quota = req.Quota
		} else {
			newBucket.Quota = nil
		}
		if err := CreateBucketDir(req.BucketName); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to create bucket"})
		}

		if err := DB.Create(newBucket).Error; err != nil {
			log.WithError(err).WithField("bucket", req.BucketName).Error("Failed to insert bucket into DB")
			return c.Status(500).JSON(fiber.Map{"error": "failed to create bucket"})
		}

		log.WithFields(log.Fields{
			"bucket":     req.BucketName,
			"userID":     user.ID,
			"ACL":        newBucket.ACL,
			"Versioning": newBucket.Versioning,
			"Quota":      newBucket.Quota,
		}).Info("Bucket created successfully")

		return c.Status(201).JSON(fiber.Map{
			"message": "bucket created successfully",
			"bucket":  newBucket,
		})
	}
}

func DeleteBucket(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		bucketName := c.Params("bucketName")
		if bucketName == "" {
			return c.Status(400).JSON(fiber.Map{"error": "bucket name is required"})
		}

		user, ok := c.Locals("user").(*db.User)
		if !ok {
			log.Error("DeleteBucket: missing user in context")
			return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}

		var bucket db.Bucket
		if err := DB.Where("bucket_name = ?", bucketName).First(&bucket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.WithField("bucket", bucketName).Warn("Bucket not found")
				return c.Status(404).JSON(fiber.Map{"error": "bucket not found"})
			}
			log.WithError(err).Error("Database error while fetching bucket")
			return c.Status(500).JSON(fiber.Map{"error": "database error"})
		}

		if bucket.UserID != user.ID {
			log.WithFields(log.Fields{
				"bucket":  bucketName,
				"user_id": user.ID,
			}).Warn("Unauthorized bucket delete attempt")
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}

		var fileCount int64
		DB.Model(&db.File{}).Where("bucket_id = ?", bucket.ID).Count(&fileCount)
		if fileCount > 0 {
			log.WithField("bucket", bucketName).Warn("Bucket not empty, cannot delete")
			return c.Status(400).JSON(fiber.Map{"error": "bucket is not empty"})
		}

		if err := DB.Delete(&bucket).Error; err != nil {
			log.WithError(err).WithField("bucket", bucketName).Error("Failed to delete bucket")
			return c.Status(500).JSON(fiber.Map{"error": "failed to delete bucket"})
		}

		log.WithField("bucket", bucketName).Info("Bucket deleted successfully")
		return c.Status(200).JSON(fiber.Map{"message": "bucket deleted successfully"})
	}
}

func GetBucketInfo(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		bucketName := c.Params("bucketName")
		if bucketName == "" {
			return c.Status(400).JSON(fiber.Map{"error": "bucket name is required"})
		}

		user, ok := c.Locals("user").(*db.User)
		if !ok {
			log.Error("GetBucketInfo: missing user in context")
			return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}

		var bucket db.Bucket
		if err := DB.Where("bucket_name = ?", bucketName).First(&bucket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.WithField("bucket", bucketName).Warn("Bucket not found")
				return c.Status(404).JSON(fiber.Map{"error": "bucket not found"})
			}
			log.WithError(err).Error("Database error while fetching bucket info")
			return c.Status(500).JSON(fiber.Map{"error": "database error"})
		}

		if bucket.UserID != user.ID {
			log.WithFields(log.Fields{
				"bucket":  bucketName,
				"user_id": user.ID,
			}).Warn("Unauthorized bucket info access attempt")
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}

		var totalSize int64
		if err := DB.Model(&db.File{}).
			Where("bucket_id = ?", bucket.ID).
			Select("COALESCE(SUM(size),0)").
			Scan(&totalSize).Error; err != nil {
			log.WithError(err).WithField("bucket", bucketName).Error("Failed to calculate bucket size")
			return c.Status(500).JSON(fiber.Map{"error": "failed to calculate bucket size"})
		}

		log.WithField("bucket", bucketName).Info("Bucket info retrieved successfully")
		return c.Status(200).JSON(fiber.Map{
			"data": fiber.Map{
				"id":          bucket.ID,
				"bucket_name": bucket.BucketName,
				"user_id":     bucket.UserID,
				"created_at":  bucket.CreatedAt,
				"updated_at":  bucket.UpdatedAt,
				"total_size":  totalSize,
			},
		})
	}
}

func EnqueueEmptyBucketTask(client *asynq.Client, DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user, ok := c.Locals("user").(*db.User)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthenticated"})
		}

		bucketName := c.Params("bucketName")
		if bucketName == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bucketName is required"})
		}

		var bucket db.Bucket
		if err := DB.Where("bucket_name = ? AND user_id = ?", bucketName, user.ID).First(&bucket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "bucket not found or you do not have permission"})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
		}

		payload, _ := json.Marshal(tasks.EmptyBucketPayload{
			UserID:     user.ID,
			BucketName: bucketName,
		})

		task := asynq.NewTask(tasks.TaskTypeEmptyBucket, payload)
		info, err := client.Enqueue(task, asynq.MaxRetry(5), asynq.Timeout(10*time.Minute))
		if err != nil {
			log.Println("Failed to enqueue task:", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to enqueue task"})
		}
		newTask := db.Task{
			ID:        info.ID,
			UserID:    user.ID,
			Type:      "empty",
			Status:    "running",
			BucketSrc: &bucketName,
			Progress:  0,
		}
		if err := DB.Create(&newTask).Error; err != nil {
			log.Println("Failed to save task record:", err)
		}

		return c.JSON(fiber.Map{"task_id": newTask.ID, "message": "task enqueued"})
	}
}

func EnqueueCopyBucketTask(client *asynq.Client, DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user := c.Locals("user").(*db.User)
		bucketSrc := c.Params("bucketSrc")
		bucketDest := c.Params("bucketDest")

		if bucketSrc == "" || bucketDest == "" {
			return c.Status(400).JSON(fiber.Map{"error": "source and destination bucket names are required"})
		}

		var srcBucket db.Bucket
		if err := DB.Where("bucket_name = ? AND user_id = ?", bucketSrc, user.ID).First(&srcBucket).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "source bucket not found or not owned by user"})
		}

		var destBucket db.Bucket
		if err := DB.Where("bucket_name = ? AND user_id = ?", bucketDest, user.ID).First(&destBucket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				destBucket = db.Bucket{
					ID:         uuid.NewString(),
					BucketName: bucketDest,
					UserID:     user.ID,
					ACL:        srcBucket.ACL,
					Versioning: srcBucket.Versioning,
					Region:     srcBucket.Region,
				}
				if err := DB.Create(&destBucket).Error; err != nil {
					return c.Status(500).JSON(fiber.Map{"error": "failed to create destination bucket"})
				}
			} else {
				return c.Status(500).JSON(fiber.Map{"error": "failed to check destination bucket"})
			}
		}

		payload, _ := json.Marshal(struct {
			UserID     string `json:"user_id"`
			BucketSrc  string `json:"bucket_src"`
			BucketDest string `json:"bucket_dest"`
		}{
			UserID:     user.ID,
			BucketSrc:  bucketSrc,
			BucketDest: bucketDest,
		})

		task := asynq.NewTask("copy_bucket", payload)

		info, err := client.Enqueue(task,
			asynq.MaxRetry(5),
			asynq.Timeout(30*time.Minute),
		)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to enqueue copy task"})
		}

		newTask := db.Task{
			ID:         info.ID,
			UserID:     user.ID,
			Type:       "copy",
			BucketSrc:  &bucketSrc,
			BucketDest: &bucketDest,
			Status:     "running",
			Progress:   0,
		}
		DB.Create(&newTask)

		return c.JSON(fiber.Map{
			"task_id": newTask.ID,
			"message": "copy bucket task enqueued",
		})
	}
}

func validateBucketName(name string) error {
	if len(name) < 3 || len(name) > 63 {
		return errors.New("bucket name must be between 3 and 63 characters")
	}
	if strings.Contains(name, " ") {
		return errors.New("bucket name cannot contain spaces")
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '.' && r != '-' {
			return errors.New("bucket name contains invalid characters")
		}
	}
	return nil
}
