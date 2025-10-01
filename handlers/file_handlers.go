package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/SysTechSalihY/mini-s3-clone/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func DownloadFile(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		bucketName := c.Params("bucketName", "")
		fileName := c.Params("fileName", "")
		versionID := c.Query("versionID", "")

		if bucketName == "" || fileName == "" {
			log.Warn("DownloadFile: bucketName or fileName missing")
			return c.Status(400).JSON(fiber.Map{"error": "bucketName and fileName are required"})
		}

		var bucket db.Bucket
		if err := DB.Where("bucket_name = ?", bucketName).First(&bucket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.WithField("bucketName", bucketName).Warn("Bucket not found")
				return c.Status(404).JSON(fiber.Map{"error": "bucket not found"})
			}
			log.WithError(err).Error("DB error fetching bucket")
			return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}

		var file db.File
		query := DB.Where("file_name = ? AND bucket_id = ?", fileName, bucket.ID)
		if versionID != "" {
			query = query.Where("version_id = ?", versionID)
		} else {
			query = query.Where("is_latest = ?", true)
		}

		if err := query.First(&file).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.WithFields(log.Fields{"file": fileName, "bucket": bucketName}).Warn("File not found")
				return c.Status(404).JSON(fiber.Map{"error": "file not found"})
			}
			log.WithError(err).Error("DB error fetching file")
			return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}

		user, ok := c.Locals("user").(*db.User)
		isOwner := ok && user.ID == bucket.UserID
		isPublic := bucket.ACL != nil && *bucket.ACL == "public-read"

		if !isOwner && !isPublic {
			log.WithFields(log.Fields{"bucket": bucketName, "file": fileName}).Warn("Unauthorized download attempt")
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}
		var versionedFileName string
		if bucket.Versioning {
			versionedFileName = fmt.Sprintf("%s_%s", file.VersionID, file.FileName)
		} else {
			versionedFileName = file.FileName
		}
		filePath := fmt.Sprintf("./storage/%s/%s", bucketName, versionedFileName)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			log.WithField("filePath", filePath).Warn("File not found on disk")
			return c.Status(404).JSON(fiber.Map{"error": "file not found"})
		}

		log.WithFields(log.Fields{"user_id": user.ID, "bucket": bucketName, "file": fileName, "versionID": file.VersionID}).Info("File download allowed")
		return c.SendFile(filePath, true)
	}
}

func CreateDownloadPresignedURL(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		bucketName := c.Query("bucket")
		fileName := c.Query("key")
		versionID := c.Query("versionID", "")
		durationStr := c.Query("duration", "3600")

		log.WithFields(log.Fields{
			"bucket":    bucketName,
			"file":      fileName,
			"versionID": versionID,
			"duration":  durationStr,
		}).Info("Received request to create download presigned URL")

		durationSec, err := strconv.Atoi(durationStr)
		if err != nil {
			log.WithError(err).WithField("duration", durationStr).Error("Invalid duration parameter")
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid duration"})
		}

		if bucketName == "" || fileName == "" {
			log.Warn("Missing bucket or key parameter")
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "bucket and key are required"})
		}

		var bucket db.Bucket
		if err := DB.Where("bucket_name = ?", bucketName).First(&bucket).Error; err != nil {
			log.WithField("bucket", bucketName).Warn("Bucket not found in database")
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "bucket not found"})
		}

		user, ok := c.Locals("user").(*db.User)
		if !ok {
			log.Warn("User not found in context")
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		if bucket.UserID != user.ID {
			log.WithFields(log.Fields{"bucketOwner": bucket.UserID, "user": user.ID}).Warn("User not authorized to generate presigned URL for this bucket")
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "only bucket owner can generate presigned URL"})
		}

		url := utils.GeneratePresignedURL(bucketName, fileName, user.SecretKey, "download", time.Duration(durationSec)*time.Second, versionID)
		log.WithFields(log.Fields{
			"user":      user.ID,
			"bucket":    bucketName,
			"file":      fileName,
			"versionID": versionID,
			"url":       url,
		}).Info("Download presigned URL generated successfully")

		return c.JSON(fiber.Map{"url": url})
	}
}

func CreateUploadPresignedURL(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		bucketName := c.Query("bucket")
		fileName := c.Query("key")
		durationStr := c.Query("duration", "3600")

		log.WithFields(log.Fields{
			"bucket":   bucketName,
			"file":     fileName,
			"duration": durationStr,
		}).Info("Received request to create upload presigned URL")

		durationSec, err := strconv.Atoi(durationStr)
		if err != nil {
			log.WithError(err).WithField("duration", durationStr).Error("Invalid duration parameter")
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid duration"})
		}

		if bucketName == "" || fileName == "" {
			log.Warn("Missing bucket or key parameter")
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "bucket and key are required"})
		}

		var bucket db.Bucket
		if err := DB.Where("bucket_name = ?", bucketName).First(&bucket).Error; err != nil {
			log.WithField("bucket", bucketName).Warn("Bucket not found in database")
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "bucket not found"})
		}

		user, ok := c.Locals("user").(*db.User)
		if !ok {
			log.Warn("User not found in context")
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		if bucket.UserID != user.ID {
			log.WithFields(log.Fields{"bucketOwner": bucket.UserID, "user": user.ID}).Warn("User not authorized to generate upload presigned URL for this bucket")
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "only bucket owner can generate presigned URL"})
		}

		url := utils.GeneratePresignedURL(bucketName, fileName, user.SecretKey, "upload", time.Duration(durationSec)*time.Second)
		log.WithFields(log.Fields{
			"user":   user.ID,
			"bucket": bucketName,
			"file":   fileName,
			"url":    url,
		}).Info("Upload presigned URL generated successfully")

		return c.JSON(fiber.Map{"url": url})
	}
}

func DownloadFilePresignedURL(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		bucketName := c.Locals("bucket").(string)
		fileName := c.Locals("key").(string)
		versionID := c.Query("versionID", "")
		operation := c.Locals("operation").(string)

		if operation != "download" {
			log.WithFields(log.Fields{
				"operation": operation,
				"bucket":    bucketName,
				"key":       fileName,
			}).Warn("Invalid operation for presigned download")
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "invalid operation for this endpoint"})
		}

		var file db.File
		query := DB.Where("file_name = ? AND bucket_id = (SELECT id FROM buckets WHERE bucket_name = ?)", fileName, bucketName)
		if versionID != "" {
			query = query.Where("version_id = ?", versionID)
		} else {
			query = query.Where("is_latest = ?", true)
		}

		if err := query.First(&file).Error; err != nil {
			log.WithFields(log.Fields{"bucket": bucketName, "file": fileName, "versionID": versionID}).Warn("File not found in DB")
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "file not found"})
		}

		versionedFileName := fmt.Sprintf("%s_%s", file.VersionID, file.FileName)
		filePath := fmt.Sprintf("./storage/%s/%s", bucketName, versionedFileName)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			log.WithField("filePath", filePath).Warn("File not found on disk")
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "file not found"})
		}

		log.WithFields(log.Fields{"bucket": bucketName, "file": fileName, "versionID": file.VersionID}).Info("Presigned file download allowed")
		return c.SendFile(filePath, true)
	}
}

func UploadFilePresignedURL(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		file, err := c.FormFile("file")
		if err != nil || file == nil {
			log.WithError(err).Error("Presigned upload: reading file error")
			return c.Status(400).JSON(fiber.Map{"error": "reading file error"})
		}

		bucketName := c.Locals("bucket").(string)
		fileName := c.Locals("key").(string)
		operation := c.Locals("operation").(string)

		if operation != "upload" {
			log.WithField("operation", operation).Warn("Invalid operation for presigned upload")
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "invalid operation for this endpoint"})
		}

		var bucket db.Bucket
		if err := DB.Where("bucket_name = ?", bucketName).First(&bucket).Error; err != nil {
			log.WithField("bucket", bucketName).Warn("Bucket not found")
			return c.Status(404).JSON(fiber.Map{"error": "bucket not found"})
		}

		var versionID string
		var versionedFileName string

		if bucket.Versioning {
			versionID = uuid.NewString()
			DB.Model(&db.File{}).
				Where("bucket_id = ? AND file_name = ? AND is_latest = ?", bucket.ID, fileName, true).
				Update("is_latest", false)

			versionedFileName = fmt.Sprintf("%s_%s", versionID, fileName)
		} else {
			versionedFileName = fileName
			var existing db.File
			if err := DB.Where("bucket_id = ? AND file_name = ? AND is_latest = ?", bucket.ID, fileName, true).First(&existing).Error; err == nil {
				log.WithFields(log.Fields{"bucket": bucketName, "file": fileName}).Warn("File exists and versioning disabled")
				return c.Status(400).JSON(fiber.Map{"error": "file already exists"})
			}
		}
		dirPath := fmt.Sprintf("./storage/%s", bucketName)
		if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
			log.WithError(err).WithField("dirPath", dirPath).Error("Failed to create bucket directory")
			return c.Status(500).JSON(fiber.Map{"error": "failed to create directory"})
		}

		filePath := filepath.Join(dirPath, versionedFileName)
		if err := c.SaveFile(file, filePath); err != nil {
			log.WithError(err).WithField("filePath", filePath).Error("Failed to save file to disk")
			return c.Status(500).JSON(fiber.Map{"error": "failed to save file"})
		}

		newFile := db.File{
			ID:          uuid.NewString(),
			FileName:    fileName,
			BucketID:    bucket.ID,
			Size:        file.Size,
			ContentType: file.Header.Get("Content-Type"),
			VersionID:   versionID,
			IsLatest:    true,
		}

		if err := DB.Create(&newFile).Error; err != nil {
			log.WithError(err).WithField("file", fileName).Error("Failed to save file metadata")
			return c.Status(500).JSON(fiber.Map{"error": "failed to save file metadata"})
		}

		logFields := log.Fields{"bucket": bucketName, "file": fileName}
		if bucket.Versioning {
			logFields["versionID"] = versionID
		}
		log.WithFields(logFields).Info("Presigned file uploaded successfully")

		resp := fiber.Map{
			"message":  "file uploaded successfully",
			"fileName": fileName,
			"bucket":   bucketName,
			"size":     file.Size,
		}
		if bucket.Versioning {
			resp["versionID"] = versionID
		}

		return c.Status(201).JSON(resp)
	}
}

func UploadFile(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		file, err := c.FormFile("file")
		if err != nil || file == nil {
			log.WithError(err).Error("UploadFile: reading file error")
			return c.Status(400).JSON(fiber.Map{"error": "reading file error"})
		}

		bucketName := c.Params("bucketName", "")
		fileName := c.Params("fileName", "")
		if bucketName == "" || fileName == "" {
			log.Warn("UploadFile: bucket or file name missing")
			return c.Status(400).JSON(fiber.Map{"error": "bucket and file names are required"})
		}

		var bucket db.Bucket
		if err := DB.Where("bucket_name = ?", bucketName).First(&bucket).Error; err != nil {
			log.WithError(err).WithField("bucket", bucketName).Warn("Bucket not found")
			return c.Status(404).JSON(fiber.Map{"error": "bucket not found"})
		}

		user, ok := c.Locals("user").(*db.User)
		if !ok || bucket.UserID != user.ID {
			log.WithFields(log.Fields{"bucket": bucketName, "user_id": user.ID}).Warn("Unauthorized upload attempt")
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}

		var versionedFileName string
		versionID := uuid.NewString()
		if bucket.Versioning {
			versionedFileName = fmt.Sprintf("%s_%s", versionID, fileName)

			DB.Model(&db.File{}).
				Where("bucket_id = ? AND file_name = ? AND is_latest = ?", bucket.ID, fileName, true).
				Update("is_latest", false)
		} else {
			versionedFileName = fileName
			var existing db.File
			if err := DB.Where("bucket_id = ? AND file_name = ? AND is_latest = ?", bucket.ID, fileName, true).First(&existing).Error; err == nil {
				log.WithFields(log.Fields{"bucket": bucketName, "file": fileName}).Warn("File already exists and versioning disabled")
				return c.Status(400).JSON(fiber.Map{"error": "file already exists"})
			}
		}
		dirPath := fmt.Sprintf("./storage/%s", bucketName)
		if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
			log.WithError(err).WithField("dirPath", dirPath).Error("Failed to create bucket directory")
			return c.Status(500).JSON(fiber.Map{"error": "failed to create directory"})
		}

		filePath := filepath.Join(dirPath, versionedFileName)
		if err := c.SaveFile(file, filePath); err != nil {
			log.WithError(err).WithField("filePath", filePath).Error("Failed to save file to disk")
			return c.Status(500).JSON(fiber.Map{"error": "failed to save file"})
		}
		newFile := db.File{
			ID:          uuid.NewString(),
			FileName:    fileName,
			BucketID:    bucket.ID,
			Size:        file.Size,
			ContentType: file.Header.Get("Content-Type"),
			VersionID:   versionID,
			IsLatest:    true,
		}

		if err := DB.Create(&newFile).Error; err != nil {
			log.WithError(err).WithField("file", fileName).Error("Failed to insert file metadata")
			return c.Status(500).JSON(fiber.Map{"error": "failed to save file metadata"})
		}

		log.WithFields(log.Fields{"user_id": user.ID, "bucket": bucketName, "file": fileName, "versionID": versionID}).Info("File uploaded successfully")
		return c.Status(201).JSON(fiber.Map{
			"message":   "file uploaded successfully",
			"fileName":  newFile.FileName,
			"bucket":    bucketName,
			"size":      newFile.Size,
			"versionID": versionID,
		})
	}
}

func UploadFileMultipart(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		bucketName := c.Params("bucketName")
		if bucketName == "" {
			return c.Status(400).JSON(fiber.Map{"error": "bucketName is required"})
		}
		files, err := c.MultipartForm()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "failed to read multipart form",
			})
		}
		user, ok := c.Locals("user").(*db.User)
		if !ok {
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}
		var bucket db.Bucket
		if err := DB.Where("bucket_name = ?", bucketName).First(&bucket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return c.Status(404).JSON(fiber.Map{"error": "bucket not found"})
			}
			return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}
		if !ok || user.ID != bucket.UserID {
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}
		uploadedFiles := []string{}
		for _, file := range files.File["files"] {

		}
		return c.SendStatus(200)
	}
}

func DeleteFile(DB *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		bucketName := c.Params("bucketName")
		fileName := c.Params("fileName")
		versionID := c.Query("versionID")

		if bucketName == "" || fileName == "" {
			return c.Status(400).JSON(fiber.Map{"error": "bucketName and fileName are required"})
		}

		var bucket db.Bucket
		if err := DB.Where("bucket_name = ?", bucketName).First(&bucket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return c.Status(404).JSON(fiber.Map{"error": "bucket not found"})
			}
			return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}

		user, ok := c.Locals("user").(*db.User)
		if !ok || user.ID != bucket.UserID {
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}

		var file db.File
		query := DB.Where("bucket_id = ? AND file_name = ?", bucket.ID, fileName)
		if versionID != "" {
			query = query.Where("version_id = ?", versionID)
		} else {
			query = query.Where("is_latest = ?", true)
		}

		if err := query.First(&file).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return c.Status(404).JSON(fiber.Map{"error": "file not found"})
			}
			return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}

		filePath := fmt.Sprintf("./storage/%s/%s", bucket.BucketName, file.FileName)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return c.Status(500).JSON(fiber.Map{"error": "failed to delete file from disk"})
		}

		if err := DB.Delete(&file).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to delete file from DB"})
		}

		if bucket.Versioning && file.IsLatest {
			var latest db.File
			if err := DB.Where("bucket_id = ? AND file_name = ?", bucket.ID, fileName).
				Order("created_at desc").Limit(1).First(&latest).Error; err == nil {
				DB.Model(&latest).Update("is_latest", true)
			}
		}

		return c.Status(200).JSON(fiber.Map{
			"message":   "file deleted successfully",
			"fileName":  file.FileName,
			"bucket":    bucket.BucketName,
			"versionID": file.VersionID,
		})
	}
}
