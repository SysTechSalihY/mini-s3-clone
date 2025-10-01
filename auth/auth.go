package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"gorm.io/gorm"
)

func GenerateKeys() (accessKey, secretKey string, err error) {
	ak := make([]byte, 16) // 32 hex chars
	sk := make([]byte, 32) // 64 hex chars
	_, err = rand.Read(ak)
	if err != nil {
		return
	}
	_, err = rand.Read(sk)
	if err != nil {
		return
	}
	accessKey = hex.EncodeToString(ak)
	secretKey = hex.EncodeToString(sk)
	return
}

func GetUserByAccessKey(DB *gorm.DB, accessKey string) (*db.User, error) {
	var user db.User
	if err := DB.Where("access_key = ?", accessKey).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

// Client should use this for secretKey
func SignRequest(secretKey, method, path string, expires int64) string {
	data := method + "\n" + path + "\n" + strconv.FormatInt(expires, 10)
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func ValidateRequest(DB *gorm.DB, accessKey, signature, method, path string, expires int64) bool {
	user, err := GetUserByAccessKey(DB, accessKey)
	if err != nil {
		return false
	}

	expectedSig := SignRequest(user.SecretKey, method, path, expires)

	return hmac.Equal([]byte(signature), []byte(expectedSig)) && time.Now().Unix() < expires
}
