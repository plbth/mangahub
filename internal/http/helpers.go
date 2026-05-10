package httpapi

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/plbth/mangahub/pkg/models"
)

func newID(prefix string) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return prefix + "_" + hex.EncodeToString(buf)
}

func sanitizeUser(user *models.User) *models.User {
	if user == nil {
		return nil
	}
	clone := *user
	clone.PasswordHash = ""
	return &clone
}
