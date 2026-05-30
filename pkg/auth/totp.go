package auth

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

const backupCodeCount = 10

func GenerateTOTPKey(username, issuer string) (*otp.Key, error) {
	return totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
	})
}

func ValidateTOTPCode(code, secret string) bool {
	return totp.Validate(code, secret)
}

// GenerateBackupCodes returns plain (shown once) and hashed (stored) backup codes.
func GenerateBackupCodes() (plain []string, hashed []string, err error) {
	plain = make([]string, backupCodeCount)
	hashed = make([]string, backupCodeCount)
	for i := 0; i < backupCodeCount; i++ {
		b := make([]byte, 4)
		if _, err = rand.Read(b); err != nil {
			return nil, nil, err
		}
		plain[i] = fmt.Sprintf("%04X-%04X", uint16(b[0])<<8|uint16(b[1]), uint16(b[2])<<8|uint16(b[3]))
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(normalizeBackupCode(plain[i])), 10)
		if hashErr != nil {
			return nil, nil, hashErr
		}
		hashed[i] = string(hash)
	}
	return plain, hashed, nil
}

// VerifyBackupCode checks input against hashed codes and returns the matching index, or -1 if none.
func VerifyBackupCode(input string, hashes []string) int {
	normalized := []byte(normalizeBackupCode(input))
	for i, hash := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(hash), normalized) == nil {
			return i
		}
	}
	return -1
}

func normalizeBackupCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(code, "-", ""))
}
