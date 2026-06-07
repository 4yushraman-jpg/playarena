// Package webhookworker implements the WebhookWorker: a background goroutine
// that claims pending webhook_deliveries rows and delivers them via signed HTTP
// POST to the registered endpoint URLs.
//
// Delivery guarantees:
//
//	At-least-once: a crash after POST but before RecordSuccess causes a retry.
//	sent_at IS NULL guard in RecordWebhookDeliverySuccess makes re-recording a no-op.
//
// Retry schedule (attempt_count incremented at claim time):
//
//	Attempt 1 fails → retry in 1 minute
//	Attempt 2 fails → retry in 5 minutes
//	Attempt 3 fails → failed_permanently = TRUE (dead-letter)
//
// HTTP 4xx (except 429) → immediate permanent failure (client bug, no retry value).
// HTTP 429, 5xx, network errors → normal retry schedule.
//
// SSRF protection: each delivery uses the SSRFSafeTransport from the webhooks
// package, which resolves DNS and verifies all IPs are public before connecting.
// Test builds inject a mock http.Client to bypass the network requirement.
package webhookworker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/webhooks"
)

const (
	maxAttempts int32 = 3
	batchSize   int32 = 10
	// deliveryTimeout is the per-request deadline for the outbound HTTP call.
	deliveryTimeout = 30 * time.Second
)

// WebhookWorker polls the database for pending webhook deliveries and delivers
// them via signed HTTPS POST. Lifecycle is managed through Start / Stop / Drain.
type WebhookWorker struct {
	repo      *Repository
	secretKey []byte // 32-byte AES-256-GCM key (same key used by the webhooks service)
	client    *http.Client
	interval  time.Duration
	log       *slog.Logger
	done      chan struct{}
}

// NewWebhookWorker constructs a WebhookWorker.
// secretKeyB64 must be the base64-encoded 32-byte AES key used to encrypt secrets.
// client may be nil, in which case a production SSRF-safe client is used.
func NewWebhookWorker(
	pool *pgxpool.Pool,
	secretKeyB64 string,
	client *http.Client,
	interval time.Duration,
	log *slog.Logger,
) (*WebhookWorker, error) {
	key, err := base64.StdEncoding.DecodeString(secretKeyB64)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(secretKeyB64)
		if err != nil {
			return nil, fmt.Errorf("webhookworker: invalid secret key encoding: %w", err)
		}
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("webhookworker: WEBHOOK_SECRET_KEY must be 32 bytes (got %d)", len(key))
	}

	if client == nil {
		client = &http.Client{
			Transport: webhooks.SSRFSafeTransport(),
			Timeout:   deliveryTimeout,
		}
	}

	return &WebhookWorker{
		repo:      NewRepository(pool),
		secretKey: key,
		client:    client,
		interval:  interval,
		log:       log,
		done:      make(chan struct{}),
	}, nil
}

// Start launches the polling loop in a background goroutine. Non-blocking.
func (w *WebhookWorker) Start() {
	go w.run()
}

// Stop signals the polling loop to exit. Non-blocking.
func (w *WebhookWorker) Stop() {
	select {
	case <-w.done:
	default:
		close(w.done)
	}
}

// Drain runs one final delivery pass. Called during graceful shutdown.
func (w *WebhookWorker) Drain(ctx context.Context) error {
	return w.runOnce(ctx)
}

func (w *WebhookWorker) run() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx := context.Background()
			if err := w.runOnce(ctx); err != nil {
				w.log.Error("webhookworker: tick error", slog.Any("error", err))
			}
		case <-w.done:
			return
		}
	}
}

func (w *WebhookWorker) runOnce(ctx context.Context) error {
	rows, err := w.repo.ClaimBatch(ctx, maxAttempts, batchSize)
	if err != nil {
		return err
	}
	for _, row := range rows {
		w.deliver(ctx, row)
	}
	return nil
}

// webhookPayload is the versioned envelope delivered to the receiver.
type webhookPayload struct {
	Version        string          `json:"version"`
	EventID        string          `json:"event_id"`
	EventType      string          `json:"event_type"`
	OrganizationID string          `json:"organization_id"`
	EntityType     string          `json:"entity_type"`
	EntityID       string          `json:"entity_id"`
	Timestamp      string          `json:"timestamp"`
	Payload        json.RawMessage `json:"payload"`
}

func (w *WebhookWorker) deliver(ctx context.Context, d db.WebhookDelivery) {
	did := pgutil.UUIDToString(d.ID)

	ep, err := w.repo.GetEndpoint(ctx, d.ID)
	if err != nil {
		w.log.Error("webhookworker: get endpoint",
			slog.String("delivery_id", did),
			slog.Any("error", err),
		)
		w.recordFailure(ctx, d)
		return
	}

	// Decrypt the endpoint secret so we can sign the payload.
	rawSecret, err := webhooks.DecryptSecret(w.secretKey, ep.SecretCiphertext)
	if err != nil {
		w.log.Error("webhookworker: decrypt secret",
			slog.String("delivery_id", did),
			slog.String("endpoint_id", pgutil.UUIDToString(ep.ID)),
			slog.Any("error", err),
		)
		w.recordFailure(ctx, d)
		return
	}

	// Capture a single instant for both the envelope timestamp and the HMAC
	// canonical string so that payload.timestamp and X-PlayArena-Timestamp
	// always represent the same second.
	now := time.Now().UTC()

	envelope := webhookPayload{
		Version:        "1",
		EventID:        pgutil.UUIDToString(d.ID),
		EventType:      string(d.EventType),
		OrganizationID: pgutil.UUIDToString(d.OrganizationID),
		EntityType:     d.EntityType,
		EntityID:       pgutil.UUIDToString(d.EntityID),
		Timestamp:      now.Format(time.RFC3339),
		Payload:        json.RawMessage(d.Payload),
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		w.log.Error("webhookworker: marshal payload",
			slog.String("delivery_id", did),
			slog.Any("error", err),
		)
		w.recordFailure(ctx, d)
		return
	}

	// Build the HMAC-SHA256 signature over: <timestamp>\n<event_id>\n<body>
	tsUnix := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(rawSecret))
	mac.Write([]byte(tsUnix + "\n" + pgutil.UUIDToString(d.ID) + "\n"))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.Url, bytes.NewReader(body))
	if err != nil {
		w.log.Error("webhookworker: build request",
			slog.String("delivery_id", did),
			slog.String("url", ep.Url),
			slog.Any("error", err),
		)
		w.recordFailure(ctx, d)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PlayArena-Signature", sig)
	req.Header.Set("X-PlayArena-Timestamp", tsUnix)
	req.Header.Set("X-PlayArena-Event-ID", pgutil.UUIDToString(d.ID))

	resp, err := w.client.Do(req)
	if err != nil {
		w.log.Error("webhookworker: http delivery failed",
			slog.String("delivery_id", did),
			slog.String("url", ep.Url),
			slog.Any("error", err),
		)
		w.recordFailure(ctx, d)
		return
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	// HTTP 4xx (except 429) → immediate permanent failure.
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
		w.log.Warn("webhookworker: permanent failure (4xx)",
			slog.String("delivery_id", did),
			slog.String("url", ep.Url),
			slog.Int("status", resp.StatusCode),
		)
		if err := w.repo.RecordFailure(ctx, d.ID, true, pgtype.Timestamptz{}); err != nil {
			w.log.Error("webhookworker: record permanent failure",
				slog.String("delivery_id", did),
				slog.Any("error", err),
			)
		}
		return
	}

	// 2xx → success.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := w.repo.RecordSuccess(ctx, d.ID); err != nil {
			w.log.Error("webhookworker: record success",
				slog.String("delivery_id", did),
				slog.Any("error", err),
			)
		} else {
			w.log.Info("webhookworker: delivered",
				slog.String("delivery_id", did),
				slog.String("url", ep.Url),
				slog.String("event_type", string(d.EventType)),
				slog.Int("attempt_count", int(d.AttemptCount)),
			)
		}
		return
	}

	// Network errors, too-many-redirects (Go follows up to 10 hops), 5xx, 429 → retry.
	w.log.Warn("webhookworker: retryable response",
		slog.String("delivery_id", did),
		slog.String("url", ep.Url),
		slog.Int("status", resp.StatusCode),
	)
	w.recordFailure(ctx, d)
}

func (w *WebhookWorker) recordFailure(ctx context.Context, d db.WebhookDelivery) {
	perm := d.AttemptCount >= maxAttempts
	var nextAt pgtype.Timestamptz
	if !perm {
		nextAt = pgtype.Timestamptz{
			Time:  time.Now().UTC().Add(retryDelay(d.AttemptCount)),
			Valid: true,
		}
	}
	if err := w.repo.RecordFailure(ctx, d.ID, perm, nextAt); err != nil {
		w.log.Error("webhookworker: record failure",
			slog.String("delivery_id", pgutil.UUIDToString(d.ID)),
			slog.Any("error", err),
		)
	}
	if perm {
		w.log.Warn("webhookworker: permanently failed",
			slog.String("delivery_id", pgutil.UUIDToString(d.ID)),
			slog.Int("attempt_count", int(d.AttemptCount)),
		)
	}
}

// retryDelay returns the wait before the next delivery attempt.
// attempt_count is incremented at claim time.
func retryDelay(attemptCount int32) time.Duration {
	switch attemptCount {
	case 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	default:
		return 15 * time.Minute
	}
}
