package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/testutil"
)

func TestSetupLoginAndSession(t *testing.T) {
	e := newEnv(t)

	// Setup cannot run twice.
	e.client().Do("POST", "/api/v1/setup", map[string]any{"setup_token": "test-setup-token", "organization_name": "X",
		"display_name": "Y", "email": "y@example.com", "password": ownerPassword}).Must(t, 409)
	st := e.client().Do("GET", "/api/v1/setup/status", nil).Must(t, 200)
	if st.Get("needs_setup") != false {
		t.Fatal("needs_setup should be false after setup")
	}

	me := e.owner.Do("GET", "/api/v1/auth/me", nil).Must(t, 200)
	if me.Str("user.email") != "owner@example.com" || me.Get("user.is_owner") != true || me.Str("organization") != "Acme Sourcing" {
		t.Fatalf("unexpected me: %s", me.Body)
	}

	// Wrong password and unknown user give the same error.
	bad := e.client().Do("POST", "/api/v1/auth/login", map[string]any{"email": "owner@example.com", "password": "nope-nope-nope"})
	unknown := e.client().Do("POST", "/api/v1/auth/login", map[string]any{"email": "ghost@example.com", "password": "nope-nope-nope"})
	if bad.Status != 401 || unknown.Status != 401 || bad.ErrorCode() != unknown.ErrorCode() {
		t.Fatalf("login errors leak account existence: %d/%s vs %d/%s", bad.Status, bad.ErrorCode(), unknown.Status, unknown.ErrorCode())
	}

	c := e.login("owner@example.com", ownerPassword)
	c.Do("POST", "/api/v1/auth/logout", nil).Must(t, 204)
	c.Do("GET", "/api/v1/auth/me", nil).Must(t, 401)
}

func TestSetupRequiresToken(t *testing.T) {
	d, url := testutil.OpenDB(t)
	_ = d
	cfg := testutil.Config(t, url)
	_ = cfg
	e := newEnvNoSetup(t, url)
	res := e.Do("POST", "/api/v1/setup", map[string]any{"setup_token": "wrong", "organization_name": "Acme",
		"display_name": "Olive", "email": "o@example.com", "password": ownerPassword}).Must(t, 422)
	if !strings.Contains(string(res.Body), "setup_token") {
		t.Fatalf("expected setup_token error: %s", res.Body)
	}
	weak := e.Do("POST", "/api/v1/setup", map[string]any{"setup_token": "test-setup-token", "organization_name": "Acme",
		"display_name": "Olive", "email": "o@example.com", "password": "short"}).Must(t, 422)
	if !strings.Contains(string(weak.Body), "password") {
		t.Fatalf("expected password error: %s", weak.Body)
	}
}

func TestCSRFAndAuthRequired(t *testing.T) {
	e := newEnv(t)
	e.client().Do("GET", "/api/v1/workspaces", nil).Must(t, 401)

	noCSRF := e.login("owner@example.com", ownerPassword)
	noCSRF.CSRF = ""
	res := noCSRF.Do("POST", "/api/v1/workspaces", map[string]any{"name": "Blocked"}).Must(t, 403)
	if res.ErrorCode() != "csrf_failed" {
		t.Fatalf("expected csrf_failed, got %s", res.ErrorCode())
	}
	// JSON content type is required for JSON endpoints.
	e.owner.Headers["Content-Type"] = "text/plain"
	e.owner.Do("POST", "/api/v1/workspaces", map[string]any{"name": "Form post"}).Must(t, 415)
	delete(e.owner.Headers, "Content-Type")
	// Unknown fields are rejected.
	e.owner.Do("POST", "/api/v1/workspaces", map[string]any{"name": "Ok", "evil": true}).Must(t, 400)
}

func TestSecurityHeadersAndCORS(t *testing.T) {
	e := newEnv(t)
	res := e.owner.Do("GET", "/api/v1/auth/me", nil).Must(t, 200)
	for _, h := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy", "Referrer-Policy"} {
		if res.Header.Get(h) == "" {
			t.Errorf("missing header %s", h)
		}
	}
	// Allowed origin gets credentialed CORS; unknown origin gets none.
	c := e.client()
	c.Headers["Origin"] = "http://localhost:5173"
	r1 := c.Do("GET", "/api/v1/setup/status", nil)
	if r1.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" || r1.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("allowed origin not reflected: %v", r1.Header)
	}
	c.Headers["Origin"] = "https://evil.example"
	r2 := c.Do("GET", "/api/v1/setup/status", nil)
	if r2.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unknown origin must not receive CORS headers")
	}
	// The pinned extension origin gets bearer-only CORS.
	c.Headers["Origin"] = "chrome-extension://olceibeplclcegianhjffambadoakkno"
	r3 := c.Do("GET", "/api/v1/setup/status", nil)
	if r3.Header.Get("Access-Control-Allow-Origin") == "" || r3.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("extension origin CORS wrong: %v", r3.Header)
	}
}

func TestRolePermissions(t *testing.T) {
	e := newEnv(t)
	viewer := e.userWithRole("viewer", "viewer@example.com")
	viewer.Do("GET", "/api/v1/users", nil).Must(t, 403)
	viewer.Do("POST", "/api/v1/workspaces", map[string]any{"name": "Nope"}).Must(t, 403)
	viewer.Do("GET", "/api/v1/workspaces", nil).Must(t, 200)
	viewer.Do("PUT", "/api/v1/settings/organization", map[string]any{"name": "Hijack"}).Must(t, 403)

	// The owner cannot be deleted or demoted by an admin.
	admin := e.userWithRole("admin", "admin@example.com")
	ownerID := e.owner.Do("GET", "/api/v1/auth/me", nil).Str("user.id")
	admin.Do("DELETE", "/api/v1/users/"+ownerID, nil).Must(t, 409)

	// Deactivating a user ends their session immediately.
	users := e.owner.Do("GET", "/api/v1/users", nil).Must(t, 200)
	for _, u := range users.Items() {
		if u["email"] == "viewer@example.com" {
			e.owner.Do("PUT", "/api/v1/users/"+u["id"].(string), map[string]any{"display_name": "Viewer",
				"role_id": u["role_id"], "is_active": false}).Must(t, 200)
		}
	}
	viewer.Do("GET", "/api/v1/auth/me", nil).Must(t, 401)
}

func TestLoginLockoutAndRateLimit(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	codes := map[int]int{}
	for i := 0; i < 25; i++ {
		res := c.Do("POST", "/api/v1/auth/login", map[string]any{"email": "owner@example.com", "password": "wrong password value"})
		codes[res.Status]++
	}
	if codes[http.StatusTooManyRequests] == 0 {
		t.Fatalf("expected rate limiting on login, got %v", codes)
	}
}

func TestExtensionPairingRefreshAndRevocation(t *testing.T) {
	e := newEnv(t)
	ext, pair := e.pairExtension()
	cfg := ext.Do("GET", "/api/v1/extension/config", nil).Must(t, 200)
	perms, _ := cfg.Get("permissions").([]any)
	for _, p := range perms {
		if p == "users.manage" || p == "settings.manage" {
			t.Fatalf("extension token must not carry %v", p)
		}
	}
	// Extension tokens cannot manage users even though the owner can.
	ext.Do("GET", "/api/v1/users", nil).Must(t, 403)

	// The pairing code is single use.
	e.client().Do("POST", "/api/v1/extension/pair", map[string]any{"code": "AAAA-BBBB"}).Must(t, 401)

	// Refresh rotates tokens; reusing the old refresh token revokes the device.
	refresh := pair["refresh_token"].(string)
	r1 := e.client().Do("POST", "/api/v1/extension/token/refresh", map[string]any{"refresh_token": refresh}).Must(t, 200)
	newAccess := r1.Str("access_token")
	reuse := e.client().Do("POST", "/api/v1/extension/token/refresh", map[string]any{"refresh_token": refresh}).Must(t, 401)
	if reuse.ErrorCode() != "refresh_token_invalid" {
		t.Fatal(reuse.ErrorCode())
	}
	c2 := e.client()
	c2.Bearer = newAccess
	res := c2.Do("GET", "/api/v1/extension/config", nil).Must(t, 401)
	if res.ErrorCode() != "token_expired" {
		t.Fatalf("revoked device token should fail with token_expired, got %s", res.ErrorCode())
	}

	// Pair again and revoke from the web UI.
	ext2, _ := e.pairExtension()
	devices := e.owner.Do("GET", "/api/v1/extension/devices", nil).Must(t, 200).Items()
	for _, d := range devices {
		if d["revoked_at"] == nil {
			e.owner.Do("DELETE", "/api/v1/extension/devices/"+d["id"].(string), nil).Must(t, 204)
		}
	}
	ext2.Do("GET", "/api/v1/extension/config", nil).Must(t, 401)

	// A refresh token can never be used as a bearer token.
	c3 := e.client()
	c3.Bearer = refresh
	c3.Do("GET", "/api/v1/extension/config", nil).Must(t, 401)
}

func TestAuditLogRecordsActions(t *testing.T) {
	e := newEnv(t)
	e.owner.Do("POST", "/api/v1/workspaces", map[string]any{"name": "Audit me"}).Must(t, 201)
	e.owner.Do("POST", "/api/v1/auth/password", map[string]any{"current_password": ownerPassword, "new_password": "another long passphrase 99"}).Must(t, 204)
	logs := e.owner.Do("GET", "/api/v1/audit-logs", nil).Must(t, 200)
	actions := map[string]bool{}
	for _, it := range logs.Items() {
		actions[it["action"].(string)] = true
		if strings.Contains(string(logs.Body), "another long passphrase") {
			t.Fatal("password leaked into audit log")
		}
	}
	for _, a := range []string{"system.setup", "workspace.create", "user.change_password"} {
		if !actions[a] {
			t.Errorf("missing audit action %s", a)
		}
	}
}

func TestSettingsValidation(t *testing.T) {
	e := newEnv(t)
	e.owner.Do("PUT", "/api/v1/settings/imports", map[string]any{"default_currency": "XXX", "auto_link_supplier_by_domain": true,
		"title_similarity_threshold": 0.5}).Must(t, 422)
	e.owner.Do("PUT", "/api/v1/settings/imports", map[string]any{"default_currency": "eur", "auto_link_supplier_by_domain": false,
		"title_similarity_threshold": 0.6}).Must(t, 200)
	s := e.owner.Do("GET", "/api/v1/settings", nil).Must(t, 200)
	if s.Str("imports.default_currency") != "EUR" {
		t.Fatalf("currency not normalized: %s", s.Body)
	}
	e.owner.Do("PUT", "/api/v1/settings/unknown", map[string]any{}).Must(t, 404)
}
