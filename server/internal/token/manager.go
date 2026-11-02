package token

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/models"
	"ucenter/internal/security"
	"ucenter/internal/store"
)

var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	jwt.RegisteredClaims
	TenantID          string   `json:"tenant_id,omitempty"`
	ClientID          string   `json:"client_id,omitempty"`
	Scope             string   `json:"scope,omitempty"`
	Roles             []string `json:"roles,omitempty"`
	Email             string   `json:"email,omitempty"`
	EmailVerified     bool     `json:"email_verified,omitempty"`
	Name              string   `json:"name,omitempty"`
	PreferredUsername string   `json:"preferred_username,omitempty"`
	Picture           string   `json:"picture,omitempty"`
	Nonce             string   `json:"nonce,omitempty"`
	AuthTime          int64    `json:"auth_time,omitempty"`
	TokenType         string   `json:"token_type,omitempty"`
	TokenVersion      int      `json:"token_version,omitempty"`
	SessionID         string   `json:"sid,omitempty"`
	UpdatedAt         int64    `json:"updated_at,omitempty"`
	AMR               []string `json:"amr,omitempty"`
}

type Key struct {
	Kid     string
	Private *rsa.PrivateKey
	Public  *rsa.PublicKey
}

type tenantKeys struct {
	keys    map[string]*Key
	primary string
}

type Manager struct {
	store  *store.Store
	cipher *security.Cipher
	mu     sync.RWMutex
	cache  map[string]*tenantKeys
}

func NewManager(s *store.Store, cipher *security.Cipher) *Manager {
	return &Manager{store: s, cipher: cipher, cache: make(map[string]*tenantKeys)}
}

func (m *Manager) keyset(ctx context.Context, tenantID primitive.ObjectID) (*tenantKeys, error) {
	key := tenantID.Hex()
	m.mu.RLock()
	set, ok := m.cache[key]
	m.mu.RUnlock()
	if ok {
		return set, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if set, ok := m.cache[key]; ok {
		return set, nil
	}
	loaded, err := m.load(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	m.cache[key] = loaded
	return loaded, nil
}

func (m *Manager) load(ctx context.Context, tenantID primitive.ObjectID) (*tenantKeys, error) {
	recs, err := m.store.ListSigningKeys(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		if _, err := generateKeyRecord(ctx, m.store, m.cipher, tenantID); err != nil {
			return nil, err
		}
		recs, err = m.store.ListSigningKeys(ctx, tenantID)
		if err != nil {
			return nil, err
		}
	}
	set := &tenantKeys{keys: make(map[string]*Key, len(recs))}
	for i := range recs {
		rec := &recs[i]
		priv, err := m.privateKey(rec)
		if err != nil {
			return nil, err
		}
		set.keys[rec.Kid] = &Key{Kid: rec.Kid, Private: priv, Public: &priv.PublicKey}
		if set.primary == "" {
			// ListSigningKeys is sorted newest-first, so this is the signing key.
			set.primary = rec.Kid
		}
	}
	if set.primary == "" {
		return nil, fmt.Errorf("no active signing key for tenant %s", tenantID.Hex())
	}
	return set, nil
}

func (m *Manager) privateKey(rec *models.SigningKey) (*rsa.PrivateKey, error) {
	pemText := rec.PrivatePEM
	if rec.PrivatePEMEnc != "" {
		decrypted, err := m.cipher.Decrypt(rec.PrivatePEMEnc)
		if err != nil {
			return nil, fmt.Errorf("decrypt signing key %s: %w", rec.Kid, err)
		}
		pemText = decrypted
	} else if pemText != "" && m.cipher != nil {
		// Migrate legacy plaintext keys to encrypted-at-rest storage.
		if encrypted, err := m.cipher.Encrypt(pemText); err == nil {
			_ = m.store.UpdateSigningKeyPrivate(context.Background(), rec.ID, encrypted)
		}
	}
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, fmt.Errorf("invalid private key PEM for key %s", rec.Kid)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	priv, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("signing key is not RSA")
	}
	return priv, nil
}

func generateKeyRecord(ctx context.Context, s *store.Store, c *security.Cipher, tenantID primitive.ObjectID) (*models.SigningKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, err
	}
	kidBytes := make([]byte, 8)
	if _, err := rand.Read(kidBytes); err != nil {
		return nil, err
	}
	privPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}))
	encrypted, err := c.Encrypt(privPEM)
	if err != nil {
		return nil, err
	}
	rec := &models.SigningKey{
		TenantID:      tenantID,
		Kid:           hex.EncodeToString(kidBytes),
		Alg:           "RS256",
		PrivatePEMEnc: encrypted,
		PublicPEM:     string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})),
		Active:        true,
	}
	if err := s.CreateSigningKey(ctx, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

func (m *Manager) KeyFor(ctx context.Context, tenantID primitive.ObjectID) (*Key, error) {
	set, err := m.keyset(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	k := set.keys[set.primary]
	if k == nil {
		return nil, fmt.Errorf("primary signing key is missing")
	}
	return k, nil
}

// Rotate creates a new signing key. The previous key stays active so JWKS
// clients can verify tokens issued before the rotation.
func (m *Manager) Rotate(ctx context.Context, tenantID primitive.ObjectID) (*Key, error) {
	rec, err := generateKeyRecord(ctx, m.store, m.cipher, tenantID)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	delete(m.cache, tenantID.Hex())
	m.mu.Unlock()
	_ = rec
	return m.KeyFor(ctx, tenantID)
}

func (m *Manager) Sign(ctx context.Context, tenantID primitive.ObjectID, claims Claims) (string, error) {
	k, err := m.KeyFor(ctx, tenantID)
	if err != nil {
		return "", err
	}
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	t.Header["kid"] = k.Kid
	return t.SignedString(k.Private)
}

func (m *Manager) Verify(ctx context.Context, tokenString string, tenantID primitive.ObjectID, issuer string) (*Claims, error) {
	unverified := &Claims{}
	parser := jwt.NewParser()
	parsedUnverified, _, err := parser.ParseUnverified(tokenString, unverified)
	if err != nil {
		return nil, ErrInvalidToken
	}
	kid, _ := parsedUnverified.Header["kid"].(string)

	set, err := m.keyset(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	key := set.keys[kid]
	if key == nil {
		// A rotation may have happened on another instance; refresh once.
		m.mu.Lock()
		delete(m.cache, tenantID.Hex())
		m.mu.Unlock()
		set, err = m.keyset(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		key = set.keys[kid]
	}
	if key == nil {
		key = set.keys[set.primary]
	}
	if key == nil {
		return nil, ErrInvalidToken
	}

	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, ErrInvalidToken
		}
		return key.Public, nil
	}, jwt.WithIssuer(issuer), jwt.WithExpirationRequired(), jwt.WithLeeway(5*time.Second))
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (m *Manager) JWKS(ctx context.Context, tenantID primitive.ObjectID) ([]JWK, error) {
	set, err := m.keyset(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]JWK, 0, len(set.keys))
	for _, key := range set.keys {
		out = append(out, JWK{
			Kty: "RSA",
			Use: "sig",
			Alg: "RS256",
			Kid: key.Kid,
			N:   base64.RawURLEncoding.EncodeToString(key.Public.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.Public.E)).Bytes()),
		})
	}
	return out, nil
}
