package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusInvited  = "invited"

	ClientTypeConfidential = "confidential"
	ClientTypePublic       = "public"

	RequestPending       = "pending"
	RequestAuthenticated = "authenticated"
	RequestApproved      = "approved"
	RequestDenied        = "denied"
	RequestCompleted     = "completed"
	RequestExpired       = "expired"
)

type Branding struct {
	LogoURL         string `bson:"logo_url" json:"logoUrl"`
	PrimaryColor    string `bson:"primary_color" json:"primaryColor"`
	BackgroundColor string `bson:"background_color" json:"backgroundColor"`
	SupportEmail    string `bson:"support_email" json:"supportEmail"`
	LoginTemplate   string `bson:"login_template" json:"loginTemplate"`
	LoginTagline    string `bson:"login_tagline" json:"loginTagline"`
}

var LoginTemplates = []string{
	"aurora", "minimal", "glass", "enterprise", "fintech",
	"healthcare", "education", "developer", "government", "retail", "handdrawn",
}

func ValidLoginTemplate(id string) bool {
	if id == "" {
		return true
	}
	for _, t := range LoginTemplates {
		if t == id {
			return true
		}
	}
	return false
}

type TenantSettings struct {
	AllowRegistration   bool     `bson:"allow_registration" json:"allowRegistration"`
	RequireMFA          bool     `bson:"require_mfa" json:"requireMfa"`
	PasswordMinLength   int      `bson:"password_min_length" json:"passwordMinLength"`
	SessionTTLHours     int      `bson:"session_ttl_hours" json:"sessionTtlHours"`
	AllowedEmailDomains []string `bson:"allowed_email_domains" json:"allowedEmailDomains"`
	AllowPasswordLogin  bool     `bson:"allow_password_login" json:"allowPasswordLogin"`
}

func DefaultSettings() TenantSettings {
	return TenantSettings{
		AllowRegistration:  false,
		RequireMFA:         false,
		PasswordMinLength:  10,
		SessionTTLHours:    12,
		AllowPasswordLogin: true,
	}
}

type Tenant struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Slug      string             `bson:"slug" json:"slug"`
	Name      string             `bson:"name" json:"name"`
	Status    string             `bson:"status" json:"status"`
	Domains   []string           `bson:"domains" json:"domains"`
	Branding  Branding           `bson:"branding" json:"branding"`
	Settings  TenantSettings     `bson:"settings" json:"settings"`
	CreatedAt time.Time          `bson:"created_at" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updatedAt"`
}

type Membership struct {
	TenantID primitive.ObjectID `bson:"tenant_id" json:"tenantId"`
	Roles    []string           `bson:"roles" json:"roles"`
	JoinedAt time.Time          `bson:"joined_at" json:"joinedAt"`
}

type MFASettings struct {
	Enabled       bool       `bson:"enabled" json:"enabled"`
	Secret        string     `bson:"secret,omitempty" json:"-"`
	RecoveryCodes []string   `bson:"recovery_codes,omitempty" json:"-"`
	EnabledAt     *time.Time `bson:"enabled_at,omitempty" json:"enabledAt"`
}

type User struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Email         string             `bson:"email" json:"email"`
	EmailVerified bool               `bson:"email_verified" json:"emailVerified"`
	Username      string             `bson:"username" json:"username"`
	Name          string             `bson:"name" json:"name"`
	Picture       string             `bson:"picture" json:"picture"`
	PasswordHash  string             `bson:"password_hash" json:"-"`
	Status        string             `bson:"status" json:"status"`
	PlatformAdmin bool               `bson:"platform_admin" json:"platformAdmin"`
	Memberships   []Membership       `bson:"memberships" json:"memberships"`
	MFA           MFASettings        `bson:"mfa" json:"mfa"`
	LastLoginAt   *time.Time         `bson:"last_login_at,omitempty" json:"lastLoginAt"`
	TokenVersion  int                `bson:"token_version" json:"-"`
	CreatedAt     time.Time          `bson:"created_at" json:"createdAt"`
	UpdatedAt     time.Time          `bson:"updated_at" json:"updatedAt"`
}

func (u *User) Membership(tenantID primitive.ObjectID) *Membership {
	for i := range u.Memberships {
		if u.Memberships[i].TenantID == tenantID {
			return &u.Memberships[i]
		}
	}
	return nil
}

func (u *User) HasRole(tenantID primitive.ObjectID, role string) bool {
	m := u.Membership(tenantID)
	if m == nil {
		return false
	}
	for _, r := range m.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type Role struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID    primitive.ObjectID `bson:"tenant_id" json:"tenantId"`
	Key         string             `bson:"key" json:"key"`
	Name        string             `bson:"name" json:"name"`
	Description string             `bson:"description" json:"description"`
	Permissions []string           `bson:"permissions" json:"permissions"`
	System      bool               `bson:"system" json:"system"`
	CreatedAt   time.Time          `bson:"created_at" json:"createdAt"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updatedAt"`
}

type Client struct {
	ID                     primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID               primitive.ObjectID `bson:"tenant_id" json:"tenantId"`
	ClientID               string             `bson:"client_id" json:"clientId"`
	ClientSecretHash       string             `bson:"client_secret_hash" json:"-"`
	ClientSecretEnc        string             `bson:"client_secret_enc,omitempty" json:"-"`
	Name                   string             `bson:"name" json:"name"`
	Description            string             `bson:"description" json:"description"`
	Type                   string             `bson:"type" json:"type"`
	RedirectURIs           []string           `bson:"redirect_uris" json:"redirectUris"`
	PostLogoutRedirectURIs []string           `bson:"post_logout_redirect_uris" json:"postLogoutRedirectUris"`
	AllowedScopes          []string           `bson:"allowed_scopes" json:"allowedScopes"`
	GrantTypes             []string           `bson:"grant_types" json:"grantTypes"`
	RequirePKCE            bool               `bson:"require_pkce" json:"requirePkce"`
	Trusted                bool               `bson:"trusted" json:"trusted"`
	AccessTokenTTL         int                `bson:"access_token_ttl" json:"accessTokenTtl"`
	IDTokenTTL             int                `bson:"id_token_ttl" json:"idTokenTtl"`
	RefreshTokenTTL        int                `bson:"refresh_token_ttl" json:"refreshTokenTtl"`
	Enabled                bool               `bson:"enabled" json:"enabled"`
	LogoURL                string             `bson:"logo_url" json:"logoUrl"`
	CreatedAt              time.Time          `bson:"created_at" json:"createdAt"`
	UpdatedAt              time.Time          `bson:"updated_at" json:"updatedAt"`
}

type AuthRequest struct {
	ID                  string              `bson:"_id" json:"id"`
	TenantID            primitive.ObjectID  `bson:"tenant_id" json:"tenantId"`
	ClientID            string              `bson:"client_id" json:"clientId"`
	RedirectURI         string              `bson:"redirect_uri" json:"redirectUri"`
	ResponseType        string              `bson:"response_type" json:"responseType"`
	Scope               string              `bson:"scope" json:"scope"`
	State               string              `bson:"state" json:"state"`
	Nonce               string              `bson:"nonce" json:"nonce"`
	CodeChallenge       string              `bson:"code_challenge" json:"-"`
	CodeChallengeMethod string              `bson:"code_challenge_method" json:"-"`
	Prompt              string              `bson:"prompt" json:"prompt"`
	LoginHint           string              `bson:"login_hint" json:"loginHint"`
	UserID              *primitive.ObjectID `bson:"user_id,omitempty" json:"userId,omitempty"`
	Status              string              `bson:"status" json:"status"`
	ExpiresAt           time.Time           `bson:"expires_at" json:"expiresAt"`
	CreatedAt           time.Time           `bson:"created_at" json:"createdAt"`
}

type AuthCode struct {
	Code                string             `bson:"_id" json:"-"`
	TenantID            primitive.ObjectID `bson:"tenant_id" json:"tenantId"`
	ClientID            string             `bson:"client_id" json:"clientId"`
	UserID              primitive.ObjectID `bson:"user_id" json:"userId"`
	SessionID           string             `bson:"session_id" json:"sessionId"`
	RedirectURI         string             `bson:"redirect_uri" json:"redirectUri"`
	Scope               string             `bson:"scope" json:"scope"`
	Nonce               string             `bson:"nonce" json:"nonce"`
	CodeChallenge       string             `bson:"code_challenge" json:"-"`
	CodeChallengeMethod string             `bson:"code_challenge_method" json:"-"`
	AuthTime            time.Time          `bson:"auth_time" json:"authTime"`
	MFA                 bool               `bson:"mfa" json:"mfa"`
	ExpiresAt           time.Time          `bson:"expires_at" json:"expiresAt"`
	Used                bool               `bson:"used" json:"used"`
	CreatedAt           time.Time          `bson:"created_at" json:"createdAt"`
}

type Session struct {
	ID        string             `bson:"_id" json:"id"`
	TenantID  primitive.ObjectID `bson:"tenant_id" json:"tenantId"`
	UserID    primitive.ObjectID `bson:"user_id" json:"userId"`
	AuthTime  time.Time          `bson:"auth_time" json:"authTime"`
	MFA       bool               `bson:"mfa" json:"mfa"`
	IP        string             `bson:"ip" json:"ip"`
	UserAgent string             `bson:"user_agent" json:"userAgent"`
	ExpiresAt time.Time          `bson:"expires_at" json:"expiresAt"`
	Revoked   bool               `bson:"revoked" json:"revoked"`
	CreatedAt time.Time          `bson:"created_at" json:"createdAt"`
}

type RefreshToken struct {
	ID        string             `bson:"_id" json:"id"`
	TenantID  primitive.ObjectID `bson:"tenant_id" json:"tenantId"`
	ClientID  string             `bson:"client_id" json:"clientId"`
	UserID    primitive.ObjectID `bson:"user_id" json:"userId"`
	SessionID string             `bson:"session_id" json:"sessionId"`
	Scope     string             `bson:"scope" json:"scope"`
	AuthTime  time.Time          `bson:"auth_time" json:"authTime"`
	MFA       bool               `bson:"mfa" json:"mfa"`
	ExpiresAt time.Time          `bson:"expires_at" json:"expiresAt"`
	Revoked   bool               `bson:"revoked" json:"revoked"`
	RotatedTo string             `bson:"rotated_to,omitempty" json:"rotatedTo,omitempty"`
	CreatedAt time.Time          `bson:"created_at" json:"createdAt"`
}

type SigningKey struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID      primitive.ObjectID `bson:"tenant_id" json:"tenantId"`
	Kid           string             `bson:"kid" json:"kid"`
	Alg           string             `bson:"alg" json:"alg"`
	PrivatePEM    string             `bson:"private_pem,omitempty" json:"-"`
	PrivatePEMEnc string             `bson:"private_pem_enc,omitempty" json:"-"`
	PublicPEM     string             `bson:"public_pem" json:"-"`
	Active        bool               `bson:"active" json:"active"`
	CreatedAt     time.Time          `bson:"created_at" json:"createdAt"`
}

type Consent struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID  primitive.ObjectID `bson:"tenant_id" json:"tenantId"`
	UserID    primitive.ObjectID `bson:"user_id" json:"userId"`
	ClientID  string             `bson:"client_id" json:"clientId"`
	Scopes    []string           `bson:"scopes" json:"scopes"`
	CreatedAt time.Time          `bson:"created_at" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updatedAt"`
}

type AuditLog struct {
	ID         primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	TenantID   primitive.ObjectID     `bson:"tenant_id" json:"tenantId"`
	ActorID    primitive.ObjectID     `bson:"actor_id,omitempty" json:"actorId,omitempty"`
	ActorEmail string                 `bson:"actor_email" json:"actorEmail"`
	Action     string                 `bson:"action" json:"action"`
	TargetType string                 `bson:"target_type" json:"targetType"`
	TargetID   string                 `bson:"target_id" json:"targetId"`
	IP         string                 `bson:"ip" json:"ip"`
	UserAgent  string                 `bson:"user_agent" json:"userAgent"`
	Metadata   map[string]interface{} `bson:"metadata,omitempty" json:"metadata,omitempty"`
	CreatedAt  time.Time              `bson:"created_at" json:"createdAt"`
}

type APIToken struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID   primitive.ObjectID `bson:"tenant_id" json:"tenantId"`
	UserID     primitive.ObjectID `bson:"user_id" json:"userId"`
	Name       string             `bson:"name" json:"name"`
	TokenHash  string             `bson:"token_hash" json:"-"`
	Prefix     string             `bson:"prefix" json:"prefix"`
	Scopes     []string           `bson:"scopes" json:"scopes"`
	ExpiresAt  *time.Time         `bson:"expires_at,omitempty" json:"expiresAt,omitempty"`
	LastUsedAt *time.Time         `bson:"last_used_at,omitempty" json:"lastUsedAt,omitempty"`
	Revoked    bool               `bson:"revoked" json:"revoked"`
	CreatedAt  time.Time          `bson:"created_at" json:"createdAt"`
}

const (
	EmailTokenResetPassword = "reset_password"
	EmailTokenVerifyEmail   = "verify_email"
	EmailTokenMFA           = "mfa_recovery"
	EmailTokenInvite        = "invite"
)

type EmailToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID  primitive.ObjectID `bson:"tenant_id" json:"tenantId"`
	UserID    primitive.ObjectID `bson:"user_id" json:"userId"`
	Type      string             `bson:"type" json:"type"`
	TokenHash string             `bson:"token_hash" json:"-"`
	ExpiresAt time.Time          `bson:"expires_at" json:"expiresAt"`
	Used      bool               `bson:"used" json:"used"`
	CreatedAt time.Time          `bson:"created_at" json:"createdAt"`
}
