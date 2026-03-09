package main

import (
	"log"

	"github.com/s-anzie/gx/core"
)

func main() {
	// Create a new GX application
	app := core.New()

	// Simple route
	app.GET("/", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{
			"message": "Welcome to GX!",
			"status":  "running",
		})
	})

	// Start the server
	log.Println("Server running on http://localhost:8080")
	log.Fatal(app.Listen(":8080"))
}
