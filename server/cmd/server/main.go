package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"ucenter/internal/api"
	"ucenter/internal/config"
	"ucenter/internal/models"
	"ucenter/internal/observability"
	"ucenter/internal/security"
	"ucenter/internal/service"
	"ucenter/internal/store"
	"ucenter/internal/token"
)

func main() {
	cfg := config.Load()
	logger := observability.NewLogger(cfg)
	slog.SetDefault(logger)
	if cfg.Dev {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	startCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := store.New(startCtx, cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		log.Fatalf("mongo connection failed: %v", err)
	}
	defer st.Close(context.Background())
	if err := st.EnsureIndexes(startCtx); err != nil {
		log.Fatalf("mongo index setup failed: %v", err)
	}

	cipher, err := security.NewCipher(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("encryption key: %v", err)
	}
	if cfg.EncryptionKey == "" {
		log.Printf("WARNING: ENCRYPTION_KEY is not set; using an insecure development key")
	}
	metrics := observability.NewMetrics("dev")
	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	defer monitorCancel()
	go metrics.StartDBMonitor(monitorCtx, st.Client, 15*time.Second)
	svc := service.New(st, token.NewManager(st, cipher), cfg)
	if err := bootstrapAdmin(startCtx, st, svc, cfg); err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}

	handler := api.New(st, svc, cfg, cipher, metrics).Router()
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("UCenter listening on %s (issuer base %s)", cfg.Addr, cfg.BaseURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

func bootstrapAdmin(ctx context.Context, st *store.Store, svc *service.Service, cfg config.Config) error {
	if cfg.BootstrapEmail == "" || cfg.BootstrapPassword == "" {
		return nil
	}
	email := security.NormalizeEmail(cfg.BootstrapEmail)
	user, err := st.FindUserByEmail(ctx, email)
	if err == store.ErrNotFound {
		hash, hashErr := security.HashPassword(cfg.BootstrapPassword)
		if hashErr != nil {
			return hashErr
		}
		user = &models.User{
			Email:         email,
			EmailVerified: true,
			Username:      "platform-admin",
			Name:          "Platform Administrator",
			PasswordHash:  hash,
			Status:        models.StatusActive,
			PlatformAdmin: true,
		}
		if err := st.CreateUser(ctx, user); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				return nil
			}
			return err
		}
		log.Printf("bootstrap platform admin created: %s", email)
	} else if err != nil {
		return err
	}

	if cfg.BootstrapTenantSlug == "" {
		return nil
	}
	tenant, err := st.FindTenantBySlug(ctx, cfg.BootstrapTenantSlug)
	if err == store.ErrNotFound {
		name := cfg.BootstrapTenantName
		if name == "" {
			name = cfg.BootstrapTenantSlug
		}
		tenant = &models.Tenant{
			Slug:     cfg.BootstrapTenantSlug,
			Name:     name,
			Status:   models.StatusActive,
			Settings: models.DefaultSettings(),
		}
		if err := st.CreateTenant(ctx, tenant); err != nil {
			return err
		}
		log.Printf("bootstrap tenant created: %s", tenant.Slug)
	} else if err != nil {
		return err
	}
	if err := svc.ProvisionTenant(ctx, tenant); err != nil {
		return err
	}
	if user.Membership(tenant.ID) == nil {
		if err := st.SetMembership(ctx, user.ID, tenant.ID, []string{"owner"}); err != nil {
			return err
		}
	}
	return nil
}
