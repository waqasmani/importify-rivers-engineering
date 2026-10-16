package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

const pairingCodeTTL = 10 * time.Minute

func (s *Server) mountExtension(r chi.Router) {
	r.With(s.requirePerm(auth.PermExtensionPair), s.requireSession).Post("/extension/pairing-codes", s.h(s.handleCreatePairingCode))
	r.With(s.requireSession).Get("/extension/devices", s.h(s.handleListDevices))
	r.With(s.requireSession).Delete("/extension/devices/{id}", s.h(s.handleRevokeDevice))
	r.Get("/extension/config", s.h(s.handleExtensionConfig))
	r.Post("/extension/disconnect", s.h(s.handleExtensionDisconnect))
}

func (s *Server) handleCreatePairingCode(w http.ResponseWriter, r *http.Request) error {
	code := auth.NewPairingCode()
	exp := time.Now().Add(pairingCodeTTL)
	if _, err := s.db.CreatePairingCode(r.Context(), store.CreatePairingCodeParams{UserID: principal(r).User.ID,
		CodeHash: auth.HashToken(auth.NormalizePairingCode(code)), ExpiresAt: exp}); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "extension.pairing_code_created", EntityType: "user", EntityID: userIDPtr(r),
		Summary: "Created a browser extension pairing code"})
	return created(w, map[string]any{"code": code, "expires_at": exp, "server_url": s.cfg.PublicURL.String()})
}

type pairInput struct {
	Code             string `json:"code"`
	DeviceName       string `json:"device_name"`
	Browser          string `json:"browser"`
	ExtensionVersion string `json:"extension_version"`
}

type tokenPair struct {
	AccessToken           string    `json:"access_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshToken          string    `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
	DeviceID              uuid.UUID `json:"device_id"`
	User                  UserView  `json:"user"`
	ServerVersion         string    `json:"server_version"`
	Organization          string    `json:"organization"`
}

func (s *Server) issueDeviceTokens(r *http.Request, q *store.Queries, user store.User, device store.ExtensionDevice) (tokenPair, error) {
	access := auth.NewToken(auth.PrefixAccess)
	refresh := auth.NewToken(auth.PrefixRefresh)
	now := time.Now()
	tp := tokenPair{AccessToken: access, AccessTokenExpiresAt: now.Add(s.cfg.AccessTokenTTL), RefreshToken: refresh,
		RefreshTokenExpiresAt: now.Add(s.cfg.RefreshTokenTTL), DeviceID: device.ID, User: userView(user),
		ServerVersion: s.cfg.Version, Organization: s.orgName(r.Context())}
	if _, err := q.CreateAPICredential(r.Context(), store.CreateAPICredentialParams{UserID: user.ID, DeviceID: &device.ID,
		Name: device.Name + " (access)", Kind: "access", TokenHash: auth.HashToken(access), TokenPrefix: auth.DisplayPrefix(access),
		Scopes: auth.ExtensionScopes, ExpiresAt: tp.AccessTokenExpiresAt}); err != nil {
		return tp, err
	}
	if _, err := q.CreateAPICredential(r.Context(), store.CreateAPICredentialParams{UserID: user.ID, DeviceID: &device.ID,
		Name: device.Name + " (refresh)", Kind: "refresh", TokenHash: auth.HashToken(refresh), TokenPrefix: auth.DisplayPrefix(refresh),
		Scopes: auth.ExtensionScopes, ExpiresAt: tp.RefreshTokenExpiresAt}); err != nil {
		return tp, err
	}
	return tp, nil
}

func (s *Server) handleExtensionPair(w http.ResponseWriter, r *http.Request) error {
	var in pairInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.DeviceName = normalize.Text(in.DeviceName)
	if in.DeviceName == "" {
		in.DeviceName = "Chrome extension"
	}
	v := valid.New()
	v.Len("device_name", in.DeviceName, 1, 120)
	v.Max("browser", in.Browser, 120)
	v.Max("extension_version", in.ExtensionVersion, 40)
	code := auth.NormalizePairingCode(in.Code)
	v.Check(len(code) == 8, "code", "must be the 8-character code shown in REA")
	if err := checkValid(v); err != nil {
		return err
	}
	invalid := &APIError{Status: http.StatusUnauthorized, Code: "invalid_pairing_code",
		Message: "The pairing code is invalid, already used or expired. Create a new code in REA under Extension setup."}
	var tp tokenPair
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		pc, err := q.ConsumePairingCode(r.Context(), auth.HashToken(code))
		if store.IsNotFound(err) {
			return invalid
		}
		if err != nil {
			return err
		}
		user, err := q.GetUserByID(r.Context(), pc.UserID)
		if err != nil {
			return err
		}
		perms, err := q.PermissionsForRole(r.Context(), user.RoleID)
		if err != nil {
			return err
		}
		allowed := false
		for _, p := range perms {
			allowed = allowed || p == auth.PermExtensionPair
		}
		if !user.IsActive || !allowed {
			return invalid
		}
		device, err := q.CreateExtensionDevice(r.Context(), store.CreateExtensionDeviceParams{UserID: user.ID, Name: in.DeviceName,
			Browser: normalize.Text(in.Browser), ExtensionVersion: normalize.Text(in.ExtensionVersion), LastIp: clientIP(r)})
		if err != nil {
			return err
		}
		tp, err = s.issueDeviceTokens(r, q, user, device)
		if err != nil {
			return err
		}
		return q.InsertAuditLog(r.Context(), store.InsertAuditLogParams{ActorUserID: &user.ID, ActorDeviceID: &device.ID,
			ActorLabel: user.Email, Action: "extension.paired", EntityType: "extension_device", EntityID: &device.ID,
			Summary: "Paired browser extension " + device.Name, IpAddress: clientIP(r), RequestID: requestID(r)})
	})
	if err != nil {
		return err
	}
	return created(w, tp)
}

type refreshInput struct {
	RefreshToken string `json:"refresh_token"`
}

func (s *Server) handleExtensionRefresh(w http.ResponseWriter, r *http.Request) error {
	var in refreshInput
	if err := decode(r, &in); err != nil {
		return err
	}
	expired := &APIError{Status: http.StatusUnauthorized, Code: "refresh_token_invalid",
		Message: "The extension's sign-in has expired or was revoked. Pair the extension again."}
	cred, err := s.db.GetCredentialByHashAny(r.Context(), auth.HashToken(in.RefreshToken))
	if store.IsNotFound(err) || (err == nil && cred.Kind != "refresh") {
		return expired
	}
	if err != nil {
		return err
	}
	if cred.RevokedAt != nil {
		// A rotated refresh token was presented again: assume it was stolen and revoke the device.
		if cred.DeviceID != nil {
			_, _ = s.db.RevokeExtensionDevice(r.Context(), *cred.DeviceID)
			_ = s.db.RevokeDeviceCredentials(r.Context(), cred.DeviceID)
			_ = s.db.InsertAuditLog(r.Context(), store.InsertAuditLogParams{ActorUserID: &cred.UserID, ActorDeviceID: cred.DeviceID,
				Action: "extension.refresh_reuse_detected", EntityType: "extension_device", EntityID: cred.DeviceID,
				Summary: "A previously used refresh token was presented; the device was revoked", IpAddress: clientIP(r), RequestID: requestID(r)})
		}
		return expired
	}
	if cred.ExpiresAt.Before(time.Now()) || cred.DeviceID == nil {
		return expired
	}
	device, err := s.db.GetExtensionDevice(r.Context(), *cred.DeviceID)
	if err != nil || device.RevokedAt != nil {
		return expired
	}
	user, err := s.db.GetUserByID(r.Context(), cred.UserID)
	if err != nil || !user.IsActive {
		return expired
	}
	var tp tokenPair
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		n, err := q.RevokeCredential(r.Context(), cred.ID)
		if err != nil {
			return err
		}
		if n == 0 { // a concurrent refresh already rotated this token
			return expired
		}
		tp, err = s.issueDeviceTokens(r, q, user, device)
		return err
	})
	if err != nil {
		return err
	}
	_ = s.db.TouchExtensionDevice(r.Context(), store.TouchExtensionDeviceParams{ID: device.ID, LastIp: clientIP(r),
		Version: truncateHeader(r.Header.Get("X-REA-Extension-Version"), 40)})
	return ok(w, tp)
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) error {
	p := principal(r)
	var filter *uuid.UUID
	if !(p.Can(auth.PermUsersManage) && r.URL.Query().Get("all") == "true") {
		filter = &p.User.ID
	}
	rows, err := s.db.ListExtensionDevices(r.Context(), filter)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

func (s *Server) revokeDevice(r *http.Request, id uuid.UUID, summary string) error {
	return s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		if _, err := q.RevokeExtensionDevice(r.Context(), id); err != nil {
			return err
		}
		if err := q.RevokeDeviceCredentials(r.Context(), &id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "extension.revoked", EntityType: "extension_device", EntityID: &id, Summary: summary})
	})
}

func (s *Server) handleRevokeDevice(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	d, err := s.db.GetExtensionDevice(r.Context(), id)
	if err != nil {
		return err
	}
	p := principal(r)
	if d.UserID != p.User.ID && !p.Can(auth.PermUsersManage) {
		return errNotFound("Device")
	}
	if err := s.revokeDevice(r, id, "Revoked browser extension "+d.Name); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) handleExtensionDisconnect(w http.ResponseWriter, r *http.Request) error {
	p := principal(r)
	if p.DeviceID == nil {
		return errBadRequest("Only a paired extension can disconnect itself.")
	}
	if err := s.revokeDevice(r, *p.DeviceID, "Extension disconnected itself"); err != nil {
		return err
	}
	return noContent(w)
}

// handleExtensionConfig gives the extension everything it needs after start-up: identity,
// effective permissions, enabled adapters and capture settings.
func (s *Server) handleExtensionConfig(w http.ResponseWriter, r *http.Request) error {
	p := principal(r)
	adapters, err := s.db.ListEnabledSourceAdapters(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{
		"user":           userView(p.User),
		"permissions":    p.PermList(),
		"device_id":      p.DeviceID,
		"server_version": s.cfg.Version,
		"organization":   s.orgName(r.Context()),
		"adapters":       adapters,
		"capture":        s.captureSettings(r.Context()),
		"imports":        s.importSettings(r.Context()),
		"ai_enabled":     s.aiEnabled(r.Context()),
	})
}
