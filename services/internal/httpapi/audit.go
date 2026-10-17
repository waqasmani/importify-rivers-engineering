package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// auditEvent describes a data-changing or security-relevant action.
type auditEvent struct {
	Action     string
	EntityType string
	EntityID   *uuid.UUID
	Summary    string
	Before     any
	After      any
	Extra      map[string]any
}

// sensitiveKeys are never written to the audit log.
var sensitiveKeys = map[string]bool{
	"password": true, "password_hash": true, "new_password": true, "current_password": true, "token": true,
	"token_hash": true, "api_key": true, "api_key_ciphertext": true, "secret": true, "csrf_token": true,
	"code_hash": true, "refresh_token": true, "access_token": true, "html_snapshot": true, "content_text": true,
	"content_markdown": true, "secret_ciphertext": true, "previous_secret_ciphertext": true, "signing_secret": true,
}

var ignoredDiffKeys = map[string]bool{"updated_at": true, "created_at": true, "normalized_name": true, "normalized_title": true}

func toMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

func redact(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if sensitiveKeys[k] {
			out[k] = "[redacted]"
			continue
		}
		if s, ok := v.(string); ok && len(s) > 1000 {
			v = s[:1000] + "…"
		}
		out[k] = v
	}
	return out
}

// diff returns {"field": {"from": x, "to": y}} for changed top-level fields.
func diff(before, after map[string]any) map[string]any {
	keys := map[string]bool{}
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	out := map[string]any{}
	for _, k := range names {
		if ignoredDiffKeys[k] {
			continue
		}
		if !reflect.DeepEqual(before[k], after[k]) {
			if sensitiveKeys[k] {
				out[k] = map[string]any{"changed": true}
				continue
			}
			out[k] = map[string]any{"from": before[k], "to": after[k]}
		}
	}
	return out
}

// audit records an event using q, so it commits or rolls back with the caller's transaction.
func (s *Server) audit(ctx context.Context, q *store.Queries, r *http.Request, ev auditEvent) error {
	changes := map[string]any{}
	b, a := toMap(ev.Before), toMap(ev.After)
	switch {
	case b != nil && a != nil:
		changes["changed"] = diff(b, a)
	case a != nil:
		changes["created"] = redact(a)
	case b != nil:
		changes["deleted"] = redact(b)
	}
	for k, v := range ev.Extra {
		if sensitiveKeys[k] {
			continue
		}
		changes[k] = v
	}
	var raw json.RawMessage
	if len(changes) > 0 {
		raw, _ = json.Marshal(changes)
	}
	params := store.InsertAuditLogParams{
		Action: ev.Action, EntityType: ev.EntityType, EntityID: ev.EntityID, Summary: truncate(ev.Summary, 500),
		Changes: raw,
	}
	if r != nil {
		params.IpAddress = clientIP(r)
		params.RequestID = requestID(r)
		if p := principal(r); p != nil && p.Integration != nil {
			id := p.Integration.ID
			params.ActorIntegrationKeyID = &id
			params.ActorLabel = p.Integration.Label()
		} else if p != nil {
			id := p.User.ID
			params.ActorUserID = &id
			params.ActorDeviceID = p.DeviceID
			params.ActorLabel = p.User.Email
		}
	}
	if q == nil {
		q = s.db.Queries
	}
	if err := q.InsertAuditLog(ctx, params); err != nil {
		s.log.Error("audit write failed", slog.String("action", ev.Action), slog.String("error", err.Error()))
		return err
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

func allPermsSorted() []string {
	out := append([]string(nil), auth.AllPermissions...)
	sort.Strings(out)
	return out
}
