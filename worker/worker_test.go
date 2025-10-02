package worker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	dbConn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = dbConn.AutoMigrate(&db.User{}, &db.Bucket{}, &db.File{}, &db.Task{})
	require.NoError(t, err)
	return dbConn
}

func TestHandleEmptyBucketTask(t *testing.T) {
	DB := setupTestDB(t)

	// create user, bucket, files, task
	user := db.User{ID: "user-1", Email: "user@example.com"}
	require.NoError(t, DB.Create(&user).Error)

	bucket := db.Bucket{ID: "bucket-1", BucketName: "testbucket", UserID: user.ID}
	require.NoError(t, DB.Create(&bucket).Error)

	files := []db.File{
		{ID: "file-1", FileName: "file1.txt", BucketID: bucket.ID},
		{ID: "file-2", FileName: "file2.txt", BucketID: bucket.ID},
	}
	for _, f := range files {
		require.NoError(t, DB.Create(&f).Error)
	}

	task := db.Task{
		ID:        "task-1",
		UserID:    user.ID,
		Type:      "empty",
		BucketSrc: &bucket.BucketName,
		Status:    "running",
	}
	require.NoError(t, DB.Create(&task).Error)

	// create bucket folder and files
	dir := filepath.Join(".", "storage", bucket.BucketName)
	require.NoError(t, os.MkdirAll(dir, 0755))
	for _, f := range files {
		path := filepath.Join(dir, f.FileName)
		require.NoError(t, os.WriteFile(path, []byte("data"), 0644))
	}

	worker := &Worker{DB: DB}
	payload := map[string]string{"UserID": user.ID, "BucketName": bucket.BucketName}
	data, _ := json.Marshal(payload)
	asynqTask := asynq.NewTask("empty_bucket", data)

	require.NoError(t, worker.HandleEmptyBucketTask(context.Background(), asynqTask))

	// check task progress and status
	var updatedTask db.Task
	require.NoError(t, DB.First(&updatedTask, "id = ?", task.ID).Error)
	require.Equal(t, 100, updatedTask.Progress)
	require.Equal(t, "completed", updatedTask.Status)

	// check files deleted
	var remaining int64
	DB.Model(&db.File{}).Where("bucket_id = ?", bucket.ID).Count(&remaining)
	require.Equal(t, int64(0), remaining)

	// cleanup
	os.RemoveAll("./storage")
}

func TestHandleCopyBucketTask(t *testing.T) {
	DB := setupTestDB(t)

	user := db.User{ID: "user-1", Email: "user@example.com"}
	require.NoError(t, DB.Create(&user).Error)

	src := db.Bucket{ID: "src-1", BucketName: "srcbucket", UserID: user.ID}
	require.NoError(t, DB.Create(&src).Error)

	files := []db.File{
		{ID: "file-1", FileName: "file1.txt", BucketID: src.ID},
	}
	for _, f := range files {
		require.NoError(t, DB.Create(&f).Error)
	}

	srcDir := filepath.Join(".", "storage", src.BucketName)
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, f.FileName), []byte("data"), 0644))
	}

	destBucketName := "destbucket"
	task := db.Task{
		ID:         "task-copy-1",
		UserID:     user.ID,
		Type:       "copy",
		BucketSrc:  &src.BucketName,
		BucketDest: &destBucketName,
		Status:     "running",
	}
	require.NoError(t, DB.Create(&task).Error)

	worker := &Worker{DB: DB}
	payload := map[string]string{
		"user_id":     user.ID,
		"bucket_src":  src.BucketName,
		"bucket_dest": destBucketName,
	}
	data, _ := json.Marshal(payload)
	asynqTask := asynq.NewTask("copy_bucket", data)

	// Run the worker
	require.NoError(t, worker.HandleCopyBucketTask(context.Background(), asynqTask))

	// Check destination bucket was created
	var destBucket db.Bucket
	require.NoError(t, DB.Where("bucket_name = ? AND user_id = ?", destBucketName, user.ID).First(&destBucket).Error)

	// Check files copied
	var copiedFiles []db.File
	require.NoError(t, DB.Where("bucket_id = ?", destBucket.ID).Find(&copiedFiles).Error)
	require.Len(t, copiedFiles, 1)
	require.Equal(t, "file1.txt", copiedFiles[0].FileName)

	os.RemoveAll("./storage")
}
