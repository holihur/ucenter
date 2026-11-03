package store

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"ucenter/internal/models"
)

func (s *Store) RecordUsage(ctx context.Context, event *models.UsageEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	_, err := s.Usage.InsertOne(ctx, event)
	return err
}

func (s *Store) UsageSummary(ctx context.Context, tenantID primitive.ObjectID, since time.Time) (map[string]int64, error) {
	cursor, err := s.Usage.Aggregate(ctx, []bson.M{
		{"$match": bson.M{"tenant_id": tenantID, "created_at": bson.M{"$gte": since}}},
		{"$group": bson.M{"_id": "$metric", "total": bson.M{"$sum": "$quantity"}}},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := map[string]int64{}
	for cursor.Next(ctx) {
		var row struct {
			ID    string `bson:"_id"`
			Total int64  `bson:"total"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		out[row.ID] = row.Total
	}
	return out, cursor.Err()
}

func (s *Store) CreateInvoice(ctx context.Context, invoice *models.Invoice) error {
	if invoice.CreatedAt.IsZero() {
		invoice.CreatedAt = time.Now().UTC()
	}
	res, err := s.Invoices.InsertOne(ctx, invoice)
	if err != nil {
		return err
	}
	invoice.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (s *Store) ListInvoices(ctx context.Context, tenantID primitive.ObjectID) ([]models.Invoice, error) {
	cursor, err := s.Invoices.Find(ctx, bson.M{"tenant_id": tenantID}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(24))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := []models.Invoice{}
	return out, cursor.All(ctx, &out)
}
