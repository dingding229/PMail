package imageproxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"

	"github.com/Jinnrry/pmail/config"
)

// Token binds proxy requests to one user and one mail. The web client gets it
// with the mail detail; mail content cannot know it, so a message cannot make
// the reader's browser call the proxy (for example by guessing mail ids) while
// its images are still blocked.
func Token(userID, emailID int) string {
	if config.Instance == nil || config.Instance.SecretKey == "" || userID <= 0 || emailID <= 0 {
		return ""
	}
	key := sha256.Sum256([]byte("pmail-image-proxy\x00" + config.Instance.SecretKey))
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(strconv.Itoa(userID) + ":" + strconv.Itoa(emailID)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:18])
}

func ValidToken(userID, emailID int, token string) bool {
	want := Token(userID, emailID)
	return want != "" && hmac.Equal([]byte(want), []byte(token))
}
