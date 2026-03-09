package main

import (
	"log"

	"github.com/s-anzie/gx/core"
)

func main() {
	app := core.New()

	app.GET("/", func(c *core.Context) core.Response {
		return c.JSON(map[string]interface{}{
			"message":  "Secure HTTPS connection!",
			"protocol": c.Proto(),
			"isHTTP2":  c.IsHTTP2(),
		})
	})

	app.GET("/info", func(c *core.Context) core.Response {
		return c.JSON(map[string]interface{}{
			"path":      c.Path(),
			"method":    c.Method(),
			"protocol":  c.Proto(),
			"clientIP":  c.ClientIP(),
			"userAgent": c.Header("User-Agent"),
		})
	})

	app.WithDevTLS("localhost", "127.0.0.1")

	log.Println("Server starting on https://localhost:8443")
	log.Println("Using self-signed certificate - browser will show security warning")
	log.Println("Visit https://localhost:8443")

	log.Fatal(app.Listen("localhost", "8443"))
}
