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
		return err
	}
	var bucket db.Bucket
	if err := w.DB.Where("bucket_name = ?", payload.BucketName).First(&bucket).Error; err != nil {
		return err
	}
	var files []db.File
	if err := w.DB.Where("bucket_id = ?", bucket.ID).Find(&files).Error; err != nil {
		return err
	}
	total := len(files)
	for i, file := range files {
		path := fmt.Sprintf("./storage/%s/%s", bucket.BucketName, file.FileName)
		_ = os.Remove(path)
		w.DB.Delete(&file)
		progress := int(float64(i+1) / float64(total) * 100)
		w.DB.Model(&db.Task{}).Where("bucket_src = ? AND user_id = ?", bucket.BucketName, payload.UserID).
			Update("progress", progress)
	}
	w.DB.Model(&db.Task{}).Where("bucket_src = ? AND user_id = ?", bucket.BucketName, payload.UserID).
		Updates(map[string]interface{}{"status": "completed", "progress": 100})

	return nil
}

func (w *Worker) HandleCopyBucketTask(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		UserID     string `json:"user_id"`
		BucketSrc  string `json:"bucket_src"`
		BucketDest string `json:"bucket_dest"`
	}
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return err
	}

	// Fetch user
	var user db.User
	if err := w.DB.Where("id = ?", payload.UserID).First(&user).Error; err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Fetch source bucket
	var srcBucket db.Bucket
	if err := w.DB.Where("bucket_name = ? AND user_id = ?", payload.BucketSrc, user.ID).First(&srcBucket).Error; err != nil {
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
				return fmt.Errorf("failed to create destination bucket: %w", err)
			}
		} else {
			return fmt.Errorf("failed to fetch destination bucket: %w", err)
		}
	}

	// Ensure destination folder exists
	destDir := fmt.Sprintf("./storage/%s", destBucket.BucketName)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination folder: %w", err)
	}

	// Fetch files from source bucket
	var files []db.File
	if err := w.DB.Where("bucket_id = ?", srcBucket.ID).Find(&files).Error; err != nil {
		return fmt.Errorf("failed to fetch files from source bucket: %w", err)
	}

	total := len(files)
	for i, f := range files {
		srcPath := fmt.Sprintf("./storage/%s/%s", srcBucket.BucketName, f.FileName)
		destPath := filepath.Join(destDir, f.FileName)

		input, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("failed to read source file %s: %w", f.FileName, err)
		}
		if err := os.WriteFile(destPath, input, 0644); err != nil {
			return fmt.Errorf("failed to write destination file %s: %w", f.FileName, err)
		}

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
			return fmt.Errorf("failed to create file record in DB: %w", err)
		}

		// Update task progress
		progress := int(float64(i+1) / float64(total) * 100)
		w.DB.Model(&db.Task{}).
			Where("bucket_src = ? AND bucket_dest = ? AND user_id = ?", payload.BucketSrc, payload.BucketDest, payload.UserID).
			Update("progress", progress)
	}

	// Mark task completed
	w.DB.Model(&db.Task{}).
		Where("bucket_src = ? AND bucket_dest = ? AND user_id = ?", payload.BucketSrc, payload.BucketDest, payload.UserID).
		Updates(map[string]interface{}{"status": "completed", "progress": 100})

	return nil
}
