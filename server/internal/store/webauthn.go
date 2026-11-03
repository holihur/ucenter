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

func (s *Store) CreateWebAuthnCredential(ctx context.Context, c *models.WebAuthnCredential) error {
	c.ID = primitive.NewObjectID()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	_, err := s.DB.Collection("webauthn_credentials").InsertOne(ctx, c)
	if IsDuplicate(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Store) ListWebAuthnCredentials(ctx context.Context, tenantID, userID primitive.ObjectID) ([]models.WebAuthnCredential, error) {
	cur, err := s.DB.Collection("webauthn_credentials").Find(ctx, bson.M{"tenant_id": tenantID, "user_id": userID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var out []models.WebAuthnCredential
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) CountWebAuthnCredentials(ctx context.Context, tenantID, userID primitive.ObjectID) (int64, error) {
	return s.DB.Collection("webauthn_credentials").CountDocuments(ctx, bson.M{"tenant_id": tenantID, "user_id": userID})
}

func (s *Store) UpdateWebAuthnCredential(ctx context.Context, id primitive.ObjectID, counter uint32) error {
	now := time.Now()
	_, err := s.DB.Collection("webauthn_credentials").UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"sign_count": counter, "last_used_at": now}})
	return err
}

func (s *Store) DeleteWebAuthnCredential(ctx context.Context, tenantID, userID, id primitive.ObjectID) error {
	res, err := s.DB.Collection("webauthn_credentials").DeleteOne(ctx, bson.M{"_id": id, "tenant_id": tenantID, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateWebAuthnSession(ctx context.Context, st *models.WebAuthnSession) error {
	if st.CreatedAt.IsZero() {
		st.CreatedAt = time.Now()
	}
	_, err := s.DB.Collection("webauthn_sessions").InsertOne(ctx, st)
	return err
}

func (s *Store) ConsumeWebAuthnSession(ctx context.Context, hash string) (*models.WebAuthnSession, error) {
	var st models.WebAuthnSession
	err := s.DB.Collection("webauthn_sessions").FindOneAndUpdate(ctx,
		bson.M{"_id": hash, "used": false, "expires_at": bson.M{"$gt": time.Now()}},
		bson.M{"$set": bson.M{"used": true}},
		options.FindOneAndUpdate().SetReturnDocument(options.Before)).Decode(&st)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &st, err
}

func (s *Store) CreateMFAPending(ctx context.Context, p *models.MFAPending) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	_, err := s.DB.Collection("mfa_pending").InsertOne(ctx, p)
	return err
}

func (s *Store) ConsumeMFAPending(ctx context.Context, hash string) (*models.MFAPending, error) {
	var p models.MFAPending
	err := s.DB.Collection("mfa_pending").FindOneAndUpdate(ctx,
		bson.M{"_id": hash, "used": false, "expires_at": bson.M{"$gt": time.Now()}},
		bson.M{"$set": bson.M{"used": true}},
		options.FindOneAndUpdate().SetReturnDocument(options.Before)).Decode(&p)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &p, err
}

func (s *Store) MarkStepUp(ctx context.Context, sessionID string) error {
	now := time.Now()
	_, err := s.Sessions.UpdateOne(ctx, bson.M{"_id": sessionID}, bson.M{"$set": bson.M{"step_up_at": now}})
	return err
}

func (s *Store) DeleteAllWebAuthnCredentials(ctx context.Context, tenantID, userID primitive.ObjectID) error {
	_, err := s.DB.Collection("webauthn_credentials").DeleteMany(ctx, bson.M{"tenant_id": tenantID, "user_id": userID})
	return err
}
