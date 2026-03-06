package cache

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

func TestCache(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"cache_hit", testCacheHit},
		{"cache_miss", testCacheMiss},
		{"cache_expiration", testCacheExpiration},
		{"only_get_cached", testOnlyGetCached},
		{"custom_key_func", testCustomKeyFunc},
		{"on_hit_callback", testOnHitCallback},
		{"backend_operations", testBackendOperations},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t)
		})
	}
}

func testCacheHit(t *testing.T) {
	callCount := 0

	app := gx.New()
	app.Install(New(DefaultTTL(1 * time.Minute)))

	app.GET("/data", func(c *core.Context) core.Response {
		callCount++
		return c.JSON(map[string]any{
			"data":  "test data",
			"count": callCount,
		})
	})

	// First request - should miss cache
	req1 := httptest.NewRequest("GET", "/data", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)

	if callCount != 1 {
		t.Errorf("Expected handler to be called once, got %d", callCount)
	}

	cacheHeader1 := w1.Header().Get("X-Cache")
	if cacheHeader1 != "MISS" {
		t.Errorf("Expected X-Cache: MISS, got %s", cacheHeader1)
	}

	// Second request - should hit cache
	req2 := httptest.NewRequest("GET", "/data", nil)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)

	if callCount != 1 {
		t.Errorf("Expected handler to be called once (cached), got %d", callCount)
	}

	cacheHeader2 := w2.Header().Get("X-Cache")
	if cacheHeader2 != "HIT" {
		t.Errorf("Expected X-Cache: HIT, got %s", cacheHeader2)
	}
}

func testCacheMiss(t *testing.T) {
	app := gx.New()
	app.Install(New(DefaultTTL(1 * time.Minute)))

	app.GET("/data", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"data": "test"})
	})

	req := httptest.NewRequest("GET", "/data", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	cacheHeader := w.Header().Get("X-Cache")
	if cacheHeader != "MISS" {
		t.Errorf("Expected X-Cache: MISS, got %s", cacheHeader)
	}
}

func testCacheExpiration(t *testing.T) {
	callCount := 0

	app := gx.New()
	app.Install(New(DefaultTTL(100 * time.Millisecond)))

	app.GET("/data", func(c *core.Context) core.Response {
		callCount++
		return c.JSON(map[string]int{"count": callCount})
	})

	// First request
	req1 := httptest.NewRequest("GET", "/data", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)

	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Second request after expiration
	req2 := httptest.NewRequest("GET", "/data", nil)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)

	if callCount != 2 {
		t.Errorf("Expected 2 calls after expiration, got %d", callCount)
	}
}

func testOnlyGetCached(t *testing.T) {
	callCount := 0

	app := gx.New()
	app.Install(New(DefaultTTL(1 * time.Minute)))

	app.POST("/data", func(c *core.Context) core.Response {
		callCount++
		return c.JSON(map[string]int{"count": callCount})
	})

	// First POST request
	req1 := httptest.NewRequest("POST", "/data", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)

	// Second POST request - should not be cached
	req2 := httptest.NewRequest("POST", "/data", nil)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)

	if callCount != 2 {
		t.Errorf("Expected 2 calls (POST not cached), got %d", callCount)
	}

	// Both should have no cache header or MISS
	cacheHeader := w1.Header().Get("X-Cache")
	if cacheHeader == "HIT" {
		t.Error("POST request should not be cached")
	}
}

func testCustomKeyFunc(t *testing.T) {
	callCount := 0

	app := gx.New()
	app.Install(New(
		DefaultTTL(1*time.Minute),
		KeyFunc(func(c *gx.Context) string {
			// Cache key only based on path, ignoring query params
			return c.Path()
		}),
	))

	app.GET("/data", func(c *core.Context) core.Response {
		callCount++
		return c.JSON(map[string]int{"count": callCount})
	})

	// First request with query param
	req1 := httptest.NewRequest("GET", "/data?page=1", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)

	// Second request with different query param
	// Should hit cache because custom key func ignores query params
	req2 := httptest.NewRequest("GET", "/data?page=2", nil)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)

	if callCount != 1 {
		t.Errorf("Expected 1 call (same cache key), got %d", callCount)
	}
}

func testOnHitCallback(t *testing.T) {
	hitCount := 0
	missCount := 0

	app := gx.New()
	app.Install(New(
		DefaultTTL(1*time.Minute),
		OnHit(func(c *gx.Context, key string) {
			hitCount++
		}),
		OnMiss(func(c *gx.Context, key string) {
			missCount++
		}),
	))

	app.GET("/data", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"data": "test"})
	})

	// First request - miss
	req1 := httptest.NewRequest("GET", "/data", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)

	if missCount != 1 {
		t.Errorf("Expected 1 miss, got %d", missCount)
	}

	// Second request - hit
	req2 := httptest.NewRequest("GET", "/data", nil)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)

	if hitCount != 1 {
		t.Errorf("Expected 1 hit, got %d", hitCount)
	}
}

func testBackendOperations(t *testing.T) {
	backend := newMemoryBackend()

	// Test Set and Get
	err := backend.Set("key1", []byte("value1"), 1*time.Minute)
	if err != nil {
		t.Errorf("Set failed: %v", err)
	}

	value, found, err := backend.Get("key1")
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if !found {
		t.Error("Expected key to be found")
	}
	if string(value) != "value1" {
		t.Errorf("Expected value1, got %s", string(value))
	}

	// Test Delete
	err = backend.Delete("key1")
	if err != nil {
		t.Errorf("Delete failed: %v", err)
	}

	_, found, err = backend.Get("key1")
	if err != nil {
		t.Errorf("Get after delete failed: %v", err)
	}
	if found {
		t.Error("Expected key to not be found after delete")
	}

	// Test Clear
	backend.Set("key2", []byte("value2"), 1*time.Minute)
	backend.Set("key3", []byte("value3"), 1*time.Minute)

	err = backend.Clear()
	if err != nil {
		t.Errorf("Clear failed: %v", err)
	}

	_, found, _ = backend.Get("key2")
	if found {
		t.Error("Expected all keys to be cleared")
	}
}
