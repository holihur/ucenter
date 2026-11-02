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

func (s *Store) CreateTenant(ctx context.Context, t *models.Tenant) error {
	now := time.Now()
	t.ID = primitive.NewObjectID()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = models.StatusActive
	}
	_, err := s.Tenants.InsertOne(ctx, t)
	if IsDuplicate(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Store) FindTenantBySlug(ctx context.Context, slug string) (*models.Tenant, error) {
	var t models.Tenant
	err := s.Tenants.FindOne(ctx, bson.M{"slug": slug}).Decode(&t)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) FindTenantByID(ctx context.Context, id primitive.ObjectID) (*models.Tenant, error) {
	var t models.Tenant
	err := s.Tenants.FindOne(ctx, bson.M{"_id": id}).Decode(&t)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) FindTenantByDomain(ctx context.Context, domain string) (*models.Tenant, error) {
	var t models.Tenant
	err := s.Tenants.FindOne(ctx, bson.M{"domains": domain}).Decode(&t)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) ListTenants(ctx context.Context) ([]models.Tenant, error) {
	cur, err := s.Tenants.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var out []models.Tenant
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpdateTenant(ctx context.Context, id primitive.ObjectID, update bson.M) error {
	update["updated_at"] = time.Now()
	_, err := s.Tenants.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
	return err
}

func (s *Store) CountTenants(ctx context.Context) (int64, error) {
	return s.Tenants.CountDocuments(ctx, bson.M{})
}

func (s *Store) CountTenantsByStatus(ctx context.Context, status string) (int64, error) {
	return s.Tenants.CountDocuments(ctx, bson.M{"status": status})
}
