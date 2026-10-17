package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/catalog"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/safefetch"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/secretbox"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

const (
	// maxResponseRead caps how much of a receiver's response is read; storedResponse caps what is kept.
	maxResponseRead = 64 << 10
	storedResponse  = 2 << 10
	fanOutBatch     = 200
	// StaleAfter is how long a delivery may stay "delivering" before it is retried (worker crash).
	StaleAfter = 5 * time.Minute
)

// Dispatcher turns catalog changes into deliveries and sends them.
type Dispatcher struct {
	cfg    *config.Config
	db     *store.DB
	box    *secretbox.Box
	log    *slog.Logger
	client *http.Client
	id     string
}

// New creates a dispatcher. id identifies the process in delivery locks.
func New(cfg *config.Config, db *store.DB, box *secretbox.Box, log *slog.Logger, id string) *Dispatcher {
	client := safefetch.NewClient(func() safefetch.Policy { return Policy(cfg) }, cfg.WebhookTimeout)
	return &Dispatcher{cfg: cfg, db: db, box: box, log: log.With(slog.String("component", "webhooks")), client: client, id: id}
}

// RunOnce sequences pending catalog changes, fans them out to enabled endpoints and sends at most
// one due delivery. It reports whether any work was done.
func (d *Dispatcher) RunOnce(ctx context.Context) bool {
	did := false
	if n, err := catalog.Sequence(ctx, d.db, false); err != nil {
		d.logErr(ctx, "sequence catalog changes", err)
	} else if n > 0 {
		did = true
	}
	if n, err := d.FanOut(ctx); err != nil {
		d.logErr(ctx, "fan out webhook deliveries", err)
	} else if n > 0 {
		did = true
	}
	sent, err := d.DeliverDue(ctx)
	if err != nil {
		d.logErr(ctx, "deliver webhook", err)
	}
	return did || sent
}

func (d *Dispatcher) logErr(ctx context.Context, what string, err error) {
	if ctx.Err() == nil {
		d.log.Error(what, slog.String("error", err.Error()))
	}
}

// FanOut creates deliveries for changes each enabled endpoint has not seen yet. Each endpoint is
// locked while it is processed, so several workers can run it concurrently.
func (d *Dispatcher) FanOut(ctx context.Context) (int, error) {
	ids, err := d.db.ListFanOutEndpointIDs(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, id := range ids {
		err := d.db.Tx(ctx, func(q *store.Queries, _ pgx.Tx) error {
			ep, err := q.LockWebhookEndpointForFanOut(ctx, id)
			if store.IsNotFound(err) {
				return nil // disabled meanwhile, or another worker holds it
			}
			if err != nil {
				return err
			}
			changes, err := q.ListCatalogChanges(ctx, store.ListCatalogChangesParams{AfterID: ep.ChangeCursor, Lim: fanOutBatch})
			if err != nil || len(changes) == 0 {
				return err
			}
			var pids []uuid.UUID
			for _, c := range changes {
				if slices.Contains(ep.Events, catalog.EventTypeOf(c.Kind)) && !catalog.Tombstone(catalog.EventTypeOf(c.Kind)) {
					pids = append(pids, c.ProductID)
				}
			}
			products, err := catalog.Load(ctx, q, pids)
			if err != nil {
				return err
			}
			for _, c := range changes {
				typ := catalog.EventTypeOf(c.Kind)
				if !slices.Contains(ep.Events, typ) || (c.SourcingOnly && !ep.IncludeCosts) {
					continue
				}
				var prod *catalog.CatalogProduct
				if p, ok := products[c.ProductID]; ok {
					prod = &p
				}
				payload, err := json.Marshal(catalog.NewEvent(c, prod, ep.IncludeCosts, "full"))
				if err != nil {
					return err
				}
				changeID, pid := c.ID, c.ProductID
				if _, err := q.CreateWebhookDelivery(ctx, store.CreateWebhookDeliveryParams{EndpointID: ep.ID, EventID: c.EventID,
					EventType: typ, ChangeID: &changeID, ProductID: &pid, Payload: payload, MaxAttempts: int32(d.cfg.WebhookMaxAttempts),
					Status: "pending"}); err != nil {
					return err
				}
				total++
			}
			return q.AdvanceWebhookCursor(ctx, store.AdvanceWebhookCursorParams{ID: ep.ID, ChangeCursor: changes[len(changes)-1].ID})
		})
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// DeliverDue claims one due delivery and attempts it.
func (d *Dispatcher) DeliverDue(ctx context.Context) (bool, error) {
	del, err := d.db.ClaimWebhookDelivery(ctx, d.id)
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	ep, err := d.db.GetWebhookEndpoint(ctx, del.EndpointID)
	if err != nil {
		return true, err
	}
	_, err = d.Attempt(ctx, ep, del)
	return true, err
}

// Result is the outcome of one HTTP attempt.
type Result struct {
	StatusCode *int32
	Latency    time.Duration
	Body       string
	Err        string
}

// OK reports a 2xx response.
func (r Result) OK() bool {
	return r.Err == "" && r.StatusCode != nil && *r.StatusCode >= 200 && *r.StatusCode < 300
}

// Attempt sends a claimed delivery (status "delivering", attempts already incremented) and records
// the result: success, a scheduled retry, or final failure. Failed attempts of real events count
// towards disabling the endpoint; test events do not.
func (d *Dispatcher) Attempt(ctx context.Context, ep store.WebhookEndpoint, del store.WebhookDelivery) (store.WebhookDelivery, error) {
	res := d.send(ctx, ep, del)
	var out store.WebhookDelivery
	var disabled string
	// The result is recorded even when the caller's context ends during the attempt.
	ctx = context.WithoutCancel(ctx)
	latency := int32(res.Latency.Milliseconds())
	err := d.db.Tx(ctx, func(q *store.Queries, _ pgx.Tx) error {
		if err := q.InsertWebhookAttempt(ctx, store.InsertWebhookAttemptParams{DeliveryID: del.ID, Attempt: del.Attempts,
			StatusCode: res.StatusCode, LatencyMs: latency, Error: res.Err, ResponseBody: res.Body}); err != nil {
			return err
		}
		params := store.FinishWebhookDeliveryParams{ID: del.ID, NextAttemptAt: time.Now(), ResponseStatus: res.StatusCode,
			ResponseBody: res.Body, LatencyMs: &latency, Error: res.Err}
		switch {
		case res.OK():
			params.Status = "succeeded"
		case del.Attempts >= del.MaxAttempts:
			params.Status = "failed"
		default:
			params.Status = "pending"
			params.NextAttemptAt = time.Now().Add(Backoff(d.cfg, int(del.Attempts)))
		}
		var err error
		if out, err = q.FinishWebhookDelivery(ctx, params); err != nil {
			return err
		}
		if del.IsTest {
			return nil
		}
		if res.OK() {
			return q.RecordWebhookSuccess(ctx, ep.ID)
		}
		cur, err := q.RecordWebhookFailure(ctx, ep.ID)
		if err != nil {
			return err
		}
		if int(cur.ConsecutiveFailures) >= d.cfg.WebhookDisableAfterFailures && cur.FailingSince != nil &&
			time.Since(*cur.FailingSince) >= d.cfg.WebhookDisableFailingFor && cur.Enabled {
			reason := fmt.Sprintf("Disabled automatically after %d consecutive failed deliveries since %s. Last error: %s",
				cur.ConsecutiveFailures, cur.FailingSince.UTC().Format(time.RFC3339), describe(res))
			if n, err := q.DisableWebhookEndpoint(ctx, store.DisableWebhookEndpointParams{ID: ep.ID, DisabledReason: reason}); err != nil {
				return err
			} else if n > 0 {
				disabled = reason
				return q.InsertAuditLog(ctx, store.InsertAuditLogParams{ActorLabel: "worker", Action: "webhook.auto_disable",
					EntityType: "webhook_endpoint", EntityID: &ep.ID, Summary: truncate("Disabled webhook "+ep.Name+": "+reason, 500)})
			}
		}
		return nil
	})
	if disabled != "" {
		d.log.Warn("webhook endpoint disabled", slog.String("endpoint_id", ep.ID.String()), slog.String("reason", disabled))
	}
	return out, err
}

func describe(r Result) string {
	if r.Err != "" {
		return r.Err
	}
	if r.StatusCode != nil {
		return "HTTP " + strconv.Itoa(int(*r.StatusCode))
	}
	return "no response"
}

// send performs one signed POST. The secret is decrypted only for signing and never logged.
func (d *Dispatcher) send(ctx context.Context, ep store.WebhookEndpoint, del store.WebhookDelivery) Result {
	secrets, err := d.secrets(ep)
	if err != nil {
		return Result{Err: "the signing secret could not be decrypted (was REA_SECRET_KEY changed?)"}
	}
	body := []byte(del.Payload)
	ts := time.Now().Unix()
	ctx, cancel := context.WithTimeout(ctx, d.cfg.WebhookTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.Url, bytes.NewReader(body))
	if err != nil {
		return Result{Err: "invalid URL"}
	}
	h := req.Header
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", UserAgent)
	h.Set(HeaderEventID, del.EventID.String())
	h.Set(HeaderEventType, del.EventType)
	h.Set(HeaderDeliveryID, del.ID.String())
	h.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	h.Set(HeaderSignature, SignatureHeader(secrets, ts, body))
	start := time.Now()
	resp, err := d.client.Do(req)
	if err != nil {
		return Result{Latency: time.Since(start), Err: describeError(err, d.cfg.WebhookTimeout)}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseRead))
	code := int32(resp.StatusCode)
	res := Result{StatusCode: &code, Latency: time.Since(start), Body: cleanText(raw, storedResponse)}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		res.Err = "redirects are not followed (HTTP " + strconv.Itoa(resp.StatusCode) + "); register the final URL"
	}
	return res
}

func (d *Dispatcher) secrets(ep store.WebhookEndpoint) ([]string, error) {
	cur, err := d.box.Open(ep.SecretCiphertext)
	if err != nil {
		return nil, err
	}
	out := []string{string(cur)}
	if len(ep.PreviousSecretCiphertext) > 0 && ep.PreviousSecretExpiresAt != nil && time.Now().Before(*ep.PreviousSecretExpiresAt) {
		if prev, err := d.box.Open(ep.PreviousSecretCiphertext); err == nil {
			out = append(out, string(prev))
		}
	}
	return out, nil
}

func describeError(err error, timeout time.Duration) string {
	switch {
	case safefetch.IsBlockedError(err):
		return "blocked: " + safefetch.ErrBlockedAddress.Error() + " (REA_WEBHOOK_ALLOW_PRIVATE_NETWORKS)"
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Client.Timeout") || strings.Contains(err.Error(), "deadline exceeded"):
		return "timed out after " + timeout.String()
	case strings.Contains(err.Error(), "connection refused"):
		return "connection refused"
	case strings.Contains(err.Error(), "no such host"):
		return "the host name could not be resolved"
	case strings.Contains(err.Error(), "certificate"):
		return "TLS certificate error: " + truncate(lastPart(err.Error()), 200)
	}
	return truncate(lastPart(err.Error()), 300)
}

func lastPart(s string) string {
	if i := strings.LastIndex(s, ": "); i >= 0 && i+2 < len(s) {
		return s[i+2:]
	}
	return s
}

// cleanText makes a response body safe to store: valid UTF-8, no NUL bytes, at most n bytes.
func cleanText(b []byte, n int) string {
	if len(b) > n {
		b = b[:n]
	}
	s := strings.ToValidUTF8(string(b), "")
	return strings.ReplaceAll(s, "\x00", "")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// TestPayload builds the body of a test event: a sample of the current catalog when one exists.
func TestPayload(ctx context.Context, q *store.Queries, includeCosts bool) (catalog.CatalogEvent, error) {
	ev := catalog.CatalogEvent{ID: uuid.New(), Type: catalog.EventTest, OccurredAt: time.Now().UTC(), ChangedFields: []string{}}
	pid, err := q.LatestCatalogProductID(ctx)
	if store.IsNotFound(err) {
		return ev, nil
	}
	if err != nil {
		return ev, err
	}
	products, err := catalog.Load(ctx, q, []uuid.UUID{pid})
	if err != nil {
		return ev, err
	}
	if p, ok := products[pid]; ok {
		if !includeCosts {
			p = p.WithoutSourcing()
		}
		ev.ProductID, ev.SKU, ev.Product = &pid, p.SKU, &p
	}
	return ev, nil
}

// SendTest records a test delivery and attempts it once, synchronously.
func (d *Dispatcher) SendTest(ctx context.Context, ep store.WebhookEndpoint) (store.WebhookDelivery, error) {
	ev, err := TestPayload(ctx, d.db.Queries, ep.IncludeCosts)
	if err != nil {
		return store.WebhookDelivery{}, err
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return store.WebhookDelivery{}, err
	}
	del, err := d.db.CreateWebhookDelivery(ctx, store.CreateWebhookDeliveryParams{EndpointID: ep.ID, EventID: ev.ID, EventType: ev.Type,
		ProductID: ev.ProductID, Payload: payload, IsTest: true, MaxAttempts: 1, Status: "delivering", Attempts: 1, LockedBy: d.id})
	if err != nil {
		return del, err
	}
	return d.Attempt(ctx, ep, del)
}
