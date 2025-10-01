package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"
)

func GeneratePresignedURL(bucket, key, secret, operation string, duration time.Duration, versionID ...string) string {
	expiration := time.Now().Add(duration).Unix()

	verID := ""
	if len(versionID) > 0 {
		verID = versionID[0]
	}

	message := fmt.Sprintf("%s:%s:%s:%d:%s", bucket, key, operation, expiration, verID)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	signature := base64.URLEncoding.EncodeToString(h.Sum(nil))

	url := fmt.Sprintf("/object/%s?bucket=%s&key=%s&expires=%d&sig=%s", operation, bucket, key, expiration, signature)
	if verID != "" {
		url += fmt.Sprintf("&versionID=%s", verID)
	}

	return url
}
