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

func (s *Store) CreateUser(ctx context.Context, u *models.User) error {
	now := time.Now()
	u.ID = primitive.NewObjectID()
	u.CreatedAt = now
	u.UpdatedAt = now
	if u.Status == "" {
		u.Status = models.StatusActive
	}
	if u.Memberships == nil {
		u.Memberships = []models.Membership{}
	}
	_, err := s.Users.InsertOne(ctx, u)
	if IsDuplicate(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Store) FindUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := s.Users.FindOne(ctx, bson.M{"email": email}).Decode(&u)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *Store) FindUserByID(ctx context.Context, id primitive.ObjectID) (*models.User, error) {
	var u models.User
	err := s.Users.FindOne(ctx, bson.M{"_id": id}).Decode(&u)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *Store) FindUsersByIDs(ctx context.Context, ids []primitive.ObjectID) (map[primitive.ObjectID]models.User, error) {
	out := make(map[primitive.ObjectID]models.User, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := s.Users.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	var users []models.User
	if err := cur.All(ctx, &users); err != nil {
		return nil, err
	}
	for _, u := range users {
		out[u.ID] = u
	}
	return out, nil
}

type UserFilter struct {
	TenantID primitive.ObjectID
	Search   string
	Status   string
	Role     string
	Page     int64
	PageSize int64
}

func (s *Store) ListUsers(ctx context.Context, f UserFilter) ([]models.User, int64, error) {
	filter := bson.M{"memberships.tenant_id": f.TenantID}
	if f.Search != "" {
		rx := primitive.Regex{Pattern: regexp.QuoteMeta(f.Search), Options: "i"}
		filter["$or"] = []bson.M{
			{"email": rx},
			{"name": rx},
			{"username": rx},
		}
	}
	if f.Status != "" {
		filter["status"] = f.Status
	}
	if f.Role != "" {
		filter["memberships"] = bson.M{"$elemMatch": bson.M{"tenant_id": f.TenantID, "roles": f.Role}}
	}
	total, err := s.Users.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 200 {
		f.PageSize = 20
	}
	cur, err := s.Users.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip((f.Page-1)*f.PageSize).
		SetLimit(f.PageSize))
	if err != nil {
		return nil, 0, err
	}
	var users []models.User
	if err := cur.All(ctx, &users); err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (s *Store) UpdateUser(ctx context.Context, id primitive.ObjectID, update bson.M) error {
	update["updated_at"] = time.Now()
	_, err := s.Users.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
	return err
}

func (s *Store) UpdateUserRaw(ctx context.Context, filter, update bson.M) error {
	_, err := s.Users.UpdateOne(ctx, filter, update)
	return err
}

func (s *Store) DeleteUser(ctx context.Context, id primitive.ObjectID) error {
	_, err := s.Users.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (s *Store) CountUsersInTenant(ctx context.Context, tenantID primitive.ObjectID, status string) (int64, error) {
	filter := bson.M{"memberships.tenant_id": tenantID}
	if status != "" {
		filter["status"] = status
	}
	return s.Users.CountDocuments(ctx, filter)
}

func (s *Store) SetMembership(ctx context.Context, userID, tenantID primitive.ObjectID, roles []string) error {
	now := time.Now()
	res, err := s.Users.UpdateOne(ctx,
		bson.M{"_id": userID, "memberships.tenant_id": tenantID},
		bson.M{"$set": bson.M{"memberships.$.roles": roles, "updated_at": now}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		_, err = s.Users.UpdateOne(ctx, bson.M{"_id": userID},
			bson.M{"$push": bson.M{"memberships": models.Membership{TenantID: tenantID, Roles: roles, JoinedAt: now}}, "$set": bson.M{"updated_at": now}})
		return err
	}
	return nil
}

func (s *Store) RemoveMembership(ctx context.Context, userID, tenantID primitive.ObjectID) error {
	_, err := s.Users.UpdateOne(ctx, bson.M{"_id": userID},
		bson.M{"$pull": bson.M{"memberships": bson.M{"tenant_id": tenantID}}, "$set": bson.M{"updated_at": time.Now()}})
	return err
}

func (s *Store) MarkLogin(ctx context.Context, userID primitive.ObjectID) error {
	now := time.Now()
	_, err := s.Users.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{"$set": bson.M{"last_login_at": now}})
	return err
}

func (s *Store) CountUsersWithRole(ctx context.Context, tenantID primitive.ObjectID, role string) (int64, error) {
	return s.Users.CountDocuments(ctx, bson.M{
		"memberships": bson.M{"$elemMatch": bson.M{"tenant_id": tenantID, "roles": role}},
	})
}

func (s *Store) BumpTokenVersion(ctx context.Context, userID primitive.ObjectID) error {
	_, err := s.Users.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{"$inc": bson.M{"token_version": 1}})
	return err
}

func (s *Store) ListUsersWithRole(ctx context.Context, tenantID primitive.ObjectID, role string) ([]models.User, error) {
	cur, err := s.Users.Find(ctx, bson.M{
		"memberships": bson.M{"$elemMatch": bson.M{"tenant_id": tenantID, "roles": role}},
	}, options.Find().SetSort(bson.D{{Key: "email", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var out []models.User
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) FindUserByExternalID(ctx context.Context, tenantID primitive.ObjectID, externalID string) (*models.User, error) {
	var u models.User
	err := s.Users.FindOne(ctx, bson.M{
		"external_id":           externalID,
		"memberships.tenant_id": tenantID,
	}).Decode(&u)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &u, err
}
