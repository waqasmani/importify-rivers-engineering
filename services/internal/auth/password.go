// Package auth implements password hashing, opaque tokens and the permission model.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters follow the OWASP recommendation (m=19 MiB, t=2, p=1) with headroom.
const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
	saltLen      = 16
)

// ErrMismatch is returned when a password does not match its hash.
var ErrMismatch = errors.New("password mismatch")

// HashPassword returns a PHC-formatted argon2id hash.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword checks a password against a hash from HashPassword in constant time.
func VerifyPassword(password, encoded string) error {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return errors.New("unsupported password hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return errors.New("unsupported argon2 version")
	}
	var mem uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return errors.New("invalid argon2 parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return err
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return err
	}
	got := argon2.IDKey([]byte(password), salt, t, mem, p, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrMismatch
	}
	return nil
}

// DummyVerify spends the same time as a real verification so that unknown e-mail addresses
// cannot be discovered by timing the login endpoint.
func DummyVerify(password string) {
	_ = argon2.IDKey([]byte(password), make([]byte, saltLen), argonTime, argonMemory, argonThreads, argonKeyLen)
}

// ValidatePassword enforces the password policy and returns a user-facing message, or "".
func ValidatePassword(pw string) string {
	n := utf8.RuneCountInString(pw)
	if n < 12 {
		return "must be at least 12 characters"
	}
	if n > 256 {
		return "must be at most 256 characters"
	}
	lower := strings.ToLower(pw)
	for _, weak := range []string{"password", "123456789", "qwertyuiop", "letmein", "changeme"} {
		if strings.Contains(lower, weak) && n < 20 {
			return "is too easy to guess"
		}
	}
	distinct := map[rune]bool{}
	for _, r := range pw {
		distinct[r] = true
	}
	if len(distinct) < 6 {
		return "must contain more varied characters"
	}
	return ""
}
