package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/catalog"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/orders"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/webhooks"
)

// Integration keys and webhooks are organization configuration: they need settings.manage and an
// interactive session, like the other settings, so an API token cannot mint new credentials.

const (
	maxWebhookEndpoints = 50
	// rotationOverlap is how long the previous secret keeps signing after a rotation.
	rotationOverlap = 24 * time.Hour
)

func (s *Server) mountIntegrations(r chi.Router) {
	r.Route("/integrations", func(r chi.Router) {
		r.Use(s.requirePerm(auth.PermSettingsManage), s.requireSession)
		r.Get("/config", s.h(s.handleIntegrationsConfig))
		r.Get("/keys", s.h(s.handleListIntegrationKeys))
		r.Post("/keys", s.h(s.handleCreateIntegrationKey))
		r.Delete("/keys/{id}", s.h(s.handleRevokeIntegrationKey))
		r.Get("/webhooks", s.h(s.handleListWebhooks))
		r.Post("/webhooks", s.h(s.handleCreateWebhook))
		r.Get("/webhooks/{id}", s.h(s.handleGetWebhook))
		r.Put("/webhooks/{id}", s.h(s.handleUpdateWebhook))
		r.Delete("/webhooks/{id}", s.h(s.handleDeleteWebhook))
		r.Post("/webhooks/{id}/rotate-secret", s.h(s.handleRotateWebhookSecret))
		r.Post("/webhooks/{id}/test", s.h(s.handleTestWebhook))
		r.Get("/webhooks/{id}/deliveries", s.h(s.handleListWebhookDeliveries))
		r.Get("/deliveries/{id}", s.h(s.handleGetWebhookDelivery))
		r.Post("/deliveries/{id}/redeliver", s.h(s.handleRedeliverWebhook))
	})
}

// ---- configuration ----

type webhookPolicyDoc struct {
	AllowPrivateNetworks bool   `json:"allow_private_networks"`
	AllowHTTPLocalhost   bool   `json:"allow_http_localhost"`
	MaxAttempts          int    `json:"max_attempts"`
	TimeoutSeconds       int    `json:"timeout_seconds"`
	DisableAfterFailures int    `json:"disable_after_failures"`
	SignatureHeader      string `json:"signature_header"`
	TimestampHeader      string `json:"timestamp_header"`
	EventIDHeader        string `json:"event_id_header"`
}

type integrationsConfigDoc struct {
	APIBaseURL string                  `json:"api_base_url"`
	Scopes     []auth.IntegrationScope `json:"scopes"`
	Events     []catalog.EventType     `json:"events"`
	// OrderEvents are the order webhook events. Their payloads contain customer names, e-mail
	// addresses, phone numbers and shipping addresses.
	OrderEvents []catalog.EventType `json:"order_events"`
	Webhooks    webhookPolicyDoc    `json:"webhooks"`
}

func (s *Server) handleIntegrationsConfig(w http.ResponseWriter, r *http.Request) error {
	return ok(w, integrationsConfigDoc{APIBaseURL: s.cfg.PublicURL.String() + "/api/v1", Scopes: auth.IntegrationScopes,
		Events: catalog.EventTypes, OrderEvents: orders.EventTypes, Webhooks: webhookPolicyDoc{AllowPrivateNetworks: s.cfg.WebhookAllowPrivate,
			AllowHTTPLocalhost: s.cfg.WebhookAllowHTTPLocalhost, MaxAttempts: s.cfg.WebhookMaxAttempts,
			TimeoutSeconds: int(s.cfg.WebhookTimeout.Seconds()), DisableAfterFailures: s.cfg.WebhookDisableAfterFailures,
			SignatureHeader: webhooks.HeaderSignature, TimestampHeader: webhooks.HeaderTimestamp, EventIDHeader: webhooks.HeaderEventID}})
}

// ---- integration keys ----

// IntegrationKeyView is an integration key without its secret.
type IntegrationKeyView struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Prefix      string     `json:"prefix"`
	Scopes      []string   `json:"scopes"`
	Status      string     `json:"status"` // active | expired | revoked
	ExpiresAt   *time.Time `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	LastUsedIP  string     `json:"last_used_ip"`
	CreatedAt   time.Time  `json:"created_at"`
	// CreatedBy is the creator's name, or the e-mail recorded at creation if the user was deleted.
	CreatedBy      string     `json:"created_by"`
	CreatorRemoved bool       `json:"creator_removed"`
	RevokedAt      *time.Time `json:"revoked_at"`
	RequestsToday  int64      `json:"requests_today"`
	Requests30d    int64      `json:"requests_30d"`
}

func keyStatus(k store.IntegrationKey) string {
	switch {
	case k.RevokedAt != nil:
		return "revoked"
	case k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()):
		return "expired"
	}
	return "active"
}

func keyView(k store.IntegrationKey, creator *string, today, month int64) IntegrationKeyView {
	v := IntegrationKeyView{ID: k.ID, Name: k.Name, Description: k.Description, Prefix: k.TokenPrefix, Scopes: k.Scopes,
		Status: keyStatus(k), ExpiresAt: k.ExpiresAt, LastUsedAt: k.LastUsedAt, LastUsedIP: k.LastUsedIp, CreatedAt: k.CreatedAt,
		CreatedBy: k.CreatedByLabel, CreatorRemoved: k.CreatedBy == nil, RevokedAt: k.RevokedAt, RequestsToday: today, Requests30d: month}
	if creator != nil {
		v.CreatedBy = *creator
	}
	return v
}

func (s *Server) handleListIntegrationKeys(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListIntegrationKeys(r.Context())
	if err != nil {
		return err
	}
	out := make([]IntegrationKeyView, 0, len(rows))
	for _, row := range rows {
		k := store.IntegrationKey{ID: row.ID, Name: row.Name, Description: row.Description, TokenPrefix: row.TokenPrefix,
			Scopes: row.Scopes, ExpiresAt: row.ExpiresAt, LastUsedAt: row.LastUsedAt, LastUsedIp: row.LastUsedIp, CreatedBy: row.CreatedBy,
			CreatedByLabel: row.CreatedByLabel, CreatedAt: row.CreatedAt, RevokedAt: row.RevokedAt, RevokedBy: row.RevokedBy}
		out = append(out, keyView(k, row.CreatedByName, row.RequestsToday, row.Requests30d))
	}
	return ok(w, Items[IntegrationKeyView]{Items: out})
}

type createIntegrationKeyInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Scopes      []string `json:"scopes"`
	// ExpiresInDays is 1-730, or 0/null for a key that does not expire.
	ExpiresInDays *int `json:"expires_in_days"`
}

type integrationKeyCreatedDoc struct {
	// Token is shown only in this response; REA stores a hash.
	Token string             `json:"token"`
	Key   IntegrationKeyView `json:"key"`
}

func (s *Server) handleCreateIntegrationKey(w http.ResponseWriter, r *http.Request) error {
	var in createIntegrationKeyInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Name, in.Description = normalize.Text(in.Name), normalize.MultilineText(in.Description)
	v := valid.New()
	v.Len("name", in.Name, 1, 120)
	v.Max("description", in.Description, 1000)
	scopes := []string{}
	for _, sc := range in.Scopes {
		if !auth.IsIntegrationScope(sc) {
			v.Add("scopes", "contains an unknown scope: "+sc)
		} else if !slices.Contains(scopes, sc) {
			scopes = append(scopes, sc)
		}
	}
	v.Check(len(scopes) > 0, "scopes", "must include at least one scope")
	if slices.Contains(scopes, auth.ScopeCatalogReadCosts) && !slices.Contains(scopes, auth.ScopeCatalogRead) {
		v.Add("scopes", "catalog.read_costs requires catalog.read")
	}
	var expires *time.Time
	if in.ExpiresInDays != nil && *in.ExpiresInDays != 0 {
		v.Check(*in.ExpiresInDays >= 1 && *in.ExpiresInDays <= 730, "expires_in_days", "must be between 1 and 730, or 0 for no expiry")
		t := time.Now().Add(time.Duration(*in.ExpiresInDays) * 24 * time.Hour)
		expires = &t
	}
	if err := checkValid(v); err != nil {
		return err
	}
	slices.SortFunc(scopes, func(a, b string) int { return scopeOrder(a) - scopeOrder(b) })
	p := principal(r)
	token := auth.NewToken(auth.PrefixIntegration)
	var out integrationKeyCreatedDoc
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		k, err := q.CreateIntegrationKey(r.Context(), store.CreateIntegrationKeyParams{Name: in.Name, Description: in.Description,
			TokenHash: auth.HashToken(token), TokenPrefix: auth.DisplayPrefix(token), Scopes: scopes, ExpiresAt: expires,
			CreatedBy: &p.User.ID, CreatedByLabel: p.User.Email})
		if err != nil {
			return err
		}
		name := p.User.DisplayName
		out = integrationKeyCreatedDoc{Token: token, Key: keyView(k, &name, 0, 0)}
		return s.audit(r.Context(), q, r, auditEvent{Action: "integration_key.create", EntityType: "integration_key", EntityID: &k.ID,
			Summary: "Created integration key " + k.Name, Extra: map[string]any{"scopes": scopes, "expires_at": expires, "prefix": k.TokenPrefix}})
	})
	if err != nil {
		return err
	}
	return created(w, out)
}

func scopeOrder(code string) int {
	for i, sc := range auth.IntegrationScopes {
		if sc.Code == code {
			return i
		}
	}
	return len(auth.IntegrationScopes)
}

func (s *Server) handleRevokeIntegrationKey(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	k, err := s.db.RevokeIntegrationKey(r.Context(), store.RevokeIntegrationKeyParams{ID: id, RevokedBy: userIDPtr(r)})
	if store.IsNotFound(err) {
		if _, gerr := s.db.GetIntegrationKey(r.Context(), id); gerr != nil {
			return gerr
		}
		return noContent(w) // already revoked
	}
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "integration_key.revoke", EntityType: "integration_key", EntityID: &id,
		Summary: "Revoked integration key " + k.Name})
	return noContent(w)
}

// ---- webhook endpoints ----

// WebhookView is a webhook endpoint without its signing secret.
type WebhookView struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	Events       []string  `json:"events"`
	Enabled      bool      `json:"enabled"`
	IncludeCosts bool      `json:"include_costs"`
	SecretPrefix string    `json:"secret_prefix"`
	// PreviousSecretExpiresAt is set after a rotation while the old secret still signs deliveries.
	PreviousSecretExpiresAt *time.Time `json:"previous_secret_expires_at"`
	ConsecutiveFailures     int32      `json:"consecutive_failures"`
	FailingSince            *time.Time `json:"failing_since"`
	LastSuccessAt           *time.Time `json:"last_success_at"`
	LastFailureAt           *time.Time `json:"last_failure_at"`
	DisabledReason          string     `json:"disabled_reason"`
	DisabledAt              *time.Time `json:"disabled_at"`
	PendingDeliveries       int64      `json:"pending_deliveries"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

func webhookView(e store.WebhookEndpoint, pending int64) WebhookView {
	v := WebhookView{ID: e.ID, Name: e.Name, URL: e.Url, Events: e.Events, Enabled: e.Enabled, IncludeCosts: e.IncludeCosts,
		SecretPrefix: e.SecretPrefix, ConsecutiveFailures: e.ConsecutiveFailures, FailingSince: e.FailingSince,
		LastSuccessAt: e.LastSuccessAt, LastFailureAt: e.LastFailureAt, DisabledReason: e.DisabledReason, DisabledAt: e.DisabledAt,
		PendingDeliveries: pending, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
	if e.PreviousSecretExpiresAt != nil && e.PreviousSecretExpiresAt.After(time.Now()) {
		v.PreviousSecretExpiresAt = e.PreviousSecretExpiresAt
	}
	return v
}

func (s *Server) webhookViewOf(r *http.Request, e store.WebhookEndpoint) (WebhookView, error) {
	n, err := s.db.CountPendingWebhookDeliveries(r.Context(), e.ID)
	return webhookView(e, n), err
}

type webhookInput struct {
	Name         string   `json:"name"`
	URL          string   `json:"url"`
	Events       []string `json:"events"`
	Enabled      bool     `json:"enabled"`
	IncludeCosts bool     `json:"include_costs"`
}

func (s *Server) validateWebhook(in *webhookInput) error {
	in.Name = normalize.Text(in.Name)
	v := valid.New()
	v.Len("name", in.Name, 1, 120)
	if err := webhooks.ValidateURL(s.cfg, in.URL); err != nil {
		v.Add("url", err.Error())
	}
	events := []string{}
	for _, e := range append(append([]catalog.EventType{}, catalog.EventTypes...), orders.EventTypes...) {
		if slices.Contains(in.Events, e.Type) {
			events = append(events, e.Type)
		}
	}
	for _, e := range in.Events {
		if !catalog.IsEventType(e) && !orders.IsEventType(e) {
			v.Add("events", "contains an unknown event type: "+e)
		}
	}
	v.Check(len(events) > 0, "events", "must include at least one event type")
	in.Events = events
	return checkValid(v)
}

type webhookCreatedDoc struct {
	// Secret signs every delivery. It is shown only in this response and stored encrypted.
	Secret  string      `json:"secret"`
	Webhook WebhookView `json:"webhook"`
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListWebhookEndpoints(r.Context())
	if err != nil {
		return err
	}
	out := make([]WebhookView, 0, len(rows))
	for _, row := range rows {
		out = append(out, webhookView(recordAs[store.WebhookEndpoint](row), row.PendingDeliveries))
	}
	return ok(w, Items[WebhookView]{Items: out})
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) error {
	var in webhookInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.validateWebhook(&in); err != nil {
		return err
	}
	secret := webhooks.NewSecret()
	sealed, err := s.box.Seal([]byte(secret))
	if err != nil {
		return err
	}
	// A new endpoint starts at the current end of the feed, so publish committed changes first.
	if _, err := catalog.Sequence(r.Context(), s.db, true); err != nil {
		return err
	}
	var out webhookCreatedDoc
	err = s.db.Tx(r.Context(), func(q *store.Queries, tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(7311102)"); err != nil {
			return err
		}
		if n, err := q.CountWebhookEndpoints(r.Context()); err != nil {
			return err
		} else if n >= maxWebhookEndpoints {
			return errConflict("too_many_webhooks", "At most 50 webhook endpoints can be registered.")
		}
		e, err := q.CreateWebhookEndpoint(r.Context(), store.CreateWebhookEndpointParams{Name: in.Name, Url: in.URL, Events: in.Events,
			Enabled: in.Enabled, IncludeCosts: in.IncludeCosts, SecretCiphertext: sealed, SecretPrefix: webhooks.SecretPrefix(secret),
			CreatedBy: userIDPtr(r)})
		if err != nil {
			return err
		}
		out = webhookCreatedDoc{Secret: secret, Webhook: webhookView(e, 0)}
		return s.audit(r.Context(), q, r, auditEvent{Action: "webhook.create", EntityType: "webhook_endpoint", EntityID: &e.ID,
			Summary: "Created webhook " + e.Name, After: out.Webhook})
	})
	if err != nil {
		return err
	}
	return created(w, out)
}

func (s *Server) handleGetWebhook(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	e, err := s.db.GetWebhookEndpoint(r.Context(), id)
	if err != nil {
		return err
	}
	v, err := s.webhookViewOf(r, e)
	if err != nil {
		return err
	}
	return ok(w, v)
}

// handleUpdateWebhook changes an endpoint. Enabling a disabled endpoint clears the automatic
// disable and resumes delivery from where it stopped.
func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in webhookInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.validateWebhook(&in); err != nil {
		return err
	}
	before, err := s.db.GetWebhookEndpoint(r.Context(), id)
	if err != nil {
		return err
	}
	e, err := s.db.UpdateWebhookEndpoint(r.Context(), store.UpdateWebhookEndpointParams{ID: id, Name: in.Name, Url: in.URL, Events: in.Events,
		IncludeCosts: in.IncludeCosts, Enabled: in.Enabled})
	if err != nil {
		return err
	}
	v, err := s.webhookViewOf(r, e)
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "webhook.update", EntityType: "webhook_endpoint", EntityID: &id,
		Summary: "Updated webhook " + e.Name, Before: webhookView(before, v.PendingDeliveries), After: v})
	if e.Enabled {
		s.deps.Wake()
	}
	return ok(w, v)
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	e, err := s.db.GetWebhookEndpoint(r.Context(), id)
	if err != nil {
		return err
	}
	if _, err := s.db.DeleteWebhookEndpoint(r.Context(), id); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "webhook.delete", EntityType: "webhook_endpoint", EntityID: &id,
		Summary: "Deleted webhook " + e.Name})
	return noContent(w)
}

// handleRotateWebhookSecret issues a new secret. The previous one keeps signing (as a second
// signature) for 24 hours so receivers can switch without dropping deliveries.
func (s *Server) handleRotateWebhookSecret(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	secret := webhooks.NewSecret()
	sealed, err := s.box.Seal([]byte(secret))
	if err != nil {
		return err
	}
	until := time.Now().Add(rotationOverlap)
	e, err := s.db.RotateWebhookSecret(r.Context(), store.RotateWebhookSecretParams{ID: id, SecretCiphertext: sealed,
		SecretPrefix: webhooks.SecretPrefix(secret), PreviousExpiresAt: &until})
	if err != nil {
		return err
	}
	v, err := s.webhookViewOf(r, e)
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "webhook.rotate_secret", EntityType: "webhook_endpoint", EntityID: &id,
		Summary: "Rotated the signing secret of webhook " + e.Name})
	return ok(w, webhookCreatedDoc{Secret: secret, Webhook: v})
}

// ---- deliveries ----

// WebhookDeliveryView is one delivery of an event to an endpoint.
type WebhookDeliveryView struct {
	ID             uuid.UUID  `json:"id"`
	EndpointID     uuid.UUID  `json:"endpoint_id"`
	EventID        uuid.UUID  `json:"event_id"`
	EventType      string     `json:"event_type"`
	ChangeID       *int64     `json:"change_id"`
	ProductID      *uuid.UUID `json:"product_id"`
	IsTest         bool       `json:"is_test"`
	Status         string     `json:"status"` // pending | delivering | succeeded | failed
	Attempts       int32      `json:"attempts"`
	MaxAttempts    int32      `json:"max_attempts"`
	NextAttemptAt  *time.Time `json:"next_attempt_at"`
	LastAttemptAt  *time.Time `json:"last_attempt_at"`
	ResponseStatus *int32     `json:"response_status"`
	LatencyMs      *int32     `json:"latency_ms"`
	Error          string     `json:"error"`
	RedeliveryOf   *uuid.UUID `json:"redelivery_of"`
	CreatedAt      time.Time  `json:"created_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

// WebhookDeliveryDetail adds the exact payload, the stored response excerpt and every attempt.
type WebhookDeliveryDetail struct {
	WebhookDeliveryView
	ResponseBody string                         `json:"response_body"`
	Payload      json.RawMessage                `json:"payload"`
	Attempts     []store.WebhookDeliveryAttempt `json:"attempt_log"`
}

func deliveryView(d store.WebhookDelivery) WebhookDeliveryView {
	v := WebhookDeliveryView{ID: d.ID, EndpointID: d.EndpointID, EventID: d.EventID, EventType: d.EventType, ChangeID: d.ChangeID,
		ProductID: d.ProductID, IsTest: d.IsTest, Status: d.Status, Attempts: d.Attempts, MaxAttempts: d.MaxAttempts,
		LastAttemptAt: d.LastAttemptAt, ResponseStatus: d.ResponseStatus, LatencyMs: d.LatencyMs, Error: d.Error,
		RedeliveryOf: d.RedeliveryOf, CreatedAt: d.CreatedAt, FinishedAt: d.FinishedAt}
	if d.Status == "pending" {
		t := d.NextAttemptAt
		v.NextAttemptAt = &t
	}
	return v
}

func (s *Server) deliveryDetail(r *http.Request, d store.WebhookDelivery) (WebhookDeliveryDetail, error) {
	attempts, err := s.db.ListWebhookAttempts(r.Context(), d.ID)
	return WebhookDeliveryDetail{WebhookDeliveryView: deliveryView(d), ResponseBody: d.ResponseBody, Payload: d.Payload, Attempts: attempts}, err
}

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	e, err := s.db.GetWebhookEndpoint(r.Context(), id)
	if err != nil {
		return err
	}
	if err := webhooks.ValidateURL(s.cfg, e.Url); err != nil {
		return errField("url", err.Error())
	}
	d, err := s.hooks.SendTest(r.Context(), e)
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "webhook.test", EntityType: "webhook_endpoint", EntityID: &id,
		Summary: "Sent a test event to webhook " + e.Name + ": " + d.Status})
	out, err := s.deliveryDetail(r, d)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	if _, err := s.db.GetWebhookEndpoint(r.Context(), id); err != nil {
		return err
	}
	status := qText(r, "status")
	if status != nil && !slices.Contains([]string{"pending", "delivering", "succeeded", "failed"}, *status) {
		return errField("status", "must be pending, delivering, succeeded or failed")
	}
	limit, offset := pagination(r)
	rows, err := s.db.ListWebhookDeliveries(r.Context(), store.ListWebhookDeliveriesParams{EndpointID: id, Status: status, Lim: limit, Off: offset})
	if err != nil {
		return err
	}
	out := make([]WebhookDeliveryView, 0, len(rows))
	var total int64
	for _, row := range rows {
		out = append(out, deliveryView(recordAs[store.WebhookDelivery](row)))
		total = row.Total
	}
	return ok(w, Page[WebhookDeliveryView]{Items: out, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetWebhookDelivery(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	d, err := s.db.GetWebhookDelivery(r.Context(), id)
	if err != nil {
		return err
	}
	out, err := s.deliveryDetail(r, d)
	if err != nil {
		return err
	}
	return ok(w, out)
}

// handleRedeliverWebhook queues the same event (same event ID and payload) again with a fresh
// retry budget. Receivers deduplicate on the event ID.
func (s *Server) handleRedeliverWebhook(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	orig, err := s.db.GetWebhookDelivery(r.Context(), id)
	if err != nil {
		return err
	}
	if orig.IsTest {
		return errConflict("test_delivery", "Test events are not redelivered; send a new test event instead.")
	}
	var d store.WebhookDelivery
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		var err error
		d, err = q.CreateWebhookDelivery(r.Context(), store.CreateWebhookDeliveryParams{EndpointID: orig.EndpointID, EventID: orig.EventID,
			EventType: orig.EventType, ChangeID: orig.ChangeID, ProductID: orig.ProductID, Payload: orig.Payload,
			MaxAttempts: int32(s.cfg.WebhookMaxAttempts), RedeliveryOf: &orig.ID, Status: "pending"})
		if err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "webhook.redeliver", EntityType: "webhook_endpoint", EntityID: &orig.EndpointID,
			Summary: "Queued a redelivery of " + orig.EventType, Extra: map[string]any{"event_id": orig.EventID, "delivery_id": orig.ID}})
	})
	if err != nil {
		return err
	}
	s.deps.Wake()
	out, err := s.deliveryDetail(r, d)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, out)
	return nil
}
