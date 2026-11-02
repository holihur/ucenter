package observability

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Metrics holds the Prometheus collectors exposed at /metrics.
type Metrics struct {
	Registry       *prometheus.Registry
	HTTPRequests   *prometheus.CounterVec
	HTTPDuration   *prometheus.HistogramVec
	TokenRequests  *prometheus.CounterVec
	Logins         *prometheus.CounterVec
	DBUp           prometheus.Gauge
	DBPingDuration prometheus.Gauge
	BuildInfo      *prometheus.GaugeVec
}

func NewMetrics(version string) *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		Registry: registry,
		HTTPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ucenter", Subsystem: "http", Name: "requests_total",
			Help: "HTTP requests by method, route and status.",
		}, []string{"method", "route", "status"}),
		HTTPDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "ucenter", Subsystem: "http", Name: "request_duration_seconds",
			Help: "HTTP request latency.", Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		TokenRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ucenter", Subsystem: "oidc", Name: "token_requests_total",
			Help: "OIDC token endpoint requests by grant type and status.",
		}, []string{"grant_type", "status"}),
		Logins: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ucenter", Subsystem: "auth", Name: "logins_total",
			Help: "Login attempts by result.",
		}, []string{"result"}),
		DBUp: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "ucenter", Subsystem: "db", Name: "up", Help: "MongoDB reachability (1/0).",
		}),
		DBPingDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "ucenter", Subsystem: "db", Name: "ping_duration_seconds", Help: "Last MongoDB ping duration.",
		}),
		BuildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "ucenter", Name: "build_info", Help: "Build information.",
		}, []string{"version"}),
	}
	registry.MustRegister(m.HTTPRequests, m.HTTPDuration, m.TokenRequests, m.Logins, m.DBUp, m.DBPingDuration, m.BuildInfo)
	m.BuildInfo.WithLabelValues(version).Set(1)
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())
		m.HTTPRequests.WithLabelValues(c.Request.Method, route, status).Inc()
		m.HTTPDuration.WithLabelValues(c.Request.Method, route).Observe(time.Since(start).Seconds())
	}
}

func (m *Metrics) TokenRequest(grantType string, status int) {
	if grantType == "" {
		grantType = "unknown"
	}
	m.TokenRequests.WithLabelValues(grantType, strconv.Itoa(status)).Inc()
}

func (m *Metrics) Login(result string) {
	m.Logins.WithLabelValues(result).Inc()
}

// StartDBMonitor periodically pings MongoDB and updates DB gauges.
func (m *Metrics) StartDBMonitor(ctx context.Context, client *mongo.Client, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	check := func() {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		start := time.Now()
		err := client.Ping(pingCtx, readpref.Primary())
		m.DBPingDuration.Set(time.Since(start).Seconds())
		if err != nil {
			m.DBUp.Set(0)
			return
		}
		m.DBUp.Set(1)
	}
	check()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
