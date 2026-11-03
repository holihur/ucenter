// Package billing defines the plan catalog, usage limits and a provider
// abstraction. The mock provider is used locally and in CI; a Stripe adapter
// can be enabled with real test credentials without changing the API.
package billing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"ucenter/internal/models"
)

type Limits struct {
	Users         int `json:"users"`
	Clients       int `json:"clients"`
	MonthlyAPI    int `json:"monthlyApi"`
	MonthlyEmails int `json:"monthlyEmails"`
}

type Plan struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	PriceCents  int      `json:"priceCents"`
	Currency    string   `json:"currency"`
	Limits      Limits   `json:"limits"`
	Features    []string `json:"features"`
}

var Plans = []Plan{
	{Key: "free", Name: "Free", Description: "For evaluating UCenter with a small team.", PriceCents: 0, Currency: "usd",
		Limits:   Limits{Users: 50, Clients: 10, MonthlyAPI: 100000, MonthlyEmails: 5000},
		Features: []string{"OIDC + PKCE", "Password, TOTP and passkeys", "Community support"}},
	{Key: "pro", Name: "Pro", Description: "For growing teams that need enterprise sign-in.", PriceCents: 2900, Currency: "usd",
		Limits:   Limits{Users: 1000, Clients: 100, MonthlyAPI: 2000000, MonthlyEmails: 100000},
		Features: []string{"SAML + social IdPs", "SCIM provisioning", "Webhooks and audit export", "Email support"}},
	{Key: "enterprise", Name: "Enterprise", Description: "High-volume identity with delegated administration.", PriceCents: 9900, Currency: "usd",
		Limits:   Limits{Users: 10000, Clients: 1000, MonthlyAPI: 20000000, MonthlyEmails: 1000000},
		Features: []string{"Custom domains and white-label", "Priority support", "Custom limits and SLA"}},
}

func Get(key string) (Plan, bool) {
	for _, p := range Plans {
		if p.Key == key {
			return p, true
		}
	}
	return Plan{}, false
}

func DefaultPlan() Plan {
	p, _ := Get("free")
	return p
}

// Normalize returns a usable subscription for legacy tenants that predate billing.
func Normalize(sub models.Subscription) models.Subscription {
	if _, ok := Get(sub.Plan); !ok {
		sub.Plan = "free"
	}
	if sub.Status == "" {
		sub.Status = "active"
	}
	if sub.CurrentPeriodStart.IsZero() {
		now := time.Now().UTC()
		sub.CurrentPeriodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	if sub.CurrentPeriodEnd.IsZero() {
		sub.CurrentPeriodEnd = sub.CurrentPeriodStart.AddDate(0, 1, 0)
	}
	return sub
}

func DefaultSubscription() models.Subscription {
	return Normalize(models.Subscription{})
}

// EffectiveLimits applies the lower of the plan limit and the tenant override.
// A tenant quota of zero means "no override".
func EffectiveLimits(sub models.Subscription, usersOverride, clientsOverride int) Limits {
	p, _ := Get(Normalize(sub).Plan)
	limits := p.Limits
	if usersOverride > 0 && usersOverride < limits.Users {
		limits.Users = usersOverride
	}
	if clientsOverride > 0 && clientsOverride < limits.Clients {
		limits.Clients = clientsOverride
	}
	return limits
}

func NewID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b[:]))
}

type Checkout struct {
	URL       string `json:"url"`
	SessionID string `json:"sessionId"`
}

type Provider interface {
	Name() string
	CreateCheckout(ctx context.Context, tenantID, plan string) (Checkout, error)
	Cancel(ctx context.Context, subscriptionID string) error
}

// MockProvider completes checkout immediately. It is deliberately deterministic
// so local development and CI can verify upgrade/cancel flows offline.
type MockProvider struct{}

func (MockProvider) Name() string { return "mock" }

func (MockProvider) CreateCheckout(_ context.Context, _ string, plan string) (Checkout, error) {
	if _, ok := Get(plan); !ok {
		return Checkout{}, fmt.Errorf("unknown plan %q", plan)
	}
	return Checkout{URL: "/admin/billing?checkout=success&plan=" + plan, SessionID: NewID("cs_mock")}, nil
}

func (MockProvider) Cancel(_ context.Context, _ string) error { return nil }
