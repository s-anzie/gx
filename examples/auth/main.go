package main

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
	"github.com/s-anzie/gx/plugins/auth"
)

const jwtSecret = "my-super-secret-key-change-in-production"

func main() {
	app := gx.New()

	// Public route - login (no auth required)
	app.POST("/login", func(c *core.Context) core.Response {
		// In real app, validate credentials here
		username := "demo_user"

		// Generate JWT token
		token, err := auth.GenerateToken(jwtSecret, jwt.MapClaims{
			"sub":      username,
			"user_id":  "user_123",
			"username": username,
			"exp":      time.Now().Add(24 * time.Hour).Unix(),
			"iat":      time.Now().Unix(),
		})

		if err != nil {
			return c.JSON(map[string]any{
				"error": "Failed to generate token",
			})
		}

		return c.JSON(map[string]any{
			"token":   token,
			"expires": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		})
	})

	// Install JWT authentication for protected routes
	authPlugin := auth.JWT(
		auth.Secret(jwtSecret),
		auth.OnInvalid(func(c *gx.Context) core.Response {
			return c.JSON(map[string]any{
				"error":   "Unauthorized",
				"message": "Invalid or missing JWT token",
			})
		}),
	)
	app.Install(authPlugin)

	// Protected routes - require valid JWT
	app.GET("/profile", func(c *core.Context) core.Response {
		gxCtx := &gx.Context{Context: c}

		// Get subject from JWT claims
		subject, _ := auth.GetSubject(gxCtx)

		// Get full claims if needed
		claims, _ := auth.GetClaims(gxCtx)

		return c.JSON(map[string]any{
			"message":  "This is your protected profile",
			"subject":  subject,
			"username": claims["username"],
			"user_id":  claims["user_id"],
		})
	})

	app.GET("/admin", func(c *core.Context) core.Response {
		gxCtx := &gx.Context{Context: c}

		claims, _ := auth.GetClaims(gxCtx)

		return c.JSON(map[string]any{
			"message": "Admin area - you are authenticated!",
			"claims":  claims,
		})
	})

	// Health check (uses auth since it's installed globally)
	app.GET("/health", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{
			"status": "ok",
		})
	})

	// Run server
	fmt.Println("JWT Auth example running on :8080")
	fmt.Println("")
	fmt.Println("Try these commands:")
	fmt.Println("  # 1. Get a JWT token")
	fmt.Println("  TOKEN=$(curl -s -X POST http://localhost:8080/login | jq -r '.token')")
	fmt.Println("  echo \"Token: $TOKEN\"")
	fmt.Println("")
	fmt.Println("  # 2. Access protected route WITH token")
	fmt.Println("  curl -H \"Authorization: Bearer $TOKEN\" http://localhost:8080/profile")
	fmt.Println("")
	fmt.Println("  # 3. Access protected route WITHOUT token (will fail)")
	fmt.Println("  curl http://localhost:8080/profile")
	fmt.Println("")
	fmt.Println("  # 4. Access admin area with token")
	fmt.Println("  curl -H \"Authorization: Bearer $TOKEN\" http://localhost:8080/admin")

	app.Listen(":8080")
}
