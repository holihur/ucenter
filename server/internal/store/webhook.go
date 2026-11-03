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

func (s *Store) CreateWebhookEndpoint(ctx context.Context, e *models.WebhookEndpoint) error {
	now := time.Now()
	e.ID = primitive.NewObjectID()
	e.CreatedAt = now
	e.UpdatedAt = now
	_, err := s.DB.Collection("webhook_endpoints").InsertOne(ctx, e)
	return err
}

func (s *Store) ListWebhookEndpoints(ctx context.Context, tenantID primitive.ObjectID) ([]models.WebhookEndpoint, error) {
	cur, err := s.DB.Collection("webhook_endpoints").Find(ctx, bson.M{"tenant_id": tenantID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var out []models.WebhookEndpoint
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) FindWebhookEndpoint(ctx context.Context, tenantID, id primitive.ObjectID) (*models.WebhookEndpoint, error) {
	var e models.WebhookEndpoint
	err := s.DB.Collection("webhook_endpoints").FindOne(ctx, bson.M{"_id": id, "tenant_id": tenantID}).Decode(&e)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &e, err
}

func (s *Store) UpdateWebhookEndpoint(ctx context.Context, tenantID, id primitive.ObjectID, update bson.M) error {
	update["updated_at"] = time.Now()
	res, err := s.DB.Collection("webhook_endpoints").UpdateOne(ctx, bson.M{"_id": id, "tenant_id": tenantID}, bson.M{"$set": update})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteWebhookEndpoint(ctx context.Context, tenantID, id primitive.ObjectID) error {
	_, err := s.DB.Collection("webhook_endpoints").DeleteOne(ctx, bson.M{"_id": id, "tenant_id": tenantID})
	return err
}

func (s *Store) CreateWebhookDelivery(ctx context.Context, d *models.WebhookDelivery) error {
	d.ID = primitive.NewObjectID()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	_, err := s.DB.Collection("webhook_deliveries").InsertOne(ctx, d)
	return err
}

// ClaimWebhookDelivery leases one due delivery to a worker.
func (s *Store) ClaimWebhookDelivery(ctx context.Context, lease time.Duration) (*models.WebhookDelivery, error) {
	now := time.Now()
	var d models.WebhookDelivery
	err := s.DB.Collection("webhook_deliveries").FindOneAndUpdate(ctx,
		bson.M{"status": bson.M{"$in": []string{"pending", "retrying"}}, "next_attempt_at": bson.M{"$lte": now}},
		bson.M{"$set": bson.M{"status": "delivering", "lease_until": now.Add(lease)}},
		options.FindOneAndUpdate().SetSort(bson.D{{Key: "next_attempt_at", Value: 1}}).SetReturnDocument(options.After)).Decode(&d)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &d, err
}

func (s *Store) UpdateWebhookDelivery(ctx context.Context, id primitive.ObjectID, update bson.M) error {
	_, err := s.DB.Collection("webhook_deliveries").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update, "$unset": bson.M{"lease_until": ""}})
	return err
}

func (s *Store) ListWebhookDeliveries(ctx context.Context, tenantID primitive.ObjectID, endpointID *primitive.ObjectID, status string, page, pageSize int64) ([]models.WebhookDelivery, int64, error) {
	filter := bson.M{"tenant_id": tenantID}
	if endpointID != nil {
		filter["endpoint_id"] = *endpointID
	}
	if status != "" {
		filter["status"] = status
	}
	total, err := s.DB.Collection("webhook_deliveries").CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 30
	}
	cur, err := s.DB.Collection("webhook_deliveries").Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetSkip((page-1)*pageSize).SetLimit(pageSize))
	if err != nil {
		return nil, 0, err
	}
	var out []models.WebhookDelivery
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
