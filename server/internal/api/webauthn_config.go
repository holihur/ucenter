package api

import (
	"net/url"
	"strings"

	"github.com/go-webauthn/webauthn/webauthn"
	"ucenter/internal/config"
)

func newWebAuthn(cfg config.Config) (*webauthn.WebAuthn, error) {
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	rpID := strings.TrimSpace(cfg.WebAuthnRPID)
	if rpID == "" {
		rpID = base.Hostname()
	}
	origins := append([]string{}, cfg.WebAuthnOrigins...)
	if len(origins) == 0 {
		origins = []string{cfg.BaseURL}
	}
	if cfg.Dev {
		origins = append(origins, "http://localhost:5173", "http://127.0.0.1:5173")
	}
	return webauthn.New(&webauthn.Config{
		RPID: rpID, RPDisplayName: "UCenter", RPOrigins: origins,
	})
}
