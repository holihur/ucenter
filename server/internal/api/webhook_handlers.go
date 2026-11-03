package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/models"
	"ucenter/internal/security"
	"ucenter/internal/webhook"
)

func webhookView(e *models.WebhookEndpoint) gin.H {
	return gin.H{
		"id": e.ID, "name": e.Name, "url": e.URL, "events": e.Events,
		"enabled": e.Enabled, "secretSet": e.SecretEnc != "", "createdAt": e.CreatedAt, "updatedAt": e.UpdatedAt,
	}
}

func (a *API) listWebhooks(c *gin.Context) {
	endpoints, err := a.Store.ListWebhookEndpoints(c.Request.Context(), a.currentTenantID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load webhooks")
		return
	}
	items := make([]gin.H, 0, len(endpoints))
	for i := range endpoints {
		items = append(items, webhookView(&endpoints[i]))
	}
	ok(c, gin.H{"items": items, "events": models.WebhookEvents, "allowPrivate": a.Cfg.WebhookAllowPrivate})
}

type webhookRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

func (a *API) createWebhook(c *gin.Context) {
	var body webhookRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.URL = strings.TrimSpace(body.URL)
	if body.Name == "" || body.URL == "" {
		fail(c, http.StatusBadRequest, "invalid_body", "Name and URL are required")
		return
	}
	if err := webhook.ValidateURL(body.URL, a.Cfg.WebhookAllowPrivate); err != nil {
		fail(c, http.StatusBadRequest, "invalid_url", err.Error())
		return
	}
	rawSecret, err := security.RandomToken(32)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to generate secret")
		return
	}
	rawSecret = "whsec_" + rawSecret
	encrypted, err := a.Cipher.Encrypt(rawSecret)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to encrypt secret")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	endpoint := &models.WebhookEndpoint{
		TenantID: a.currentTenantID(c), Name: body.Name, URL: body.URL,
		SecretEnc: encrypted, Events: body.Events, Enabled: enabled,
	}
	if err := a.Store.CreateWebhookEndpoint(c.Request.Context(), endpoint); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to create webhook")
		return
	}
	a.audit(c, "webhook.created", "webhook", endpoint.ID.Hex(), map[string]interface{}{"url": endpoint.URL, "events": endpoint.Events})
	c.JSON(http.StatusCreated, gin.H{"webhook": webhookView(endpoint), "secret": rawSecret})
}

func (a *API) updateWebhook(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Webhook not found")
		return
	}
	existing, err := a.Store.FindWebhookEndpoint(c.Request.Context(), a.currentTenantID(c), id)
	if err != nil {
		fail(c, http.StatusNotFound, "not_found", "Webhook not found")
		return
	}
	var body webhookRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	update := bson.M{}
	if strings.TrimSpace(body.Name) != "" {
		update["name"] = strings.TrimSpace(body.Name)
	}
	if strings.TrimSpace(body.URL) != "" && body.URL != existing.URL {
		if err := webhook.ValidateURL(body.URL, a.Cfg.WebhookAllowPrivate); err != nil {
			fail(c, http.StatusBadRequest, "invalid_url", err.Error())
			return
		}
		update["url"] = strings.TrimSpace(body.URL)
	}
	if body.Events != nil {
		update["events"] = body.Events
	}
	if body.Enabled != nil {
		update["enabled"] = *body.Enabled
	}
	if len(update) == 0 {
		ok(c, gin.H{"webhook": webhookView(existing)})
		return
	}
	if err := a.Store.UpdateWebhookEndpoint(c.Request.Context(), a.currentTenantID(c), id, update); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to update webhook")
		return
	}
	a.audit(c, "webhook.updated", "webhook", id.Hex(), nil)
	updated, _ := a.Store.FindWebhookEndpoint(c.Request.Context(), a.currentTenantID(c), id)
	ok(c, gin.H{"webhook": webhookView(updated)})
}

func (a *API) deleteWebhook(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Webhook not found")
		return
	}
	if err := a.Store.DeleteWebhookEndpoint(c.Request.Context(), a.currentTenantID(c), id); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to delete webhook")
		return
	}
	a.audit(c, "webhook.deleted", "webhook", id.Hex(), nil)
	ok(c, gin.H{"ok": true})
}

func (a *API) rotateWebhookSecret(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Webhook not found")
		return
	}
	if _, err := a.Store.FindWebhookEndpoint(c.Request.Context(), a.currentTenantID(c), id); err != nil {
		fail(c, http.StatusNotFound, "not_found", "Webhook not found")
		return
	}
	raw, _ := security.RandomToken(32)
	raw = "whsec_" + raw
	encrypted, _ := a.Cipher.Encrypt(raw)
	if err := a.Store.UpdateWebhookEndpoint(c.Request.Context(), a.currentTenantID(c), id, bson.M{"secret_enc": encrypted}); err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to rotate secret")
		return
	}
	a.audit(c, "webhook.secret.rotated", "webhook", id.Hex(), nil)
	ok(c, gin.H{"secret": raw})
}

func (a *API) testWebhook(c *gin.Context) {
	id, valid := parseObjectID(c.Param("id"))
	if !valid {
		fail(c, http.StatusNotFound, "not_found", "Webhook not found")
		return
	}
	if a.Svc.Webhooks == nil {
		fail(c, http.StatusBadRequest, "webhooks_disabled", "Webhooks are not configured")
		return
	}
	if err := a.Svc.Webhooks.Test(c.Request.Context(), a.currentTenantID(c), id); err != nil {
		fail(c, http.StatusBadRequest, "webhook_error", err.Error())
		return
	}
	a.audit(c, "webhook.test", "webhook", id.Hex(), nil)
	ok(c, gin.H{"ok": true})
}

func (a *API) listWebhookDeliveries(c *gin.Context) {
	var endpointID *primitive.ObjectID
	if raw := strings.TrimSpace(c.Query("endpointId")); raw != "" {
		if id, err := primitive.ObjectIDFromHex(raw); err == nil {
			endpointID = &id
		}
	}
	page, _ := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 64)
	pageSize, _ := strconv.ParseInt(c.DefaultQuery("pageSize", "30"), 10, 64)
	deliveries, total, err := a.Store.ListWebhookDeliveries(c.Request.Context(), a.currentTenantID(c), endpointID, c.Query("status"), page, pageSize)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal_error", "Unable to load deliveries")
		return
	}
	items := make([]gin.H, 0, len(deliveries))
	for _, d := range deliveries {
		payload := map[string]interface{}{}
		_ = json.Unmarshal([]byte(d.Payload), &payload)
		items = append(items, gin.H{
			"id": d.ID, "endpointId": d.EndpointID, "event": d.Event, "status": d.Status,
			"attempts": d.Attempts, "nextAttemptAt": d.NextAttemptAt, "responseCode": d.ResponseCode,
			"error": d.Error, "createdAt": d.CreatedAt, "deliveredAt": d.DeliveredAt, "payload": payload,
		})
	}
	ok(c, gin.H{"items": items, "total": total, "page": page, "pageSize": pageSize, "now": time.Now()})
}
