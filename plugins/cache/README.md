# Cache Plugin

HTTP response caching middleware for GX framework with configurable TTL and storage backends.

## Features

✅ **Smart Caching** - Automatically caches GET requests only  
✅ **Flexible Storage** - In-memory backend included, Redis/external backends supported  
✅ **TTL Control** - Configurable time-to-live per cache entry  
✅ **Custom Keys** - Customizable cache key generation  
✅ **Cache Events** - OnHit/OnMiss callbacks for monitoring  
✅ **Cache Headers** - X-Cache header indicates HIT or MISS  
✅ **Thread-Safe** - Concurrent access handled safely  

## Installation

```go
import "github.com/s-anzie/gx/plugins/cache"
```

## Basic Usage

```go
app := gx.New()

// Install with default in-memory backend
app.Install(cache.New(
    cache.DefaultTTL(5 * time.Minute),
))

app.GET("/api/users", func(c *core.Context) core.Response {
    // This response will be cached for 5 minutes
    return c.JSON(users)
})
```

## Configuration Options

### Default TTL

```go
cache.DefaultTTL(10 * time.Minute)
```

### Custom Cache Key

```go
cache.KeyFunc(func(c *gx.Context) string {
    // Include user ID in cache key for per-user caching
    userID := c.Get("user_id").(string)
    return fmt.Sprintf("%s:%s:%s", userID, c.Method(), c.Path())
})
```

### Cache Event Callbacks

```go
cache.OnHit(func(c *gx.Context, key string) {
    log.Printf("Cache HIT: %s", c.Path())
}),

cache.OnMiss(func(c *gx.Context, key string) {
    log.Printf("Cache MISS: %s", c.Path())
}),
```

### Custom Backend

```go
// Implement the Backend interface
type redisBackend struct {
    client *redis.Client
}

func (r *redisBackend) Get(key string) ([]byte, bool, error) {
    val, err := r.client.Get(ctx, key).Bytes()
    if err == redis.Nil {
        return nil, false, nil
    }
    return val, err == nil, err
}

func (r *redisBackend) Set(key string, value []byte, ttl time.Duration) error {
    return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *redisBackend) Delete(key string) error {
    return r.client.Del(ctx, key).Err()
}

func (r *redisBackend) Clear() error {
    return r.client.FlushDB(ctx).Err()
}

// Use it
app.Install(cache.New(
    cache.WithBackend(&redisBackend{client: redisClient}),
    cache.DefaultTTL(5 * time.Minute),
))
```

## How It Works

1. **GET Requests Only** - Only GET requests are cached (POST, PUT, DELETE are never cached)
2. **Cache Key Generation** - SHA256 hash of `method:path:query` by default
3. **Cache Lookup** - Checks backend for existing cached response
4. **Cache Hit** - Returns cached response with `X-Cache: HIT` header
5. **Cache Miss** - Executes handler, caches successful response (2xx), adds `X-Cache: MISS`
6. **Expiration** - TTL tracked per entry, expired entries removed automatically

## Response Headers

The plugin adds cache status headers to all responses:

- `X-Cache: HIT` - Response served from cache
- `X-Cache: MISS` - Response generated and cached

## Backend Interface

Custom backends must implement:

```go
type Backend interface {
    Get(key string) ([]byte, bool, error)
    Set(key string, value []byte, ttl time.Duration) error
    Delete(key string) error
    Clear() error
}
```

## Limitations

- Only caches GET requests
- Caches 2xx status code responses only
- Default key includes query parameters (may be customized)

## Performance Considerations

- **In-Memory Backend**: Fast but limited by RAM, not distributed
- **External Backend**: Slower but scalable across instances
- **TTL Cleanup**: In-memory backend runs cleanup every 1 minute

## Example

See [examples/cache/main.go](../../examples/cache/main.go) for a complete working example.

```bash
cd examples/cache
go run main.go

# Test cache behavior
curl -i http://localhost:8080/time  # X-Cache: MISS
curl -i http://localhost:8080/time  # X-Cache: HIT (same response, cached)
```

## Tests

```bash
cd plugins/cache
go test -v
```

All 7 tests passing:
- ✅ cache_hit
- ✅ cache_miss  
- ✅ cache_expiration
- ✅ only_get_cached
- ✅ custom_key_func
- ✅ on_hit_callback
- ✅ backend_operations
