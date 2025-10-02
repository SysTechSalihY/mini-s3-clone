// handlers/auth_test.go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func TestSignUp(t *testing.T) {
	dbConn := setupTestDB(t)
	app := setupFiber()
	app.Post("/signup", SignUp(dbConn))

	body := SignUpRequest{
		Email:    "test1@example.com",
		Password: "supersecret",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/signup", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
}

func TestDuplicateSignUp(t *testing.T) {
	dbConn := setupTestDB(t)
	app := setupFiber()
	app.Post("/signup", SignUp(dbConn))

	email := "dup1@example.com"

	// first signup
	body := SignUpRequest{Email: email, Password: "123456"}
	jsonBody, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/signup", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	resp1, _ := app.Test(req)
	assert.Equal(t, 201, resp1.StatusCode)

	// second signup with same email
	req2 := httptest.NewRequest("POST", "/signup", bytes.NewReader(jsonBody))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := app.Test(req2)
	//Sqlite does not return record not found
	assert.Equal(t, 500, resp2.StatusCode)
}

func TestVerifyEmail(t *testing.T) {
	dbConn := setupTestDB(t)

	// create a user + token
	user := db.User{
		ID:           uuid.NewString(),
		Email:        "verify1@example.com",
		PasswordHash: "hash",
		SecretKey:    "sk1",
		AccessKey:    "ak1",
	}
	dbConn.Create(&user)

	token := "verifytoken1"
	verification := db.EmailVerification{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	dbConn.Create(&verification)

	app := setupFiber()
	app.Get("/verify-email", VerifyEmail(dbConn))

	req := httptest.NewRequest("GET", "/verify-email?token="+token, nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestVerifyEmailExpired(t *testing.T) {
	dbConn := setupTestDB(t)

	user := db.User{
		ID:           uuid.NewString(),
		Email:        "expired1@example.com",
		PasswordHash: "hash",
		SecretKey:    "sk2",
		AccessKey:    "ak2",
	}
	dbConn.Create(&user)

	token := "expiredtoken1"
	verification := db.EmailVerification{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	dbConn.Create(&verification)

	app := setupFiber()
	app.Get("/verify-email", VerifyEmail(dbConn))

	req := httptest.NewRequest("GET", "/verify-email?token="+token, nil)
	resp, _ := app.Test(req)

	assert.Equal(t, 400, resp.StatusCode)
}

func TestCreateSecretKey(t *testing.T) {
	dbConn := setupTestDB(t)

	// create a test user
	password := "mypassword"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	user := db.User{
		ID:           uuid.NewString(),
		Email:        "secret1@example.com",
		PasswordHash: string(hashed),
		SecretKey:    "oldsk",
		AccessKey:    "oldak",
	}
	dbConn.Create(&user)

	app := setupFiber()
	app.Post("/create-secret", CreateSecretKey(dbConn))

	body := CreateAccessRequest{Email: user.Email, Password: password}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/create-secret", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
