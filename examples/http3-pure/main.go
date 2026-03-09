package main

import (
	"log"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

func main() {
	app := gx.New(
		gx.WithDevTLS("localhost", "127.0.0.1"),
		gx.WithHTTP3(),
	)

	app.GET("/", func(c *gx.Context) gx.Response {
		return c.JSON(map[string]string{
			"message": "HTTP/3 pure - no Alt-Svc shim",
			"proto":   c.Proto(),
		})
	})

	log.Println("HTTP/3 server (no Alt-Svc shim)")
	log.Println("Clients must explicitly use HTTP/3")

	log.Fatal(app.Listen(":8443",
		core.WithoutAltSvcShim(),
		core.AltSvcMaxAge(12*time.Hour),
	))
}
