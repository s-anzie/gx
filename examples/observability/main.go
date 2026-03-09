package main

import (
	"context"
	"log"
	"log/slog"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

// ── Handlers ─────────────────────────────────────────────────────────────────

func indexHandler(c *gx.Context) core.Response {
	// Use structured logging
	c.Log().Info("index page requested",
		"user_agent", c.Header("User-Agent"),
		"client_ip", c.ClientIP(),
	)

	return c.JSON(map[string]any{
		"message": "Observability example",
		"version": "1.0.0",
	})
}

func userHandler(c *gx.Context) core.Response {
	userID := c.Param("id")

	// Create a child span for the database operation
	span := c.Trace("fetch-user-from-db")
	span.SetAttr("user.id", userID)
	defer span.End()

	// Simulate database query
	time.Sleep(10 * time.Millisecond)

	c.Log().Info("user fetched successfully",
		"user_id", userID,
		"duration_ms", 10,
	)

	return c.JSON(map[string]any{
		"id":   userID,
		"name": "User " + userID,
	})
}

func slowHandler(c *gx.Context) core.Response {
	// Create multiple child spans
	span1 := c.Trace("external-api-call")
	span1.SetAttr("api.endpoint", "https://api.example.com")
	time.Sleep(50 * time.Millisecond)
	span1.End()

	span2 := c.Trace("cache-lookup")
	span2.SetAttr("cache.key", "user:123")
	time.Sleep(20 * time.Millisecond)
	span2.End()

	c.Log().Warn("slow operation completed",
		"total_duration_ms", 70,
	)

	return c.Text("Slow operation completed")
}

func errorHandler(c *gx.Context) core.Response {
	c.Log().Error("simulated error occurred",
		"error", "database connection failed",
		"retry_count", 3,
	)

	return c.Fail(gx.ErrInternal.With("reason", "database unavailable"))
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	// Create GX app with observability enabled
	app := gx.New(
		gx.WithEnvironment(gx.Development),

		// Enable tracing
		gx.WithTracing("observability-example",
			gx.OTELExporter("http://localhost:4318"), // Jaeger endpoint
			gx.TracingSampler(1.0),                   // Sample 100% of requests
		),

		// Enable metrics
		gx.WithMetrics(
			gx.PrometheusExporter(":9090"), // Prometheus scrape endpoint
		),

		// Enable structured logging
		gx.WithStructuredLogs(
			gx.LogLevel(slog.LevelInfo),
			gx.WithLogFormat(gx.LogText), // Use text format in development
		),
	)

	// Boot hooks
	app.OnBoot(func(ctx context.Context) error {
		log.Println("🚀 Application starting with observability enabled")
		log.Println("   - Tracing: OpenTelemetry (no-op until real OTEL integration)")
		log.Println("   - Metrics: Prometheus (no-op until real Prometheus integration)")
		log.Println("   - Logging: Structured with slog")
		return nil
	})

	// Register health check
	app.Health("app", func(ctx context.Context) error {
		return nil // Always healthy
	})

	app.RegisterHealthEndpoints()

	// Routes
	app.GET("/", gx.WrapHandlerWithApp(app, indexHandler))
	app.GET("/users/:id", gx.WrapHandlerWithApp(app, userHandler))
	app.GET("/slow", gx.WrapHandlerWithApp(app, slowHandler))
	app.GET("/error", gx.WrapHandlerWithApp(app, errorHandler))

	// Boot the app
	if err := app.Boot(context.Background()); err != nil {
		log.Fatalf("Boot failed: %v", err)
	}

	// Start health checks
	go app.StartHealthChecks(context.Background())

	// Start server
	log.Println("\n📡 Server starting on :8081")
	log.Println("\nEndpoints:")
	log.Println("  GET  /              - Index with logging")
	log.Println("  GET  /users/:id     - User lookup with tracing")
	log.Println("  GET  /slow          - Multi-span operation")
	log.Println("  GET  /error         - Error logging example")
	log.Println("  GET  /health        - Health check")
	log.Println("\nTry:")
	log.Println("  curl http://localhost:8081/")
	log.Println("  curl http://localhost:8081/users/123")
	log.Println("  curl http://localhost:8081/slow")
	log.Println("  curl http://localhost:8081/error")

	if err := app.Listen(":8080"); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
