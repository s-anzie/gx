package main

import (
	"log"

	"github.com/s-anzie/gx"
)

func main() {
	app := gx.New(
		gx.WithTLS("./certs/cert.pem", "./certs/key.pem"),
		gx.WithHTTP3(), // Enable HTTP/3 support
	)

	app.GET("/", func(c *gx.Context) gx.Response {
		return c.JSON(map[string]interface{}{
			"message":  "HTTP/3 server running!",
			"protocol": c.Proto(),
			"isHTTP3":  c.IsHTTP3(),
		})
	})

	app.GET("/info", func(c *gx.Context) gx.Response {
		return c.JSON(map[string]interface{}{
			"path":     c.Path(),
			"method":   c.Method(),
			"protocol": c.Proto(),
		})
	})

	log.Println("HTTP/3 server starting on https://localhost:8443")
	log.Println("With automatic Alt-Svc bootstrap shim")
	log.Println("Visit https://localhost:8443")

	log.Fatal(app.Listen(":8443"))
}
