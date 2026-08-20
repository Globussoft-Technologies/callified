// Package webhook provides fire-and-forget HTTP webhook dispatch with HMAC-SHA256 signing.
// Port of webhook_dispatch.py.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/trace"
)

// maxWebhookRetries is the number of immediate retries before a webhook goes to the DLQ.
const maxWebhookRetries = 3

// retryableWebhookStatus returns true for network errors or 5xx responses.
func retryableWebhookStatus(statusCode int, err error) bool {
	if err != nil {
		return true
	}
	return statusCode >= 500 || statusCode == 429 || statusCode == 408
}

// Dispatcher fans out webhook events to all registered org endpoints.
type Dispatcher struct {
	database *db.DB
	client   *http.Client
	log      *zap.Logger
}

// New creates a Dispatcher with a 10-second HTTP timeout per delivery.
func New(database *db.DB, log *zap.Logger) *Dispatcher {
	return &Dispatcher{
		database: database,
		client:   &http.Client{Timeout: 10 * time.Second},
		log:      log,
	}
}

// Dispatch sends event+data to all active webhooks for the org asynchronously.
// It is fire-and-forget: returns immediately; each delivery runs in its own goroutine.
func (d *Dispatcher) Dispatch(ctx context.Context, orgID int64, event string, data any) {
	traceID := trace.FromContext(ctx)
	hooks, err := d.database.GetActiveWebhooksForEvent(orgID, event)
	if err != nil {
		d.log.Warn("webhook dispatch: fetch hooks", zap.Error(err))
		return
	}
	for _, wh := range hooks {
		wh := wh
		go d.deliver(wh, event, data, traceID)
	}
}

func (d *Dispatcher) deliver(wh db.Webhook, event string, data any, traceID string) {
	payload := map[string]any{
		"event":     event,
		"org_id":    wh.OrgID,
		"timestamp": time.Now().Unix(),
		"trace_id":  traceID,
		"data":      data,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		d.log.Warn("webhook deliver: marshal", zap.Error(err))
		return
	}

	lastStatus := 0
	lastResp := ""
	for attempt := 0; attempt < maxWebhookRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		status, respBody, err := d.sendOnce(wh, event, body, traceID)
		lastStatus = status
		lastResp = respBody
		if err == nil && !retryableWebhookStatus(status, nil) {
			break
		}
		d.log.Warn("webhook deliver: attempt failed, will retry",
			zap.Int64("webhook_id", wh.ID),
			zap.String("event", event),
			zap.Int("attempt", attempt+1),
			zap.Int("status", status),
			zap.Error(err))
	}

	if logErr := d.database.LogWebhookDelivery(wh.ID, event, lastStatus, lastResp); logErr != nil {
		d.log.Warn("webhook deliver: log failure", zap.Error(logErr))
	}

	if retryableWebhookStatus(lastStatus, nil) {
		if dlqErr := d.database.EnqueueWebhookDLQ(wh.ID, wh.OrgID, event, string(body), lastStatus, lastResp); dlqErr != nil {
			d.log.Warn("webhook deliver: DLQ enqueue failed", zap.Error(dlqErr))
		} else {
			d.log.Warn("webhook deliver: moved to DLQ",
				zap.Int64("webhook_id", wh.ID),
				zap.String("event", event),
				zap.Int("status", lastStatus))
		}
		return
	}

	d.log.Info("webhook delivered",
		zap.Int64("webhook_id", wh.ID),
		zap.String("event", event),
		zap.Int("status", lastStatus),
	)
}

func (d *Dispatcher) sendOnce(wh db.Webhook, event string, body []byte, traceID string) (int, string, error) {
	req, err := http.NewRequest(http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err.Error(), err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Callified-Event", event)
	if traceID != "" {
		req.Header.Set(trace.HeaderName, traceID)
	}
	if wh.SecretKey != "" {
		req.Header.Set("X-Callified-Signature", computeHMAC(wh.SecretKey, body))
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err.Error(), err
	}
	defer resp.Body.Close()

	// Capture a snippet of the response for debugging without reading large bodies.
	respBody := ""
	if resp.ContentLength <= 4096 {
		buf := make([]byte, 4096)
		n, _ := resp.Body.Read(buf)
		respBody = string(buf[:n])
	} else {
		respBody = fmt.Sprintf("<body %d bytes>", resp.ContentLength)
	}
	return resp.StatusCode, respBody, nil
}

// RetryDLQ attempts to redeliver up to `limit` DLQ entries. Successful retries
// are removed from the DLQ; failures are updated with the latest response.
func (d *Dispatcher) RetryDLQ(ctx context.Context, limit int) error {
	entries, err := d.database.GetPendingWebhookDLQ(limit)
	if err != nil {
		return fmt.Errorf("fetch webhook dlq: %w", err)
	}
	for _, e := range entries {
		wh := db.Webhook{ID: e.WebhookID, OrgID: e.OrgID, URL: "", Event: e.Event}
		// URL is not stored in DLQ; fetch it from webhooks table.
		if hooks, err := d.database.GetWebhooksByOrg(e.OrgID); err == nil {
			for _, h := range hooks {
				if h.ID == e.WebhookID {
					wh = h
					break
				}
			}
		}
		if wh.URL == "" {
			d.log.Warn("webhook dlq retry: webhook URL unavailable, skipping", zap.Int64("dlq_id", e.ID))
			continue
		}

		// Extract trace_id from the stored payload so retries keep the same trace.
		traceID := ""
		var payloadMap map[string]any
		if err := json.Unmarshal([]byte(e.Payload), &payloadMap); err == nil {
			if v, ok := payloadMap["trace_id"].(string); ok {
				traceID = v
			}
		}

		status, respBody, sendErr := d.sendOnce(wh, e.Event, []byte(e.Payload), traceID)
		if sendErr == nil && !retryableWebhookStatus(status, nil) {
			if delErr := d.database.DeleteWebhookDLQ(e.ID); delErr != nil {
				d.log.Warn("webhook dlq retry: delete failed", zap.Error(delErr))
			}
			_ = d.database.LogWebhookDelivery(wh.ID, e.Event, status, respBody)
			d.log.Info("webhook dlq retry: success", zap.Int64("dlq_id", e.ID), zap.Int("status", status))
			continue
		}

		if updErr := d.database.IncrementWebhookDLQAttempts(e.ID); updErr != nil {
			d.log.Warn("webhook dlq retry: increment attempts failed", zap.Error(updErr))
		}
		d.log.Warn("webhook dlq retry: still failing",
			zap.Int64("dlq_id", e.ID),
			zap.Int("status", status),
			zap.Error(sendErr))
	}
	return nil
}

// computeHMAC returns hex(HMAC-SHA256(secret, payload)).
func computeHMAC(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature validates a webhook signature header value against the payload.
func VerifySignature(secret string, payload []byte, sig string) bool {
	return hmac.Equal([]byte(computeHMAC(secret, payload)), []byte(sig))
}
