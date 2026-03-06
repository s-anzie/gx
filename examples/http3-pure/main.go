package main

import (
	"log"
	"time"

	"github.com/s-anzie/gx/core"
)

func main() {
	app := core.New()

	app.GET("/", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{
			"message": "HTTP/3 pure - no Alt-Svc shim",
			"proto":   c.Proto(),
		})
	})

	app.WithDevTLS("localhost", "127.0.0.1")

	log.Println("HTTP/3 server (no Alt-Svc shim)")
	log.Println("Clients must explicitly use HTTP/3")

	log.Fatal(app.ListenH3(":8443",
		core.WithoutAltSvcShim(),
		core.AltSvcMaxAge(12*time.Hour),
	))
}
