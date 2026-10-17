// Package webhooks delivers catalog changes to registered HTTPS endpoints: it fans changes out
// into per-endpoint deliveries, signs and sends them through the SSRF-safe client, retries with
// exponential backoff and jitter, and disables endpoints that keep failing.
package webhooks

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/safefetch"
)

// Header names sent with every delivery.
const (
	HeaderEventID    = "X-REA-Event-Id"
	HeaderEventType  = "X-REA-Event-Type"
	HeaderDeliveryID = "X-REA-Delivery-Id"
	HeaderTimestamp  = "X-REA-Timestamp"
	HeaderSignature  = "X-REA-Signature"
	UserAgent        = "REA-Webhooks/1.0"
	secretPrefix     = "whsec_"
)

// NewSecret returns a new random signing secret ("whsec_" + 256 bits, base64url).
func NewSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return secretPrefix + base64.RawURLEncoding.EncodeToString(b)
}

// SecretPrefix is the non-secret part of a secret shown in the UI.
func SecretPrefix(secret string) string {
	if len(secret) <= 12 {
		return secret
	}
	return secret[:12]
}

// Sign returns hex(HMAC-SHA256(secret, timestamp + "." + body)). The key is the complete secret
// string, including its "whsec_" prefix.
func Sign(secret string, timestamp int64, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(strconv.FormatInt(timestamp, 10)))
	m.Write([]byte("."))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

// SignatureHeader signs body with every secret ("v1=<hex>", comma separated). During a secret
// rotation both the new and the previous secret sign, so receivers can switch without downtime.
func SignatureHeader(secrets []string, timestamp int64, body []byte) string {
	parts := make([]string, 0, len(secrets))
	for _, s := range secrets {
		parts = append(parts, "v1="+Sign(s, timestamp, body))
	}
	return strings.Join(parts, ",")
}

// Verify checks a signature header against a secret with a tolerance for the timestamp age. It is
// the reference implementation of the receiver side shown in the documentation.
func Verify(secret, header string, timestamp int64, body []byte, now time.Time, tolerance time.Duration) bool {
	if d := now.Sub(time.Unix(timestamp, 0)); d > tolerance || d < -tolerance {
		return false
	}
	want := Sign(secret, timestamp, body)
	for _, part := range strings.Split(header, ",") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "v1="); ok && hmac.Equal([]byte(v), []byte(want)) {
			return true
		}
	}
	return false
}

// Backoff returns the delay before the next attempt after `attempt` failed attempts: the base
// delay doubled per attempt, capped, with ±20 % jitter so retries from many endpoints spread out.
func Backoff(cfg *config.Config, attempt int) time.Duration {
	d := cfg.WebhookRetryBase
	for i := 1; i < attempt && d < cfg.WebhookRetryMax; i++ {
		d *= 2
	}
	d = min(d, cfg.WebhookRetryMax)
	span := int64(d) * 2 / 5
	if span <= 0 {
		return d
	}
	n, err := rand.Int(rand.Reader, big.NewInt(span+1))
	if err != nil {
		return d
	}
	return d - time.Duration(span/2) + time.Duration(n.Int64())
}

// Policy is the outbound address policy for webhook targets.
func Policy(cfg *config.Config) safefetch.Policy {
	return safefetch.Policy{AllowPrivate: cfg.WebhookAllowPrivate, AllowLoopback: cfg.WebhookAllowHTTPLocalhost}
}

// Errors returned by ValidateURL.
var (
	ErrURLInvalid = errors.New("must be an absolute https:// URL without credentials")
	ErrURLHTTP    = errors.New("must use https:// (plain http:// is only allowed to localhost when REA_WEBHOOK_ALLOW_HTTP_LOCALHOST is on)")
	ErrURLPrivate = errors.New("points to a private, loopback or link-local address; set REA_WEBHOOK_ALLOW_PRIVATE_NETWORKS to allow it")
)

// ValidateURL checks a webhook URL when it is saved. The resolved address is checked again on
// every delivery.
func ValidateURL(cfg *config.Config, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !u.IsAbs() || u.Host == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || len(raw) > 2048 {
		return ErrURLInvalid
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !cfg.WebhookAllowHTTPLocalhost || !safefetch.IsLocalhostName(u.Hostname()) {
			return ErrURLHTTP
		}
	default:
		return ErrURLInvalid
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return ErrURLInvalid
		}
	}
	if err := Policy(cfg).CheckHost(u); err != nil {
		return ErrURLPrivate
	}
	return nil
}
