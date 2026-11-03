package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr                string
	MongoURI            string
	MongoDB             string
	BaseURL             string
	WebDir              string
	CookieSecure        bool
	CookieName          string
	BootstrapEmail      string
	BootstrapPassword   string
	BootstrapTenantSlug string
	BootstrapTenantName string
	Dev                 bool
	AllowedOrigins      []string
	TrustedProxies      []string
	MailDriver          string
	SMTPHost            string
	SMTPPort            string
	SMTPUser            string
	SMTPPass            string
	SMTPFrom            string
	SMTPFromName        string
	SMTPTLS             bool
	EncryptionKey       string
	MetricsToken        string
	LogFormat           string
	LogLevel            string
	RedisURL            string
	RedisPrefix         string
	WebAuthnRPID        string
	WebAuthnOrigins     []string
}

func Load() Config {
	c := Config{
		Addr:                env("ADDR", ":8080"),
		MongoURI:            env("MONGO_URI", "mongodb://127.0.0.1:27017"),
		MongoDB:             env("MONGO_DB", "ucenter"),
		BaseURL:             strings.TrimRight(env("BASE_URL", "http://localhost:8080"), "/"),
		WebDir:              env("WEB_DIR", "../web/dist"),
		CookieSecure:        envBool("COOKIE_SECURE", false),
		CookieName:          env("COOKIE_NAME", "uc_session"),
		BootstrapEmail:      env("BOOTSTRAP_ADMIN_EMAIL", ""),
		BootstrapPassword:   env("BOOTSTRAP_ADMIN_PASSWORD", ""),
		BootstrapTenantSlug: env("BOOTSTRAP_TENANT_SLUG", ""),
		BootstrapTenantName: env("BOOTSTRAP_TENANT_NAME", ""),
		Dev:                 envBool("DEV", false),
		MailDriver:          env("MAIL_DRIVER", "mock"),
		SMTPHost:            env("SMTP_HOST", ""),
		SMTPPort:            env("SMTP_PORT", "587"),
		SMTPUser:            env("SMTP_USER", ""),
		SMTPPass:            env("SMTP_PASS", ""),
		SMTPFrom:            env("SMTP_FROM", "no-reply@ucenter.local"),
		SMTPFromName:        env("SMTP_FROM_NAME", "UCenter"),
		SMTPTLS:             envBool("SMTP_TLS", false),
		EncryptionKey:       env("ENCRYPTION_KEY", ""),
		MetricsToken:        env("METRICS_TOKEN", ""),
		LogFormat:           env("LOG_FORMAT", ""),
		LogLevel:            env("LOG_LEVEL", "info"),
		RedisURL:            env("REDIS_URL", ""),
		RedisPrefix:         env("REDIS_PREFIX", "ucenter:rl:"),
		WebAuthnRPID:        env("WEBAUTHN_RP_ID", ""),
	}
	if origins := env("ALLOWED_ORIGINS", ""); origins != "" {
		c.AllowedOrigins = strings.Split(origins, ",")
	}
	if proxies := env("TRUSTED_PROXIES", ""); proxies != "" {
		c.TrustedProxies = strings.Split(proxies, ",")
	}
	if origins := env("WEBAUTHN_ORIGINS", ""); origins != "" {
		c.WebAuthnOrigins = strings.Split(origins, ",")
	}
	return c
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
