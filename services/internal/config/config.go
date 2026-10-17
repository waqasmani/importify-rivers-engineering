// Package config loads REA configuration from environment variables.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the complete runtime configuration. Every value comes from the environment so that
// secrets stay out of source control and images.
type Config struct {
	DatabaseURL       string
	HTTPAddr          string
	PublicURL         *url.URL
	AllowedOrigins    []string // origins allowed to make credentialed (cookie) requests
	ExtensionOrigins  []string // chrome-extension:// origins allowed to make bearer-token requests
	SecretKey         []byte
	DataDir           string
	WebDir            string
	CookieSecure      bool
	SessionTTL        time.Duration
	SessionIdle       time.Duration
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	MaxUploadBytes    int64
	MaxArchiveBytes   int64
	MaxArchiveFiles   int
	ServerFetch       bool
	FetchAllowPrivate bool
	WorkerMode        string // embedded | off
	WorkerConcurrency int
	LogLevel          string
	LogFormat         string
	AutoMigrate       bool
	TrustedProxies    []netip.Prefix
	RepoRoots         []string
	SetupToken        string
	PgDumpPath        string
	Version           string

	// Catalog API and webhooks.
	IntegrationRateLimit        int           // requests per minute per integration key
	CatalogChangeRetention      time.Duration // how long change feed entries are kept
	WebhookAllowPrivate         bool          // allow webhook targets on private, loopback and link-local addresses
	WebhookAllowHTTPLocalhost   bool          // allow plain http:// webhook URLs to localhost
	WebhookTimeout              time.Duration // per delivery attempt, connect to last byte
	WebhookMaxAttempts          int
	WebhookRetryBase            time.Duration // first retry delay; doubles per attempt
	WebhookRetryMax             time.Duration // upper bound of one retry delay
	WebhookDisableAfterFailures int           // consecutive failed attempts before an endpoint is disabled
	WebhookDisableFailingFor    time.Duration // ...and for at least this long
	WebhookConcurrency          int
	WebhookLogRetention         time.Duration // finished deliveries are kept this long
}

// DefaultExtensionID is the ID Chrome derives from the public key pinned in the extension manifest.
const DefaultExtensionID = "olceibeplclcegianhjffambadoakkno"

// Load reads configuration from the environment.
func Load() (*Config, error) {
	c := &Config{
		DatabaseURL: env("REA_DATABASE_URL", ""),
		HTTPAddr:    env("REA_HTTP_ADDR", ":8080"),
		DataDir:     env("REA_DATA_DIR", "./data"),
		WebDir:      env("REA_WEB_DIR", ""),
		LogLevel:    env("REA_LOG_LEVEL", "info"),
		LogFormat:   env("REA_LOG_FORMAT", "json"),
		WorkerMode:  env("REA_WORKER_MODE", "embedded"),
		SetupToken:  env("REA_SETUP_TOKEN", ""),
		PgDumpPath:  env("REA_PG_DUMP", "pg_dump"),
		Version:     env("REA_VERSION", "dev"),
	}
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("REA_DATABASE_URL is required"))
	}
	pub := env("REA_PUBLIC_URL", "http://localhost:8080")
	u, err := url.Parse(pub)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("REA_PUBLIC_URL must be an absolute http(s) URL, got %q", pub))
	} else {
		u.Path = strings.TrimRight(u.Path, "/")
		c.PublicURL = u
	}
	if key := env("REA_SECRET_KEY", ""); key != "" {
		b, err := base64.StdEncoding.DecodeString(key)
		if err != nil || len(b) != 32 {
			errs = append(errs, errors.New("REA_SECRET_KEY must be 32 random bytes encoded as base64 (openssl rand -base64 32)"))
		}
		c.SecretKey = b
	} else {
		errs = append(errs, errors.New("REA_SECRET_KEY is required (generate with: openssl rand -base64 32)"))
	}
	if c.PublicURL != nil {
		c.AllowedOrigins = append(c.AllowedOrigins, c.PublicURL.Scheme+"://"+c.PublicURL.Host)
		c.CookieSecure = c.PublicURL.Scheme == "https"
	}
	for _, o := range splitList(env("REA_ALLOWED_ORIGINS", "")) {
		o = strings.TrimRight(o, "/")
		if strings.HasPrefix(o, "chrome-extension://") {
			c.ExtensionOrigins = append(c.ExtensionOrigins, o)
			continue
		}
		c.AllowedOrigins = append(c.AllowedOrigins, o)
	}
	c.ExtensionOrigins = append(c.ExtensionOrigins, "chrome-extension://"+DefaultExtensionID)
	if v := os.Getenv("REA_COOKIE_SECURE"); v != "" {
		c.CookieSecure = v == "true" || v == "1"
	}
	c.SessionTTL = dur("REA_SESSION_TTL", 7*24*time.Hour, &errs)
	c.SessionIdle = dur("REA_SESSION_IDLE", 24*time.Hour, &errs)
	c.AccessTokenTTL = dur("REA_ACCESS_TOKEN_TTL", time.Hour, &errs)
	c.RefreshTokenTTL = dur("REA_REFRESH_TOKEN_TTL", 30*24*time.Hour, &errs)
	c.MaxUploadBytes = int64(intv("REA_MAX_UPLOAD_MB", 25, &errs)) << 20
	c.MaxArchiveBytes = int64(intv("REA_MAX_ARCHIVE_UNCOMPRESSED_MB", 300, &errs)) << 20
	c.MaxArchiveFiles = intv("REA_MAX_ARCHIVE_FILES", 20000, &errs)
	c.WorkerConcurrency = intv("REA_WORKER_CONCURRENCY", 2, &errs)
	c.ServerFetch = boolv("REA_SERVER_FETCH_ENABLED", false)
	c.FetchAllowPrivate = boolv("REA_FETCH_ALLOW_PRIVATE_NETWORKS", false)
	c.AutoMigrate = boolv("REA_AUTO_MIGRATE", true)
	c.IntegrationRateLimit = intv("REA_INTEGRATION_RATE_LIMIT", 600, &errs)
	c.CatalogChangeRetention = time.Duration(intv("REA_CATALOG_CHANGES_RETENTION_DAYS", 90, &errs)) * 24 * time.Hour
	c.WebhookAllowPrivate = boolv("REA_WEBHOOK_ALLOW_PRIVATE_NETWORKS", false)
	c.WebhookAllowHTTPLocalhost = boolv("REA_WEBHOOK_ALLOW_HTTP_LOCALHOST", false)
	c.WebhookTimeout = dur("REA_WEBHOOK_TIMEOUT", 10*time.Second, &errs)
	c.WebhookMaxAttempts = intv("REA_WEBHOOK_MAX_ATTEMPTS", 10, &errs)
	c.WebhookRetryBase = dur("REA_WEBHOOK_RETRY_BASE", time.Minute, &errs)
	c.WebhookRetryMax = dur("REA_WEBHOOK_RETRY_MAX", 6*time.Hour, &errs)
	c.WebhookDisableAfterFailures = intv("REA_WEBHOOK_DISABLE_AFTER_FAILURES", 20, &errs)
	c.WebhookDisableFailingFor = dur("REA_WEBHOOK_DISABLE_FAILING_FOR", time.Hour, &errs)
	c.WebhookConcurrency = intv("REA_WEBHOOK_CONCURRENCY", 2, &errs)
	c.WebhookLogRetention = time.Duration(intv("REA_WEBHOOK_LOG_RETENTION_DAYS", 30, &errs)) * 24 * time.Hour
	if c.WebhookMaxAttempts > 50 {
		errs = append(errs, errors.New("REA_WEBHOOK_MAX_ATTEMPTS must be at most 50"))
	}
	if c.WebhookTimeout > 2*time.Minute {
		errs = append(errs, errors.New("REA_WEBHOOK_TIMEOUT must be at most 2m"))
	}
	if c.WorkerMode != "embedded" && c.WorkerMode != "off" {
		errs = append(errs, errors.New("REA_WORKER_MODE must be embedded or off"))
	}
	for _, p := range splitList(env("REA_TRUSTED_PROXIES", "")) {
		pr, err := parsePrefix(p)
		if err != nil {
			errs = append(errs, fmt.Errorf("REA_TRUSTED_PROXIES: %w", err))
			continue
		}
		c.TrustedProxies = append(c.TrustedProxies, pr)
	}
	for _, p := range splitList(env("REA_REPO_ROOTS", "")) {
		abs, err := filepath.Abs(p)
		if err != nil {
			errs = append(errs, fmt.Errorf("REA_REPO_ROOTS: %w", err))
			continue
		}
		c.RepoRoots = append(c.RepoRoots, filepath.Clean(abs))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return c, nil
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		return netip.ParsePrefix(s)
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// IsTrustedProxy reports whether a remote address belongs to a configured reverse proxy.
func (c *Config) IsTrustedProxy(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	a = a.Unmap()
	for _, p := range c.TrustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func dur(k string, def time.Duration, errs *[]error) time.Duration {
	v := env(k, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		*errs = append(*errs, fmt.Errorf("%s must be a positive duration such as 24h", k))
		return def
	}
	return d
}

func intv(k string, def int, errs *[]error) int {
	v := env(k, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		*errs = append(*errs, fmt.Errorf("%s must be a positive integer", k))
		return def
	}
	return n
}

func boolv(k string, def bool) bool {
	v := strings.ToLower(env(k, ""))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}
