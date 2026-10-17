package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// Token prefixes make leaked tokens easy to recognise and route to the right lookup.
const (
	PrefixSession = "reas_"
	PrefixAccess  = "reaa_"
	PrefixRefresh = "rear_"
	PrefixPAT     = "reap_"
)

// NewToken returns a random 256-bit token with the given prefix.
func NewToken(prefix string) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}

// HashToken returns the SHA-256 digest stored in the database. Tokens have 256 bits of entropy,
// so a fast hash is sufficient and allows indexed lookups.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// DisplayPrefix is the non-secret part of a token shown in the UI.
func DisplayPrefix(token string) string {
	if len(token) <= 12 {
		return token
	}
	return token[:12]
}

// pairingAlphabet avoids ambiguous characters (0/O, 1/I/L).
const pairingAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// NewPairingCode returns an 8-character one-time code formatted as XXXX-XXXX.
func NewPairingCode() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	var sb strings.Builder
	for i, v := range b {
		if i == 4 {
			sb.WriteByte('-')
		}
		sb.WriteByte(pairingAlphabet[int(v)%len(pairingAlphabet)])
	}
	return sb.String()
}

// NormalizePairingCode uppercases a code and strips separators and whitespace.
func NormalizePairingCode(code string) string {
	code = strings.ToUpper(code)
	var sb strings.Builder
	for _, r := range code {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
