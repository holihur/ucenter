package ratelimit

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter is a fixed-window rate limiter shared by login, token and email flows.
type Limiter interface {
	Allow(ctx context.Context, key string) (bool, error)
	Reset(ctx context.Context, key string) error
}

type Memory struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func NewMemory(max int, window time.Duration) *Memory {
	return &Memory{attempts: map[string][]time.Time{}, max: max, window: window}
}

func (l *Memory) Allow(_ context.Context, key string) (bool, error) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	kept := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		l.attempts[key] = kept
		return false, nil
	}
	l.attempts[key] = append(kept, now)
	if len(l.attempts) > 5000 {
		for k, list := range l.attempts {
			active := false
			for _, t := range list {
				if t.After(cutoff) {
					active = true
					break
				}
			}
			if !active {
				delete(l.attempts, k)
			}
		}
	}
	return true, nil
}

func (l *Memory) Reset(_ context.Context, key string) error {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
	return nil
}

var incrScript = redis.NewScript(`
local c = redis.call('INCR', KEYS[1])
if c == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return c
`)

type Redis struct {
	client   *redis.Client
	max      int
	window   time.Duration
	prefix   string
	fallback *Memory
	logger   *slog.Logger
	lastWarn atomic.Int64
}

func NewRedis(client *redis.Client, max int, window time.Duration, prefix string, logger *slog.Logger) *Redis {
	if logger == nil {
		logger = slog.Default()
	}
	return &Redis{
		client:   client,
		max:      max,
		window:   window,
		prefix:   prefix,
		fallback: NewMemory(max, window),
		logger:   logger,
	}
}

func (l *Redis) Allow(ctx context.Context, key string) (bool, error) {
	opCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	count, err := incrScript.Run(opCtx, l.client, []string{l.prefix + key}, l.window.Milliseconds()).Int64()
	if err != nil {
		now := time.Now().Unix()
		if now-l.lastWarn.Load() > 60 {
			l.lastWarn.Store(now)
			l.logger.Warn("redis_rate_limiter_unavailable_falling_back", "error", err)
		}
		return l.fallback.Allow(ctx, key)
	}
	return count <= int64(l.max), nil
}

func (l *Redis) Reset(ctx context.Context, key string) error {
	_ = l.fallback.Reset(ctx, key)
	opCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	return l.client.Del(opCtx, l.prefix+key).Err()
}

// Connect creates a Redis client from a URL and pings it. A failed connection is
// returned so the caller can fall back to the in-memory limiter.
func Connect(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}
