package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/billing"
	"ucenter/internal/models"
	"ucenter/internal/service"
)

func (a *API) recordUsage(tenantID primitive.ObjectID, metric string, quantity int64, metadata map[string]interface{}) {
	event := &models.UsageEvent{TenantID: tenantID, Metric: metric, Quantity: quantity, Metadata: metadata}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = a.Store.RecordUsage(ctx, event)
	}()
}

func (a *API) meterAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Writer.Status() >= 500 {
			return
		}
		if v, exists := c.Get(ctxBundle); exists {
			if bundle, ok := v.(*service.SessionBundle); ok && bundle.Tenant != nil {
				a.recordUsage(bundle.Tenant.ID, "api_requests", 1, map[string]interface{}{"method": c.Request.Method, "path": c.FullPath()})
			}
		}
	}
}

func monthStart() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func (a *API) billingOverview(c *gin.Context) {
	ctx := c.Request.Context()
	tenant := a.currentTenant(c)
	sub := billing.Normalize(tenant.Subscription)
	plan, _ := billing.Get(sub.Plan)
	limits := billing.EffectiveLimits(sub, tenant.Settings.QuotaUsers, tenant.Settings.QuotaClients)
	usage, err := a.Store.UsageSummary(ctx, tenant.ID, monthStart())
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load usage")
		return
	}
	invoices, _ := a.Store.ListInvoices(ctx, tenant.ID)
	users, _ := a.Store.CountUsersInTenant(ctx, tenant.ID, "")
	clients, _ := a.Store.CountClients(ctx, tenant.ID)
	ok(c, gin.H{
		"subscription": sub,
		"plan":         plan,
		"plans":        billing.Plans,
		"limits":       limits,
		"usage":        usage,
		"counts":       gin.H{"users": users, "clients": clients},
		"invoices":     invoices,
		"periodStart":  monthStart(),
		"provider":     a.Billing.Name(),
	})
}

type checkoutRequest struct {
	Plan string `json:"plan"`
}

func (a *API) billingCheckout(c *gin.Context) {
	var body checkoutRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	plan, exists := billing.Get(body.Plan)
	if !exists {
		fail(c, http.StatusBadRequest, "invalid_plan", "Unknown plan")
		return
	}
	ctx := c.Request.Context()
	tenant := a.currentTenant(c)
	checkout := billing.Checkout{URL: "/admin/billing?checkout=success&plan=" + plan.Key, SessionID: billing.NewID("cs_mock")}
	var err error
	if plan.PriceCents > 0 {
		checkout, err = a.Billing.CreateCheckout(ctx, tenant.ID.Hex(), plan.Key)
		if err != nil {
			fail(c, http.StatusBadGateway, "billing_error", err.Error())
			return
		}
	}
	now := time.Now().UTC()
	sub := models.Subscription{
		Plan: plan.Key, Status: "active", Provider: a.Billing.Name(),
		CustomerID:         "cus_mock_" + tenant.ID.Hex(),
		SubscriptionID:     billing.NewID("sub_mock"),
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
	}
	if err := a.Store.UpdateTenant(ctx, tenant.ID, bson.M{"subscription": sub}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to update subscription")
		return
	}
	if plan.PriceCents > 0 {
		_ = a.Store.CreateInvoice(ctx, &models.Invoice{
			TenantID: tenant.ID, Number: "INV-" + now.Format("20060102") + "-" + billing.NewID("x")[2:8],
			Plan: plan.Key, AmountCents: plan.PriceCents, Currency: plan.Currency, Status: "paid", CreatedAt: now,
		})
	}
	a.Svc.Audit(ctx, tenant.ID, a.currentBundle(c).User, "billing.plan_changed", "subscription", sub.SubscriptionID, a.clientIP(c), c.Request.UserAgent(), map[string]interface{}{"plan": plan.Key})
	ok(c, gin.H{"checkout": checkout, "subscription": sub})
}

func (a *API) billingCancel(c *gin.Context) {
	ctx := c.Request.Context()
	tenant := a.currentTenant(c)
	sub := billing.Normalize(tenant.Subscription)
	if sub.SubscriptionID != "" {
		if err := a.Billing.Cancel(ctx, sub.SubscriptionID); err != nil {
			fail(c, http.StatusBadGateway, "billing_error", err.Error())
			return
		}
	}
	sub.CancelAtPeriodEnd = true
	if err := a.Store.UpdateTenant(ctx, tenant.ID, bson.M{"subscription": sub}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to cancel subscription")
		return
	}
	a.Svc.Audit(ctx, tenant.ID, a.currentBundle(c).User, "billing.canceled", "subscription", sub.SubscriptionID, a.clientIP(c), c.Request.UserAgent(), nil)
	ok(c, gin.H{"subscription": sub})
}

func (a *API) billingResume(c *gin.Context) {
	ctx := c.Request.Context()
	tenant := a.currentTenant(c)
	sub := billing.Normalize(tenant.Subscription)
	sub.CancelAtPeriodEnd = false
	if sub.Status == "canceled" {
		sub.Status = "active"
	}
	if err := a.Store.UpdateTenant(ctx, tenant.ID, bson.M{"subscription": sub}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to resume subscription")
		return
	}
	ok(c, gin.H{"subscription": sub})
}

func (a *API) listInvoices(c *gin.Context) {
	tenant := a.currentTenant(c)
	invoices, err := a.Store.ListInvoices(c.Request.Context(), tenant.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load invoices")
		return
	}
	ok(c, gin.H{"items": invoices})
}
