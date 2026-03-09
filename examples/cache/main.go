package main

import (
	"fmt"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
	"github.com/s-anzie/gx/plugins/cache"
)

func main() {
	app := gx.New()

	// Install cache plugin with 30 second TTL
	app.Install(cache.New(
		cache.DefaultTTL(30*time.Second),
		cache.OnHit(func(c *gx.Context, key string) {
			fmt.Printf("Cache HIT: %s\n", c.Path())
		}),
		cache.OnMiss(func(c *gx.Context, key string) {
			fmt.Printf("Cache MISS: %s\n", c.Path())
		}),
	))

	// Counter to demonstrate caching
	requestCount := 0

	// Simple endpoint that returns current time and request count
	app.GET("/time", func(c *core.Context) core.Response {
		requestCount++
		return c.JSON(map[string]any{
			"time":          time.Now().Format(time.RFC3339),
			"request_count": requestCount,
			"message":       "This response is cached for 30 seconds",
		})
	})

	// Endpoint with custom key based on query parameter
	app.GET("/data", func(c *core.Context) core.Response {
		page := c.Query("page")
		if page == "" {
			page = "1"
		}

		return c.JSON(map[string]any{
			"page":      page,
			"timestamp": time.Now().Unix(),
			"data":      fmt.Sprintf("Content for page %s", page),
		})
	})

	// POST endpoint - should NOT be cached
	app.POST("/submit", func(c *core.Context) core.Response {
		requestCount++
		return c.JSON(map[string]any{
			"message":       "POST requests are not cached",
			"request_count": requestCount,
		})
	})

	// Info endpoint
	app.GET("/", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{
			"cache_example": "GX Cache Plugin Demo",
			"endpoints":     "GET /time (cached), GET /data?page=1 (cached), POST /submit (not cached)",
			"ttl":           "30 seconds",
			"tip":           "Watch the X-Cache header: HIT or MISS",
		})
	})

	fmt.Println("🚀 Cache example server starting on http://localhost:8080")
	fmt.Println("")
	fmt.Println("Try these commands:")
	fmt.Println("  curl -i http://localhost:8080/time          # First request - cache MISS")
	fmt.Println("  curl -i http://localhost:8080/time          # Second request - cache HIT")
	fmt.Println("  curl -i http://localhost:8080/data?page=1   # Different query params = different cache key")
	fmt.Println("  curl -i http://localhost:8080/data?page=2")
	fmt.Println("  curl -X POST http://localhost:8080/submit   # POST not cached")
	fmt.Println("")

	if err := app.Listen("localhost", "8080"); err != nil {
		panic(err)
	}
}
