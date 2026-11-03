package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/models"
)

// PurgeTenant removes all tenant-scoped data. Global users are deleted only when
// they have no remaining memberships and are not platform administrators.
func (s *Store) PurgeTenant(ctx context.Context, tenantID primitive.ObjectID) error {
	cur, err := s.Users.Find(ctx, bson.M{"memberships.tenant_id": tenantID})
	if err == nil {
		var users []struct {
			ID            primitive.ObjectID `bson:"_id"`
			Memberships   []struct{}         `bson:"memberships"`
			PlatformAdmin bool               `bson:"platform_admin"`
		}
		_ = cur.All(ctx, &users)
		for _, u := range users {
			if len(u.Memberships) <= 1 && !u.PlatformAdmin {
				_, _ = s.Users.DeleteOne(ctx, bson.M{"_id": u.ID})
			} else {
				_, _ = s.Users.UpdateOne(ctx, bson.M{"_id": u.ID}, bson.M{"$pull": bson.M{"memberships": bson.M{"tenant_id": tenantID}}})
			}
		}
	}
	for _, coll := range []string{
		"roles", "clients", "auth_requests", "auth_codes", "sessions", "refresh_tokens",
		"signing_keys", "consents", "audit_logs", "api_tokens", "email_tokens",
		"mail_outbox", "identity_providers", "user_identities", "idp_states",
		"webauthn_credentials", "webauthn_sessions", "mfa_pending",
	} {
		_, _ = s.DB.Collection(coll).DeleteMany(ctx, bson.M{"tenant_id": tenantID})
	}
	_, err = s.Tenants.DeleteOne(ctx, bson.M{"_id": tenantID})
	return err
}

func (s *Store) ListPurgeableTenants(ctx context.Context) ([]models.Tenant, error) {
	cur, err := s.Tenants.Find(ctx, bson.M{"purge_after": bson.M{"$lte": time.Now()}})
	if err != nil {
		return nil, err
	}
	var out []models.Tenant
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
