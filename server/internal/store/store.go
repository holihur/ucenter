package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"ucenter/internal/models"
)

var ErrNotFound = errors.New("not found")
var ErrDuplicate = errors.New("duplicate key")

type Store struct {
	Client   *mongo.Client
	DB       *mongo.Database
	Tenants  *mongo.Collection
	Users    *mongo.Collection
	Roles    *mongo.Collection
	Clients  *mongo.Collection
	Requests *mongo.Collection
	Codes    *mongo.Collection
	Sessions *mongo.Collection
	Refresh  *mongo.Collection
	Keys     *mongo.Collection
	Consents *mongo.Collection
	Audit    *mongo.Collection
}

func New(ctx context.Context, uri, dbName string) (*Store, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetConnectTimeout(10*time.Second))
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		return nil, err
	}
	db := client.Database(dbName)
	return &Store{
		Client:   client,
		DB:       db,
		Tenants:  db.Collection("tenants"),
		Users:    db.Collection("users"),
		Roles:    db.Collection("roles"),
		Clients:  db.Collection("clients"),
		Requests: db.Collection("auth_requests"),
		Codes:    db.Collection("auth_codes"),
		Sessions: db.Collection("sessions"),
		Refresh:  db.Collection("refresh_tokens"),
		Keys:     db.Collection("signing_keys"),
		Consents: db.Collection("consents"),
		Audit:    db.Collection("audit_logs"),
	}, nil
}

func (s *Store) Close(ctx context.Context) error {
	return s.Client.Disconnect(ctx)
}

func (s *Store) EnsureIndexes(ctx context.Context) error {
	type spec struct {
		coll   *mongo.Collection
		models []mongo.IndexModel
	}
	specs := []spec{
		{s.Tenants, []mongo.IndexModel{
			{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "domains", Value: 1}}},
		}},
		{s.Users, []mongo.IndexModel{
			{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "memberships.tenant_id", Value: 1}}},
			{Keys: bson.D{{Key: "created_at", Value: -1}}},
		}},
		{s.Roles, []mongo.IndexModel{
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "key", Value: 1}}, Options: options.Index().SetUnique(true)},
		}},
		{s.Clients, []mongo.IndexModel{
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "client_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		}},
		{s.Requests, []mongo.IndexModel{
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		}},
		{s.Codes, []mongo.IndexModel{
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		}},
		{s.Sessions, []mongo.IndexModel{
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "user_id", Value: 1}}},
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		}},
		{s.Refresh, []mongo.IndexModel{
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
			{Keys: bson.D{{Key: "user_id", Value: 1}}},
		}},
		{s.Keys, []mongo.IndexModel{
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "kid", Value: 1}}, Options: options.Index().SetUnique(true)},
		}},
		{s.Consents, []mongo.IndexModel{
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "user_id", Value: 1}, {Key: "client_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		}},
		{s.Audit, []mongo.IndexModel{
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "created_at", Value: -1}}},
		}},
		{s.DB.Collection("api_tokens"), []mongo.IndexModel{
			{Keys: bson.D{{Key: "token_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "user_id", Value: 1}}},
		}},
		{s.DB.Collection("email_tokens"), []mongo.IndexModel{
			{Keys: bson.D{{Key: "token_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		}},
		{s.DB.Collection("identity_providers"), []mongo.IndexModel{
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "type", Value: 1}}, Options: options.Index().SetUnique(true)},
		}},
		{s.DB.Collection("user_identities"), []mongo.IndexModel{
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "provider", Value: 1}, {Key: "subject", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "provider", Value: 1}, {Key: "union_id", Value: 1}}, Options: options.Index().SetSparse(true)},
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "user_id", Value: 1}}},
		}},
		{s.DB.Collection("idp_states"), []mongo.IndexModel{
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		}},
		{s.DB.Collection("webauthn_credentials"), []mongo.IndexModel{
			{Keys: bson.D{{Key: "credential_id", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "user_id", Value: 1}}},
		}},
		{s.DB.Collection("webauthn_sessions"), []mongo.IndexModel{
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		}},
		{s.DB.Collection("mfa_pending"), []mongo.IndexModel{
			{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		}},
	}
	for _, sp := range specs {
		if _, err := sp.coll.Indexes().CreateMany(ctx, sp.models); err != nil {
			return err
		}
	}
	return nil
}

func IsDuplicate(err error) bool {
	return mongo.IsDuplicateKeyError(err)
}

func (s *Store) CreateAPIToken(ctx context.Context, t *models.APIToken) error {
	t.ID = primitive.NewObjectID()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	_, err := s.DB.Collection("api_tokens").InsertOne(ctx, t)
	if IsDuplicate(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Store) FindAPITokenByHash(ctx context.Context, hash string) (*models.APIToken, error) {
	var t models.APIToken
	err := s.DB.Collection("api_tokens").FindOne(ctx, bson.M{
		"token_hash": hash, "revoked": false,
		"$or": []bson.M{{"expires_at": nil}, {"expires_at": bson.M{"$gt": time.Now()}}},
	}).Decode(&t)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) ListAPITokens(ctx context.Context, tenantID, userID primitive.ObjectID) ([]models.APIToken, error) {
	cur, err := s.DB.Collection("api_tokens").Find(ctx, bson.M{"tenant_id": tenantID, "user_id": userID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var out []models.APIToken
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) RevokeAPIToken(ctx context.Context, tenantID, userID, id primitive.ObjectID) error {
	res, err := s.DB.Collection("api_tokens").UpdateOne(ctx,
		bson.M{"_id": id, "tenant_id": tenantID, "user_id": userID},
		bson.M{"$set": bson.M{"revoked": true}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TouchAPIToken(ctx context.Context, id primitive.ObjectID) {
	_, _ = s.DB.Collection("api_tokens").UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"last_used_at": time.Now()}})
}
