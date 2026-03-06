package openapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

func TestOpenAPI(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"spec_generation", testSpecGeneration},
		{"docs_ui_scalar", testDocsUIScalar},
		{"docs_ui_swagger", testDocsUISwagger},
		{"contract_with_body", testContractWithBody},
		{"contract_with_query", testContractWithQuery},
		{"contract_with_errors", testContractWithErrors},
		{"multiple_routes", testMultipleRoutes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t)
		})
	}
}

func testSpecGeneration(t *testing.T) {
	app := gx.New()

	app.Install(New(
		Title("Test API"),
		Version("1.0.0"),
	))

	// Simple route without contract
	app.GET("/health", func(c *gx.Context) core.Response {
		return c.JSON(map[string]string{"status": "ok"})
	})

	// Boot the app to generate spec
	if err := app.Boot(nil); err != nil {
		t.Fatalf("Failed to boot app: %v", err)
	}

	// Request the OpenAPI spec
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Parse spec
	var spec OpenAPISpec
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("Failed to parse spec: %v", err)
	}

	if spec.OpenAPI != "3.1.0" {
		t.Errorf("Expected OpenAPI 3.1.0, got %s", spec.OpenAPI)
	}

	if spec.Info.Title != "Test API" {
		t.Errorf("Expected title 'Test API', got %s", spec.Info.Title)
	}

	if spec.Info.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got %s", spec.Info.Version)
	}
}

func testDocsUIScalar(t *testing.T) {
	app := gx.New()

	app.Install(New(
		Title("Test API"),
		UI(Scalar),
	))

	app.Boot(nil)

	req := httptest.NewRequest("GET", "/docs", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	html := w.Body.String()
	if !strings.Contains(html, "api-reference") {
		t.Error("Expected Scalar UI HTML to contain 'api-reference'")
	}

	if !strings.Contains(html, "@scalar/api-reference") {
		t.Error("Expected Scalar UI HTML to contain '@scalar/api-reference'")
	}
}

func testDocsUISwagger(t *testing.T) {
	app := gx.New()

	app.Install(New(
		Title("Test API"),
		UI(Swagger),
	))

	app.Boot(nil)

	req := httptest.NewRequest("GET", "/docs", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	html := w.Body.String()
	if !strings.Contains(html, "swagger-ui") {
		t.Error("Expected Swagger UI HTML to contain 'swagger-ui'")
	}
}

func testContractWithBody(t *testing.T) {
	type CreateUserRequest struct {
		Name  string `json:"name" validate:"required"`
		Email string `json:"email" validate:"required,email"`
	}

	type UserResponse struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	app := gx.New()

	app.Install(New(
		Title("Test API"),
		Version("1.0.0"),
	))

	contract := gx.Contract{
		Summary: "Create a user",
		Tags:    []string{"users"},
		Body:    gx.Schema[CreateUserRequest](),
		Output:  gx.Schema[UserResponse](),
	}

	app.POST("/users", contract, func(c *gx.Context) core.Response {
		return c.JSON(UserResponse{ID: 1, Name: "Test", Email: "test@example.com"})
	})

	app.Boot(nil)

	// Get spec
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	var spec OpenAPISpec
	json.Unmarshal(w.Body.Bytes(), &spec)

	// Check if route exists
	if _, exists := spec.Paths["/users"]; !exists {
		t.Error("Expected /users path to exist in spec")
	}

	// Check if POST operation exists
	if spec.Paths["/users"].Post == nil {
		t.Error("Expected POST operation on /users")
	}

	post := spec.Paths["/users"].Post
	if post.Summary != "Create a user" {
		t.Errorf("Expected summary 'Create a user', got %s", post.Summary)
	}

	if len(post.Tags) != 1 || post.Tags[0] != "users" {
		t.Errorf("Expected tags ['users'], got %v", post.Tags)
	}

	// Check request body
	if post.RequestBody == nil {
		t.Error("Expected request body to be present")
	}
}

func testContractWithQuery(t *testing.T) {
	type ListQuery struct {
		Page  int    `json:"page" validate:"min=1"`
		Limit int    `json:"limit" validate:"min=1,max=100"`
		Sort  string `json:"sort"`
	}

	app := gx.New()

	app.Install(New(Title("Test API")))

	contract := gx.Contract{
		Summary: "List users",
		Query:   gx.Schema[ListQuery](),
	}

	app.GET("/users", contract, func(c *gx.Context) core.Response {
		return c.JSON([]string{})
	})

	app.Boot(nil)

	// Get spec
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	var spec OpenAPISpec
	json.Unmarshal(w.Body.Bytes(), &spec)

	get := spec.Paths["/users"].Get
	if get == nil {
		t.Fatal("Expected GET operation on /users")
	}

	// Check parameters
	if len(get.Parameters) != 3 {
		t.Errorf("Expected 3 query parameters, got %d", len(get.Parameters))
	}

	// Verify parameter types
	for _, param := range get.Parameters {
		if param.In != "query" {
			t.Errorf("Expected parameter in 'query', got %s", param.In)
		}
	}
}

func testContractWithErrors(t *testing.T) {
	app := gx.New()

	app.Install(New(Title("Test API")))

	contract := gx.Contract{
		Summary: "Get user",
		Errors: []gx.AppError{
			{Status: 404, Code: "user_not_found", Message: "User not found"},
			{Status: 403, Code: "forbidden", Message: "Access denied"},
		},
	}

	app.GET("/users/:id", contract, func(c *gx.Context) core.Response {
		return c.JSON(map[string]string{"id": "1"})
	})

	app.Boot(nil)

	// Get spec
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	var spec OpenAPISpec
	json.Unmarshal(w.Body.Bytes(), &spec)

	get := spec.Paths["/users/:id"].Get
	if get == nil {
		t.Fatal("Expected GET operation")
	}

	// Check error responses
	if _, exists := get.Responses["404"]; !exists {
		t.Error("Expected 404 response in spec")
	}

	if _, exists := get.Responses["403"]; !exists {
		t.Error("Expected 403 response in spec")
	}

	// Check success response
	if _, exists := get.Responses["200"]; !exists {
		t.Error("Expected 200 response in spec")
	}
}

func testMultipleRoutes(t *testing.T) {
	app := gx.New()

	app.Install(New(Title("Test API")))

	contract1 := gx.Contract{
		Summary: "Get user",
		Tags:    []string{"users"},
	}

	contract2 := gx.Contract{
		Summary: "Create user",
		Tags:    []string{"users"},
	}

	contract3 := gx.Contract{
		Summary: "List products",
		Tags:    []string{"products"},
	}

	app.GET("/users/:id", contract1, func(c *gx.Context) core.Response {
		return c.JSON(nil)
	})

	app.POST("/users", contract2, func(c *gx.Context) core.Response {
		return c.JSON(nil)
	})

	app.GET("/products", contract3, func(c *gx.Context) core.Response {
		return c.JSON(nil)
	})

	app.Boot(nil)

	// Get spec
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	var spec OpenAPISpec
	json.Unmarshal(w.Body.Bytes(), &spec)

	// Check all paths exist
	if len(spec.Paths) != 3 {
		t.Errorf("Expected 3 paths, got %d", len(spec.Paths))
	}

	if _, exists := spec.Paths["/users/:id"]; !exists {
		t.Error("Expected /users/:id path")
	}

	if _, exists := spec.Paths["/users"]; !exists {
		t.Error("Expected /users path")
	}

	if _, exists := spec.Paths["/products"]; !exists {
		t.Error("Expected /products path")
	}
}
