package ratelimit

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestMemoryLimiter(t *testing.T) {
	ctx := context.Background()
	l := NewMemory(2, time.Minute)
	if ok, _ := l.Allow(ctx, "a"); !ok {
		t.Fatal("first attempt should be allowed")
	}
	if ok, _ := l.Allow(ctx, "a"); !ok {
		t.Fatal("second attempt should be allowed")
	}
	if ok, _ := l.Allow(ctx, "a"); ok {
		t.Fatal("third attempt should be rejected")
	}
	if ok, _ := l.Allow(ctx, "b"); !ok {
		t.Fatal("independent key should be allowed")
	}
	_ = l.Reset(ctx, "a")
	if ok, _ := l.Allow(ctx, "a"); !ok {
		t.Fatal("reset should clear the key")
	}
}

func TestRedisLimiterSharedAcrossInstances(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	l1 := NewRedis(client, 2, time.Minute, "test:", slog.Default())
	l2 := NewRedis(client, 2, time.Minute, "test:", slog.Default())
	if ok, err := l1.Allow(ctx, "shared"); !ok || err != nil {
		t.Fatalf("first attempt: %v %v", ok, err)
	}
	if ok, err := l2.Allow(ctx, "shared"); !ok || err != nil {
		t.Fatalf("second attempt from another instance: %v %v", ok, err)
	}
	if ok, _ := l1.Allow(ctx, "shared"); ok {
		t.Fatal("shared counter should reject the third attempt")
	}
	if err := l1.Reset(ctx, "shared"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := l2.Allow(ctx, "shared"); !ok {
		t.Fatal("reset should clear the shared key")
	}
}

func TestRedisLimiterFallsBackWhenUnavailable(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	ctx := context.Background()
	l := NewRedis(client, 1, time.Minute, "test:", slog.Default())
	if ok, _ := l.Allow(ctx, "fallback"); !ok {
		t.Fatal("first attempt should be allowed")
	}
	srv.Close()
	// The Redis call fails; the in-memory fallback still enforces the limit.
	if ok, _ := l.Allow(ctx, "fallback"); ok {
		t.Fatal("fallback should reject the second attempt")
	}
	_ = client.Close()
}
