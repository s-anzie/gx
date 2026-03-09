package main

import (
	"fmt"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
	"github.com/s-anzie/gx/plugins/ratelimit"
)

func main() {
	app := gx.New()

	// Install rate limiting: 10 requests per minute per IP
	app.Install(ratelimit.New(
		ratelimit.PerIP(10, time.Minute),
		ratelimit.OnExceeded(func(c *gx.Context) core.Response {
			return c.JSON(map[string]any{
				"error":      "Too many requests",
				"message":    "Please slow down and try again later",
				"retryAfter": 60,
			})
		}),
	))

	// Public endpoint with rate limiting
	app.GET("/api/data", func(c *core.Context) core.Response {
		return c.JSON(map[string]any{
			"data": "This endpoint is rate limited to 10 requests per minute",
			"time": time.Now().Format(time.RFC3339),
		})
	})

	// Status endpoint
	app.GET("/status", func(c *core.Context) core.Response {
		return c.JSON(map[string]any{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// Run server
	fmt.Println("Rate limit example running on :8080")
	fmt.Println("")
	fmt.Println("Try these commands:")
	fmt.Println("  # Test rate limiting (10 requests/minute)")
	fmt.Println("  for i in {1..15}; do curl http://localhost:8080/api/data; echo ''; sleep 0.5; done")
	fmt.Println("")
	fmt.Println("  # Check rate limit headers:")
	fmt.Println("  curl -I http://localhost:8080/api/data | grep -i 'x-ratelimit'")
	fmt.Println("")
	fmt.Println("  # Test without rate limiting:")
	fmt.Println("  curl http://localhost:8080/status")

	app.Listen("localhost", "8080")
}
