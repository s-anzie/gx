package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

// Config holds cache configuration
type Config struct {
	// DefaultTTL is the default time-to-live for cached responses
	DefaultTTL time.Duration

	// KeyFunc generates the cache key for a request
	KeyFunc func(*gx.Context) string

	// Backend is the storage backend (default: in-memory)
	Backend Backend

	// OnHit is called when a cache hit occurs (optional)
	OnHit func(*gx.Context, string)

	// OnMiss is called when a cache miss occurs (optional)
	OnMiss func(*gx.Context, string)
}

// Backend is the interface for cache storage
type Backend interface {
	// Get retrieves a value from the cache
	Get(key string) ([]byte, bool, error)

	// Set stores a value in the cache with the given TTL
	Set(key string, value []byte, ttl time.Duration) error

	// Delete removes a value from the cache
	Delete(key string) error

	// Clear removes all values from the cache
	Clear() error
}

// cachePlugin implements the cache plugin
type cachePlugin struct {
	config Config
}

// New creates a new cache plugin
func New(options ...Option) gx.Plugin {
	config := Config{
		DefaultTTL: 5 * time.Minute,
		KeyFunc:    defaultKeyFunc,
		Backend:    newMemoryBackend(),
		OnHit:      nil,
		OnMiss:     nil,
	}

	// Apply options
	for _, opt := range options {
		opt(&config)
	}

	return &cachePlugin{config: config}
}

// Name returns the plugin name
func (p *cachePlugin) Name() string {
	return "cache"
}

// OnBoot is called when the application boots
func (p *cachePlugin) OnBoot(app *gx.App) error {
	return nil
}

// OnRequest processes each request for caching
func (p *cachePlugin) OnRequest(c *gx.Context, next core.Handler) core.Response {
	// Only cache GET requests
	if c.Method() != "GET" {
		return next(c.Context)
	}

	// Generate cache key
	key := p.config.KeyFunc(c)

	// Try to get from cache
	if cached, found, err := p.config.Backend.Get(key); err == nil && found {
		if p.config.OnHit != nil {
			p.config.OnHit(c, key)
		}

		// Return cached response
		return &cachedResponse{
			data:        cached,
			cacheStatus: "HIT",
		}
	}

	// Cache miss - call next handler
	if p.config.OnMiss != nil {
		p.config.OnMiss(c, key)
	}

	// Wrap the writer to capture response
	cw := &cacheWriter{
		ResponseWriter: c.Writer,
		plugin:         p,
		key:            key,
	}
	c.Writer = cw

	// Execute the handler
	response := next(c.Context)

	// Mark as cache miss
	c.Writer.Header().Set("X-Cache", "MISS")

	return response
}

// OnShutdown is called when the application shuts down
func (p *cachePlugin) OnShutdown(ctx context.Context) error {
	return nil
}

// ── Options ──────────────────────────────────────────────────────────────────

// Option is a functional option for cache configuration
type Option func(*Config)

// DefaultTTL sets the default cache TTL
func DefaultTTL(ttl time.Duration) Option {
	return func(c *Config) {
		c.DefaultTTL = ttl
	}
}

// WithBackend sets the cache backend
func WithBackend(backend Backend) Option {
	return func(c *Config) {
		c.Backend = backend
	}
}

// KeyFunc sets the cache key generation function
func KeyFunc(fn func(*gx.Context) string) Option {
	return func(c *Config) {
		c.KeyFunc = fn
	}
}

// OnHit sets the cache hit callback
func OnHit(fn func(*gx.Context, string)) Option {
	return func(c *Config) {
		c.OnHit = fn
	}
}

// OnMiss sets the cache miss callback
func OnMiss(fn func(*gx.Context, string)) Option {
	return func(c *Config) {
		c.OnMiss = fn
	}
}

// ── In-Memory Backend ────────────────────────────────────────────────────────

type memoryBackend struct {
	mu    sync.RWMutex
	items map[string]*cacheItem
}

type cacheItem struct {
	value  []byte
	expiry time.Time
}

func newMemoryBackend() *memoryBackend {
	mb := &memoryBackend{
		items: make(map[string]*cacheItem),
	}

	// Start cleanup goroutine
	go mb.cleanup()

	return mb
}

func (mb *memoryBackend) Get(key string) ([]byte, bool, error) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	item, exists := mb.items[key]
	if !exists {
		return nil, false, nil
	}

	// Check if expired
	if time.Now().After(item.expiry) {
		return nil, false, nil
	}

	return item.value, true, nil
}

func (mb *memoryBackend) Set(key string, value []byte, ttl time.Duration) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	mb.items[key] = &cacheItem{
		value:  value,
		expiry: time.Now().Add(ttl),
	}

	return nil
}

func (mb *memoryBackend) Delete(key string) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	delete(mb.items, key)
	return nil
}

func (mb *memoryBackend) Clear() error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	mb.items = make(map[string]*cacheItem)
	return nil
}

// cleanup periodically removes expired items
func (mb *memoryBackend) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		mb.mu.Lock()
		now := time.Now()
		for key, item := range mb.items {
			if now.After(item.expiry) {
				delete(mb.items, key)
			}
		}
		mb.mu.Unlock()
	}
}

// ── Helper Functions ─────────────────────────────────────────────────────────

// defaultKeyFunc generates a cache key from method, path, and query params
func defaultKeyFunc(c *gx.Context) string {
	// Create a deterministic key from request
	data := fmt.Sprintf("%s:%s:%s", c.Method(), c.Path(), c.Request.URL.RawQuery)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// ── Cache Response and Writer  ───────────────────────────────────────────────

// cachedResponse represents a cached HTTP response
type cachedResponse struct {
	data        []byte
	cacheStatus string
}

func (r *cachedResponse) Status() int {
	return 200
}

func (r *cachedResponse) Headers() http.Header {
	h := make(http.Header)
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("X-Cache", r.cacheStatus)
	return h
}

func (r *cachedResponse) Write(w http.ResponseWriter) error {
	// Write headers
	for k, v := range r.Headers() {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	w.WriteHeader(r.Status())

	if r.data != nil {
		_, err := w.Write(r.data)
		return err
	}

	return nil
}

// cacheWriter wraps http.ResponseWriter to capture responses for caching
type cacheWriter struct {
	http.ResponseWriter
	plugin     *cachePlugin
	key        string
	buffer     []byte
	statusCode int
}

func (w *cacheWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *cacheWriter) Write(data []byte) (int, error) {
	// Capture the response data
	w.buffer = append(w.buffer, data...)

	// Write through to the underlying writer
	return w.ResponseWriter.Write(data)
}

// Close implements io.Closer to save the cached response
func (w *cacheWriter) Close() error {
	// Only cache successful responses (2xx status codes)
	if w.statusCode == 0 {
		w.statusCode = 200 // Default status
	}

	if w.statusCode >= 200 && w.statusCode < 300 && len(w.buffer) > 0 {
		// Cache the response
		w.plugin.config.Backend.Set(w.key, w.buffer, w.plugin.config.DefaultTTL)
	}

	// Call Close on underlying writer if it implements io.Closer
	if closer, ok := w.ResponseWriter.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}
