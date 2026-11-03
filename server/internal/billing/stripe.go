package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// StripeProvider is a minimal test-mode adapter for Stripe Checkout. It never
// embeds a key; the key is supplied at runtime. Price data is inlined from the
// plan catalog so no Stripe dashboard setup is required for a test run.
type StripeProvider struct {
	SecretKey  string
	BaseURL    string
	SuccessURL string
	CancelURL  string
	HTTP       *http.Client
}

func NewStripe(secret, successURL, cancelURL string) *StripeProvider {
	return &StripeProvider{SecretKey: secret, BaseURL: "https://api.stripe.com", SuccessURL: successURL, CancelURL: cancelURL,
		HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (p *StripeProvider) Name() string { return "stripe" }

func (p *StripeProvider) CreateCheckout(ctx context.Context, tenantID, planKey string) (Checkout, error) {
	plan, ok := Get(planKey)
	if !ok {
		return Checkout{}, fmt.Errorf("unknown plan %q", planKey)
	}
	if strings.TrimSpace(p.SecretKey) == "" {
		return Checkout{}, fmt.Errorf("stripe secret key is not configured")
	}
	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("success_url", p.SuccessURL)
	form.Set("cancel_url", p.CancelURL)
	form.Set("client_reference_id", tenantID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", plan.Currency)
	form.Set("line_items[0][price_data][unit_amount]", fmt.Sprintf("%d", plan.PriceCents))
	form.Set("line_items[0][price_data][recurring][interval]", "month")
	form.Set("line_items[0][price_data][product_data][name]", "UCenter "+plan.Name)
	endpoint := strings.TrimRight(p.BaseURL, "/") + "/v1/checkout/sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Checkout{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.SecretKey, "")
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return Checkout{}, err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return Checkout{}, fmt.Errorf("stripe checkout failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return Checkout{}, err
	}
	return Checkout{URL: out.URL, SessionID: out.ID}, nil
}

func (p *StripeProvider) Cancel(ctx context.Context, subscriptionID string) error {
	if strings.TrimSpace(p.SecretKey) == "" {
		return fmt.Errorf("stripe secret key is not configured")
	}
	endpoint := strings.TrimRight(p.BaseURL, "/") + "/v1/subscriptions/" + url.PathEscape(subscriptionID)
	form := url.Values{"cancel_at_period_end": {"true"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.SecretKey, "")
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("stripe cancel failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}
