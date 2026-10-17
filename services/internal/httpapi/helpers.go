package httpapi

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/htmlx"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func sanitizeSnapshot(src string) (string, error) { return htmlx.SanitizeSnapshot(src) }
