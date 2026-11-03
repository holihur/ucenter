package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/config"
	"ucenter/internal/models"
	"ucenter/internal/security"
	"ucenter/internal/store"
)

type Dispatcher struct {
	store  *store.Store
	cipher *security.Cipher
	cfg    config.Config
	http   *http.Client
	logger *slog.Logger
}

func New(s *store.Store, cipher *security.Cipher, cfg config.Config, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		store: s, cipher: cipher, cfg: cfg, logger: logger,
		http: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return fmt.Errorf("webhook redirects are not allowed")
			},
		},
	}
}

func (d *Dispatcher) Publish(ctx context.Context, tenantID primitive.ObjectID, event string, data map[string]interface{}) {
	endpoints, err := d.store.ListWebhookEndpoints(ctx, tenantID)
	if err != nil {
		return
	}
	if len(endpoints) == 0 {
		return
	}
	eventID, _ := security.RandomHex(12)
	envelope := map[string]interface{}{
		"id": eventID, "type": event, "tenant_id": tenantID.Hex(),
		"created_at": time.Now().UTC().Format(time.RFC3339), "data": data,
	}
	payload, _ := json.Marshal(envelope)
	for i := range endpoints {
		endpoint := &endpoints[i]
		if !endpoint.Enabled {
			continue
		}
		if len(endpoint.Events) > 0 && !contains(endpoint.Events, event) && !contains(endpoint.Events, "*") {
			continue
		}
		delivery := &models.WebhookDelivery{
			TenantID: tenantID, EndpointID: endpoint.ID, Event: event, Payload: string(payload),
			Status: "pending", NextAttemptAt: time.Now(),
		}
		_ = d.store.CreateWebhookDelivery(ctx, delivery)
	}
}

// Test enqueues a sample delivery for one endpoint regardless of its event filter.
func (d *Dispatcher) Test(ctx context.Context, tenantID primitive.ObjectID, endpointID primitive.ObjectID) error {
	endpoint, err := d.store.FindWebhookEndpoint(ctx, tenantID, endpointID)
	if err != nil {
		return err
	}
	eventID, _ := security.RandomHex(12)
	payload, _ := json.Marshal(map[string]interface{}{
		"id": eventID, "type": "webhook.test", "tenant_id": tenantID.Hex(),
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"data":       map[string]interface{}{"message": "This is a test delivery from UCenter", "endpoint": endpoint.Name},
	})
	return d.store.CreateWebhookDelivery(ctx, &models.WebhookDelivery{
		TenantID: tenantID, EndpointID: endpoint.ID, Event: "webhook.test",
		Payload: string(payload), Status: "pending", NextAttemptAt: time.Now(),
	})
}

func (d *Dispatcher) Start(ctx context.Context) {
	interval := time.Duration(d.cfg.WebhookWorkerSeconds) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.process(ctx)
		}
	}
}

func (d *Dispatcher) process(ctx context.Context) {
	for i := 0; i < 10; i++ {
		delivery, err := d.store.ClaimWebhookDelivery(ctx, time.Minute)
		if err != nil {
			return
		}
		d.deliver(ctx, delivery)
	}
}

func (d *Dispatcher) deliver(ctx context.Context, delivery *models.WebhookDelivery) {
	endpoint, err := d.store.FindWebhookEndpoint(ctx, delivery.TenantID, delivery.EndpointID)
	if err != nil || !endpoint.Enabled {
		_ = d.store.UpdateWebhookDelivery(ctx, delivery.ID, bson.M{"status": "dead", "error": "endpoint is missing or disabled"})
		return
	}
	secret, err := d.cipher.Decrypt(endpoint.SecretEnc)
	if err != nil {
		_ = d.store.UpdateWebhookDelivery(ctx, delivery.ID, bson.M{"status": "dead", "error": "endpoint secret cannot be decrypted"})
		return
	}
	if err := ValidateURL(endpoint.URL, d.cfg.WebhookAllowPrivate); err != nil {
		d.fail(ctx, delivery, 0, "ssrf: "+err.Error())
		return
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + delivery.Payload))
	signature := hex.EncodeToString(mac.Sum(nil))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader([]byte(delivery.Payload)))
	if err != nil {
		d.fail(ctx, delivery, 0, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "UCenter-Webhooks/1.0")
	req.Header.Set("X-UCenter-Event", delivery.Event)
	req.Header.Set("X-UCenter-Delivery", delivery.ID.Hex())
	req.Header.Set("X-UCenter-Timestamp", timestamp)
	req.Header.Set("X-UCenter-Signature", "sha256="+signature)
	resp, err := d.http.Do(req)
	if err != nil {
		d.fail(ctx, delivery, 0, err.Error())
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		now := time.Now()
		_ = d.store.UpdateWebhookDelivery(ctx, delivery.ID, bson.M{
			"status": "delivered", "attempts": delivery.Attempts + 1,
			"response_code": resp.StatusCode, "delivered_at": now, "error": "",
		})
		return
	}
	d.fail(ctx, delivery, resp.StatusCode, fmt.Sprintf("HTTP %d", resp.StatusCode))
}

func (d *Dispatcher) fail(ctx context.Context, delivery *models.WebhookDelivery, code int, message string) {
	attempts := delivery.Attempts + 1
	max := d.cfg.WebhookMaxAttempts
	if max <= 0 {
		max = 5
	}
	update := bson.M{"attempts": attempts, "response_code": code, "error": message}
	if attempts >= max {
		update["status"] = "dead"
	} else {
		base := d.cfg.WebhookRetryBase
		if base <= 0 {
			base = 30
		}
		delay := time.Duration(base) * time.Second * time.Duration(1<<min(attempts-1, 10))
		update["status"] = "retrying"
		update["next_attempt_at"] = time.Now().Add(delay)
	}
	_ = d.store.UpdateWebhookDelivery(ctx, delivery.ID, update)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// ValidateURL blocks SSRF targets. In development private/loopback targets are
// allowed for local receivers, but link-local and metadata addresses are always
// rejected. Production requires https and globally routable addresses.
func ValidateURL(raw string, allowPrivate bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("invalid webhook URL")
	}
	if u.User != nil {
		return fmt.Errorf("webhook URL must not contain credentials")
	}
	if u.Scheme != "https" && !(allowPrivate && u.Scheme == "http") {
		return fmt.Errorf("webhook URL must use https")
	}
	ips, err := net.LookupIP(u.Hostname())
	if err != nil {
		return fmt.Errorf("cannot resolve webhook host")
	}
	for _, ip := range ips {
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("webhook target is not allowed")
		}
		if !allowPrivate && (ip.IsPrivate() || ip.IsLoopback()) {
			return fmt.Errorf("webhook target is not publicly routable")
		}
	}
	return nil
}
