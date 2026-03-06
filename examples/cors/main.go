package main

import (
	"fmt"
	"log"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
	"github.com/s-anzie/gx/plugins/cors"
)

func main() {
	app := gx.New(
		gx.WithEnvironment(gx.Development),
		gx.WithStructuredLogs(),
	)

	// Install CORS plugin with custom configuration
	app.Install(cors.New(cors.Config{
		Origins:       []string{"https://app.example.com", "http://localhost:3000"},
		Methods:       []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		Headers:       []string{"Authorization", "Content-Type", "X-Request-ID"},
		ExposeHeaders: []string{"X-Request-ID", "X-Total-Count"},
		Credentials:   true,
		MaxAge:        12 * time.Hour,
	}))

	// Example API endpoints
	app.GET("/", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
		return c.JSON(map[string]string{
			"message": "CORS-enabled API",
			"version": "1.0.0",
		})
	}))

	app.GET("/users", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
		users := []map[string]any{
			{"id": 1, "name": "Alice"},
			{"id": 2, "name": "Bob"},
			{"id": 3, "name": "Charlie"},
		}

		// Add custom header that will be exposed via CORS
		c.Writer.Header().Set("X-Total-Count", "3")
		c.Writer.Header().Set("X-Request-ID", "req-123")

		return c.JSON(users)
	}))

	app.POST("/users", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
		var user map[string]any
		if err := c.Context.BindJSON(&user); err != nil {
			return c.Fail(gx.ErrBadRequest.With("error", err.Error()))
		}

		user["id"] = 4
		return core.WithStatus(c.Context.JSON(user), 201)
	}))

	// Test with wildcard CORS (separate endpoint for demo)
	api := app.Group("/api/public")

	api.GET("/status", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
		return c.JSON(map[string]any{
			"status": "healthy",
			"time":   time.Now().Unix(),
		})
	}))

	fmt.Println("🚀 CORS example server starting on :8083")
	fmt.Println("📡 Try these commands from a browser or curl:")
	fmt.Println("")
	fmt.Println("# Preflight request")
	fmt.Println("curl -X OPTIONS http://localhost:8083/users \\")
	fmt.Println("  -H 'Origin: https://app.example.com' \\")
	fmt.Println("  -H 'Access-Control-Request-Method: POST' \\")
	fmt.Println("  -v")
	fmt.Println("")
	fmt.Println("# GET request with CORS")
	fmt.Println("curl http://localhost:8083/users \\")
	fmt.Println("  -H 'Origin: https://app.example.com' \\")
	fmt.Println("  -v")
	fmt.Println("")
	fmt.Println("# Public API (wildcard)")
	fmt.Println("curl http://localhost:8083/api/public/status \\")
	fmt.Println("  -H 'Origin: http://any-origin.com' \\")
	fmt.Println("  -v")

	if err := app.Listen(":8083"); err != nil {
		log.Fatal(err)
	}
}
