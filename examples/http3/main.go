package main

import (
	"log"

	"github.com/s-anzie/gx/core"
)

func main() {
	app := core.New()

	app.GET("/", func(c *core.Context) core.Response {
		return c.JSON(map[string]interface{}{
			"message":  "HTTP/3 server running!",
			"protocol": c.Proto(),
			"isHTTP3":  c.IsHTTP3(),
		})
	})

	app.GET("/info", func(c *core.Context) core.Response {
		return c.JSON(map[string]interface{}{
			"path":     c.Path(),
			"method":   c.Method(),
			"protocol": c.Proto(),
		})
	})

	app.WithDevTLS("localhost", "127.0.0.1")

	log.Println("HTTP/3 server starting on https://localhost:8443")
	log.Println("With Alt-Svc bootstrap shim")
	log.Println("Visit https://localhost:8443")

	log.Fatal(app.ListenH3WithGracefulShutdown(":8443"))
}
