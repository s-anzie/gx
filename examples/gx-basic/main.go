package main

import (
	"context"
	"log"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

// ── Domain Types ─────────────────────────────────────────────────────────────

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

type UserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

// ── Domain Errors ────────────────────────────────────────────────────────────

var (
	ErrUserNotFound = gx.E(404, "USER_NOT_FOUND", "User does not exist")
	ErrEmailTaken   = gx.E(409, "EMAIL_TAKEN", "Email already registered")
)

// ── Contracts ────────────────────────────────────────────────────────────────

var CreateUser = gx.Contract{
	Summary:     "Create a new user",
	Description: "Creates a new user account with the provided information",
	Tags:        []string{"users"},
	Body:        gx.Schema[CreateUserRequest](),
	Output:      gx.Schema[UserResponse](),
	Errors:      []gx.AppError{ErrEmailTaken, gx.ErrValidation},
}

var GetUser = gx.Contract{
	Summary: "Get user by ID",
	Tags:    []string{"users"},
	Output:  gx.Schema[UserResponse](),
	Errors:  []gx.AppError{ErrUserNotFound},
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// In-memory store for demo
var users = make(map[string]UserResponse)
var userCount = 0

func createUserHandler(c *gx.Context) core.Response {
	// Get validated request body (guaranteed to be present and valid)
	req := gx.Typed[CreateUserRequest](c.Context)

	// Check for duplicate email (simple check for demo)
	for _, user := range users {
		if user.Email == req.Email {
			return c.Fail(ErrEmailTaken.With("email", req.Email))
		}
	}

	// Create user
	userCount++
	id := string(rune('0' + userCount))
	user := UserResponse{
		ID:    id,
		Name:  req.Name,
		Email: req.Email,
		Age:   req.Age,
	}

	users[id] = user

	return c.Created(user)
}

func getUserHandler(c *gx.Context) core.Response {
	id := c.Param("id")

	user, exists := users[id]
	if !exists {
		return c.Fail(ErrUserNotFound.With("id", id))
	}

	return c.JSON(user)
}

func listUsersHandler(c *gx.Context) core.Response {
	userList := make([]UserResponse, 0, len(users))
	for _, user := range users {
		userList = append(userList, user)
	}

	return c.JSON(map[string]any{
		"users": userList,
		"count": len(userList),
	})
}

// ── Main ─────────────────────────────────────────────────────────────────────

func main() {
	// Create GX app with configuration
	app := gx.New(
		gx.WithEnvironment(gx.Development),
		gx.ReadTimeout(10*time.Second),
		gx.WriteTimeout(10*time.Second),
		gx.MaxBodySize(5<<20), // 5 MB
	)

	// Register health checks
	app.Health("memory", func(ctx context.Context) error {
		// Always healthy for this demo
		return nil
	}, gx.HealthInterval(10*time.Second))

	// Register health endpoints
	app.RegisterHealthEndpoints()

	// Boot hooks
	app.OnBoot(func(ctx context.Context) error {
		log.Println("Application booting...")
		// Initialize database, cache, etc.
		return nil
	})

	app.OnShutdown(func(ctx context.Context) error {
		log.Println("Application shutting down...")
		// Close database connections, etc.
		return nil
	})

	// Routes - use core handlers wrapped with contracts
	api := app.Group("/api/v1")
	users := api.Group("/users")

	users.GET("", gx.WrapHandler(listUsersHandler))
	users.GET("/:id", gx.WithContract(GetUser, gx.WrapHandler(getUserHandler)))
	users.POST("", gx.WithContract(CreateUser, gx.WrapHandler(createUserHandler)))

	// Boot the application
	if err := app.Boot(context.Background()); err != nil {
		log.Fatalf("Boot failed: %v", err)
	}

	// Start health checks
	go app.StartHealthChecks(context.Background())

	// Start server
	log.Println("GX example server starting on :8080")
	log.Println("Try:")
	log.Println("  curl http://localhost:8080/health")
	log.Println("  curl http://localhost:8080/api/v1/users")
	log.Println("  curl -X POST http://localhost:8080/api/v1/users -H 'Content-Type: application/json' -d '{\"name\":\"Alice\",\"email\":\"alice@example.com\",\"age\":25}'")

	if err := app.Listen("localhost", "8080"); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
