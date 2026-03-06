package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

func TestRateLimit(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"basic_rate_limit", testBasicRateLimit},
		{"rate_limit_exceeded", testRateLimitExceeded},
		{"rate_limit_window_reset", testRateLimitWindowReset},
		{"rate_limit_headers", testRateLimitHeaders},
		{"per_user_rate_limit", testPerUserRateLimit},
		{"custom_on_exceeded", testCustomOnExceeded},
		{"backend_error_allows_request", testBackendErrorAllowsRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t)
		})
	}
}

func testBasicRateLimit(t *testing.T) {
	app := gx.New()
	app.Install(New(PerIP(3, time.Second)))

	app.GET("/test", func(c *core.Context) core.Response {
		return c.Text("OK")
	})

	// First 3 requests should succeed
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("Request %d: expected status 200, got %d", i, w.Code)
		}
	}
}

func testRateLimitExceeded(t *testing.T) {
	app := gx.New()
	app.Install(New(PerIP(2, time.Second)))

	app.GET("/test", func(c *core.Context) core.Response {
		return c.Text("OK")
	})

	// First 2 requests succeed
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("Request %d: expected status 200, got %d", i, w.Code)
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 429 {
		t.Errorf("Rate limited request: expected status 429, got %d", w.Code)
	}

	// Check for Retry-After header
	retryAfter := w.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("Expected Retry-After header, got none")
	}
}

func testRateLimitWindowReset(t *testing.T) {
	app := gx.New()
	app.Install(New(PerIP(2, 100*time.Millisecond)))

	app.GET("/test", func(c *core.Context) core.Response {
		return c.Text("OK")
	})

	// First 2 requests succeed
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("Request %d: expected status 200, got %d", i, w.Code)
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 429 {
		t.Errorf("Rate limited request: expected status 429, got %d", w.Code)
	}

	// Wait for window to reset
	time.Sleep(150 * time.Millisecond)

	// Request should succeed again
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w = httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("After window reset: expected status 200, got %d", w.Code)
	}
}

func testRateLimitHeaders(t *testing.T) {
	app := gx.New()
	app.Install(New(PerIP(5, time.Second)))

	app.GET("/test", func(c *core.Context) core.Response {
		return c.Text("OK")
	})

	// First request
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	// Check headers
	limit := w.Header().Get("X-RateLimit-Limit")
	if limit != "5" {
		t.Errorf("Expected X-RateLimit-Limit: 5, got %s", limit)
	}

	remaining := w.Header().Get("X-RateLimit-Remaining")
	if remaining != "4" {
		t.Errorf("Expected X-RateLimit-Remaining: 4, got %s", remaining)
	}

	// Second request
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w = httptest.NewRecorder()

	app.ServeHTTP(w, req)

	remaining = w.Header().Get("X-RateLimit-Remaining")
	if remaining != "3" {
		t.Errorf("Expected X-RateLimit-Remaining: 3, got %s", remaining)
	}
}

func testPerUserRateLimit(t *testing.T) {
	app := gx.New()
	app.Install(New(PerUser(2, time.Second)))

	app.GET("/test", func(c *core.Context) core.Response {
		return c.Text("OK")
	})

	// Requests from different IPs
	// Without user_id in context, will fallback to IP-based limiting
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1." + string(rune('0'+i)) + ":1234"
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		// Each IP gets its own bucket, so all succeed
		if w.Code != 200 {
			t.Errorf("Request %d: expected status 200, got %d", i, w.Code)
		}
	}
}

func testCustomOnExceeded(t *testing.T) {
	customCalled := false

	app := gx.New()
	app.Install(New(
		PerIP(1, time.Second),
		OnExceeded(func(c *gx.Context) core.Response {
			customCalled = true
			return c.JSON(map[string]string{
				"error": "Custom rate limit exceeded",
			})
		}),
	))

	app.GET("/test", func(c *core.Context) core.Response {
		return c.Text("OK")
	})

	// First request succeeds
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("First request: expected status 200, got %d", w.Code)
	}

	// Second request should trigger custom handler
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w = httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("Rate limited request: expected status 200 (JSON default), got %d", w.Code)
	}

	if !customCalled {
		t.Error("Custom OnExceeded handler was not called")
	}
}

func testBackendErrorAllowsRequest(t *testing.T) {
	// Create a backend that always errors
	errorBackend := &mockBackend{
		shouldError: true,
	}

	app := gx.New()
	app.Install(New(
		PerIP(1, time.Second),
		WithBackend(errorBackend),
	))

	app.GET("/test", func(c *core.Context) core.Response {
		return c.Text("OK")
	})

	// Request should succeed despite backend error
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("Expected status 200 (backend error allows request), got %d", w.Code)
	}
}

// ── Mock Backend ─────────────────────────────────────────────────────────────

type mockBackend struct {
	shouldError bool
	count       int
}

func (mb *mockBackend) Increment(key string, window time.Duration, limit int) (int, bool, error) {
	if mb.shouldError {
		return 0, false, http.ErrAbortHandler
	}

	mb.count++
	return mb.count, mb.count > limit, nil
}

func (mb *mockBackend) Reset(key string) error {
	mb.count = 0
	return nil
}
