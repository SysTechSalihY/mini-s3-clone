package handlers

import (
	"testing"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	dbConn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	err = dbConn.AutoMigrate(&db.User{}, &db.EmailVerification{}, &db.Bucket{}, &db.File{}, &db.Task{})
	assert.NoError(t, err)
	return dbConn
}

func setupFiber() *fiber.App {
	return fiber.New()
}
