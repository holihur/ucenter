package store

import (
	"context"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"ucenter/internal/models"
)

func (s *Store) CreateAuthRequest(ctx context.Context, r *models.AuthRequest) error {
	_, err := s.Requests.InsertOne(ctx, r)
	return err
}

func (s *Store) FindAuthRequest(ctx context.Context, id string) (*models.AuthRequest, error) {
	var r models.AuthRequest
	err := s.Requests.FindOne(ctx, bson.M{"_id": id}).Decode(&r)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &r, err
}

func (s *Store) UpdateAuthRequest(ctx context.Context, id string, update bson.M) error {
	_, err := s.Requests.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
	return err
}

func (s *Store) CreateAuthCode(ctx context.Context, c *models.AuthCode) error {
	_, err := s.Codes.InsertOne(ctx, c)
	return err
}

// ConsumeAuthCode atomically marks a code as used and returns it.
func (s *Store) ConsumeAuthCode(ctx context.Context, codeHash string) (*models.AuthCode, error) {
	var c models.AuthCode
	err := s.Codes.FindOneAndUpdate(ctx,
		bson.M{"_id": codeHash, "used": false, "expires_at": bson.M{"$gt": time.Now()}},
		bson.M{"$set": bson.M{"used": true}},
		options.FindOneAndUpdate().SetReturnDocument(options.Before)).Decode(&c)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &c, err
}

func (s *Store) CreateSession(ctx context.Context, sess *models.Session) error {
	_, err := s.Sessions.InsertOne(ctx, sess)
	return err
}

func (s *Store) FindSession(ctx context.Context, tokenHash string) (*models.Session, error) {
	var sess models.Session
	err := s.Sessions.FindOne(ctx, bson.M{
		"_id": tokenHash, "revoked": false, "expires_at": bson.M{"$gt": time.Now()},
	}).Decode(&sess)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &sess, err
}

func (s *Store) RevokeSession(ctx context.Context, tokenHash string) error {
	_, err := s.Sessions.UpdateOne(ctx, bson.M{"_id": tokenHash}, bson.M{"$set": bson.M{"revoked": true}})
	return err
}

func (s *Store) RevokeSessionByID(ctx context.Context, id string) error {
	return s.RevokeSession(ctx, id)
}

func (s *Store) ListSessions(ctx context.Context, tenantID primitive.ObjectID, limit int64) ([]models.Session, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	cur, err := s.Sessions.Find(ctx, bson.M{
		"tenant_id": tenantID, "revoked": false, "expires_at": bson.M{"$gt": time.Now()},
	}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	var out []models.Session
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) RevokeAllUserSessions(ctx context.Context, tenantID, userID primitive.ObjectID) error {
	_, err := s.Sessions.UpdateMany(ctx,
		bson.M{"tenant_id": tenantID, "user_id": userID, "revoked": false},
		bson.M{"$set": bson.M{"revoked": true}})
	return err
}

func (s *Store) CreateRefreshToken(ctx context.Context, t *models.RefreshToken) error {
	_, err := s.Refresh.InsertOne(ctx, t)
	return err
}

func (s *Store) FindRefreshToken(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	var t models.RefreshToken
	err := s.Refresh.FindOne(ctx, bson.M{"_id": tokenHash}).Decode(&t)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := s.Refresh.UpdateOne(ctx, bson.M{"_id": tokenHash}, bson.M{"$set": bson.M{"revoked": true}})
	return err
}

func (s *Store) RotateRefreshToken(ctx context.Context, oldHash, newHash string) error {
	_, err := s.Refresh.UpdateOne(ctx, bson.M{"_id": oldHash},
		bson.M{"$set": bson.M{"revoked": true, "rotated_to": newHash}})
	return err
}

func (s *Store) RevokeUserRefreshTokens(ctx context.Context, tenantID, userID primitive.ObjectID) error {
	_, err := s.Refresh.UpdateMany(ctx,
		bson.M{"tenant_id": tenantID, "user_id": userID, "revoked": false},
		bson.M{"$set": bson.M{"revoked": true}})
	return err
}

func (s *Store) FindConsent(ctx context.Context, tenantID, userID primitive.ObjectID, clientID string) (*models.Consent, error) {
	var c models.Consent
	err := s.Consents.FindOne(ctx, bson.M{"tenant_id": tenantID, "user_id": userID, "client_id": clientID}).Decode(&c)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &c, err
}

func (s *Store) UpsertConsent(ctx context.Context, tenantID, userID primitive.ObjectID, clientID string, scopes []string) error {
	now := time.Now()
	_, err := s.Consents.UpdateOne(ctx,
		bson.M{"tenant_id": tenantID, "user_id": userID, "client_id": clientID},
		bson.M{
			"$set": bson.M{"scopes": scopes, "updated_at": now},
			"$setOnInsert": bson.M{
				"tenant_id": tenantID, "user_id": userID, "client_id": clientID, "created_at": now,
			},
		}, options.Update().SetUpsert(true))
	return err
}

func (s *Store) ActiveSigningKey(ctx context.Context, tenantID primitive.ObjectID) (*models.SigningKey, error) {
	var k models.SigningKey
	err := s.Keys.FindOne(ctx, bson.M{"tenant_id": tenantID, "active": true}).Decode(&k)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &k, err
}

func (s *Store) UpdateSigningKeyPrivate(ctx context.Context, id primitive.ObjectID, encrypted string) error {
	_, err := s.Keys.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"private_pem_enc": encrypted, "private_pem": ""}})
	return err
}

func (s *Store) CreateSigningKey(ctx context.Context, k *models.SigningKey) error {
	k.ID = primitive.NewObjectID()
	k.CreatedAt = time.Now()
	_, err := s.Keys.InsertOne(ctx, k)
	return err
}

func (s *Store) ListSigningKeys(ctx context.Context, tenantID primitive.ObjectID) ([]models.SigningKey, error) {
	cur, err := s.Keys.Find(ctx, bson.M{"tenant_id": tenantID, "active": true},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var out []models.SigningKey
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) CreateAudit(ctx context.Context, a *models.AuditLog) error {
	a.ID = primitive.NewObjectID()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	_, err := s.Audit.InsertOne(ctx, a)
	return err
}

type AuditFilter struct {
	TenantID primitive.ObjectID
	Action   string
	Search   string
	Page     int64
	PageSize int64
}

func (s *Store) ListAudit(ctx context.Context, f AuditFilter) ([]models.AuditLog, int64, error) {
	filter := bson.M{"tenant_id": f.TenantID}
	if f.Action != "" {
		filter["action"] = f.Action
	}
	if f.Search != "" {
		rx := primitive.Regex{Pattern: regexp.QuoteMeta(f.Search), Options: "i"}
		filter["$or"] = []bson.M{
			{"actor_email": rx},
			{"action": rx},
			{"target_id": rx},
		}
	}
	total, err := s.Audit.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 200 {
		f.PageSize = 30
	}
	cur, err := s.Audit.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip((f.Page-1)*f.PageSize).SetLimit(f.PageSize))
	if err != nil {
		return nil, 0, err
	}
	var out []models.AuditLog
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (s *Store) CountRecentAudit(ctx context.Context, tenantID primitive.ObjectID, since time.Time) (int64, error) {
	return s.Audit.CountDocuments(ctx, bson.M{"tenant_id": tenantID, "created_at": bson.M{"$gte": since}})
}

func (s *Store) RevokeClientRefreshTokens(ctx context.Context, tenantID primitive.ObjectID, clientID string) error {
	_, err := s.Refresh.UpdateMany(ctx,
		bson.M{"tenant_id": tenantID, "client_id": clientID, "revoked": false},
		bson.M{"$set": bson.M{"revoked": true}})
	return err
}

func (s *Store) CountActiveSessions(ctx context.Context, tenantID primitive.ObjectID) (int64, error) {
	return s.Sessions.CountDocuments(ctx, bson.M{
		"tenant_id": tenantID, "revoked": false, "expires_at": bson.M{"$gt": time.Now()},
	})
}

func (s *Store) CreateEmailToken(ctx context.Context, t *models.EmailToken) error {
	t.ID = primitive.NewObjectID()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	_, err := s.DB.Collection("email_tokens").InsertOne(ctx, t)
	return err
}

func (s *Store) ConsumeEmailToken(ctx context.Context, hash, kind string) (*models.EmailToken, error) {
	var t models.EmailToken
	err := s.DB.Collection("email_tokens").FindOneAndUpdate(ctx,
		bson.M{"token_hash": hash, "type": kind, "used": false, "expires_at": bson.M{"$gt": time.Now()}},
		bson.M{"$set": bson.M{"used": true}},
		options.FindOneAndUpdate().SetReturnDocument(options.Before)).Decode(&t)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) ListMailOutbox(ctx context.Context, tenantID primitive.ObjectID, limit int64) ([]bson.M, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	cur, err := s.DB.Collection("mail_outbox").Find(ctx, bson.M{"tenant_id": tenantID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	var out []bson.M
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
