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

func (s *Store) CreateRole(ctx context.Context, r *models.Role) error {
	now := time.Now()
	r.ID = primitive.NewObjectID()
	r.CreatedAt = now
	r.UpdatedAt = now
	_, err := s.Roles.InsertOne(ctx, r)
	if IsDuplicate(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Store) FindRoleByKey(ctx context.Context, tenantID primitive.ObjectID, key string) (*models.Role, error) {
	var r models.Role
	err := s.Roles.FindOne(ctx, bson.M{"tenant_id": tenantID, "key": key}).Decode(&r)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &r, err
}

func (s *Store) FindRoleByID(ctx context.Context, tenantID, id primitive.ObjectID) (*models.Role, error) {
	var r models.Role
	err := s.Roles.FindOne(ctx, bson.M{"tenant_id": tenantID, "_id": id}).Decode(&r)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &r, err
}

func (s *Store) ListRoles(ctx context.Context, tenantID primitive.ObjectID) ([]models.Role, error) {
	cur, err := s.Roles.Find(ctx, bson.M{"tenant_id": tenantID},
		options.Find().SetSort(bson.D{{Key: "system", Value: -1}, {Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var out []models.Role
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) RolesByKeys(ctx context.Context, tenantID primitive.ObjectID, keys []string) ([]models.Role, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	cur, err := s.Roles.Find(ctx, bson.M{"tenant_id": tenantID, "key": bson.M{"$in": keys}})
	if err != nil {
		return nil, err
	}
	var out []models.Role
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpdateRole(ctx context.Context, tenantID, id primitive.ObjectID, update bson.M) error {
	update["updated_at"] = time.Now()
	_, err := s.Roles.UpdateOne(ctx, bson.M{"tenant_id": tenantID, "_id": id}, bson.M{"$set": update})
	return err
}

func (s *Store) DeleteRole(ctx context.Context, tenantID, id primitive.ObjectID) error {
	_, err := s.Roles.DeleteOne(ctx, bson.M{"tenant_id": tenantID, "_id": id})
	return err
}

func (s *Store) CountRoles(ctx context.Context, tenantID primitive.ObjectID) (int64, error) {
	return s.Roles.CountDocuments(ctx, bson.M{"tenant_id": tenantID})
}

func (s *Store) CreateClient(ctx context.Context, c *models.Client) error {
	now := time.Now()
	c.ID = primitive.NewObjectID()
	c.CreatedAt = now
	c.UpdatedAt = now
	_, err := s.Clients.InsertOne(ctx, c)
	if IsDuplicate(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Store) FindClientByClientID(ctx context.Context, tenantID primitive.ObjectID, clientID string) (*models.Client, error) {
	var c models.Client
	err := s.Clients.FindOne(ctx, bson.M{"tenant_id": tenantID, "client_id": clientID}).Decode(&c)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &c, err
}

func (s *Store) FindClientByID(ctx context.Context, tenantID, id primitive.ObjectID) (*models.Client, error) {
	var c models.Client
	err := s.Clients.FindOne(ctx, bson.M{"tenant_id": tenantID, "_id": id}).Decode(&c)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &c, err
}

func (s *Store) ListClients(ctx context.Context, tenantID primitive.ObjectID) ([]models.Client, error) {
	cur, err := s.Clients.Find(ctx, bson.M{"tenant_id": tenantID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var out []models.Client
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpdateClient(ctx context.Context, tenantID, id primitive.ObjectID, update bson.M) error {
	update["updated_at"] = time.Now()
	_, err := s.Clients.UpdateOne(ctx, bson.M{"tenant_id": tenantID, "_id": id}, bson.M{"$set": update})
	return err
}

func (s *Store) DeleteClient(ctx context.Context, tenantID, id primitive.ObjectID) error {
	_, err := s.Clients.DeleteOne(ctx, bson.M{"tenant_id": tenantID, "_id": id})
	return err
}

func (s *Store) CountClients(ctx context.Context, tenantID primitive.ObjectID) (int64, error) {
	return s.Clients.CountDocuments(ctx, bson.M{"tenant_id": tenantID})
}
