package compress_test

import (
	"compress/gzip"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
	"github.com/s-anzie/gx/plugins/compress"
)

func TestCompress(t *testing.T) {
	t.Run("gzip_compression", func(t *testing.T) {
		app := gx.New()
		app.Install(compress.New(compress.Config{
			Algorithms: []compress.Algorithm{compress.Gzip},
			MinSize:    10, // Very small for testing
		}))

		app.GET("/test", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
			return c.JSON(map[string]string{
				"message": "This is a test response that should be compressed",
			})
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		if w.Header().Get("Content-Encoding") != "gzip" {
			t.Errorf("expected Content-Encoding: gzip, got %s", w.Header().Get("Content-Encoding"))
		}

		// Decompress and verify content
		reader, err := gzip.NewReader(w.Body)
		if err != nil {
			t.Fatalf("failed to create gzip reader: %v", err)
		}
		defer reader.Close()

		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("failed to read gzip body: %v", err)
		}

		if !strings.Contains(string(body), "message") {
			t.Errorf("decompressed body missing expected content: %s", string(body))
		}
	})

	t.Run("brotli_compression", func(t *testing.T) {
		app := gx.New()
		app.Install(compress.New(compress.Config{
			Algorithms: []compress.Algorithm{compress.Brotli},
			MinSize:    10,
		}))

		app.GET("/test", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
			return c.JSON(map[string]string{
				"message": "This is a test response that should be brotli compressed",
			})
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "br")
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		if w.Header().Get("Content-Encoding") != "br" {
			t.Errorf("expected Content-Encoding: br, got %s", w.Header().Get("Content-Encoding"))
		}

		// Decompress and verify content
		reader := brotli.NewReader(w.Body)
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("failed to read brotli body: %v", err)
		}

		if !strings.Contains(string(body), "brotli compressed") {
			t.Errorf("decompressed body missing expected content: %s", string(body))
		}
	})

	t.Run("algorithm_priority", func(t *testing.T) {
		app := gx.New()
		app.Install(compress.New(compress.Config{
			Algorithms: []compress.Algorithm{compress.Brotli, compress.Gzip},
			MinSize:    10,
		}))

		app.GET("/test", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
			return c.JSON(map[string]string{"message": "test"})
		}))

		// Client accepts both, should prefer Brotli (first in list)
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip, br")
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "br" {
			t.Errorf("expected Content-Encoding: br (priority), got %s", w.Header().Get("Content-Encoding"))
		}
	})

	t.Run("no_compression_when_not_accepted", func(t *testing.T) {
		app := gx.New()
		app.Install(compress.New(compress.DefaultConfig()))

		app.GET("/test", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
			return c.JSON(map[string]string{"message": "test"})
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		// No Accept-Encoding header
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "" {
			t.Errorf("expected no Content-Encoding, got %s", w.Header().Get("Content-Encoding"))
		}
	})

	t.Run("min_size_threshold", func(t *testing.T) {
		app := gx.New()
		app.Install(compress.New(compress.Config{
			Algorithms: []compress.Algorithm{compress.Gzip},
			MinSize:    1000, // 1KB minimum
		}))

		app.GET("/small", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
			return c.JSON(map[string]string{"msg": "ok"}) // Very small response
		}))

		req := httptest.NewRequest("GET", "/small", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		// Should not be compressed (too small)
		if w.Header().Get("Content-Encoding") == "gzip" {
			t.Error("small response should not be compressed")
		}

		// Body should be valid JSON without decompression
		body := w.Body.String()
		if !strings.Contains(body, "msg") {
			t.Errorf("expected JSON response, got: %s", body)
		}
	})

	t.Run("large_response_compressed", func(t *testing.T) {
		app := gx.New()
		app.Install(compress.New(compress.Config{
			Algorithms: []compress.Algorithm{compress.Gzip},
			MinSize:    100,
		}))

		app.GET("/large", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
			// Create a large response
			largeData := make([]map[string]string, 50)
			for i := range largeData {
				largeData[i] = map[string]string{
					"id":      "user-" + string(rune(i)),
					"name":    "User Name",
					"email":   "user@example.com",
					"message": "This is a longer message to increase the response size",
				}
			}
			return c.JSON(largeData)
		}))

		req := httptest.NewRequest("GET", "/large", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "gzip" {
			t.Error("large response should be compressed")
		}
	})

	t.Run("default_config", func(t *testing.T) {
		config := compress.DefaultConfig()

		if len(config.Algorithms) != 2 {
			t.Errorf("expected 2 algorithms, got %d", len(config.Algorithms))
		}

		if config.MinSize != 1024 {
			t.Errorf("expected MinSize 1024, got %d", config.MinSize)
		}

		if config.Level != -1 {
			t.Errorf("expected Level -1, got %d", config.Level)
		}
	})
}
