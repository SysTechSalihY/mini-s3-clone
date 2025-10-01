package auth

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Dummy User struct for testing
type User struct {
	AccessKey string
	SecretKey string
	ID        string
}

// In-memory DB helper
func setupTestDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&User{})
	return db
}

func TestGenerateKeys(t *testing.T) {
	ak, sk, err := GenerateKeys()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ak) != 32 || len(sk) != 64 {
		t.Fatalf("keys have wrong length: ak=%d, sk=%d", len(ak), len(sk))
	}
}

func TestSignAndValidateRequest(t *testing.T) {
	db := setupTestDB()
	user := User{AccessKey: "testAK", SecretKey: "testSK"}
	db.Create(&user)

	method := "GET"
	path := "/bucket/file.txt"
	expires := time.Now().Add(time.Minute).Unix()

	sig := SignRequest(user.SecretKey, method, path, expires)

	valid := ValidateRequest(db, user.AccessKey, sig, method, path, expires)
	if !valid {
		t.Fatal("expected request to be valid")
	}
}
