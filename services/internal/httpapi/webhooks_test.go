package httpapi_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
)

// receiver is a local webhook endpoint that records every request.
type receiver struct {
	srv    *httptest.Server
	mu     sync.Mutex
	reqs   []receivedRequest
	status int
	body   string
}

type receivedRequest struct {
	header http.Header
	body   []byte
}

func newReceiver(t *testing.T) *receiver {
	rc := &receiver{status: 200, body: "ok"}
	rc.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.reqs = append(rc.reqs, receivedRequest{header: r.Header.Clone(), body: b})
		status, body := rc.status, rc.body
		rc.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(rc.srv.Close)
	return rc
}

func (rc *receiver) respond(status int, body string) {
	rc.mu.Lock()
	rc.status, rc.body = status, body
	rc.mu.Unlock()
}

func (rc *receiver) requests() []receivedRequest {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return append([]receivedRequest(nil), rc.reqs...)
}

// verifyIndependently checks the signature with crypto/hmac directly, as a receiver written in any
// language would: hex(HMAC-SHA256(secret, timestamp + "." + raw body)).
func verifyIndependently(t *testing.T, secret string, r receivedRequest) {
	t.Helper()
	ts := r.header.Get("X-REA-Timestamp")
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || time.Since(time.Unix(n, 0)) > 5*time.Minute {
		t.Fatalf("bad timestamp %q", ts)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(r.body)
	want := "v1=" + hex.EncodeToString(mac.Sum(nil))
	for _, part := range strings.Split(r.header.Get("X-REA-Signature"), ",") {
		if hmac.Equal([]byte(part), []byte(want)) {
			return
		}
	}
	t.Fatalf("signature %q does not match %q", r.header.Get("X-REA-Signature"), want)
}

func localWebhooks(c *config.Config) {
	// Only these tests point webhooks at a local server.
	c.WebhookAllowPrivate = true
	c.WebhookAllowHTTPLocalhost = true
}

func (e *env) waitFor(what string, cond func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		e.worker.DrainWebhooks(e.t.Context())
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatalf("timed out waiting for %s", what)
}

var allEvents = []string{"product.published", "product.updated", "product.price_changed", "product.availability_changed",
	"product.unpublished", "product.deleted"}

func TestWebhookDeliveryIsSignedAndLogged(t *testing.T) {
	e := newEnvWith(t, localWebhooks)
	rc := newReceiver(t)
	e.publish("Existing before the webhook", "OLD-1", nil)
	created := e.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "Shop", "url": rc.srv.URL + "/hooks/rea",
		"events": allEvents, "enabled": true}).Must(t, 201)
	secret, hookID := created.Str("secret"), created.Str("webhook.id")
	if !strings.HasPrefix(secret, "whsec_") || created.Str("webhook.secret_prefix") != secret[:12] {
		t.Fatalf("secret wrong: %s", created.Body)
	}
	// The secret is encrypted at rest and never returned again.
	var sealed []byte
	if err := e.db.Pool.QueryRow(t.Context(), "SELECT secret_ciphertext FROM webhook_endpoints WHERE id = $1", hookID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte(secret)) || bytes.Contains(sealed, []byte(secret[6:])) {
		t.Fatal("webhook secret stored in plaintext")
	}
	if opened, err := e.api.Box().Open(sealed); err != nil || string(opened) != secret {
		t.Fatal("stored secret does not decrypt with REA_SECRET_KEY")
	}
	for _, body := range [][]byte{e.owner.Do("GET", "/api/v1/integrations/webhooks", nil).Must(t, 200).Body,
		e.owner.Do("GET", "/api/v1/integrations/webhooks/"+hookID, nil).Must(t, 200).Body,
		e.owner.Do("GET", "/api/v1/audit-logs", nil).Must(t, 200).Body} {
		if strings.Contains(string(body), secret[6:]) {
			t.Fatal("secret returned after creation")
		}
	}

	// Changes from before the registration are not replayed; new ones are delivered.
	pid := e.publish("Bottle", "B-1", map[string]any{"cost": "3", "cost_currency": "EUR"})
	e.waitFor("the published event", func() bool { return len(rc.requests()) == 1 })
	r := rc.requests()[0]
	verifyIndependently(t, secret, r)
	var ev map[string]any
	if err := json.Unmarshal(r.body, &ev); err != nil {
		t.Fatal(err)
	}
	product, _ := ev["product"].(map[string]any)
	if ev["type"] != "product.published" || ev["product_id"] != pid || product["title"] != "Bottle" || product["sourcing"] != nil ||
		r.header.Get("X-REA-Event-Id") != ev["id"] || r.header.Get("X-REA-Event-Type") != "product.published" ||
		r.header.Get("Content-Type") != "application/json" || r.header.Get("X-REA-Delivery-Id") == "" {
		t.Fatalf("delivery wrong: %v %s", r.header, r.body)
	}
	log := e.owner.Do("GET", "/api/v1/integrations/webhooks/"+hookID+"/deliveries", nil).Must(t, 200)
	if log.Get("total") != float64(1) || log.Str("items.0.status") != "succeeded" || log.Get("items.0.response_status") != float64(200) ||
		log.Get("items.0.latency_ms") == nil || log.Get("items.0.attempts") != float64(1) {
		t.Fatalf("delivery log wrong: %s", log.Body)
	}
	detail := e.owner.Do("GET", "/api/v1/integrations/deliveries/"+log.Str("items.0.id"), nil).Must(t, 200)
	if detail.Str("response_body") != "ok" || len(detail.Get("attempt_log").([]any)) != 1 || detail.Str("payload.id") != ev["id"] {
		t.Fatalf("delivery detail wrong: %s", detail.Body)
	}

	// Subscriptions filter events; include_costs adds the sourcing block.
	e.owner.Do("PUT", "/api/v1/integrations/webhooks/"+hookID, map[string]any{"name": "Shop", "url": rc.srv.URL + "/hooks/rea",
		"events": []string{"product.price_changed"}, "enabled": true, "include_costs": true}).Must(t, 200)
	e.updateProduct(pid, map[string]any{"title": "Steel bottle"})
	e.updateProduct(pid, map[string]any{"price": "11.00"})
	e.waitFor("the price event", func() bool { return len(rc.requests()) == 2 })
	e.worker.DrainWebhooks(t.Context())
	if len(rc.requests()) != 2 {
		t.Fatal("unsubscribed event delivered")
	}
	r = rc.requests()[1]
	verifyIndependently(t, secret, r)
	_ = json.Unmarshal(r.body, &ev)
	if ev["type"] != "product.price_changed" || ev["product"].(map[string]any)["sourcing"].(map[string]any)["cost"] != "3" {
		t.Fatalf("price event wrong: %s", r.body)
	}

	// Rotation: the new secret signs, and the previous one too for a grace period.
	rot := e.owner.Do("POST", "/api/v1/integrations/webhooks/"+hookID+"/rotate-secret", nil).Must(t, 200)
	newSecret := rot.Str("secret")
	if newSecret == secret || rot.Get("webhook.previous_secret_expires_at") == nil {
		t.Fatalf("rotation wrong: %s", rot.Body)
	}
	e.updateProduct(pid, map[string]any{"price": "12.00"})
	e.waitFor("a delivery after rotation", func() bool { return len(rc.requests()) == 3 })
	r = rc.requests()[2]
	if len(strings.Split(r.header.Get("X-REA-Signature"), ",")) != 2 {
		t.Fatalf("expected two signatures during rotation: %s", r.header.Get("X-REA-Signature"))
	}
	verifyIndependently(t, newSecret, r)
	verifyIndependently(t, secret, r)

	// Test events are signed, logged and do not count as failures.
	test := e.owner.Do("POST", "/api/v1/integrations/webhooks/"+hookID+"/test", nil).Must(t, 200)
	if test.Str("status") != "succeeded" || test.Get("is_test") != true || test.Str("event_type") != "webhook.test" {
		t.Fatalf("test event wrong: %s", test.Body)
	}
	r = rc.requests()[3]
	verifyIndependently(t, newSecret, r)
	_ = json.Unmarshal(r.body, &ev)
	if ev["type"] != "webhook.test" || ev["product"] == nil {
		t.Fatalf("test payload should carry a sample product: %s", r.body)
	}
	e.owner.Do("POST", "/api/v1/integrations/deliveries/"+test.Str("id")+"/redeliver", nil).Must(t, 409)

	// Redelivery sends the same event again under a new delivery ID.
	first := log.Str("items.0.id")
	re := e.owner.Do("POST", "/api/v1/integrations/deliveries/"+first+"/redeliver", nil).Must(t, 202)
	if re.Str("redelivery_of") != first || re.Str("status") != "pending" {
		t.Fatalf("redelivery wrong: %s", re.Body)
	}
	e.waitFor("the redelivery", func() bool { return len(rc.requests()) == 5 })
	r = rc.requests()[4]
	if r.header.Get("X-REA-Event-Id") != rc.requests()[0].header.Get("X-REA-Event-Id") ||
		r.header.Get("X-REA-Delivery-Id") == rc.requests()[0].header.Get("X-REA-Delivery-Id") || !bytes.Equal(r.body, rc.requests()[0].body) {
		t.Fatal("redelivery must repeat the event with a new delivery ID")
	}
	verifyIndependently(t, newSecret, r)

	// Only administrators manage webhooks.
	member := e.userWithRole("member", "member@example.com")
	member.Do("GET", "/api/v1/integrations/webhooks", nil).Must(t, 403)
	member.Do("POST", "/api/v1/integrations/webhooks/"+hookID+"/test", nil).Must(t, 403)
	e.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "No events", "url": rc.srv.URL, "events": []string{}, "enabled": true}).Must(t, 422)
	e.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "Bad event", "url": rc.srv.URL, "events": []string{"order.created"}, "enabled": true}).Must(t, 422)
	e.owner.Do("DELETE", "/api/v1/integrations/webhooks/"+hookID, nil).Must(t, 204)
	e.owner.Do("GET", "/api/v1/integrations/deliveries/"+first, nil).Must(t, 404)
}

func TestWebhookRetriesBackoffAndAutoDisable(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) {
		localWebhooks(c)
		c.WebhookMaxAttempts = 3
		c.WebhookRetryBase = time.Millisecond
		c.WebhookRetryMax = 4 * time.Millisecond
		c.WebhookDisableAfterFailures = 5
		c.WebhookDisableFailingFor = 0
	})
	rc := newReceiver(t)
	rc.respond(503, strings.Repeat("x", 10_000))
	hook := e.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "Flaky ERP", "url": rc.srv.URL,
		"events": allEvents, "enabled": true}).Must(t, 201)
	id := hook.Str("webhook.id")
	e.publish("One", "R-1", nil)
	e.publish("Two", "R-2", nil)
	state := func() *struct {
		enabled bool
		reason  string
	} {
		w := e.owner.Do("GET", "/api/v1/integrations/webhooks/"+id, nil).Must(t, 200)
		return &struct {
			enabled bool
			reason  string
		}{w.Get("enabled") == true, w.Str("disabled_reason")}
	}
	e.waitFor("automatic disable", func() bool { return !state().enabled })
	// Five failed attempts in total: one delivery used its three attempts, the other is waiting.
	if n := len(rc.requests()); n != 5 {
		t.Fatalf("expected 5 attempts before disabling, got %d", n)
	}
	reason := state().reason
	if !strings.Contains(reason, "5 consecutive failed deliveries") || !strings.Contains(reason, "HTTP 503") {
		t.Fatalf("disable reason not visible: %q", reason)
	}
	deliveries := e.owner.Do("GET", "/api/v1/integrations/webhooks/"+id+"/deliveries", nil).Must(t, 200).Items()
	statuses := map[string]float64{}
	for _, d := range deliveries {
		statuses[d["status"].(string)] = d["attempts"].(float64)
	}
	if statuses["failed"] != 3 || statuses["pending"] != 2 {
		t.Fatalf("expected one failed (3 attempts) and one pending (2 attempts): %v", deliveries)
	}
	var failedID string
	for _, d := range deliveries {
		if d["status"] == "failed" {
			failedID = d["id"].(string)
		} else if d["next_attempt_at"] == nil {
			t.Fatalf("pending delivery without a next attempt: %v", d)
		}
	}
	detail := e.owner.Do("GET", "/api/v1/integrations/deliveries/"+failedID, nil).Must(t, 200)
	attempts := detail.Get("attempt_log").([]any)
	if len(attempts) != 3 || len(detail.Str("response_body")) > 2048 || detail.Get("response_status") != float64(503) {
		t.Fatalf("attempt log wrong: %s", detail.Body)
	}
	var last time.Time
	for _, a := range attempts {
		at, _ := time.Parse(time.RFC3339Nano, a.(map[string]any)["attempted_at"].(string))
		if !at.After(last) {
			t.Fatal("attempts not spaced in time")
		}
		last = at
	}
	audit := e.owner.Do("GET", "/api/v1/audit-logs?action=webhook.auto_disable", nil).Must(t, 200).Items()
	if len(audit) != 1 || audit[0]["entity_id"] != id {
		t.Fatalf("auto disable not audited: %v", audit)
	}
	// Nothing is sent while disabled.
	e.worker.DrainWebhooks(t.Context())
	if len(rc.requests()) != 5 {
		t.Fatal("disabled endpoint still receives deliveries")
	}

	// After the receiver recovers, re-enabling resumes the pending delivery.
	rc.respond(200, "ok")
	re := e.owner.Do("PUT", "/api/v1/integrations/webhooks/"+id, map[string]any{"name": "Flaky ERP", "url": rc.srv.URL, "events": allEvents, "enabled": true}).Must(t, 200)
	if re.Str("disabled_reason") != "" || re.Get("consecutive_failures") != float64(0) {
		t.Fatalf("re-enable did not reset: %s", re.Body)
	}
	e.waitFor("the pending delivery", func() bool { return len(rc.requests()) == 6 })
	e.owner.Do("POST", "/api/v1/integrations/deliveries/"+failedID+"/redeliver", nil).Must(t, 202)
	e.waitFor("the redelivery", func() bool { return len(rc.requests()) == 7 })
	if w := e.owner.Do("GET", "/api/v1/integrations/webhooks/"+id, nil).Must(t, 200); w.Get("last_success_at") == nil || w.Get("pending_deliveries") != float64(0) {
		t.Fatalf("endpoint state after recovery: %s", w.Body)
	}
}

func TestWebhookTargetsAreSSRFProtected(t *testing.T) {
	strict := newEnv(t)
	for _, u := range []string{"http://127.0.0.1:9/hook", "https://127.0.0.1/hook", "https://10.0.0.5/hook", "https://192.168.0.10/hook",
		"https://169.254.169.254/latest/meta-data", "https://[::1]/hook", "https://localhost/hook", "http://example.com/hook",
		"ftp://example.com/hook", "https://user:secret@example.com/hook", "javascript:alert(1)"} {
		res := strict.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "x", "url": u, "events": allEvents, "enabled": true})
		if res.Status != 422 || res.Get("error.fields.url") == nil {
			t.Errorf("%s accepted: %d %s", u, res.Status, res.Body)
		}
	}
	strict.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "Public", "url": "https://hooks.example.com/rea",
		"events": allEvents, "enabled": false}).Must(t, 201)

	// The resolved address is checked again on every connection, so a target that becomes private
	// (DNS rebinding, or the operator turning the allowance off) is blocked at dial time.
	e := newEnvWith(t, localWebhooks)
	rc := newReceiver(t)
	hook := e.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "Local", "url": rc.srv.URL, "events": allEvents,
		"enabled": true}).Must(t, 201).Str("webhook.id")
	e.cfg.WebhookAllowPrivate = false
	e.cfg.WebhookAllowHTTPLocalhost = false
	e.publish("Blocked", "S-1", nil)
	e.waitFor("a blocked attempt", func() bool {
		items := e.owner.Do("GET", "/api/v1/integrations/webhooks/"+hook+"/deliveries", nil).Must(t, 200).Items()
		return len(items) == 1 && items[0]["attempts"].(float64) >= 1
	})
	d := e.owner.Do("GET", "/api/v1/integrations/webhooks/"+hook+"/deliveries", nil).Must(t, 200).Items()[0]
	if !strings.Contains(d["error"].(string), "blocked") || d["response_status"] != nil || len(rc.requests()) != 0 {
		t.Fatalf("private target not blocked at dial time: %v (receiver got %d)", d, len(rc.requests()))
	}
	// The test button applies the same rules before sending.
	e.owner.Do("POST", "/api/v1/integrations/webhooks/"+hook+"/test", nil).Must(t, 422)
}
