package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// Principal is the authenticated caller.
type Principal struct {
	User         store.User // zero for integration keys
	Perms        map[string]bool
	Method       string // "session", "token" or "integration"
	SessionID    uuid.UUID
	CSRFToken    string
	CredentialID uuid.UUID
	DeviceID     *uuid.UUID
	// Integration is set for integration keys, which act for the organization, not for a user.
	Integration *IntegrationActor
}

// IntegrationActor identifies the integration key behind a request.
type IntegrationActor struct {
	ID     uuid.UUID
	Name   string
	Prefix string
}

// Label is how the key appears in the audit log.
func (a *IntegrationActor) Label() string {
	return "integration key " + a.Name + " (" + a.Prefix + "…)"
}

// Can reports whether the principal holds a permission.
func (p *Principal) Can(perm string) bool { return p != nil && p.Perms[perm] }

// PermList returns the principal's permissions as a sorted slice.
func (p *Principal) PermList() []string {
	out := make([]string, 0, len(p.Perms))
	for _, code := range allPermsSorted() {
		if p.Perms[code] {
			out = append(out, code)
		}
	}
	return out
}

type ctxKey int

const (
	principalKey ctxKey = iota
	requestIDKey
	clientIPKey
)

func withPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func principal(r *http.Request) *Principal {
	p, _ := r.Context().Value(principalKey).(*Principal)
	return p
}

func requestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey).(string)
	return id
}

func clientIP(r *http.Request) string {
	ip, _ := r.Context().Value(clientIPKey).(string)
	return ip
}

func userIDPtr(r *http.Request) *uuid.UUID {
	p := principal(r)
	if p == nil || p.Integration != nil {
		return nil
	}
	id := p.User.ID
	return &id
}
