package main

import (
	"fmt"

	"github.com/s-anzie/gx/core"
)

// ── Handlers ─────────────────────────────────────────────────────────────────

func listUsers(c *core.Context) core.Response {
	return c.JSON(map[string]interface{}{
		"users": []map[string]string{
			{"id": "1", "name": "Alice"},
			{"id": "2", "name": "Bob"},
		},
	})
}

func getUser(c *core.Context) core.Response {
	id := c.Param("id")
	return c.JSON(map[string]string{
		"id":   id,
		"name": "User " + id,
	})
}

func createUser(c *core.Context) core.Response {
	return c.Created(map[string]string{
		"id":   "3",
		"name": "NewUser",
	})
}

func listUserPosts(c *core.Context) core.Response {
	userID := c.Param("userId")
	return c.JSON(map[string]interface{}{
		"userId": userID,
		"posts": []map[string]string{
			{"id": "1", "title": "Post 1"},
			{"id": "2", "title": "Post 2"},
		},
	})
}

func listProducts(c *core.Context) core.Response {
	return c.JSON(map[string]interface{}{
		"products": []map[string]string{
			{"id": "1", "name": "Laptop"},
			{"id": "2", "name": "Phone"},
		},
	})
}

func getStats(c *core.Context) core.Response {
	return c.JSON(map[string]interface{}{
		"total_users": 100,
		"total_posts": 500,
	})
}

func healthCheck(c *core.Context) core.Response {
	return c.JSON(map[string]string{
		"status": "healthy",
	})
}

// ── Domain Routers ──────────────────────────────────────────────────────────

// UsersRouter creates an autonomous router for user operations
func UsersRouter() *core.Router {
	r := core.NewRouter()

	r.GET("", listUsers)
	r.GET("/:id", getUser)
	r.POST("", createUser)

	// Mount sub-resource
	r.Mount("/:userId/posts", postsRouter())

	return r
}

// postsRouter creates a router for posts under users
func postsRouter() *core.Router {
	r := core.NewRouter()
	r.GET("", listUserPosts)
	return r
}

// ProductsRouter creates an autonomous router for product operations
func ProductsRouter() *core.Router {
	r := core.NewRouter()
	r.GET("", listProducts)
	return r
}

// AdminRouter creates an autonomous router for admin operations
func AdminRouter() *core.Router {
	r := core.NewRouter()
	r.GET("/stats", getStats)
	return r
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	app := core.New()

	// Global health check
	app.GET("/health", healthCheck)

	// Mount domain-specific routers at their prefixes
	fmt.Println("Mounting autonomous routers:")
	fmt.Println("  - Users API at   /api/v1/users")
	fmt.Println("  - Products API at /api/v1/products")
	fmt.Println("  - Admin API at    /api/v1/admin")
	fmt.Println("")

	app.Mount("/api/v1/users", UsersRouter())
	app.Mount("/api/v1/products", ProductsRouter())
	app.Mount("/api/v1/admin", AdminRouter())

	fmt.Println("🚀 Router autonome example server starting on http://localhost:8080")
	fmt.Println("")
	fmt.Println("Try these endpoints:")
	fmt.Println("  GET  /health                        - Health check")
	fmt.Println("  GET  /api/v1/users                  - List users")
	fmt.Println("  GET  /api/v1/users/123              - Get user by ID")
	fmt.Println("  POST /api/v1/users                  - Create user")
	fmt.Println("  GET  /api/v1/users/123/posts        - List user posts")
	fmt.Println("  GET  /api/v1/products               - List products")
	fmt.Println("  GET  /api/v1/admin/stats            - Admin stats")
	fmt.Println("")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
