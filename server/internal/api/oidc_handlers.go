package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/models"
	"ucenter/internal/security"
	"ucenter/internal/service"
	"ucenter/internal/token"
)

func (a *API) oidcTenant(c *gin.Context) (*models.Tenant, bool) {
	t, err := a.Store.FindTenantBySlug(c.Request.Context(), strings.ToLower(c.Param("slug")))
	if err != nil {
		fail(c, http.StatusNotFound, "tenant_not_found", "Unknown tenant")
		return nil, false
	}
	if t.Status != models.StatusActive {
		fail(c, http.StatusForbidden, "tenant_disabled", "This tenant is disabled")
		return nil, false
	}
	return t, true
}

func oauthError(c *gin.Context, status int, code, description string) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.AbortWithStatusJSON(status, gin.H{"error": code, "error_description": description})
}

func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
}

func (a *API) discovery(c *gin.Context) {
	t, ok := a.oidcTenant(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, a.Svc.Discovery(t))
}

func (a *API) jwks(c *gin.Context) {
	t, ok := a.oidcTenant(c)
	if !ok {
		return
	}
	keys, err := a.Svc.Tokens.JWKS(c.Request.Context(), t.ID)
	if err != nil {
		oauthError(c, http.StatusInternalServerError, "server_error", "Unable to load signing keys")
		return
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, gin.H{"keys": keys})
}

func (a *API) authorize(c *gin.Context) {
	ctx := c.Request.Context()
	t, ok := a.oidcTenant(c)
	if !ok {
		return
	}
	clientID := c.Query("client_id")
	redirectURI := c.Query("redirect_uri")
	responseType := c.Query("response_type")
	scope := c.Query("scope")
	state := c.Query("state")
	nonce := c.Query("nonce")
	challenge := c.Query("code_challenge")
	method := c.Query("code_challenge_method")
	prompt := c.Query("prompt")
	loginHint := c.Query("login_hint")

	client, err := a.Store.FindClientByClientID(ctx, t.ID, clientID)
	if err != nil {
		oauthError(c, http.StatusBadRequest, "invalid_request", "Unknown client_id")
		return
	}
	registeredRedirect := false
	for _, u := range client.RedirectURIs {
		if u == redirectURI {
			registeredRedirect = true
			break
		}
	}
	redirectError := func(code, description string) {
		values := url.Values{"error": {code}, "error_description": {description}}
		if state != "" {
			values.Set("state", state)
		}
		if registeredRedirect {
			c.Redirect(http.StatusFound, service.AppendQuery(redirectURI, values))
			return
		}
		oauthError(c, http.StatusBadRequest, code, description)
	}

	if err := a.Svc.ValidateAuthorizeParams(t, client, redirectURI, responseType, scope, challenge, method); err != nil {
		redirectError("invalid_request", err.Error())
		return
	}

	req := &models.AuthRequest{
		ID:                  mustToken(24),
		TenantID:            t.ID,
		ClientID:            client.ClientID,
		RedirectURI:         redirectURI,
		ResponseType:        responseType,
		Scope:               scope,
		State:               state,
		Nonce:               nonce,
		CodeChallenge:       challenge,
		CodeChallengeMethod: method,
		Prompt:              prompt,
		LoginHint:           loginHint,
		Status:              models.RequestPending,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
		CreatedAt:           time.Now(),
	}
	if err := a.Store.CreateAuthRequest(ctx, req); err != nil {
		oauthError(c, http.StatusInternalServerError, "server_error", "Unable to start authorization")
		return
	}
	noStore(c)

	b, sessErr := a.Svc.SessionFromRaw(ctx, a.sessionCookie(c))
	hasSession := sessErr == nil && b.Tenant.ID == t.ID

	if promptHas(prompt, "none") {
		if !hasSession {
			redirectError("login_required", "The user is not signed in")
			return
		}
		_ = a.Store.UpdateAuthRequest(ctx, req.ID, bson.M{"status": models.RequestAuthenticated, "user_id": b.User.ID})
		req.Status = models.RequestAuthenticated
		req.UserID = &b.User.ID
		redirectURL, err := a.Svc.CompleteAuthRequest(ctx, req, t, b.User, b.Session, false)
		if errors.Is(err, service.ErrConsentRequired) {
			redirectError("consent_required", "User consent is required")
			return
		}
		if err != nil {
			redirectError("server_error", err.Error())
			return
		}
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	if hasSession && !promptHas(prompt, "login") {
		_ = a.Store.UpdateAuthRequest(ctx, req.ID, bson.M{"status": models.RequestAuthenticated, "user_id": b.User.ID})
		c.Redirect(http.StatusFound, "/consent?request="+url.QueryEscape(req.ID))
		return
	}
	c.Redirect(http.StatusFound, "/login?request="+url.QueryEscape(req.ID))
}

func (a *API) token(c *gin.Context) {
	ctx := c.Request.Context()
	t, ok := a.oidcTenant(c)
	if !ok {
		return
	}
	noStore(c)
	if err := c.Request.ParseForm(); err != nil {
		oauthError(c, http.StatusBadRequest, "invalid_request", "Unable to parse form body")
		return
	}
	clientID := c.PostForm("client_id")
	clientSecret := c.PostForm("client_secret")
	if u, p, hasBasic := c.Request.BasicAuth(); hasBasic {
		clientID, clientSecret = u, p
	}
	client, err := a.Svc.AuthenticateClient(ctx, t, clientID, clientSecret)
	if err != nil {
		c.Header("WWW-Authenticate", `Basic realm="oidc"`)
		oauthError(c, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return
	}
	if ok, _ := a.tokenLimiter.Allow(ctx, a.clientIP(c)+"|token|"+client.ClientID); !ok {
		oauthError(c, http.StatusTooManyRequests, "slow_down", "Too many token requests")
		return
	}
	grantType := c.PostForm("grant_type")
	defer func() {
		if a.Metrics != nil {
			a.Metrics.TokenRequest(grantType, c.Writer.Status())
		}
	}()
	switch grantType {
	case "authorization_code":
		a.exchangeCode(c, t, client)
	case "refresh_token":
		raw := c.PostForm("refresh_token")
		if raw == "" {
			oauthError(c, http.StatusBadRequest, "invalid_request", "refresh_token is required")
			return
		}
		resp, err := a.Svc.RefreshTokens(ctx, t, client, raw, c.PostForm("scope"))
		if err != nil {
			oauthError(c, http.StatusBadRequest, "invalid_grant", err.Error())
			return
		}
		c.JSON(http.StatusOK, resp)
	case "client_credentials":
		resp, err := a.Svc.ClientCredentials(ctx, t, client, c.PostForm("scope"))
		if err != nil {
			oauthError(c, http.StatusBadRequest, "invalid_grant", err.Error())
			return
		}
		c.JSON(http.StatusOK, resp)
	default:
		oauthError(c, http.StatusBadRequest, "unsupported_grant_type", "Unsupported grant_type")
	}
}

func (a *API) exchangeCode(c *gin.Context, t *models.Tenant, client *models.Client) {
	ctx := c.Request.Context()
	rawCode := c.PostForm("code")
	if rawCode == "" {
		oauthError(c, http.StatusBadRequest, "invalid_request", "code is required")
		return
	}
	code, err := a.Store.ConsumeAuthCode(ctx, security.HashToken(rawCode))
	if err != nil {
		oauthError(c, http.StatusBadRequest, "invalid_grant", "Authorization code is invalid, expired or already used")
		return
	}
	if code.ClientID != client.ClientID || code.TenantID != t.ID {
		oauthError(c, http.StatusBadRequest, "invalid_grant", "Authorization code was not issued to this client")
		return
	}
	redirectURI := c.PostForm("redirect_uri")
	if redirectURI == "" || redirectURI != code.RedirectURI {
		oauthError(c, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the authorization request")
		return
	}
	if !security.VerifyPKCE(c.PostForm("code_verifier"), code.CodeChallenge, code.CodeChallengeMethod) {
		oauthError(c, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}
	user, err := a.Store.FindUserByID(ctx, code.UserID)
	if err != nil || user.Status != models.StatusActive || (user.Membership(t.ID) == nil && !user.PlatformAdmin) {
		oauthError(c, http.StatusBadRequest, "invalid_grant", "The user is no longer active in this tenant")
		return
	}
	resp, err := a.Svc.IssueTokens(ctx, service.IssueParams{
		Tenant:    t,
		Client:    client,
		User:      user,
		SessionID: code.SessionID,
		Scope:     code.Scope,
		Nonce:     code.Nonce,
		AuthTime:  code.AuthTime,
		MFA:       code.MFA,
	})
	if err != nil {
		oauthError(c, http.StatusInternalServerError, "server_error", "Unable to issue tokens")
		return
	}
	a.Svc.Audit(ctx, t.ID, user, "oidc.token.issued", "client", client.ClientID, a.clientIP(c), c.Request.UserAgent(),
		map[string]interface{}{"scope": code.Scope, "grant": "authorization_code"})
	c.JSON(http.StatusOK, resp)
}

func (a *API) bearerToken(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	if v := c.PostForm("access_token"); v != "" {
		return v
	}
	return c.Query("access_token")
}

func (a *API) userinfo(c *gin.Context) {
	t, ok := a.oidcTenant(c)
	if !ok {
		return
	}
	noStore(c)
	tokenString := a.bearerToken(c)
	if tokenString == "" {
		c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
		oauthError(c, http.StatusUnauthorized, "invalid_token", "A bearer access token is required")
		return
	}
	claims, err := a.Svc.Tokens.Verify(c.Request.Context(), tokenString, t.ID, a.Svc.Issuer(t))
	if err != nil || claims.TokenType != "access" || strings.HasPrefix(claims.Subject, "client:") {
		c.Header("WWW-Authenticate", `Bearer error="invalid_token"`)
		oauthError(c, http.StatusUnauthorized, "invalid_token", "The access token is invalid or expired")
		return
	}
	user, err := a.Store.FindUserByID(c.Request.Context(), mustObjectID(claims.Subject))
	if err != nil || user.Status != models.StatusActive || (user.Membership(t.ID) == nil && !user.PlatformAdmin) {
		oauthError(c, http.StatusUnauthorized, "invalid_token", "The user is no longer active")
		return
	}
	if !tokenVersionValid(claims, user) {
		oauthError(c, http.StatusUnauthorized, "invalid_token", "The access token has been revoked")
		return
	}
	scopes := service.SplitScopes(claims.Scope)
	out := gin.H{
		"sub":        user.ID.Hex(),
		"tenant_id":  t.ID.Hex(),
		"updated_at": user.UpdatedAt.Unix(),
	}
	if hasScope(scopes, "profile") {
		out["name"] = user.Name
		out["preferred_username"] = user.Username
		if user.Picture != "" {
			out["picture"] = user.Picture
		}
	}
	if hasScope(scopes, "email") {
		out["email"] = user.Email
		out["email_verified"] = user.EmailVerified
	}
	if hasScope(scopes, "roles") {
		roles := []string{}
		if m := user.Membership(t.ID); m != nil {
			roles = m.Roles
		}
		out["roles"] = roles
	}
	c.JSON(http.StatusOK, out)
}

func (a *API) revokeToken(c *gin.Context) {
	t, ok := a.oidcTenant(c)
	if !ok {
		return
	}
	noStore(c)
	clientID := c.PostForm("client_id")
	clientSecret := c.PostForm("client_secret")
	if u, p, hasBasic := c.Request.BasicAuth(); hasBasic {
		clientID, clientSecret = u, p
	}
	client, err := a.Svc.AuthenticateClient(c.Request.Context(), t, clientID, clientSecret)
	if err != nil {
		oauthError(c, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return
	}
	raw := c.PostForm("token")
	if raw == "" {
		oauthError(c, http.StatusBadRequest, "invalid_request", "token is required")
		return
	}
	rt, err := a.Store.FindRefreshToken(c.Request.Context(), security.HashToken(raw))
	if err == nil && rt.ClientID == client.ClientID && rt.TenantID == t.ID {
		_ = a.Store.RevokeRefreshToken(c.Request.Context(), rt.ID)
	}
	// Access tokens are self-contained JWTs; revocation returns success even if
	// the token cannot be invalidated before its short expiry.
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *API) introspect(c *gin.Context) {
	t, ok := a.oidcTenant(c)
	if !ok {
		return
	}
	noStore(c)
	clientID := c.PostForm("client_id")
	clientSecret := c.PostForm("client_secret")
	if u, p, hasBasic := c.Request.BasicAuth(); hasBasic {
		clientID, clientSecret = u, p
	}
	client, err := a.Svc.AuthenticateClient(c.Request.Context(), t, clientID, clientSecret)
	if err != nil {
		oauthError(c, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return
	}
	raw := c.PostForm("token")
	if raw == "" {
		oauthError(c, http.StatusBadRequest, "invalid_request", "token is required")
		return
	}
	if claims, err := a.Svc.Tokens.Verify(c.Request.Context(), raw, t.ID, a.Svc.Issuer(t)); err == nil {
		if claims.TokenType == "access" && !strings.HasPrefix(claims.Subject, "client:") {
			if user, userErr := a.Store.FindUserByID(c.Request.Context(), mustObjectID(claims.Subject)); userErr != nil || user.Status != models.StatusActive || !tokenVersionValid(claims, user) {
				c.JSON(http.StatusOK, gin.H{"active": false})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"active":     true,
			"token_type": "Bearer",
			"client_id":  claims.ClientID,
			"sub":        claims.Subject,
			"scope":      claims.Scope,
			"exp":        claims.ExpiresAt.Unix(),
			"iat":        claims.IssuedAt.Unix(),
			"tenant_id":  t.ID.Hex(),
			"email":      claims.Email,
			"roles":      claims.Roles,
		})
		return
	}
	if rt, err := a.Store.FindRefreshToken(c.Request.Context(), security.HashToken(raw)); err == nil &&
		rt.ClientID == client.ClientID && !rt.Revoked && time.Now().Before(rt.ExpiresAt) {
		c.JSON(http.StatusOK, gin.H{
			"active":     true,
			"token_type": "refresh_token",
			"client_id":  rt.ClientID,
			"sub":        rt.UserID.Hex(),
			"scope":      rt.Scope,
			"exp":        rt.ExpiresAt.Unix(),
			"tenant_id":  t.ID.Hex(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"active": false})
}

func (a *API) endSession(c *gin.Context) {
	ctx := c.Request.Context()
	t, ok := a.oidcTenant(c)
	if !ok {
		return
	}
	noStore(c)
	postLogout := c.Query("post_logout_redirect_uri")
	state := c.Query("state")
	clientID := c.Query("client_id")
	idTokenHint := c.Query("id_token_hint")

	if postLogout != "" {
		var client *models.Client
		if clientID != "" {
			client, _ = a.Store.FindClientByClientID(ctx, t.ID, clientID)
		}
		if client == nil && idTokenHint != "" {
			if claims, err := a.Svc.Tokens.Verify(ctx, idTokenHint, t.ID, a.Svc.Issuer(t)); err == nil {
				client, _ = a.Store.FindClientByClientID(ctx, t.ID, claims.ClientID)
			}
		}
		valid := false
		if client != nil {
			for _, u := range client.PostLogoutRedirectURIs {
				if u == postLogout {
					valid = true
					break
				}
			}
		}
		if !valid {
			oauthError(c, http.StatusBadRequest, "invalid_request", "post_logout_redirect_uri is not registered for this client")
			return
		}
	}
	a.Svc.EndSession(ctx, a.sessionCookie(c))
	a.clearSessionCookie(c)
	if postLogout != "" {
		values := url.Values{}
		if state != "" {
			values.Set("state", state)
		}
		c.Redirect(http.StatusFound, service.AppendQuery(postLogout, values))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func mustToken(n int) string {
	v, err := security.RandomToken(n)
	if err != nil {
		return security.HashToken(time.Now().String())
	}
	return v
}

func mustObjectID(hex string) (id primitive.ObjectID) {
	id, _ = primitive.ObjectIDFromHex(hex)
	return id
}

func tokenVersionValid(claims *token.Claims, user *models.User) bool {
	return claims.TokenVersion == user.TokenVersion
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}
