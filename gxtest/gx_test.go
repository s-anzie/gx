package gxtest

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

// ── Test Types ───────────────────────────────────────────────────────────────

type TestRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type TestResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ── AppError Tests ───────────────────────────────────────────────────────────

func TestAppError(t *testing.T) {
	t.Run("basic error", func(t *testing.T) {
		err := gx.E(404, "NOT_FOUND", "Resource not found")

		if err.Status != 404 {
			t.Errorf("expected status 404, got %d", err.Status)
		}
		if err.Code != "NOT_FOUND" {
			t.Errorf("expected code NOT_FOUND, got %s", err.Code)
		}
		if err.Message != "Resource not found" {
			t.Errorf("expected message 'Resource not found', got %s", err.Message)
		}
	})

	t.Run("error with details", func(t *testing.T) {
		err := gx.E(409, "CONFLICT", "Resource already exists").
			With("id", "123").
			With("type", "user")

		if len(err.Details) != 2 {
			t.Errorf("expected 2 details, got %d", len(err.Details))
		}
		if err.Details["id"] != "123" {
			t.Errorf("expected id=123, got %v", err.Details["id"])
		}
	})

	t.Run("error response", func(t *testing.T) {
		err := gx.E(422, "VALIDATION_ERROR", "Invalid input")
		resp := err.ToResponse()

		if resp.Status() != 422 {
			t.Errorf("expected status 422, got %d", resp.Status())
		}

		// Test write
		w := httptest.NewRecorder()
		if err := resp.Write(w); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}

		if w.Code != 422 {
			t.Errorf("expected status code 422, got %d", w.Code)
		}

		if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("expected JSON content type, got %s", ct)
		}
	})
}

// ── Typed Access Tests ───────────────────────────────────────────────────────

func TestTypedAccess(t *testing.T) {
	t.Run("set and get typed value", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		c := &core.Context{}
		c.Request = req
		c.Writer = w

		// Set typed value
		testReq := &TestRequest{Name: "Alice", Email: "alice@example.com"}
		gx.SetTyped(c, testReq)

		// Get typed value
		retrieved := gx.Typed[TestRequest](c)

		if retrieved.Name != "Alice" {
			t.Errorf("expected name Alice, got %s", retrieved.Name)
		}
		if retrieved.Email != "alice@example.com" {
			t.Errorf("expected email alice@example.com, got %s", retrieved.Email)
		}
	})

	t.Run("try typed - value exists", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		c := &core.Context{}
		c.Request = req
		c.Writer = w

		testReq := &TestRequest{Name: "Bob", Email: "bob@example.com"}
		gx.SetTyped(c, testReq)

		retrieved, ok := gx.TryTyped[TestRequest](c)
		if !ok {
			t.Error("expected TryTyped to succeed")
		}
		if retrieved.Name != "Bob" {
			t.Errorf("expected name Bob, got %s", retrieved.Name)
		}
	})

	t.Run("try typed - value not exists", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		c := &core.Context{}
		c.Request = req
		c.Writer = w

		_, ok := gx.TryTyped[TestRequest](c)
		if ok {
			t.Error("expected TryTyped to fail for missing value")
		}
	})
}

// ── Contract Tests ───────────────────────────────────────────────────────────

func TestContract(t *testing.T) {
	t.Run("schema creation", func(t *testing.T) {
		schema := gx.Schema[TestRequest]()

		if schema.IsZero() {
			t.Error("expected schema to be non-zero")
		}

		typeName := schema.TypeName()
		if typeName == "" {
			t.Error("expected non-empty type name")
		}
	})

	t.Run("contract with body", func(t *testing.T) {
		contract := gx.Contract{
			Summary: "Test endpoint",
			Body:    gx.Schema[TestRequest](),
			Output:  gx.Schema[TestResponse](),
		}

		if contract.Body.IsZero() {
			t.Error("expected Body schema to be set")
		}
		if contract.Output.IsZero() {
			t.Error("expected Output schema to be set")
		}
	})
}

// ── Health Check Tests ───────────────────────────────────────────────────────

func TestHealthChecks(t *testing.T) {
	t.Run("health endpoint", func(t *testing.T) {
		app := gx.New()

		// Register a health check
		app.Health("test", func(ctx context.Context) error {
			return nil // Always healthy
		})

		// Register health endpoints
		app.RegisterHealthEndpoints()

		// Test /health endpoint
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "\"status\":\"ok\"") {
			t.Errorf("expected status ok in response, got: %s", body)
		}
	})

	t.Run("liveness endpoint", func(t *testing.T) {
		app := gx.New()
		app.RegisterHealthEndpoints()

		req := httptest.NewRequest("GET", "/health/live", nil)
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("readiness endpoint", func(t *testing.T) {
		app := gx.New()

		// Register a healthy check
		app.Health("database", func(ctx context.Context) error {
			return nil
		}, gx.HealthCritical(true))

		app.RegisterHealthEndpoints()

		req := httptest.NewRequest("GET", "/health/ready", nil)
		w := httptest.NewRecorder()

		app.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})
}

// ── Lifecycle Tests ──────────────────────────────────────────────────────────

func TestLifecycle(t *testing.T) {
	t.Run("boot hooks", func(t *testing.T) {
		app := gx.New()

		bootCalled := false
		app.OnBoot(func(ctx context.Context) error {
			bootCalled = true
			return nil
		})

		if err := app.Boot(context.Background()); err != nil {
			t.Fatalf("boot failed: %v", err)
		}

		if !bootCalled {
			t.Error("expected boot hook to be called")
		}
	})

	t.Run("shutdown hooks", func(t *testing.T) {
		app := gx.New()

		shutdownCalled := false
		app.OnShutdown(func(ctx context.Context) error {
			shutdownCalled = true
			return nil
		})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := app.ShutdownGracefully(ctx); err != nil {
			t.Fatalf("shutdown failed: %v", err)
		}

		if !shutdownCalled {
			t.Error("expected shutdown hook to be called")
		}
	})
}

// ── Environment Tests ────────────────────────────────────────────────────────

func TestEnvironment(t *testing.T) {
	t.Run("default environment", func(t *testing.T) {
		app := gx.New()

		if app.Environment() != gx.Development {
			t.Errorf("expected Development environment, got %s", app.Environment())
		}
		if !app.IsDevelopment() {
			t.Error("expected IsDevelopment to be true")
		}
	})

	t.Run("production environment", func(t *testing.T) {
		app := gx.New(gx.WithEnvironment(gx.Production))

		if app.Environment() != gx.Production {
			t.Errorf("expected Production environment, got %s", app.Environment())
		}
		if !app.IsProduction() {
			t.Error("expected IsProduction to be true")
		}
		if app.IsDevelopment() {
			t.Error("expected IsDevelopment to be false")
		}
	})
}

// ── Observability Tests ──────────────────────────────────────────────────────

func TestObservability(t *testing.T) {
	t.Run("tracing enabled", func(t *testing.T) {
		app := gx.New(
			gx.WithTracing("test-service",
				gx.OTELExporter("http://localhost:4318"),
			),
		)

		// Tracer should be available
		tracer := app.Tracer()
		if tracer == nil {
			t.Error("expected tracer to be available")
		}

		// Create a span
		span := tracer.Start(context.Background(), "test-operation")
		if span == nil {
			t.Error("expected span to be created")
		}
		span.SetAttr("test.key", "test.value")
		span.End()
	})

	t.Run("metrics enabled", func(t *testing.T) {
		app := gx.New(
			gx.WithMetrics(
				gx.PrometheusExporter(":9090"),
			),
		)

		// Metrics registry should be available
		metrics := app.Metrics()
		if metrics == nil {
			t.Error("expected metrics registry to be available")
		}

		// Create metrics
		counter := metrics.Counter("test_counter", "Test counter", "label")
		counter.Inc()
		counter.Add(5)

		histogram := metrics.Histogram("test_histogram", "Test histogram")
		histogram.Observe(1.5)

		gauge := metrics.Gauge("test_gauge", "Test gauge")
		gauge.Set(10)
		gauge.Inc()
		gauge.Dec()
	})

	t.Run("structured logging enabled", func(t *testing.T) {
		app := gx.New(
			gx.WithStructuredLogs(
				gx.WithLogFormat(gx.LogJSON),
			),
		)

		// Logger should be available
		logger := app.Logger()
		if logger == nil {
			t.Error("expected logger to be available")
		}

		// Log some messages
		logger.Info("test message", "key", "value")
		logger.Warn("warning message", "count", 42)
		logger.Error("error message", "error", "test error")
		logger.Debug("debug message")

		// With creates a child logger
		childLogger := logger.With("request_id", "12345")
		childLogger.Info("child log")
	})

	t.Run("context span and logging", func(t *testing.T) {
		app := gx.New(
			gx.WithTracing("test-service"),
			gx.WithStructuredLogs(),
		)

		var spanCalled, logCalled bool

		handler := func(c *gx.Context) core.Response {
			// Access span
			span := c.Span()
			if span != nil {
				spanCalled = true
				span.SetAttr("handler", "test")
			}

			// Access logger
			logger := c.Log()
			if logger != nil {
				logCalled = true
				logger.Info("handler executed")
			}

			return c.JSON(map[string]string{"status": "ok"})
		}

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		// Create GX context with app
		coreHandler := gx.WrapHandlerWithApp(app, handler)
		c := &core.Context{Request: req, Writer: w}
		coreHandler(c)

		if !spanCalled {
			t.Error("expected span to be accessed in handler")
		}
		if !logCalled {
			t.Error("expected logger to be accessed in handler")
		}
	})

	t.Run("child span creation", func(t *testing.T) {
		app := gx.New(gx.WithTracing("test-service"))

		handler := func(c *gx.Context) core.Response {
			// Create child spans
			span1 := c.Trace("operation-1")
			span1.SetAttr("op", "1")
			span1.End()

			span2 := c.Trace("operation-2")
			span2.SetAttr("op", "2")
			span2.End()

			return c.Text("ok")
		}

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		coreHandler := gx.WrapHandlerWithApp(app, handler)
		c := &core.Context{Request: req, Writer: w}
		coreHandler(c)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})
}

// TestChannels tests the Channel abstraction and SSE-based channels
func TestChannels(t *testing.T) {
	t.Run("channel_contract", func(t *testing.T) {
		type Message struct {
			Text string
		}

		contract := gx.ChannelContract{
			Summary:       "Test channel",
			Description:   "A test channel",
			Tags:          []string{"test"},
			InputMessage:  gx.Schema[Message](),
			OutputMessage: gx.Schema[Message](),
			Errors: []gx.AppError{
				gx.ErrUnauthorized,
			},
		}

		if contract.Summary != "Test channel" {
			t.Errorf("expected summary 'Test channel', got %s", contract.Summary)
		}

		if len(contract.Tags) != 1 || contract.Tags[0] != "test" {
			t.Errorf("expected tags [test], got %v", contract.Tags)
		}
	})

	t.Run("sse_channel_creation", func(t *testing.T) {
		app := gx.New()

		handler := func(c *gx.Context) core.Response {
			ch, err := c.SSEChannel()
			if err != nil {
				t.Errorf("SSEChannel() returned error: %v", err)
				return c.Fail(gx.ErrInternal)
			}

			if ch == nil {
				t.Error("SSEChannel() returned nil channel")
				return c.Fail(gx.ErrInternal)
			}

			proto := ch.Proto()
			if proto != "sse" {
				t.Errorf("expected proto 'sse', got %s", proto)
			}

			ch.Close()
			return nil
		}

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		coreHandler := gx.WrapHandlerWithApp(app, handler)
		c := &core.Context{Request: req, Writer: w}
		coreHandler(c)
	})

	t.Run("sse_channel_send", func(t *testing.T) {
		app := gx.New()

		type TestData struct {
			Message string
			Count   int
		}

		handler := func(c *gx.Context) core.Response {
			ch, err := c.SSEChannel()
			if err != nil {
				return c.Fail(gx.ErrInternal)
			}
			defer ch.Close()

			// Send structured data
			data := TestData{
				Message: "Hello SSE",
				Count:   42,
			}

			if err := ch.Send(data); err != nil {
				t.Errorf("Send() returned error: %v", err)
			}

			return nil
		}

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		coreHandler := gx.WrapHandlerWithApp(app, handler)
		c := &core.Context{Request: req, Writer: w}
		coreHandler(c)

		// Check SSE headers were set
		if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("expected Content-Type 'text/event-stream', got %s", ct)
		}

		if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("expected Cache-Control 'no-cache', got %s", cc)
		}

		// Check data was sent
		body := w.Body.String()
		if !strings.Contains(body, "data:") {
			t.Errorf("expected SSE data, got: %s", body)
		}
	})

	t.Run("channel_registration", func(t *testing.T) {
		app := gx.New()

		type Message struct {
			Text string
		}

		contract := gx.ChannelContract{
			Summary:       "Chat channel",
			InputMessage:  gx.Schema[Message](),
			OutputMessage: gx.Schema[Message](),
		}

		handler := func(c *gx.Context, ch gx.Channel) error {
			return nil
		}

		// Register channel (placeholder for WebSocket/QUIC)
		app.Channel("/chat/:room", contract, handler)

		// This is a placeholder test - full WebSocket/QUIC implementation pending
	})

	t.Run("noop_channel", func(t *testing.T) {
		// Test that no-op channel doesn't panic
		ch := gx.NewNoopChannel()

		if ch.Proto() != "noop" {
			t.Errorf("expected proto 'noop', got %s", ch.Proto())
		}

		// These should not panic
		_ = ch.Send(map[string]string{"test": "data"})
		_ = ch.SendRaw([]byte("test"))

		select {
		case <-ch.Done():
			t.Error("channel should not be done initially")
		default:
		}

		ch.Close()

		select {
		case <-ch.Done():
			// Expected
		default:
			t.Error("channel should be done after Close()")
		}
	})
}
