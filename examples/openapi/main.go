package main

import (
	"fmt"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
	"github.com/s-anzie/gx/plugins/openapi"
)

// ── Request/Response Types ──────────────────────────────────────────────────

type CreateUserRequest struct {
	Name  string `json:"name" validate:"required,min=2" description:"User's full name" example:"John Doe"`
	Email string `json:"email" validate:"required,email" description:"User's email address" example:"john@example.com"`
	Age   int    `json:"age" validate:"omitempty,min=18" description:"User's age" example:"25"`
}

type UserResponse struct {
	ID        int    `json:"id" description:"User ID" example:"123"`
	Name      string `json:"name" description:"User name" example:"John Doe"`
	Email     string `json:"email" description:"User email" example:"john@example.com"`
	Age       int    `json:"age" description:"User age" example:"25"`
	CreatedAt string `json:"created_at" description:"Creation timestamp" example:"2026-03-05T10:00:00Z"`
}

type ListUsersQuery struct {
	Page  int    `json:"page" validate:"min=1" description:"Page number" example:"1"`
	Limit int    `json:"limit" validate:"min=1,max=100" description:"Items per page" example:"10"`
	Sort  string `json:"sort" description:"Sort field" example:"name"`
}

type UpdateUserRequest struct {
	Name  string `json:"name" validate:"omitempty,min=2" description:"Updated name"`
	Email string `json:"email" validate:"omitempty,email" description:"Updated email"`
}

// ── Contracts ────────────────────────────────────────────────────────────────

var CreateUserContract = gx.Contract{
	Summary:     "Create a new user",
	Description: "Creates a new user in the system with the provided information",
	Tags:        []string{"users"},
	Body:        gx.Schema[CreateUserRequest](),
	Output:      gx.Schema[UserResponse](),
	Errors: []gx.AppError{
		{Status: 400, Code: "validation_error", Message: "Invalid request data"},
		{Status: 409, Code: "email_exists", Message: "Email already registered"},
	},
}

var ListUsersContract = gx.Contract{
	Summary:     "List all users",
	Description: "Retrieves a paginated list of users",
	Tags:        []string{"users"},
	Query:       gx.Schema[ListUsersQuery](),
	Output:      gx.Schema[[]UserResponse](),
}

var GetUserContract = gx.Contract{
	Summary:     "Get user by ID",
	Description: "Retrieves a single user by their ID",
	Tags:        []string{"users"},
	Output:      gx.Schema[UserResponse](),
	Errors: []gx.AppError{
		{Status: 404, Code: "user_not_found", Message: "User not found"},
	},
}

var UpdateUserContract = gx.Contract{
	Summary:     "Update user",
	Description: "Updates an existing user's information",
	Tags:        []string{"users"},
	Body:        gx.Schema[UpdateUserRequest](),
	Output:      gx.Schema[UserResponse](),
	Errors: []gx.AppError{
		{Status: 404, Code: "user_not_found", Message: "User not found"},
		{Status: 400, Code: "validation_error", Message: "Invalid request data"},
	},
}

var DeleteUserContract = gx.Contract{
	Summary:     "Delete user",
	Description: "Deletes a user from the system",
	Tags:        []string{"users"},
	Errors: []gx.AppError{
		{Status: 404, Code: "user_not_found", Message: "User not found"},
	},
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func main() {
	app := gx.New()

	// Install OpenAPI plugin with Scalar UI
	app.Install(openapi.New(
		openapi.Title("User Management API"),
		openapi.Version("1.0.0"),
		openapi.Description("A comprehensive API for managing users in the system"),
		openapi.UI(openapi.Scalar), // Can also use Swagger or Redoc
		openapi.AddServer("http://localhost:8080", "Development server"),
		openapi.AddServer("https://api.example.com", "Production server"),
	))

	// Register routes with contracts
	app.POST("/users", CreateUserContract, func(c *gx.Context) core.Response {
		return c.JSON(UserResponse{
			ID:        123,
			Name:      "John Doe",
			Email:     "john@example.com",
			Age:       25,
			CreatedAt: "2026-03-05T10:00:00Z",
		})
	})

	app.GET("/users", ListUsersContract, func(c *gx.Context) core.Response {
		users := []UserResponse{
			{
				ID:        1,
				Name:      "Alice Smith",
				Email:     "alice@example.com",
				Age:       28,
				CreatedAt: "2026-01-15T09:00:00Z",
			},
			{
				ID:        2,
				Name:      "Bob Johnson",
				Email:     "bob@example.com",
				Age:       32,
				CreatedAt: "2026-02-20T14:30:00Z",
			},
		}
		return c.JSON(users)
	})

	app.GET("/users/:id", GetUserContract, func(c *gx.Context) core.Response {
		return c.JSON(UserResponse{
			ID:        123,
			Name:      "John Doe",
			Email:     "john@example.com",
			Age:       25,
			CreatedAt: "2026-03-05T10:00:00Z",
		})
	})

	app.PUT("/users/:id", UpdateUserContract, func(c *gx.Context) core.Response {
		return c.JSON(UserResponse{
			ID:        123,
			Name:      "Jane Doe",
			Email:     "jane@example.com",
			Age:       26,
			CreatedAt: "2026-03-05T10:00:00Z",
		})
	})

	app.DELETE("/users/:id", DeleteUserContract, func(c *gx.Context) core.Response {
		return c.NoContent()
	})

	// Health check without contract
	app.GET("/health", func(c *gx.Context) core.Response {
		return c.JSON(map[string]string{
			"status":  "healthy",
			"version": "1.0.0",
		})
	})

	// Boot the app (generates OpenAPI spec)
	if err := app.Boot(nil); err != nil {
		panic(err)
	}

	fmt.Println("🚀 OpenAPI example server starting on http://localhost:8080")
	fmt.Println("")
	fmt.Println("📚 API Documentation:")
	fmt.Println("  Docs UI:     http://localhost:8080/docs")
	fmt.Println("  OpenAPI Spec: http://localhost:8080/openapi.json")
	fmt.Println("")
	fmt.Println("📋 API Endpoints:")
	fmt.Println("  POST   /users       - Create user")
	fmt.Println("  GET    /users       - List users")
	fmt.Println("  GET    /users/:id   - Get user")
	fmt.Println("  PUT    /users/:id   - Update user")
	fmt.Println("  DELETE /users/:id   - Delete user")
	fmt.Println("  GET    /health      - Health check")
	fmt.Println("")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
