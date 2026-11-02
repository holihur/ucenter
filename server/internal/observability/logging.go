package observability

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"ucenter/internal/config"
	"ucenter/internal/security"
)

func NewLogger(cfg config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	format := strings.ToLower(cfg.LogFormat)
	if format == "" {
		if cfg.Dev {
			format = "text"
		} else {
			format = "json"
		}
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

// RequestLogger emits one structured line per request. Query strings are not
// logged so tokens and secrets never end up in logs.
func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" || c.Request.URL.Path == "/healthz" || c.Request.URL.Path == "/readyz" {
			c.Next()
			return
		}
		start := time.Now()
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID, _ = security.RandomHex(8)
			c.Header("X-Request-ID", requestID)
		}
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		attrs := []any{
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"route", route,
			"status", c.Writer.Status(),
			"duration_ms", float64(time.Since(start).Microseconds()) / 1000.0,
			"ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		}
		if c.Writer.Status() >= 500 {
			logger.Error("http_request", attrs...)
		} else {
			logger.Info("http_request", attrs...)
		}
	}
}
