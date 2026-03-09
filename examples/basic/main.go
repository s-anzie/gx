package main

import (
	"log"
	"time"

	"github.com/s-anzie/gx/core"
)

func main() {
	// Create a new app
	app := core.New()

	// Add global middleware - logger
	app.Use(logger)

	// Define routes
	app.GET("/", homeHandler)
	app.GET("/hello/:name", helloHandler)
	app.GET("/users/:id", getUserHandler)
	app.POST("/users", createUserHandler)

	// Create a group
	api := app.Group("/api/v1")
	api.Use(requestID)

	// Routes in the group
	api.GET("/status", statusHandler)
	api.GET("/products/:id", getProductHandler)

	// Start server with graceful shutdown
	log.Fatal(app.Listen("localhost", "8080"))
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func homeHandler(c *core.Context) core.Response {
	return c.JSON(map[string]string{
		"message": "Welcome to GX Framework",
		"version": "0.1.0",
	})
}

func helloHandler(c *core.Context) core.Response {
	name := c.Param("name")
	return c.JSON(map[string]string{
		"message": "Hello, " + name + "!",
	})
}

func getUserHandler(c *core.Context) core.Response {
	id := c.Param("id")
	return c.JSON(map[string]interface{}{
		"id":    id,
		"name":  "John Doe",
		"email": "john@example.com",
	})
}

func createUserHandler(c *core.Context) core.Response {
	var user struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := c.BindJSON(&user); err != nil {
		return c.Text("Invalid request body: %v", err)
	}

	return c.Created(map[string]interface{}{
		"id":    "123",
		"name":  user.Name,
		"email": user.Email,
	})
}

func statusHandler(c *core.Context) core.Response {
	return c.JSON(map[string]string{
		"status": "healthy",
	})
}

func getProductHandler(c *core.Context) core.Response {
	id := c.Param("id")
	return c.JSON(map[string]interface{}{
		"id":    id,
		"name":  "Product " + id,
		"price": 99.99,
	})
}

// ── Middleware ───────────────────────────────────────────────────────────────

func logger(c *core.Context, next core.Handler) core.Response {
	start := time.Now()

	// Execute next handler
	res := next(c)

	// Log after execution
	duration := time.Since(start)
	log.Printf("[%s] %s %s - %d (%v)",
		c.ClientIP(),
		c.Method(),
		c.Path(),
		res.Status(),
		duration,
	)

	return res
}

func requestID(c *core.Context, next core.Handler) core.Response {
	// Set a request ID
	c.Set("request_id", "req-12345")

	// Add to response header
	res := next(c)
	return core.WithHeader(res, "X-Request-ID", "req-12345")
}
