package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/config"
	"ucenter/internal/models"
	"ucenter/internal/security"
	"ucenter/internal/store"
	"ucenter/internal/token"
	"ucenter/internal/webhook"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrMFARequired        = errors.New("mfa required")
	ErrInvalidMFA         = errors.New("invalid verification code")
	ErrAccountDisabled    = errors.New("account is disabled")
	ErrNoMembership       = errors.New("user is not a member of this tenant")
	ErrInvalidClient      = errors.New("invalid client")
	ErrInvalidGrant       = errors.New("invalid grant")
	ErrInvalidScope       = errors.New("invalid scope")
	ErrConsentRequired    = errors.New("consent required")
	ErrInvalidRequest     = errors.New("invalid request")
	ErrTenantDisabled     = errors.New("tenant is disabled")
)

type Service struct {
	Store    *store.Store
	Tokens   *token.Manager
	Config   config.Config
	Webhooks *webhook.Dispatcher
}

func New(s *store.Store, t *token.Manager, c config.Config) *Service {
	return &Service{Store: s, Tokens: t, Config: c}
}

func (s *Service) Issuer(t *models.Tenant) string {
	return s.Config.BaseURL + "/t/" + t.Slug
}

func (s *Service) BaseURL() string { return s.Config.BaseURL }

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

type SessionBundle struct {
	Session *models.Session
	User    *models.User
	Tenant  *models.Tenant
	Raw     string
}

func (s *Service) LoginPassword(ctx context.Context, tenant *models.Tenant, email, password string) (*models.User, error) {
	if tenant.Status != models.StatusActive {
		return nil, ErrTenantDisabled
	}
	user, err := s.Store.FindUserByEmail(ctx, security.NormalizeEmail(email))
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if user.Status != models.StatusActive {
		return nil, ErrAccountDisabled
	}
	if user.Membership(tenant.ID) == nil && !user.PlatformAdmin {
		return nil, ErrNoMembership
	}
	if !security.VerifyPassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) VerifyMFA(user *models.User, code string) ([]string, bool) {
	code = strings.TrimSpace(code)
	if code == "" {
		return user.MFA.RecoveryCodes, false
	}
	if security.VerifyTOTP(user.MFA.Secret, code) {
		return user.MFA.RecoveryCodes, true
	}
	if remaining, ok := security.ConsumeRecoveryCode(code, user.MFA.RecoveryCodes); ok {
		return remaining, true
	}
	return user.MFA.RecoveryCodes, false
}

func (s *Service) StartSession(ctx context.Context, tenant *models.Tenant, user *models.User, mfa bool, ip, ua string) (*models.Session, string, error) {
	raw, err := security.RandomToken(32)
	if err != nil {
		return nil, "", err
	}
	ttlHours := tenant.Settings.SessionTTLHours
	if ttlHours <= 0 {
		ttlHours = 12
	}
	now := time.Now()
	sess := &models.Session{
		ID:        security.HashToken(raw),
		TenantID:  tenant.ID,
		UserID:    user.ID,
		AuthTime:  now,
		MFA:       mfa,
		IP:        ip,
		UserAgent: ua,
		ExpiresAt: now.Add(time.Duration(ttlHours) * time.Hour),
		CreatedAt: now,
	}
	if err := s.Store.CreateSession(ctx, sess); err != nil {
		return nil, "", err
	}
	_ = s.Store.MarkLogin(ctx, user.ID)
	return sess, raw, nil
}

func (s *Service) SessionFromRaw(ctx context.Context, raw string) (*SessionBundle, error) {
	if raw == "" {
		return nil, store.ErrNotFound
	}
	sess, err := s.Store.FindSession(ctx, security.HashToken(raw))
	if err != nil {
		return nil, err
	}
	user, err := s.Store.FindUserByID(ctx, sess.UserID)
	if err != nil || user.Status != models.StatusActive {
		return nil, store.ErrNotFound
	}
	if user.Membership(sess.TenantID) == nil && !user.PlatformAdmin {
		return nil, store.ErrNotFound
	}
	tenant, err := s.Store.FindTenantByID(ctx, sess.TenantID)
	if err != nil || tenant.Status != models.StatusActive {
		return nil, store.ErrNotFound
	}
	return &SessionBundle{Session: sess, User: user, Tenant: tenant, Raw: raw}, nil
}

func (s *Service) EndSession(ctx context.Context, raw string) {
	if raw == "" {
		return
	}
	hash := security.HashToken(raw)
	sess, err := s.Store.FindSession(ctx, hash)
	if err == nil {
		_ = s.Store.RevokeUserRefreshTokens(ctx, sess.TenantID, sess.UserID)
	}
	_ = s.Store.RevokeSession(ctx, hash)
}

func (s *Service) Permissions(ctx context.Context, tenantID primitive.ObjectID, roles []string) []string {
	roleDocs, err := s.Store.RolesByKeys(ctx, tenantID, roles)
	if err != nil {
		return nil
	}
	set := map[string]bool{}
	for _, r := range roleDocs {
		for _, p := range r.Permissions {
			set[p] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (s *Service) Can(ctx context.Context, user *models.User, tenantID primitive.ObjectID, permission string) bool {
	if user.PlatformAdmin {
		return true
	}
	m := user.Membership(tenantID)
	if m == nil {
		return false
	}
	perms := s.Permissions(ctx, tenantID, m.Roles)
	for _, p := range perms {
		if p == permission {
			return true
		}
	}
	return false
}

func (s *Service) Audit(ctx context.Context, tenantID primitive.ObjectID, actor *models.User, action, targetType, targetID, ip, ua string, metadata map[string]interface{}) {
	a := &models.AuditLog{
		TenantID:   tenantID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		IP:         ip,
		UserAgent:  ua,
		Metadata:   metadata,
	}
	if actor != nil {
		a.ActorID = actor.ID
		a.ActorEmail = actor.Email
	}
	_ = s.Store.CreateAudit(ctx, a)
	if s.Webhooks != nil {
		data := map[string]interface{}{
			"action": action, "target_type": targetType, "target_id": targetID,
			"actor_email": a.ActorEmail, "ip": ip,
		}
		for k, v := range metadata {
			data[k] = v
		}
		s.Webhooks.Publish(ctx, tenantID, action, data)
	}
}

// ---- OIDC helpers ----

func SplitScopes(scope string) []string {
	if strings.TrimSpace(scope) == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range strings.Fields(scope) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func ScopeSubset(requested, allowed []string) bool {
	set := map[string]bool{}
	for _, s := range allowed {
		set[s] = true
	}
	for _, s := range requested {
		if !set[s] {
			return false
		}
	}
	return true
}

func (s *Service) ValidateAuthorizeParams(tenant *models.Tenant, client *models.Client, redirectURI, responseType, scope, challenge, method string) error {
	if tenant.Status != models.StatusActive {
		return ErrTenantDisabled
	}
	if client == nil || !client.Enabled {
		return ErrInvalidClient
	}
	validRedirect := false
	for _, u := range client.RedirectURIs {
		if u == redirectURI {
			validRedirect = true
			break
		}
	}
	if !validRedirect {
		return fmt.Errorf("%w: redirect_uri is not registered", ErrInvalidRequest)
	}
	if responseType != "code" {
		return fmt.Errorf("%w: only response_type=code is supported", ErrInvalidRequest)
	}
	scopes := SplitScopes(scope)
	if len(scopes) == 0 {
		return fmt.Errorf("%w: scope is required", ErrInvalidScope)
	}
	if !ScopeSubset(scopes, client.AllowedScopes) {
		return fmt.Errorf("%w: requested scope is not allowed for this client", ErrInvalidScope)
	}
	hasOpenID := false
	for _, sc := range scopes {
		if sc == "openid" {
			hasOpenID = true
		}
	}
	if !hasOpenID {
		return fmt.Errorf("%w: openid scope is required", ErrInvalidScope)
	}
	if client.Type == models.ClientTypePublic || client.RequirePKCE {
		if challenge == "" {
			return fmt.Errorf("%w: PKCE code_challenge is required", ErrInvalidRequest)
		}
		if method != "" && method != "S256" && method != "plain" {
			return fmt.Errorf("%w: unsupported code_challenge_method", ErrInvalidRequest)
		}
	}
	return nil
}

// AuthenticateClient resolves a client from client_id/secret values. Basic auth
// credentials are handled by the caller.
func (s *Service) AuthenticateClient(ctx context.Context, tenant *models.Tenant, clientID, clientSecret string) (*models.Client, error) {
	if clientID == "" {
		return nil, ErrInvalidClient
	}
	client, err := s.Store.FindClientByClientID(ctx, tenant.ID, clientID)
	if err != nil || !client.Enabled {
		return nil, ErrInvalidClient
	}
	switch client.Type {
	case models.ClientTypePublic:
		return client, nil
	case models.ClientTypeConfidential:
		if !security.VerifyClientSecret(clientSecret, client.ClientSecretHash) {
			return nil, ErrInvalidClient
		}
		return client, nil
	default:
		return nil, ErrInvalidClient
	}
}

func (s *Service) CompleteAuthRequest(ctx context.Context, req *models.AuthRequest, tenant *models.Tenant, user *models.User, sess *models.Session, grantConsent bool) (string, error) {
	if req.Status == models.RequestCompleted {
		return "", ErrInvalidRequest
	}
	if time.Now().After(req.ExpiresAt) {
		return "", fmt.Errorf("%w: authorization request expired", ErrInvalidRequest)
	}
	client, err := s.Store.FindClientByClientID(ctx, tenant.ID, req.ClientID)
	if err != nil || !client.Enabled {
		return "", ErrInvalidClient
	}
	scopes := SplitScopes(req.Scope)

	if !client.Trusted {
		consent, err := s.Store.FindConsent(ctx, tenant.ID, user.ID, client.ClientID)
		covered := err == nil && ScopeSubset(scopes, consent.Scopes)
		if !covered && !grantConsent {
			return "", ErrConsentRequired
		}
		if grantConsent {
			existing := []string{}
			if err == nil {
				existing = consent.Scopes
			}
			merged := append(existing, scopes...)
			_ = s.Store.UpsertConsent(ctx, tenant.ID, user.ID, client.ClientID, merged)
		}
	}

	rawCode, err := security.RandomToken(32)
	if err != nil {
		return "", err
	}
	now := time.Now()
	code := &models.AuthCode{
		Code:                security.HashToken(rawCode),
		TenantID:            tenant.ID,
		ClientID:            client.ClientID,
		UserID:              user.ID,
		SessionID:           sess.ID,
		RedirectURI:         req.RedirectURI,
		Scope:               req.Scope,
		Nonce:               req.Nonce,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		AuthTime:            sess.AuthTime,
		MFA:                 sess.MFA,
		ExpiresAt:           now.Add(5 * time.Minute),
		CreatedAt:           now,
	}
	if err := s.Store.CreateAuthCode(ctx, code); err != nil {
		return "", err
	}
	_ = s.Store.UpdateAuthRequest(ctx, req.ID, bson.M{"status": models.RequestCompleted, "user_id": user.ID})
	s.Audit(ctx, tenant.ID, user, "oidc.authorize.approved", "client", client.ClientID, "", "", map[string]interface{}{"scope": req.Scope})
	return AppendQuery(req.RedirectURI, url.Values{"code": {rawCode}, "state": {req.State}}), nil
}

func DenyAuthRequest(ctx context.Context, req *models.AuthRequest) string {
	_ = ctx
	values := url.Values{"error": {"access_denied"}, "error_description": {"The user denied the authorization request"}}
	if req.State != "" {
		values.Set("state", req.State)
	}
	return AppendQuery(req.RedirectURI, values)
}

func AppendQuery(base string, values url.Values) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	for k, vs := range values {
		for _, v := range vs {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

type IssueParams struct {
	Tenant    *models.Tenant
	Client    *models.Client
	User      *models.User
	SessionID string
	Scope     string
	Nonce     string
	AuthTime  time.Time
	MFA       bool
}

func (s *Service) IssueTokens(ctx context.Context, p IssueParams) (*TokenResponse, error) {
	now := time.Now()
	accessTTL := p.Client.AccessTokenTTL
	if accessTTL <= 0 {
		accessTTL = 3600
	}
	idTTL := p.Client.IDTokenTTL
	if idTTL <= 0 {
		idTTL = accessTTL
	}
	refreshTTL := p.Client.RefreshTokenTTL
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * 3600
	}
	issuer := s.Issuer(p.Tenant)
	scopes := SplitScopes(p.Scope)
	var roles []string
	if m := p.User.Membership(p.Tenant.ID); m != nil {
		roles = m.Roles
	}
	amr := []string{"pwd"}
	if p.MFA {
		amr = append(amr, "otp")
	}
	base := token.Claims{
		RegisteredClaims:  jwtClaims(issuer, p.User.ID.Hex(), p.Client.ClientID, now, accessTTL),
		TenantID:          p.Tenant.ID.Hex(),
		ClientID:          p.Client.ClientID,
		Scope:             p.Scope,
		Roles:             roles,
		Email:             p.User.Email,
		EmailVerified:     p.User.EmailVerified,
		Name:              p.User.Name,
		PreferredUsername: p.User.Username,
		Picture:           p.User.Picture,
		AuthTime:          p.AuthTime.Unix(),
		SessionID:         p.SessionID,
		TokenType:         "access",
		UpdatedAt:         p.User.UpdatedAt.Unix(),
	}
	base.AMR = amr
	access, err := s.Tokens.Sign(ctx, p.Tenant.ID, base)
	if err != nil {
		return nil, err
	}
	resp := &TokenResponse{
		AccessToken: access,
		TokenType:   "Bearer",
		ExpiresIn:   accessTTL,
		Scope:       p.Scope,
	}

	if hasScope(scopes, "openid") {
		idClaims := base
		idClaims.RegisteredClaims = jwtClaims(issuer, p.User.ID.Hex(), p.Client.ClientID, now, idTTL)
		idClaims.TokenType = "id"
		idClaims.Nonce = p.Nonce
		idClaims.Scope = ""
		idToken, err := s.Tokens.Sign(ctx, p.Tenant.ID, idClaims)
		if err != nil {
			return nil, err
		}
		resp.IDToken = idToken
	}

	if hasScope(scopes, "offline_access") && hasGrant(p.Client.GrantTypes, "refresh_token") {
		rawRefresh, err := security.RandomToken(40)
		if err != nil {
			return nil, err
		}
		rt := &models.RefreshToken{
			ID:        security.HashToken(rawRefresh),
			TenantID:  p.Tenant.ID,
			ClientID:  p.Client.ClientID,
			UserID:    p.User.ID,
			SessionID: p.SessionID,
			Scope:     p.Scope,
			AuthTime:  p.AuthTime,
			MFA:       p.MFA,
			ExpiresAt: now.Add(time.Duration(refreshTTL) * time.Second),
			CreatedAt: now,
		}
		if err := s.Store.CreateRefreshToken(ctx, rt); err != nil {
			return nil, err
		}
		resp.RefreshToken = rawRefresh
	}
	return resp, nil
}

func (s *Service) RefreshTokens(ctx context.Context, tenant *models.Tenant, client *models.Client, rawToken, requestedScope string) (*TokenResponse, error) {
	rt, err := s.Store.FindRefreshToken(ctx, security.HashToken(rawToken))
	if err != nil {
		return nil, ErrInvalidGrant
	}
	if rt.Revoked {
		// Reuse of a rotated token indicates replay. Revoke its successor so
		// a stolen token cannot keep a session alive.
		if rt.RotatedTo != "" {
			_ = s.Store.RevokeRefreshToken(ctx, rt.RotatedTo)
		}
		return nil, ErrInvalidGrant
	}
	if rt.ClientID != client.ClientID || time.Now().After(rt.ExpiresAt) {
		return nil, ErrInvalidGrant
	}
	sess, err := s.Store.FindSession(ctx, rt.SessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: session is no longer active", ErrInvalidGrant)
	}
	user, err := s.Store.FindUserByID(ctx, rt.UserID)
	if err != nil || user.Status != models.StatusActive || user.Membership(tenant.ID) == nil {
		return nil, fmt.Errorf("%w: user is no longer active", ErrInvalidGrant)
	}
	scope := rt.Scope
	if requestedScope != "" {
		if !ScopeSubset(SplitScopes(requestedScope), SplitScopes(rt.Scope)) {
			return nil, ErrInvalidScope
		}
		scope = requestedScope
	}
	_ = s.Store.RevokeRefreshToken(ctx, rt.ID)

	// Rotate: issue a new refresh token as part of IssueTokens.
	p := IssueParams{
		Tenant:    tenant,
		Client:    client,
		User:      user,
		SessionID: sess.ID,
		Scope:     scope,
		AuthTime:  rt.AuthTime,
		MFA:       rt.MFA,
	}
	resp, err := s.IssueTokens(ctx, p)
	if err != nil {
		return nil, err
	}
	if resp.RefreshToken != "" {
		_ = s.Store.RotateRefreshToken(ctx, rt.ID, security.HashToken(resp.RefreshToken))
	}
	return resp, nil
}

func (s *Service) ClientCredentials(ctx context.Context, tenant *models.Tenant, client *models.Client, scope string) (*TokenResponse, error) {
	if client.Type != models.ClientTypeConfidential {
		return nil, ErrInvalidClient
	}
	if !hasGrant(client.GrantTypes, "client_credentials") {
		return nil, fmt.Errorf("%w: client_credentials is not enabled for this client", ErrInvalidGrant)
	}
	if scope == "" {
		scope = "openid"
	}
	scopes := SplitScopes(scope)
	if !ScopeSubset(scopes, client.AllowedScopes) {
		return nil, ErrInvalidScope
	}
	now := time.Now()
	ttl := client.AccessTokenTTL
	if ttl <= 0 {
		ttl = 3600
	}
	claims := token.Claims{
		RegisteredClaims: jwtClaims(s.Issuer(tenant), "client:"+client.ClientID, client.ClientID, now, ttl),
		TenantID:         tenant.ID.Hex(),
		ClientID:         client.ClientID,
		Scope:            scope,
		TokenType:        "access",
	}
	access, err := s.Tokens.Sign(ctx, tenant.ID, claims)
	if err != nil {
		return nil, err
	}
	return &TokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: ttl, Scope: scope}, nil
}

func (s *Service) Discovery(tenant *models.Tenant) map[string]interface{} {
	issuer := s.Issuer(tenant)
	return map[string]interface{}{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oidc/authorize",
		"token_endpoint":                        issuer + "/oidc/token",
		"userinfo_endpoint":                     issuer + "/oidc/userinfo",
		"jwks_uri":                              issuer + "/oidc/jwks",
		"end_session_endpoint":                  issuer + "/oidc/end_session",
		"revocation_endpoint":                   issuer + "/oidc/revoke",
		"introspection_endpoint":                issuer + "/oidc/introspect",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", "client_credentials"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email", "offline_access", "roles"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
		"code_challenge_methods_supported":      []string{"S256", "plain"},
		"claims_supported": []string{
			"iss", "sub", "aud", "exp", "iat", "auth_time", "nonce", "email", "email_verified",
			"name", "preferred_username", "picture", "tenant_id", "roles", "updated_at",
		},
	}
}

func jwtClaims(issuer, subject, audience string, now time.Time, ttl int) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   subject,
		Audience:  jwt.ClaimStrings{audience},
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(ttl) * time.Second)),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
		ID:        randomJTI(),
	}
}

func randomJTI() string {
	v, err := security.RandomHex(12)
	if err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return v
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

func hasGrant(grants []string, want string) bool {
	for _, g := range grants {
		if g == want {
			return true
		}
	}
	return false
}

// ProvisionTenant creates the system roles for a newly created tenant and
// ensures a signing key exists so discovery/JWKS are immediately usable.
func (s *Service) ProvisionTenant(ctx context.Context, tenant *models.Tenant) error {
	now := time.Now()
	for _, r := range models.SystemRoles {
		role := r
		role.ID = primitive.NewObjectID()
		role.TenantID = tenant.ID
		role.CreatedAt = now
		role.UpdatedAt = now
		if err := s.Store.CreateRole(ctx, &role); err != nil && !store.IsDuplicate(err) {
			return err
		}
	}
	_, err := s.Tokens.KeyFor(ctx, tenant.ID)
	return err
}

// PermissionsForRoles returns the union of permissions for role keys.
func (s *Service) PermissionsForRoles(ctx context.Context, tenantID primitive.ObjectID, roles []string) []string {
	return s.Permissions(ctx, tenantID, roles)
}
