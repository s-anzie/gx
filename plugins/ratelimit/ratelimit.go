package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

// Config holds rate limiting configuration
type Config struct {
	// Limit is the maximum number of requests allowed
	Limit int

	// Window is the time window for the rate limit
	Window time.Duration

	// KeyFunc extracts the rate limit key from the request (default: IP address)
	KeyFunc func(*gx.Context) string

	// OnExceeded is called when the rate limit is exceeded
	OnExceeded func(*gx.Context) core.Response

	// Backend is the storage backend (default: in-memory)
	Backend Backend
}

// Backend is the interface for rate limit storage
type Backend interface {
	// Increment increments the counter for the given key
	// Returns the new count and whether the limit was exceeded
	Increment(key string, window time.Duration, limit int) (int, bool, error)

	// Reset resets the counter for the given key
	Reset(key string) error
}

// rateLimitPlugin implements the rate limiting plugin
type rateLimitPlugin struct {
	config Config
}

// New creates a new rate limiting plugin
func New(options ...Option) gx.Plugin {
	config := Config{
		Limit:  100,
		Window: time.Minute,
		KeyFunc: func(c *gx.Context) string {
			return c.ClientIP()
		},
		OnExceeded: func(c *gx.Context) core.Response {
			return c.Fail(gx.ErrTooManyRequests)
		},
		Backend: newMemoryBackend(),
	}

	// Apply options
	for _, opt := range options {
		opt(&config)
	}

	return &rateLimitPlugin{config: config}
}

// Name returns the plugin name
func (p *rateLimitPlugin) Name() string {
	return "ratelimit"
}

// OnBoot is called when the application boots
func (p *rateLimitPlugin) OnBoot(app *gx.App) error {
	return nil
}

// OnRequest processes each request for rate limiting
func (p *rateLimitPlugin) OnRequest(c *gx.Context, next core.Handler) core.Response {
	// Extract the rate limit key
	key := p.config.KeyFunc(c)

	// Increment the counter
	count, exceeded, err := p.config.Backend.Increment(key, p.config.Window, p.config.Limit)
	if err != nil {
		// Log error but don't block the request
		c.Log().Error("rate limit backend error", "error", err)
		return next(c.Context)
	}

	// Add rate limit headers
	remaining := p.config.Limit - count
	if remaining < 0 {
		remaining = 0
	}
	c.Writer.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", p.config.Limit))
	c.Writer.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

	// Check if limit exceeded
	if exceeded {
		c.Writer.Header().Set("Retry-After", fmt.Sprintf("%d", int(p.config.Window.Seconds())))
		return p.config.OnExceeded(c)
	}

	return next(c.Context)
}

// OnShutdown is called when the application shuts down
func (p *rateLimitPlugin) OnShutdown(ctx context.Context) error {
	return nil
}

// Option is a functional option for rate limit configuration
type Option func(*Config)

// PerIP configures rate limiting per IP address
func PerIP(limit int, window time.Duration) Option {
	return func(c *Config) {
		c.Limit = limit
		c.Window = window
		c.KeyFunc = func(ctx *gx.Context) string {
			return ctx.ClientIP()
		}
	}
}

// PerUser configures rate limiting per user (requires authentication)
func PerUser(limit int, window time.Duration) Option {
	return func(c *Config) {
		c.Limit = limit
		c.Window = window
		c.KeyFunc = func(ctx *gx.Context) string {
			// Try to get user ID from typed context
			if userID, ok := gx.TryTyped[string](ctx.Context); ok {
				return "user:" + *userID
			}
			// Fallback to IP
			return ctx.ClientIP()
		}
	}
}

// WithBackend sets a custom storage backend
func WithBackend(backend Backend) Option {
	return func(c *Config) {
		c.Backend = backend
	}
}

// OnExceeded sets the callback for when rate limit is exceeded
func OnExceeded(fn func(*gx.Context) core.Response) Option {
	return func(c *Config) {
		c.OnExceeded = fn
	}
}

// ── In-Memory Backend ────────────────────────────────────────────────────────

type memoryBackend struct {
	mu      sync.RWMutex
	buckets map[string]*bucket
}

type bucket struct {
	count  int
	expiry time.Time
}

func newMemoryBackend() *memoryBackend {
	mb := &memoryBackend{
		buckets: make(map[string]*bucket),
	}

	// Start cleanup goroutine
	go mb.cleanup()

	return mb
}

func (mb *memoryBackend) Increment(key string, window time.Duration, limit int) (int, bool, error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	now := time.Now()
	b, exists := mb.buckets[key]

	// Check if bucket expired or doesn't exist
	if !exists || b.expiry.Before(now) {
		mb.buckets[key] = &bucket{
			count:  1,
			expiry: now.Add(window),
		}
		return 1, false, nil
	}

	// Increment existing bucket
	b.count++
	exceeded := b.count > limit
	return b.count, exceeded, nil
}

func (mb *memoryBackend) Reset(key string) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	delete(mb.buckets, key)
	return nil
}

// cleanup periodically removes expired buckets
func (mb *memoryBackend) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		mb.mu.Lock()
		now := time.Now()
		for key, b := range mb.buckets {
			if b.expiry.Before(now) {
				delete(mb.buckets, key)
			}
		}
		mb.mu.Unlock()
	}
}
