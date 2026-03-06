package cors_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
	"github.com/s-anzie/gx/plugins/cors"
)

func TestCORS(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		app := gx.New()
		app.Install(cors.New(cors.DefaultConfig()))

		app.GET("/test", func(c *core.Context) core.Response {
			return c.JSON(map[string]string{"message": "ok"})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("expected Access-Control-Allow-Origin: *, got %s", w.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("specific_origins", func(t *testing.T) {
		app := gx.New()
		app.Install(cors.New(cors.Config{
			Origins: []string{"https://app.example.com", "https://admin.example.com"},
			Methods: []string{"GET", "POST"},
			Headers: []string{"Authorization", "Content-Type"},
		}))

		app.GET("/test", func(c *core.Context) core.Response {
			return c.JSON(map[string]string{"message": "ok"})
		})

		// Test allowed origin
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://app.example.com")
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
			t.Errorf("expected Access-Control-Allow-Origin: https://app.example.com, got %s", w.Header().Get("Access-Control-Allow-Origin"))
		}

		if w.Header().Get("Vary") != "Origin" {
			t.Error("expected Vary: Origin header")
		}
	})

	t.Run("disallowed_origin", func(t *testing.T) {
		app := gx.New()
		app.Install(cors.New(cors.Config{
			Origins: []string{"https://app.example.com"},
		}))

		app.GET("/test", func(c *core.Context) core.Response {
			return c.JSON(map[string]string{"message": "ok"})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://malicious.com")
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		// Should not have CORS headers
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("disallowed origin should not receive CORS headers")
		}
	})

	t.Run("preflight_request", func(t *testing.T) {
		app := gx.New()
		app.Install(cors.New(cors.Config{
			Origins:     []string{"https://app.example.com"},
			Methods:     []string{"GET", "POST", "PUT", "DELETE"},
			Headers:     []string{"Authorization", "Content-Type"},
			Credentials: true,
			MaxAge:      24 * time.Hour,
		}))

		app.POST("/test", func(c *core.Context) core.Response {
			return c.JSON(map[string]string{"message": "ok"})
		})

		// Register OPTIONS handler (CORS plugin will handle the response)
		app.OPTIONS("/test", func(c *core.Context) core.Response {
			return nil // CORS plugin will handle this
		})

		// Send OPTIONS preflight request
		req := httptest.NewRequest("OPTIONS", "/test", nil)
		req.Header.Set("Origin", "https://app.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Authorization")
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		// Check status
		if w.Code != 204 {
			t.Errorf("expected status 204, got %d", w.Code)
		}

		// Check headers
		if w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
			t.Errorf("unexpected Allow-Origin: %s", w.Header().Get("Access-Control-Allow-Origin"))
		}

		if w.Header().Get("Access-Control-Allow-Methods") != "GET, POST, PUT, DELETE" {
			t.Errorf("unexpected Allow-Methods: %s", w.Header().Get("Access-Control-Allow-Methods"))
		}

		if w.Header().Get("Access-Control-Allow-Headers") != "Authorization, Content-Type" {
			t.Errorf("unexpected Allow-Headers: %s", w.Header().Get("Access-Control-Allow-Headers"))
		}

		if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Error("expected Allow-Credentials: true")
		}

		if w.Header().Get("Access-Control-Max-Age") != "86400" {
			t.Errorf("expected Max-Age: 86400, got %s", w.Header().Get("Access-Control-Max-Age"))
		}
	})

	t.Run("credentials", func(t *testing.T) {
		app := gx.New()
		app.Install(cors.New(cors.Config{
			Origins:     []string{"https://app.example.com"},
			Credentials: true,
		}))

		app.GET("/test", func(c *core.Context) core.Response {
			return c.JSON(map[string]string{"message": "ok"})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://app.example.com")
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Error("expected Allow-Credentials: true")
		}
	})

	t.Run("expose_headers", func(t *testing.T) {
		app := gx.New()
		app.Install(cors.New(cors.Config{
			Origins:       []string{"*"},
			ExposeHeaders: []string{"X-Request-ID", "X-Total-Count"},
		}))

		app.GET("/test", func(c *core.Context) core.Response {
			return c.JSON(map[string]string{"message": "ok"})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "https://app.example.com")
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Expose-Headers") != "X-Request-ID, X-Total-Count" {
			t.Errorf("unexpected Expose-Headers: %s", w.Header().Get("Access-Control-Expose-Headers"))
		}
	})
}
