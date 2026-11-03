package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"ucenter/internal/models"
)

func (s *Store) ListIdentityProviders(ctx context.Context, tenantID primitive.ObjectID) ([]models.IdentityProvider, error) {
	cur, err := s.DB.Collection("identity_providers").Find(ctx, bson.M{"tenant_id": tenantID})
	if err != nil {
		return nil, err
	}
	var out []models.IdentityProvider
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) FindIdentityProvider(ctx context.Context, tenantID primitive.ObjectID, typ string) (*models.IdentityProvider, error) {
	var p models.IdentityProvider
	err := s.DB.Collection("identity_providers").FindOne(ctx, bson.M{"tenant_id": tenantID, "type": typ}).Decode(&p)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &p, err
}

func (s *Store) UpsertIdentityProvider(ctx context.Context, p *models.IdentityProvider) error {
	now := time.Now()
	p.UpdatedAt = now
	_, err := s.DB.Collection("identity_providers").UpdateOne(ctx,
		bson.M{"tenant_id": p.TenantID, "type": p.Type},
		bson.M{
			"$set": bson.M{
				"name": p.Name, "enabled": p.Enabled, "config_enc": p.ConfigEnc,
				"auto_provision": p.AutoProvision, "default_roles": p.DefaultRoles,
				"trust_email": p.TrustEmail, "updated_at": now,
			},
			"$setOnInsert": bson.M{"tenant_id": p.TenantID, "type": p.Type, "created_at": now},
		}, options.Update().SetUpsert(true))
	return err
}

func (s *Store) DeleteIdentityProvider(ctx context.Context, tenantID primitive.ObjectID, typ string) error {
	_, err := s.DB.Collection("identity_providers").DeleteOne(ctx, bson.M{"tenant_id": tenantID, "type": typ})
	return err
}

func (s *Store) FindUserIdentity(ctx context.Context, tenantID primitive.ObjectID, provider, subject string) (*models.UserIdentity, error) {
	var i models.UserIdentity
	err := s.DB.Collection("user_identities").FindOne(ctx, bson.M{"tenant_id": tenantID, "provider": provider, "subject": subject}).Decode(&i)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &i, err
}

func (s *Store) FindUserIdentityByUnionID(ctx context.Context, tenantID primitive.ObjectID, provider, unionID string) (*models.UserIdentity, error) {
	if unionID == "" {
		return nil, ErrNotFound
	}
	var i models.UserIdentity
	err := s.DB.Collection("user_identities").FindOne(ctx, bson.M{"tenant_id": tenantID, "provider": provider, "union_id": unionID}).Decode(&i)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &i, err
}

func (s *Store) ListUserIdentities(ctx context.Context, tenantID, userID primitive.ObjectID) ([]models.UserIdentity, error) {
	cur, err := s.DB.Collection("user_identities").Find(ctx, bson.M{"tenant_id": tenantID, "user_id": userID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var out []models.UserIdentity
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) LinkUserIdentity(ctx context.Context, i *models.UserIdentity) error {
	i.ID = primitive.NewObjectID()
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now()
	}
	_, err := s.DB.Collection("user_identities").InsertOne(ctx, i)
	if IsDuplicate(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Store) TouchUserIdentity(ctx context.Context, id primitive.ObjectID) {
	now := time.Now()
	_, _ = s.DB.Collection("user_identities").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"last_login_at": now}})
}

func (s *Store) DeleteUserIdentity(ctx context.Context, tenantID, userID primitive.ObjectID, id primitive.ObjectID) error {
	res, err := s.DB.Collection("user_identities").DeleteOne(ctx, bson.M{"_id": id, "tenant_id": tenantID, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateIdPState(ctx context.Context, st *models.IdPState) error {
	_, err := s.DB.Collection("idp_states").InsertOne(ctx, st)
	return err
}

func (s *Store) ConsumeIdPState(ctx context.Context, hash string) (*models.IdPState, error) {
	var st models.IdPState
	err := s.DB.Collection("idp_states").FindOneAndUpdate(ctx,
		bson.M{"_id": hash, "used": false, "expires_at": bson.M{"$gt": time.Now()}},
		bson.M{"$set": bson.M{"used": true}},
		options.FindOneAndUpdate().SetReturnDocument(options.Before)).Decode(&st)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &st, err
}
