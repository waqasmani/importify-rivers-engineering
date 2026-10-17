package httpapi_test

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/httpapi"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/testutil"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/worker"
)

const ownerPassword = "correct horse battery staple 42"

type env struct {
	t      *testing.T
	cfg    *config.Config
	db     *store.DB
	srv    *httptest.Server
	api    *httpapi.Server
	worker *worker.Worker
	owner  *testutil.Client
}

// newEnv starts an API server on a fresh database and completes first-run setup.
func newEnv(t *testing.T) *env {
	t.Helper()
	d, url := testutil.OpenDB(t)
	cfg := testutil.Config(t, url)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	api, err := httpapi.New(cfg, d, log, httpapi.Deps{})
	if err != nil {
		t.Fatal(err)
	}
	w := worker.New(cfg, d, log, api.Box(), api.Files())
	if err := api.PrepareSetup(t.Context()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.Routes())
	t.Cleanup(srv.Close)
	e := &env{t: t, cfg: cfg, db: d, srv: srv, api: api, worker: w}
	e.owner = e.client()
	res := e.owner.Do("POST", "/api/v1/setup", map[string]any{
		"setup_token": "test-setup-token", "organization_name": "Acme Sourcing", "display_name": "Olive Owner",
		"email": "owner@example.com", "password": ownerPassword,
	}).Must(t, 201)
	e.owner.CSRF = res.Str("csrf_token")
	return e
}

func (e *env) client() *testutil.Client {
	c := testutil.NewClient(e.t, e.srv)
	c.Validate = contractValidator(e.t)
	return c
}

// login signs a user in and returns a client holding the session.
func (e *env) login(email, password string) *testutil.Client {
	c := e.client()
	res := c.Do("POST", "/api/v1/auth/login", map[string]any{"email": email, "password": password}).Must(e.t, 200)
	c.CSRF = res.Str("csrf_token")
	return c
}

// userWithRole creates a user with a system role and signs them in.
func (e *env) userWithRole(roleKey, email string) *testutil.Client {
	e.t.Helper()
	roles := e.owner.Do("GET", "/api/v1/roles", nil).Must(e.t, 200)
	var roleID string
	for _, r := range roles.Items() {
		if r["key"] == roleKey {
			roleID = r["id"].(string)
		}
	}
	if roleID == "" {
		e.t.Fatalf("role %s not found", roleKey)
	}
	e.owner.Do("POST", "/api/v1/users", map[string]any{"email": email, "display_name": roleKey + " user",
		"password": ownerPassword, "role_id": roleID}).Must(e.t, 201)
	return e.login(email, ownerPassword)
}

// pairExtension pairs a device for the owner and returns a bearer client plus the token pair.
func (e *env) pairExtension() (*testutil.Client, map[string]any) {
	e.t.Helper()
	code := e.owner.Do("POST", "/api/v1/extension/pairing-codes", map[string]any{}).Must(e.t, 201).Str("code")
	anon := e.client()
	res := anon.Do("POST", "/api/v1/extension/pair", map[string]any{"code": code, "device_name": "Test Chrome",
		"browser": "Chrome 150", "extension_version": "1.0.0"}).Must(e.t, 201)
	c := e.client()
	c.Bearer = res.Str("access_token")
	return c, res.JSON
}
