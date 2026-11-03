package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"ucenter/internal/billing"
	"ucenter/internal/config"
	"ucenter/internal/models"
	"ucenter/internal/security"
	"ucenter/internal/service"
	"ucenter/internal/store"
	"ucenter/internal/token"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	st, err := store.New(ctx, cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		log.Fatalf("mongo: %v", err)
	}
	defer st.Close(context.Background())
	if err := st.EnsureIndexes(ctx); err != nil {
		log.Fatalf("indexes: %v", err)
	}
	cipher, err := security.NewCipher(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("encryption key: %v", err)
	}
	svc := service.New(st, token.NewManager(st, cipher), cfg)

	acme := ensureTenant(ctx, st, svc, models.Tenant{
		Slug: "acme",
		Name: "Acme Corporation",
		Branding: models.Branding{
			PrimaryColor:    "#4f46e5",
			BackgroundColor: "#f8fafc",
			SupportEmail:    "support@acme.test",
			LoginTemplate:   "aurora",
			LoginTagline:    "One identity for the whole Acme ecosystem.",
		},
		Subscription: billing.DefaultSubscription(),
		Settings: func() models.TenantSettings {
			s := models.DefaultSettings()
			s.AllowRegistration = true
			s.PasswordMinLength = 10
			s.AllowedEmailDomains = []string{"acme.test"}
			return s
		}(),
	})
	globex := ensureTenant(ctx, st, svc, models.Tenant{
		Slug: "globex",
		Name: "Globex Industries",
		Branding: models.Branding{
			PrimaryColor:    "#0f766e",
			BackgroundColor: "#f0fdfa",
			SupportEmail:    "it@globex.test",
			LoginTemplate:   "fintech",
			LoginTagline:    "Secure access to the Globex platform.",
		},
		Subscription: billing.DefaultSubscription(),
		Settings:     models.DefaultSettings(),
	})

	users := []struct {
		Tenant   *models.Tenant
		Email    string
		Name     string
		Roles    []string
		Platform bool
	}{
		{acme, "owner@acme.test", "Olivia Owner", []string{"owner"}, false},
		{acme, "admin@acme.test", "Adam Admin", []string{"admin"}, false},
		{acme, "member@acme.test", "Mia Member", []string{"member"}, false},
		{globex, "owner@globex.test", "Grace Globex", []string{"owner"}, false},
	}
	for _, u := range users {
		if err := ensureUser(ctx, st, u.Tenant.ID, u.Email, u.Name, "Password123!", u.Roles, false); err != nil {
			log.Fatalf("seed user %s: %v", u.Email, err)
		}
	}
	if err := ensureUser(ctx, st, acme.ID, "admin@ucenter.local", "Platform Administrator", "Admin12345!", nil, true); err != nil {
		log.Fatalf("seed platform admin: %v", err)
	}

	ensureRole(ctx, st, acme.ID, models.Role{
		Key:         "auditor",
		Name:        "Auditor",
		Description: "Read-only access to security events and tenant configuration",
		Permissions: []string{"audit:read", "tenant:read", "users:read"},
	})

	ensureClient(ctx, st, cipher, acme.ID, models.Client{
		ClientID:               "app_demo_acme",
		Name:                   "Acme Dashboard",
		Description:            "Demo confidential web application using the authorization code flow",
		Type:                   models.ClientTypeConfidential,
		RedirectURIs:           []string{"http://localhost:3000/callback", "http://localhost:5173/callback"},
		PostLogoutRedirectURIs: []string{"http://localhost:3000", "http://localhost:5173"},
		AllowedScopes:          []string{"openid", "profile", "email", "offline_access", "roles"},
		GrantTypes:             []string{"authorization_code", "refresh_token"},
		RequirePKCE:            true,
		Trusted:                false,
		AccessTokenTTL:         3600,
		IDTokenTTL:             3600,
		RefreshTokenTTL:        2592000,
		Enabled:                true,
		LogoURL:                "",
	}, "demo-secret-acme")

	ensureClient(ctx, st, cipher, acme.ID, models.Client{
		ClientID:        "app_demo_cli",
		Name:            "Acme CLI",
		Description:     "Demo public client using PKCE",
		Type:            models.ClientTypePublic,
		RedirectURIs:    []string{"http://127.0.0.1:8080/callback"},
		AllowedScopes:   []string{"openid", "profile", "email", "offline_access", "roles"},
		GrantTypes:      []string{"authorization_code", "refresh_token"},
		RequirePKCE:     true,
		AccessTokenTTL:  3600,
		IDTokenTTL:      3600,
		RefreshTokenTTL: 2592000,
		Enabled:         true,
	}, "")

	ensureClient(ctx, st, cipher, globex.ID, models.Client{
		ClientID:               "app_demo_globex",
		Name:                   "Globex Portal",
		Description:            "Second tenant demo application",
		Type:                   models.ClientTypeConfidential,
		RedirectURIs:           []string{"http://localhost:3000/callback"},
		PostLogoutRedirectURIs: []string{"http://localhost:3000"},
		AllowedScopes:          []string{"openid", "profile", "email", "offline_access", "roles"},
		GrantTypes:             []string{"authorization_code", "refresh_token"},
		RequirePKCE:            true,
		AccessTokenTTL:         3600,
		IDTokenTTL:             3600,
		RefreshTokenTTL:        2592000,
		Enabled:                true,
	}, "demo-secret-globex")

	fmt.Println()
	fmt.Println("──────────────────────────────────────────────────────────────")
	fmt.Println(" UCenter demo data ready")
	fmt.Println("──────────────────────────────────────────────────────────────")
	fmt.Printf(" Platform admin : admin@ucenter.local / Admin12345!\n")
	fmt.Printf(" Acme owner     : owner@acme.test / Password123!\n")
	fmt.Printf(" Acme admin     : admin@acme.test / Password123!\n")
	fmt.Printf(" Acme member    : member@acme.test / Password123!\n")
	fmt.Printf(" Globex owner   : owner@globex.test / Password123!\n\n")
	fmt.Printf(" OIDC issuer    : %s/t/acme\n", cfg.BaseURL)
	fmt.Printf(" Discovery      : %s/t/acme/.well-known/openid-configuration\n", cfg.BaseURL)
	fmt.Printf(" Demo client    : app_demo_acme / demo-secret-acme\n")
	fmt.Printf(" Redirect URIs  : http://localhost:3000/callback, http://localhost:5173/callback\n")
	fmt.Printf(" Globex client  : app_demo_globex / demo-secret-globex\n")
	fmt.Println("──────────────────────────────────────────────────────────────")
}

func ensureTenant(ctx context.Context, st *store.Store, svc *service.Service, t models.Tenant) *models.Tenant {
	existing, err := st.FindTenantBySlug(ctx, t.Slug)
	if err == nil {
		_ = svc.ProvisionTenant(ctx, existing)
		return existing
	}
	if err := st.CreateTenant(ctx, &t); err != nil {
		log.Fatalf("create tenant %s: %v", t.Slug, err)
	}
	if err := svc.ProvisionTenant(ctx, &t); err != nil {
		log.Fatalf("provision tenant %s: %v", t.Slug, err)
	}
	log.Printf("created tenant %s", t.Slug)
	return &t
}

func ensureUser(ctx context.Context, st *store.Store, tenantID primitive.ObjectID, email, name, password string, roles []string, platform bool) error {
	email = security.NormalizeEmail(email)
	existing, err := st.FindUserByEmail(ctx, email)
	if err == nil {
		if platform && !existing.PlatformAdmin {
			_ = st.UpdateUser(ctx, existing.ID, bson.M{"platform_admin": true})
		}
		if len(roles) > 0 && existing.Membership(tenantID) == nil {
			return st.SetMembership(ctx, existing.ID, tenantID, roles)
		}
		return nil
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	user := &models.User{
		Email:         email,
		EmailVerified: true,
		Username:      strings.Split(email, "@")[0],
		Name:          name,
		PasswordHash:  hash,
		Status:        models.StatusActive,
		PlatformAdmin: platform,
	}
	if len(roles) > 0 {
		user.Memberships = []models.Membership{{TenantID: tenantID, Roles: roles, JoinedAt: time.Now()}}
	}
	if err := st.CreateUser(ctx, user); err != nil {
		return err
	}
	log.Printf("created user %s", email)
	return nil
}

func ensureRole(ctx context.Context, st *store.Store, tenantID primitive.ObjectID, role models.Role) {
	if _, err := st.FindRoleByKey(ctx, tenantID, role.Key); err == nil {
		return
	}
	role.TenantID = tenantID
	if err := st.CreateRole(ctx, &role); err != nil {
		log.Printf("role %s: %v", role.Key, err)
	}
}

func ensureClient(ctx context.Context, st *store.Store, cipher *security.Cipher, tenantID primitive.ObjectID, client models.Client, secret string) {
	if _, err := st.FindClientByClientID(ctx, tenantID, client.ClientID); err == nil {
		return
	}
	client.TenantID = tenantID
	if client.Type == models.ClientTypeConfidential && secret != "" {
		client.ClientSecretHash = security.HashClientSecret(secret)
		if encrypted, err := cipher.Encrypt(secret); err == nil {
			client.ClientSecretEnc = encrypted
		}
	}
	if err := st.CreateClient(ctx, &client); err != nil {
		log.Printf("client %s: %v", client.ClientID, err)
	}
}
