package totp

import (
	"github.com/gitwise-io/gitwise/internal/services/user"
)

// verifyPasswordArgon2id delegates to the user package's exported VerifyPassword.
func verifyPasswordArgon2id(password, encoded string) bool {
	return user.VerifyPassword(password, encoded)
}
