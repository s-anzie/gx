package main

import (
	"fmt"
	"log"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
	"github.com/s-anzie/gx/plugins/compress"
)

func main() {
	app := gx.New(
		gx.WithEnvironment(gx.Development),
		gx.WithStructuredLogs(),
	)

	// Install compression plugin
	app.Install(compress.New(compress.Config{
		Algorithms: []compress.Algorithm{compress.Brotli, compress.Gzip},
		MinSize:    1024, // Only compress responses >= 1KB
		Level:      -1,   // Default compression level
	}))

	// Small response (won't be compressed)
	app.GET("/small", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
		return c.JSON(map[string]string{
			"message": "This is a small response",
		})
	}))

	// Large response (will be compressed)
	app.GET("/users", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
		// Generate a large dataset
		users := make([]map[string]any, 100)
		for i := range users {
			users[i] = map[string]any{
				"id":       i + 1,
				"name":     fmt.Sprintf("User %d", i+1),
				"email":    fmt.Sprintf("user%d@example.com", i+1),
				"bio":      "This is a longer biography text to make the response larger and ensure it gets compressed by the compression middleware.",
				"location": "San Francisco, CA",
				"website":  "https://example.com",
			}
		}

		c.Log().Info("returning large user list", "count", len(users))
		return c.JSON(users)
	}))

	// Large text response
	app.GET("/article", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
		article := `
Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor 
incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud 
exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute 
irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla 
pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia 
deserunt mollit anim id est laborum.

Sed ut perspiciatis unde omnis iste natus error sit voluptatem accusantium doloremque 
laudantium, totam rem aperiam, eaque ipsa quae ab illo inventore veritatis et quasi 
architecto beatae vitae dicta sunt explicabo. Nemo enim ipsam voluptatem quia voluptas 
sit aspernatur aut odit aut fugit, sed quia consequuntur magni dolores eos qui ratione 
voluptatem sequi nesciunt.

At vero eos et accusamus et iusto odio dignissimos ducimus qui blanditiis praesentium 
voluptatum deleniti atque corrupti quos dolores et quas molestias excepturi sint 
occaecati cupiditate non provident, similique sunt in culpa qui officia deserunt 
mollitia animi, id est laborum et dolorum fuga.
`
		return c.Context.Text("%s", article)
	}))

	// Stats endpoint
	app.GET("/", gx.WrapHandlerWithApp(app, func(c *gx.Context) core.Response {
		return c.JSON(map[string]any{
			"service":     "Compression Example",
			"version":     "1.0.0",
			"compression": "brotli, gzip",
			"min_size":    "1KB",
		})
	}))

	fmt.Println("🚀 Compression example server starting on :8084")
	fmt.Println("")
	fmt.Println("📦 Test compression with these commands:")
	fmt.Println("")
	fmt.Println("# Small response (no compression):")
	fmt.Println("curl -v http://localhost:8084/small")
	fmt.Println("")
	fmt.Println("# Large response with gzip:")
	fmt.Println("curl -v -H 'Accept-Encoding: gzip' http://localhost:8084/users | head -20")
	fmt.Println("")
	fmt.Println("# Large response with brotli:")
	fmt.Println("curl -v -H 'Accept-Encoding: br' http://localhost:8084/users --compressed | jq '.[:3]'")
	fmt.Println("")
	fmt.Println("# Text article with compression:")
	fmt.Println("curl -v -H 'Accept-Encoding: gzip' http://localhost:8084/article --compressed")
	fmt.Println("")
	fmt.Println("# Compare sizes:")
	fmt.Println("echo 'Without compression:' && curl -s http://localhost:8084/users | wc -c")
	fmt.Println("echo 'With gzip:' && curl -s -H 'Accept-Encoding: gzip' http://localhost:8084/users | wc -c")

	if err := app.Listen(":8080"); err != nil {
		log.Fatal(err)
	}
}
