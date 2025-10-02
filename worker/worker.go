package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Worker struct {
	DB *gorm.DB
}

func (w *Worker) HandleEmptyBucketTask(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		UserID     string
		BucketName string
	}
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		log.WithError(err).Error("Failed to unmarshal empty bucket task payload")
		return err
	}
	log.WithFields(log.Fields{
		"user_id":     payload.UserID,
		"bucket_name": payload.BucketName,
	}).Info("Starting empty bucket task")

	var bucket db.Bucket
	if err := w.DB.Where("bucket_name = ?", payload.BucketName).First(&bucket).Error; err != nil {
		log.WithError(err).WithField("bucket", payload.BucketName).Error("Bucket not found")
		return err
	}

	var files []db.File
	if err := w.DB.Where("bucket_id = ?", bucket.ID).Find(&files).Error; err != nil {
		log.WithError(err).Error("Failed to fetch files for bucket")
		return err
	}

	total := len(files)
	log.WithFields(log.Fields{
		"bucket":     bucket.BucketName,
		"file_count": total,
		"user_id":    payload.UserID,
	}).Info("Emptying bucket")

	for i, file := range files {
		path := fmt.Sprintf("./storage/%s/%s", bucket.BucketName, file.FileName)
		if err := os.Remove(path); err != nil {
			log.WithError(err).WithField("file", file.FileName).Warn("Failed to remove file from storage")
		} else {
			log.WithField("file", file.FileName).Info("Deleted file from storage")
		}

		if err := w.DB.Delete(&file).Error; err != nil {
			log.WithError(err).WithField("file", file.FileName).Warn("Failed to delete file record from DB")
		}

		progress := int(float64(i+1) / float64(total) * 100)
		w.DB.Model(&db.Task{}).Where("bucket_src = ? AND user_id = ?", bucket.BucketName, payload.UserID).
			Update("progress", progress)
		log.WithFields(log.Fields{
			"progress": progress,
			"bucket":   bucket.BucketName,
		}).Info("Progress updated")
	}

	w.DB.Model(&db.Task{}).Where("bucket_src = ? AND user_id = ?", bucket.BucketName, payload.UserID).
		Updates(map[string]interface{}{"status": "completed", "progress": 100})
	log.WithField("bucket", bucket.BucketName).Info("Empty bucket task completed successfully")
	return nil
}

func (w *Worker) HandleCopyBucketTask(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		UserID     string `json:"user_id"`
		BucketSrc  string `json:"bucket_src"`
		BucketDest string `json:"bucket_dest"`
	}
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		log.WithError(err).Error("Failed to unmarshal copy bucket task payload")
		return err
	}

	log.WithFields(log.Fields{
		"user_id":     payload.UserID,
		"bucket_src":  payload.BucketSrc,
		"bucket_dest": payload.BucketDest,
	}).Info("Starting copy bucket task")

	// Fetch user
	var user db.User
	if err := w.DB.Where("id = ?", payload.UserID).First(&user).Error; err != nil {
		log.WithError(err).Error("User not found for copy bucket task")
		return fmt.Errorf("user not found: %w", err)
	}

	// Fetch source bucket
	var srcBucket db.Bucket
	if err := w.DB.Where("bucket_name = ? AND user_id = ?", payload.BucketSrc, user.ID).First(&srcBucket).Error; err != nil {
		log.WithError(err).WithField("bucket", payload.BucketSrc).Error("Source bucket not found or not owned by user")
		return fmt.Errorf("source bucket not found or not owned by user: %w", err)
	}

	// Fetch or create destination bucket
	var destBucket db.Bucket
	if err := w.DB.Where("bucket_name = ? AND user_id = ?", payload.BucketDest, user.ID).First(&destBucket).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			destBucket = db.Bucket{
				ID:         uuid.NewString(),
				BucketName: payload.BucketDest,
				UserID:     user.ID,
				ACL:        srcBucket.ACL,
				Versioning: srcBucket.Versioning,
				Region:     srcBucket.Region,
			}
			if err := w.DB.Create(&destBucket).Error; err != nil {
				log.WithError(err).Error("Failed to create destination bucket")
				return fmt.Errorf("failed to create destination bucket: %w", err)
			}
			log.WithField("bucket", destBucket.BucketName).Info("Destination bucket created")
		} else {
			log.WithError(err).Error("Failed to fetch destination bucket")
			return fmt.Errorf("failed to fetch destination bucket: %w", err)
		}
	} else {
		log.WithField("bucket", destBucket.BucketName).Info("Destination bucket already exists")
	}

	// Ensure destination folder exists
	destDir := fmt.Sprintf("./storage/%s", destBucket.BucketName)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		log.WithError(err).Error("Failed to create destination folder")
		return fmt.Errorf("failed to create destination folder: %w", err)
	}

	// Fetch files from source bucket
	var files []db.File
	if err := w.DB.Where("bucket_id = ?", srcBucket.ID).Find(&files).Error; err != nil {
		log.WithError(err).Error("Failed to fetch files from source bucket")
		return fmt.Errorf("failed to fetch files from source bucket: %w", err)
	}

	total := len(files)
	log.WithFields(log.Fields{
		"bucket_src":  srcBucket.BucketName,
		"bucket_dest": destBucket.BucketName,
		"file_count":  total,
	}).Info("Copying files")

	for i, f := range files {
		srcPath := fmt.Sprintf("./storage/%s/%s", srcBucket.BucketName, f.FileName)
		destPath := filepath.Join(destDir, f.FileName)

		input, err := os.ReadFile(srcPath)
		if err != nil {
			log.WithError(err).WithField("file", f.FileName).Error("Failed to read source file")
			return fmt.Errorf("failed to read source file %s: %w", f.FileName, err)
		}
		if err := os.WriteFile(destPath, input, 0644); err != nil {
			log.WithError(err).WithField("file", f.FileName).Error("Failed to write destination file")
			return fmt.Errorf("failed to write destination file %s: %w", f.FileName, err)
		}
		log.WithField("file", f.FileName).Info("Copied file to destination bucket")

		newFile := db.File{
			ID:          uuid.NewString(),
			FileName:    f.FileName,
			BucketID:    destBucket.ID,
			Size:        f.Size,
			ContentType: f.ContentType,
			VersionID:   f.VersionID,
			IsLatest:    f.IsLatest,
		}
		if err := w.DB.Create(&newFile).Error; err != nil {
			log.WithError(err).WithField("file", f.FileName).Error("Failed to create DB record for copied file")
			return fmt.Errorf("failed to create file record in DB: %w", err)
		}

		// Update task progress
		progress := int(float64(i+1) / float64(total) * 100)
		w.DB.Model(&db.Task{}).
			Where("bucket_src = ? AND bucket_dest = ? AND user_id = ?", payload.BucketSrc, payload.BucketDest, payload.UserID).
			Update("progress", progress)

		log.WithFields(log.Fields{
			"progress": progress,
			"file":     f.FileName,
		}).Info("Updated copy progress")
	}

	// Mark task completed
	w.DB.Model(&db.Task{}).
		Where("bucket_src = ? AND bucket_dest = ? AND user_id = ?", payload.BucketSrc, payload.BucketDest, payload.UserID).
		Updates(map[string]interface{}{"status": "completed", "progress": 100})

	log.WithFields(log.Fields{
		"bucket_src":  payload.BucketSrc,
		"bucket_dest": payload.BucketDest,
		"user_id":     payload.UserID,
	}).Info("Copy bucket task completed successfully")

	return nil
}
