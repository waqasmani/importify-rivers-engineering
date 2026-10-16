// Package testutil provides real PostgreSQL databases and HTTP clients for integration tests.
//
// Tests need REA_TEST_DATABASE_URL pointing at a PostgreSQL server where the role may create
// databases, for example postgres://rea:rea@127.0.0.1:5432/rea?sslmode=disable. Each test gets its
// own database cloned from a migrated template, so tests are isolated and can run in parallel.
package testutil

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/db"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/migrate"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// AdminURL returns the configured test server URL or skips the test.
func AdminURL(t testing.TB) string {
	u := os.Getenv("REA_TEST_DATABASE_URL")
	if u == "" {
		if os.Getenv("REA_REQUIRE_DB_TESTS") == "1" {
			t.Fatal("REA_TEST_DATABASE_URL is required (REA_REQUIRE_DB_TESTS=1)")
		}
		t.Skip("REA_TEST_DATABASE_URL is not set; skipping database integration test")
	}
	return u
}

func withDatabase(base, name string) string {
	u, err := url.Parse(base)
	if err != nil {
		panic(err)
	}
	u.Path = "/" + name
	return u.String()
}

func migrationsHash() string {
	h := sha256.New()
	entries, _ := fs.ReadDir(db.Migrations, "migrations")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, n := range names {
		b, _ := fs.ReadFile(db.Migrations, "migrations/"+n)
		h.Write([]byte(n))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func ensureTemplate(ctx context.Context, admin string) (string, error) {
	tmpl := "rea_tmpl_" + migrationsHash()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(7311002)"); err != nil {
		return "", err
	}
	defer func() { _, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock(7311002)") }()
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", tmpl).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return tmpl, nil
	}
	building := tmpl + "_build"
	_, _ = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+building)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+building); err != nil {
		return "", err
	}
	if _, err := migrate.Up(ctx, withDatabase(admin, building)); err != nil {
		return "", fmt.Errorf("migrate template: %w", err)
	}
	if _, err := conn.Exec(ctx, "ALTER DATABASE "+building+" RENAME TO "+tmpl); err != nil {
		return "", err
	}
	return tmpl, nil
}

// NewDatabase creates an isolated, fully migrated database and returns its URL.
func NewDatabase(t testing.TB) string {
	t.Helper()
	admin := AdminURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tmpl, err := ensureTemplate(ctx, admin)
	if err != nil {
		t.Fatalf("prepare template database: %v", err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "rea_test_" + hex.EncodeToString(b)
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	var lastErr error
	for i := 0; i < 20; i++ { // the template may briefly have connections from a concurrent clone
		if _, lastErr = conn.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE "+tmpl); lastErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("create test database: %v", lastErr)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), admin)
		if err != nil {
			return
		}
		defer c.Close(context.Background())
		_, _ = c.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return withDatabase(admin, name)
}

// OpenDB opens a store on a fresh database.
func OpenDB(t testing.TB) (*store.DB, string) {
	t.Helper()
	u := NewDatabase(t)
	d, err := store.Open(context.Background(), u)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(d.Close)
	return d, u
}

// Config returns a configuration suitable for tests.
func Config(t testing.TB, dbURL string) *config.Config {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	t.Setenv("REA_DATABASE_URL", dbURL)
	t.Setenv("REA_SECRET_KEY", base64.StdEncoding.EncodeToString(key))
	t.Setenv("REA_PUBLIC_URL", "http://rea.test")
	t.Setenv("REA_DATA_DIR", t.TempDir())
	t.Setenv("REA_SETUP_TOKEN", "test-setup-token")
	t.Setenv("REA_ALLOWED_ORIGINS", "http://localhost:5173")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	cfg.Version = "test"
	return cfg
}

// Lines splits text into trimmed non-empty lines (helper for fixture assertions).
func Lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
