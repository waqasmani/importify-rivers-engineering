package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/migrate"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

func contextWith(ctx context.Context, k ctxKey, v any) context.Context {
	return context.WithValue(ctx, k, v)
}

func (s *Server) mountIdentity(r chi.Router) {
	r.Post("/auth/logout", s.h(s.handleLogout))
	r.Get("/auth/me", s.h(s.handleMe))
	r.With(s.requireSession).Put("/auth/me", s.h(s.handleUpdateMe))
	r.With(s.requireSession).Post("/auth/password", s.h(s.handleChangePassword))
	r.With(s.requireSession).Get("/auth/sessions", s.h(s.handleListSessions))
	r.With(s.requireSession).Delete("/auth/sessions/{id}", s.h(s.handleRevokeSession))
	r.With(s.requireSession).Get("/auth/tokens", s.h(s.handleListTokens))
	r.With(s.requireSession).Post("/auth/tokens", s.h(s.handleCreateToken))
	r.With(s.requireSession).Delete("/auth/tokens/{id}", s.h(s.handleRevokeToken))

	r.Get("/system/info", s.h(s.handleSystemInfo))
	r.Get("/permissions", s.h(s.handleListPermissions))
	r.Get("/users/directory", s.h(s.handleUserDirectory))

	r.Group(func(r chi.Router) {
		r.Use(s.requirePerm(auth.PermUsersManage), s.requireSession)
		r.Get("/users", s.h(s.handleListUsers))
		r.Post("/users", s.h(s.handleCreateUser))
		r.Get("/users/{id}", s.h(s.handleGetUser))
		r.Put("/users/{id}", s.h(s.handleUpdateUser))
		r.Delete("/users/{id}", s.h(s.handleDeleteUser))
		r.Post("/users/{id}/password", s.h(s.handleResetUserPassword))
		r.Post("/users/{id}/transfer-ownership", s.h(s.handleTransferOwnership))
		r.Post("/users/{id}/unlock", s.h(s.handleUnlockUser))
		r.Get("/roles", s.h(s.handleListRoles))
		r.Post("/roles", s.h(s.handleCreateRole))
		r.Put("/roles/{id}", s.h(s.handleUpdateRole))
		r.Delete("/roles/{id}", s.h(s.handleDeleteRole))
	})

	r.Get("/settings", s.h(s.handleGetSettings))
	r.With(s.requirePerm(auth.PermSettingsManage), s.requireSession).Put("/settings/{key}", s.h(s.handlePutSetting))

	r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/workspaces", s.h(s.handleListWorkspaces))
	r.With(s.requirePerm(auth.PermAnalysisWrite)).Post("/workspaces", s.h(s.handleCreateWorkspace))
	r.With(s.requirePerm(auth.PermAnalysisWrite)).Put("/workspaces/{id}", s.h(s.handleUpdateWorkspace))
	r.With(s.requirePerm(auth.PermAnalysisDelete)).Delete("/workspaces/{id}", s.h(s.handleDeleteWorkspace))

	r.With(s.requirePerm(auth.PermAuditRead)).Get("/audit-logs", s.h(s.handleListAuditLogs))
	r.Get("/activity", s.h(s.handleRecordActivity))

	s.mountExtension(r)
}

// ---- health ----

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) error {
	return ok(w, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.db.Pool.Ping(ctx); err != nil {
		return errUnavailable("database_unavailable", "The database is not reachable.")
	}
	st, err := migrate.GetStatus(ctx, s.cfg.DatabaseURL)
	if err != nil {
		return errUnavailable("migrations_unknown", "Could not read the migration state.")
	}
	if st.Pending > 0 {
		return errUnavailable("migrations_pending", "Database migrations are pending.")
	}
	return ok(w, map[string]any{"status": "ready", "schema_version": st.Current})
}

// ---- setup ----

// PrepareSetup creates the one-time setup token when no users exist. The token is written to a
// file readable only by the service account, never to logs.
func (s *Server) PrepareSetup(ctx context.Context) error {
	n, err := s.db.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.cfg.SetupToken != "" {
		s.setupToken = s.cfg.SetupToken
		s.log.Info("initial setup required; use the REA_SETUP_TOKEN value configured for this server")
		return nil
	}
	s.setupToken = auth.NewToken("")[:24]
	p := filepath.Join(s.cfg.DataDir, "setup-token")
	if err := os.MkdirAll(s.cfg.DataDir, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(s.setupToken+"\n"), 0o600); err != nil {
		return err
	}
	abs, _ := filepath.Abs(p)
	s.log.Info("initial setup required; the one-time setup token was written to a file", slog.String("path", abs))
	return nil
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) error {
	n, err := s.db.CountUsers(r.Context())
	if err != nil {
		return err
	}
	hint := "Read the token from the setup-token file in the server data directory, or use REA_SETUP_TOKEN."
	return ok(w, map[string]any{"needs_setup": n == 0, "setup_token_hint": hint})
}

type setupInput struct {
	SetupToken       string `json:"setup_token"`
	OrganizationName string `json:"organization_name"`
	DisplayName      string `json:"display_name"`
	Email            string `json:"email"`
	Password         string `json:"password"`
	WorkspaceName    string `json:"workspace_name"`
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) error {
	var in setupInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.DisplayName = normalize.Text(in.DisplayName)
	in.OrganizationName = normalize.Text(in.OrganizationName)
	in.WorkspaceName = normalize.Text(in.WorkspaceName)
	if in.WorkspaceName == "" {
		in.WorkspaceName = "General"
	}
	v := valid.New()
	v.Len("organization_name", in.OrganizationName, 1, 120)
	v.Len("display_name", in.DisplayName, 1, 120)
	v.Required("email", in.Email)
	v.Email("email", in.Email)
	if msg := auth.ValidatePassword(in.Password); msg != "" {
		v.Add("password", msg)
	}
	v.Len("workspace_name", in.WorkspaceName, 1, 120)
	s.setupMu.Lock()
	token := s.setupToken
	s.setupMu.Unlock()
	if token == "" || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(in.SetupToken)), []byte(token)) != 1 {
		v.Add("setup_token", "is incorrect")
	}
	if err := checkValid(v); err != nil {
		if _, bad := v.Errs["setup_token"]; bad {
			n, cerr := s.db.CountUsers(r.Context())
			if cerr == nil && n > 0 {
				return errConflict("already_set_up", "This server has already been set up. Sign in instead.")
			}
		}
		return err
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	var user store.User
	err = s.db.Tx(r.Context(), func(q *store.Queries, tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(7311001)"); err != nil {
			return err
		}
		n, err := q.CountUsers(r.Context())
		if err != nil {
			return err
		}
		if n > 0 {
			return errConflict("already_set_up", "This server has already been set up. Sign in instead.")
		}
		role, err := q.GetRoleByKey(r.Context(), "admin")
		if err != nil {
			return err
		}
		user, err = q.CreateUser(r.Context(), store.CreateUserParams{Email: in.Email, DisplayName: in.DisplayName,
			PasswordHash: hash, RoleID: role.ID, IsOwner: true})
		if err != nil {
			return err
		}
		org, _ := json.Marshal(map[string]string{"name": in.OrganizationName})
		if _, err := q.UpsertSetting(r.Context(), store.UpsertSettingParams{Key: "organization", Value: org, UpdatedBy: &user.ID}); err != nil {
			return err
		}
		if _, err := q.CreateWorkspace(r.Context(), store.CreateWorkspaceParams{Name: in.WorkspaceName,
			Slug: normalize.Slug(in.WorkspaceName), CreatedBy: &user.ID}); err != nil {
			return err
		}
		return q.InsertAuditLog(r.Context(), store.InsertAuditLogParams{ActorUserID: &user.ID, ActorLabel: user.Email,
			Action: "system.setup", EntityType: "user", EntityID: &user.ID, Summary: "Initial owner account created",
			IpAddress: clientIP(r), RequestID: requestID(r)})
	})
	if err != nil {
		return err
	}
	s.setupMu.Lock()
	s.setupToken = ""
	s.setupMu.Unlock()
	_ = os.Remove(filepath.Join(s.cfg.DataDir, "setup-token"))
	if err := s.startSession(w, r, user); err != nil {
		return err
	}
	return s.handleMeFor(w, r, user, true)
}

// ---- sessions ----

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user store.User) error {
	token := auth.NewToken(auth.PrefixSession)
	csrf := auth.NewToken("")
	sess, err := s.db.CreateSession(r.Context(), store.CreateSessionParams{UserID: user.ID, TokenHash: auth.HashToken(token),
		CsrfToken: csrf, IpAddress: clientIP(r), UserAgent: truncate(r.UserAgent(), 300), ExpiresAt: time.Now().Add(s.cfg.SessionTTL)})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: s.sessionCookieName(), Value: token, Path: "/", HttpOnly: true,
		Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: int(s.cfg.SessionTTL.Seconds())})
	// Make the new session visible to the response builder.
	perms, err := s.db.PermissionsForRole(r.Context(), user.RoleID)
	if err != nil {
		return err
	}
	pm := map[string]bool{}
	for _, p := range perms {
		pm[p] = true
	}
	*r = *r.WithContext(withPrincipal(r.Context(), &Principal{User: user, Perms: pm, Method: "session", SessionID: sess.ID, CSRFToken: csrf}))
	return nil
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

const (
	maxLoginFailures = 10
	lockMinutes      = 15
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) error {
	var in loginInput
	if err := decode(r, &in); err != nil {
		return err
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || in.Password == "" || len(in.Password) > 256 {
		return errValidation(valid.Errors{"email": "and password are required"})
	}
	invalid := &APIError{Status: http.StatusUnauthorized, Code: "invalid_credentials", Message: "The e-mail address or password is incorrect."}
	user, err := s.db.GetUserByEmail(r.Context(), email)
	if store.IsNotFound(err) {
		auth.DummyVerify(in.Password)
		_ = s.audit(r.Context(), nil, r, auditEvent{Action: "auth.login_failed", Summary: "Unknown account " + truncate(email, 100)})
		return invalid
	}
	if err != nil {
		return err
	}
	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		auth.DummyVerify(in.Password)
		return &APIError{Status: http.StatusUnauthorized, Code: "account_locked",
			Message: "Too many failed sign-in attempts. Try again in " + lockMinutesLeft(*user.LockedUntil) + "."}
	}
	if err := auth.VerifyPassword(in.Password, user.PasswordHash); err != nil {
		_, _ = s.db.RecordLoginFailure(r.Context(), store.RecordLoginFailureParams{ID: user.ID, MaxFailures: maxLoginFailures, LockMinutes: lockMinutes})
		_ = s.audit(r.Context(), nil, r, auditEvent{Action: "auth.login_failed", EntityType: "user", EntityID: &user.ID, Summary: "Wrong password"})
		return invalid
	}
	if !user.IsActive {
		return &APIError{Status: http.StatusUnauthorized, Code: "account_disabled", Message: "This account is disabled. Contact an administrator."}
	}
	if err := s.db.RecordLoginSuccess(r.Context(), user.ID); err != nil {
		return err
	}
	if err := s.startSession(w, r, user); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "auth.login", EntityType: "user", EntityID: &user.ID, Summary: "Signed in"})
	return s.handleMeFor(w, r, user, false)
}

func lockMinutesLeft(t time.Time) string {
	m := int(time.Until(t).Minutes()) + 1
	if m <= 1 {
		return "a minute"
	}
	return strconv.Itoa(m) + " minutes"
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) error {
	p := principal(r)
	if p.Method == "session" {
		if err := s.db.RevokeSession(r.Context(), p.SessionID); err != nil {
			return err
		}
	}
	http.SetCookie(w, &http.Cookie{Name: s.sessionCookieName(), Value: "", Path: "/", HttpOnly: true,
		Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "auth.logout", EntityType: "user", EntityID: &p.User.ID, Summary: "Signed out"})
	return noContent(w)
}

// UserView is the public representation of a user.
type UserView struct {
	ID          uuid.UUID  `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	RoleID      uuid.UUID  `json:"role_id"`
	RoleKey     string     `json:"role_key,omitempty"`
	RoleName    string     `json:"role_name,omitempty"`
	IsOwner     bool       `json:"is_owner"`
	IsActive    bool       `json:"is_active"`
	LastLoginAt *time.Time `json:"last_login_at"`
	LockedUntil *time.Time `json:"locked_until"`
	CreatedAt   time.Time  `json:"created_at"`
}

func userView(u store.User) UserView {
	return UserView{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, RoleID: u.RoleID, IsOwner: u.IsOwner,
		IsActive: u.IsActive, LastLoginAt: u.LastLoginAt, LockedUntil: u.LockedUntil, CreatedAt: u.CreatedAt}
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) error {
	return s.handleMeFor(w, r, principal(r).User, false)
}

func (s *Server) handleMeFor(w http.ResponseWriter, r *http.Request, user store.User, isNew bool) error {
	p := principal(r)
	role, err := s.db.GetRole(r.Context(), user.RoleID)
	if err != nil {
		return err
	}
	uv := userView(user)
	uv.RoleKey, uv.RoleName = role.Key, role.Name
	org := s.orgName(r.Context())
	resp := map[string]any{"user": uv, "permissions": p.PermList(), "auth_method": p.Method, "organization": org}
	if p.Method == "session" {
		resp["csrf_token"] = p.CSRFToken
	}
	if p.DeviceID != nil {
		resp["device_id"] = p.DeviceID
	}
	if isNew {
		return created(w, resp)
	}
	return ok(w, resp)
}

func (s *Server) orgName(ctx context.Context) string {
	st, err := s.db.GetSetting(ctx, "organization")
	if err != nil {
		return ""
	}
	var o struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(st.Value, &o)
	return o.Name
}

type updateMeInput struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) error {
	var in updateMeInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.DisplayName = normalize.Text(in.DisplayName)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	v := valid.New()
	v.Len("display_name", in.DisplayName, 1, 120)
	v.Required("email", in.Email)
	v.Email("email", in.Email)
	if err := checkValid(v); err != nil {
		return err
	}
	p := principal(r)
	u, err := s.db.UpdateUserProfile(r.Context(), store.UpdateUserProfileParams{ID: p.User.ID, DisplayName: in.DisplayName, Email: in.Email})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "user.update_profile", EntityType: "user", EntityID: &u.ID,
		Summary: "Updated own profile", Before: userView(p.User), After: userView(u)})
	return s.handleMeFor(w, r, u, false)
}

type changePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) error {
	var in changePasswordInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p := principal(r)
	if err := auth.VerifyPassword(in.CurrentPassword, p.User.PasswordHash); err != nil {
		return errField("current_password", "is incorrect")
	}
	if msg := auth.ValidatePassword(in.NewPassword); msg != "" {
		return errField("new_password", msg)
	}
	hash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		return err
	}
	if err := s.db.UpdateUserPassword(r.Context(), store.UpdateUserPasswordParams{ID: p.User.ID, PasswordHash: hash}); err != nil {
		return err
	}
	if err := s.db.RevokeUserSessionsExcept(r.Context(), store.RevokeUserSessionsExceptParams{UserID: p.User.ID, ID: p.SessionID}); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "user.change_password", EntityType: "user", EntityID: &p.User.ID,
		Summary: "Changed own password; other sessions were signed out"})
	return noContent(w)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) error {
	p := principal(r)
	rows, err := s.db.ListUserSessions(r.Context(), p.User.ID)
	if err != nil {
		return err
	}
	out := make([]SessionView, 0, len(rows))
	for _, s := range rows {
		out = append(out, SessionView{s.ID, s.IpAddress, s.UserAgent, s.CreatedAt, s.LastSeenAt, s.ExpiresAt, s.ID == p.SessionID})
	}
	return ok(w, map[string]any{"items": out})
}

func (s *Server) handleRevokeSession(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	p := principal(r)
	rows, err := s.db.ListUserSessions(r.Context(), p.User.ID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(rows, func(x store.UserSession) bool { return x.ID == id }) {
		return errNotFound("Session")
	}
	if err := s.db.RevokeSession(r.Context(), id); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "auth.session_revoked", EntityType: "user", EntityID: &p.User.ID, Summary: "Revoked a session"})
	return noContent(w)
}

// ---- personal access tokens ----

type tokenView struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func credView(c store.ApiCredential) tokenView {
	return tokenView{c.ID, c.Name, c.TokenPrefix, c.Scopes, c.ExpiresAt, c.LastUsedAt, c.CreatedAt}
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListPersonalTokens(r.Context(), principal(r).User.ID)
	if err != nil {
		return err
	}
	out := make([]tokenView, 0, len(rows))
	for _, c := range rows {
		out = append(out, credView(c))
	}
	return ok(w, map[string]any{"items": out})
}

type createTokenInput struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays int      `json:"expires_in_days"`
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) error {
	var in createTokenInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p := principal(r)
	in.Name = normalize.Text(in.Name)
	v := valid.New()
	v.Len("name", in.Name, 1, 80)
	v.Check(in.ExpiresInDays >= 1 && in.ExpiresInDays <= 365, "expires_in_days", "must be between 1 and 365")
	v.Check(len(in.Scopes) > 0, "scopes", "must include at least one permission")
	for _, sc := range in.Scopes {
		if !auth.IsKnownPermission(sc) {
			v.Add("scopes", "contains an unknown permission: "+sc)
		} else if !p.Can(sc) {
			v.Add("scopes", "cannot include a permission you do not have: "+sc)
		} else if sc == auth.PermUsersManage || sc == auth.PermBackupsManage {
			v.Add("scopes", sc+" cannot be granted to an API token")
		}
	}
	if err := checkValid(v); err != nil {
		return err
	}
	token := auth.NewToken(auth.PrefixPAT)
	c, err := s.db.CreateAPICredential(r.Context(), store.CreateAPICredentialParams{UserID: p.User.ID, Name: in.Name,
		Kind: "personal", TokenHash: auth.HashToken(token), TokenPrefix: auth.DisplayPrefix(token), Scopes: in.Scopes,
		ExpiresAt: time.Now().Add(time.Duration(in.ExpiresInDays) * 24 * time.Hour)})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "auth.token_created", EntityType: "user", EntityID: &p.User.ID,
		Summary: "Created API token " + in.Name, Extra: map[string]any{"scopes": in.Scopes}})
	return created(w, map[string]any{"token": token, "credential": credView(c)})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	p := principal(r)
	rows, err := s.db.ListPersonalTokens(r.Context(), p.User.ID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(rows, func(c store.ApiCredential) bool { return c.ID == id }) {
		return errNotFound("Token")
	}
	if _, err := s.db.RevokeCredential(r.Context(), id); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "auth.token_revoked", EntityType: "user", EntityID: &p.User.ID, Summary: "Revoked an API token"})
	return noContent(w)
}

// ---- system ----

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) error {
	st, err := migrate.GetStatus(r.Context(), s.cfg.DatabaseURL)
	if err != nil {
		return err
	}
	providers, err := s.db.CountEnabledAIProviders(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{
		"version":                  s.cfg.Version,
		"schema_version":           st.Current,
		"public_url":               s.cfg.PublicURL.String(),
		"worker_mode":              s.cfg.WorkerMode,
		"server_fetch":             s.cfg.ServerFetch,
		"repository_paths_enabled": len(s.cfg.RepoRoots) > 0,
		"ai_providers":             providers,
		"max_upload_mb":            s.cfg.MaxUploadBytes >> 20,
		"extension_origins":        s.cfg.ExtensionOrigins,
		"telemetry":                "none",
	})
}

func (s *Server) handleListPermissions(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListPermissions(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

// handleUserDirectory lists active users with minimal fields so any signed-in user can pick an
// owner or assignee.
func (s *Server) handleUserDirectory(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListUsers(r.Context(), store.ListUsersParams{Lim: 1000, Off: 0})
	if err != nil {
		return err
	}
	out := []DirectoryUser{}
	for _, u := range rows {
		if u.User.IsActive {
			out = append(out, DirectoryUser{u.User.ID, u.User.DisplayName, u.User.Email})
		}
	}
	return ok(w, map[string]any{"items": out})
}

// ---- users ----

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	q := qText(r, "q")
	if q != nil {
		esc := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(*q)
		q = &esc
	}
	rows, err := s.db.ListUsers(r.Context(), store.ListUsersParams{Q: q, Lim: limit, Off: offset})
	if err != nil {
		return err
	}
	total, err := s.db.CountUsersMatching(r.Context(), q)
	if err != nil {
		return err
	}
	out := make([]UserView, 0, len(rows))
	for _, row := range rows {
		uv := userView(row.User)
		uv.RoleKey, uv.RoleName = row.RoleKey, row.RoleName
		out = append(out, uv)
	}
	return ok(w, Page[UserView]{Items: out, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	u, err := s.db.GetUserByID(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, userView(u))
}

type createUserInput struct {
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Password    string    `json:"password"`
	RoleID      uuid.UUID `json:"role_id"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) error {
	var in createUserInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.DisplayName = normalize.Text(in.DisplayName)
	v := valid.New()
	v.Required("email", in.Email)
	v.Email("email", in.Email)
	v.Len("display_name", in.DisplayName, 1, 120)
	if msg := auth.ValidatePassword(in.Password); msg != "" {
		v.Add("password", msg)
	}
	if err := checkValid(v); err != nil {
		return err
	}
	if _, err := s.db.GetRole(r.Context(), in.RoleID); err != nil {
		return errField("role_id", "does not exist")
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	var u store.User
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		var err error
		u, err = q.CreateUser(r.Context(), store.CreateUserParams{Email: in.Email, DisplayName: in.DisplayName, PasswordHash: hash, RoleID: in.RoleID})
		if err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "user.create", EntityType: "user", EntityID: &u.ID,
			Summary: "Created user " + u.Email, After: userView(u)})
	})
	if err != nil {
		return err
	}
	return created(w, userView(u))
}

type updateUserInput struct {
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email,omitempty"` // empty keeps the current address
	RoleID      uuid.UUID `json:"role_id"`
	IsActive    bool      `json:"is_active"`
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in updateUserInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.DisplayName = normalize.Text(in.DisplayName)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	p := principal(r)
	before, err := s.db.GetUserByID(r.Context(), id)
	if err != nil {
		return err
	}
	if in.Email == "" {
		in.Email = before.Email
	}
	v := valid.New()
	v.Len("display_name", in.DisplayName, 1, 120)
	v.Email("email", in.Email)
	if err := checkValid(v); err != nil {
		return err
	}
	role, err := s.db.GetRole(r.Context(), in.RoleID)
	if err != nil {
		return errField("role_id", "does not exist")
	}
	if before.IsOwner {
		if !p.User.IsOwner {
			return errForbidden("Only the owner can change the owner account.")
		}
		if !in.IsActive || role.Key != "admin" {
			return errConflict("owner_protected", "The owner must stay active with the Administrator role. Transfer ownership first.")
		}
	}
	if id == p.User.ID && (!in.IsActive || role.ID != before.RoleID) {
		return errConflict("self_protected", "You cannot deactivate yourself or change your own role.")
	}
	var u store.User
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		u, err = q.UpdateUserAdmin(r.Context(), store.UpdateUserAdminParams{ID: id, DisplayName: in.DisplayName, RoleID: in.RoleID, IsActive: in.IsActive, Email: in.Email})
		if err != nil {
			return err
		}
		if !u.IsActive {
			if err := q.RevokeUserSessions(r.Context(), id); err != nil {
				return err
			}
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "user.update", EntityType: "user", EntityID: &id,
			Summary: "Updated user " + u.Email, Before: userView(before), After: userView(u)})
	})
	if err != nil {
		return err
	}
	return ok(w, userView(u))
}

// handleUnlockUser clears a sign-in lockout caused by repeated failed passwords.
func (s *Server) handleUnlockUser(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	u, err := s.db.GetUserByID(r.Context(), id)
	if err != nil {
		return err
	}
	if u.IsOwner && !principal(r).User.IsOwner {
		return errForbidden("Only the owner can change the owner account.")
	}
	if _, err := s.db.UnlockUser(r.Context(), id); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "user.unlock", EntityType: "user", EntityID: &id, Summary: "Cleared the sign-in lockout of " + u.Email})
	return noContent(w)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	if id == principal(r).User.ID {
		return errConflict("self_protected", "You cannot delete your own account.")
	}
	before, err := s.db.GetUserByID(r.Context(), id)
	if err != nil {
		return err
	}
	if before.IsOwner {
		return errConflict("owner_protected", "The owner account cannot be deleted. Transfer ownership first.")
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		if err := q.DeleteUser(r.Context(), id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "user.delete", EntityType: "user", EntityID: &id,
			Summary: "Deleted user " + before.Email, Before: userView(before)})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

type resetPasswordInput struct {
	Password string `json:"password"`
}

func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in resetPasswordInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if msg := auth.ValidatePassword(in.Password); msg != "" {
		return errField("password", msg)
	}
	u, err := s.db.GetUserByID(r.Context(), id)
	if err != nil {
		return err
	}
	if u.IsOwner && !principal(r).User.IsOwner {
		return errForbidden("Only the owner can reset the owner's password.")
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		if err := q.UpdateUserPassword(r.Context(), store.UpdateUserPasswordParams{ID: id, PasswordHash: hash}); err != nil {
			return err
		}
		if err := q.RevokeUserSessions(r.Context(), id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "user.reset_password", EntityType: "user", EntityID: &id,
			Summary: "Reset password for " + u.Email + "; their sessions were signed out"})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) handleTransferOwnership(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	p := principal(r)
	if !p.User.IsOwner {
		return errForbidden("Only the owner can transfer ownership.")
	}
	target, err := s.db.GetUserByID(r.Context(), id)
	if err != nil {
		return err
	}
	role, err := s.db.GetRole(r.Context(), target.RoleID)
	if err != nil {
		return err
	}
	if !target.IsActive || role.Key != "admin" {
		return errConflict("invalid_target", "Ownership can only be transferred to an active Administrator.")
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		if err := q.TransferOwnership(r.Context(), id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "user.transfer_ownership", EntityType: "user", EntityID: &id,
			Summary: "Transferred ownership to " + target.Email})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

// ---- roles ----

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListRoles(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

type roleInput struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func validateRole(in *roleInput, creating bool) error {
	in.Name = normalize.Text(in.Name)
	in.Description = normalize.Text(in.Description)
	v := valid.New()
	if creating {
		v.Check(len(in.Key) >= 2 && len(in.Key) <= 41 && in.Key == strings.ToLower(in.Key) && normalize.Slug(in.Key) == strings.ReplaceAll(in.Key, "_", "-"),
			"key", "must be 2-41 lowercase letters, digits or underscores, starting with a letter")
	}
	v.Len("name", in.Name, 1, 80)
	v.Max("description", in.Description, 500)
	for _, p := range in.Permissions {
		if !auth.IsKnownPermission(p) {
			v.Add("permissions", "contains an unknown permission: "+p)
		}
	}
	return checkValid(v)
}

func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) error {
	var in roleInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := validateRole(&in, true); err != nil {
		return err
	}
	var role store.Role
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		var err error
		role, err = q.CreateRole(r.Context(), store.CreateRoleParams{Key: in.Key, Name: in.Name, Description: in.Description})
		if err != nil {
			return err
		}
		if err := q.AddRolePermissions(r.Context(), store.AddRolePermissionsParams{RoleID: role.ID, Codes: in.Permissions}); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "role.create", EntityType: "role", EntityID: &role.ID,
			Summary: "Created role " + role.Name, Extra: map[string]any{"permissions": in.Permissions}})
	})
	if err != nil {
		return err
	}
	return created(w, role)
}

func (s *Server) handleUpdateRole(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in roleInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := validateRole(&in, false); err != nil {
		return err
	}
	existing, err := s.db.GetRole(r.Context(), id)
	if err != nil {
		return err
	}
	if existing.Key == "admin" {
		return errConflict("role_protected", "The Administrator role always has every permission and cannot be edited.")
	}
	before, _ := s.db.PermissionsForRole(r.Context(), id)
	var role store.Role
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		var err error
		role, err = q.UpdateRole(r.Context(), store.UpdateRoleParams{ID: id, Name: in.Name, Description: in.Description})
		if err != nil {
			return err
		}
		if err := q.ClearRolePermissions(r.Context(), id); err != nil {
			return err
		}
		if err := q.AddRolePermissions(r.Context(), store.AddRolePermissionsParams{RoleID: id, Codes: in.Permissions}); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "role.update", EntityType: "role", EntityID: &id,
			Summary: "Updated role " + role.Name, Extra: map[string]any{"permissions_before": before, "permissions_after": in.Permissions}})
	})
	if err != nil {
		return err
	}
	return ok(w, role)
}

func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	role, err := s.db.GetRole(r.Context(), id)
	if err != nil {
		return err
	}
	n, err := s.db.DeleteRole(r.Context(), id)
	if err != nil {
		return err
	}
	if n == 0 {
		return errConflict("role_in_use", "System roles and roles assigned to users cannot be deleted.")
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "role.delete", EntityType: "role", EntityID: &id, Summary: "Deleted role " + role.Name})
	return noContent(w)
}

// ---- workspaces ----

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListWorkspaces(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

type workspaceInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

func (in *workspaceInput) validate() error {
	in.Name = normalize.Text(in.Name)
	in.Description = normalize.MultilineText(in.Description)
	if in.Slug == "" {
		in.Slug = normalize.Slug(in.Name)
	}
	v := valid.New()
	v.Len("name", in.Name, 1, 120)
	v.Check(normalize.Slug(in.Slug) == in.Slug, "slug", "must contain only lowercase letters, digits and hyphens")
	v.Max("description", in.Description, 2000)
	return checkValid(v)
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) error {
	var in workspaceInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	ws, err := s.db.CreateWorkspace(r.Context(), store.CreateWorkspaceParams{Name: in.Name, Slug: in.Slug, Description: in.Description, CreatedBy: userIDPtr(r)})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "workspace.create", EntityType: "workspace", EntityID: &ws.ID, Summary: "Created workspace " + ws.Name})
	return created(w, ws)
}

func (s *Server) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in workspaceInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	before, err := s.db.GetWorkspace(r.Context(), id)
	if err != nil {
		return err
	}
	ws, err := s.db.UpdateWorkspace(r.Context(), store.UpdateWorkspaceParams{ID: id, Name: in.Name, Slug: in.Slug, Description: in.Description})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "workspace.update", EntityType: "workspace", EntityID: &id,
		Summary: "Updated workspace " + ws.Name, Before: before, After: ws})
	return ok(w, ws)
}

func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	ws, err := s.db.GetWorkspace(r.Context(), id)
	if err != nil {
		return err
	}
	n, err := s.db.DeleteWorkspace(r.Context(), id)
	if err != nil {
		return err
	}
	if n == 0 {
		return errConflict("workspace_in_use", "Move or delete the workspace's analysis projects first.")
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "workspace.delete", EntityType: "workspace", EntityID: &id, Summary: "Deleted workspace " + ws.Name})
	return noContent(w)
}

// ---- audit ----

func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) error {
	actor, err := qUUID(r, "actor_user_id")
	if err != nil {
		return err
	}
	entityID, err := qUUID(r, "entity_id")
	if err != nil {
		return err
	}
	limit, _ := pagination(r)
	var before *int64
	if v := qInt(r, "before_id"); v != nil {
		b := int64(*v)
		before = &b
	}
	rows, err := s.db.ListAuditLogs(r.Context(), store.ListAuditLogsParams{EntityType: qText(r, "entity_type"), EntityID: entityID,
		ActorUserID: actor, Action: qText(r, "action"), BeforeID: before, Lim: limit})
	if err != nil {
		return err
	}
	var next *int64
	if len(rows) == int(limit) {
		next = &rows[len(rows)-1].ID
	}
	return ok(w, map[string]any{"items": rows, "next_before_id": next})
}

// entityReadPerm maps an entity type to the permission needed to read it.
func entityReadPerm(entityType string) string {
	switch entityType {
	case "company", "contact", "supplier", "lead", "opportunity", "task":
		return auth.PermCRMRead
	case "product", "captured_page", "product_import":
		return auth.PermProductsRead
	case "analysis_project", "analysis_source":
		return auth.PermAnalysisRead
	}
	return auth.PermAuditRead
}

// handleRecordActivity returns the audit timeline of one record.
func (s *Server) handleRecordActivity(w http.ResponseWriter, r *http.Request) error {
	et := r.URL.Query().Get("entity_type")
	id, err := qUUID(r, "entity_id")
	if err != nil {
		return err
	}
	if et == "" || id == nil {
		return errBadRequest("entity_type and entity_id are required.")
	}
	if err := s.can(r, entityReadPerm(et)); err != nil {
		return err
	}
	rows, err := s.db.ListAuditLogs(r.Context(), store.ListAuditLogsParams{EntityType: &et, EntityID: id, Lim: 200})
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}
