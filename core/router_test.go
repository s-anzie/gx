package core_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/s-anzie/gx/core"
)

// ────────────────────────────────────────────────────────────────────────────
// Router Autonomous Tests
// ────────────────────────────────────────────────────────────────────────────

func TestRouterNewRouter(t *testing.T) {
	r := core.NewRouter()
	if r == nil {
		t.Fatal("NewRouter() should not return nil")
	}
}

func TestRouterRouting(t *testing.T) {
	r := core.NewRouter()

	r.GET("/hello", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"message": "hello"})
	})

	r.POST("/create", func(c *core.Context) core.Response {
		return c.Created(map[string]string{"id": "1"})
	})

	app := core.New()
	app.Mount("", r) // Mount root

	server := httptest.NewServer(http.HandlerFunc(app.ServeHTTP))
	defer server.Close()

	// Test GET
	resp, err := http.Get(server.URL + "/hello")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	// Test POST
	resp, err = http.Post(server.URL+"/create", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected 201, got %d", resp.StatusCode)
	}
}

func TestRouterUse(t *testing.T) {
	r := core.NewRouter()

	middlewareCalled := false
	r.Use(func(c *core.Context, next core.Handler) core.Response {
		middlewareCalled = true
		return next(c)
	})

	r.GET("/test", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"status": "ok"})
	})

	app := core.New()
	app.Mount("", r)

	server := httptest.NewServer(http.HandlerFunc(app.ServeHTTP))
	defer server.Close()

	http.Get(server.URL + "/test")

	if !middlewareCalled {
		t.Error("Router middleware should be called")
	}
}

func TestRouterMount(t *testing.T) {
	// Sub-router for users
	usersRouter := core.NewRouter()
	usersRouter.GET("/list", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"type": "users"})
	})
	usersRouter.GET("/:id", func(c *core.Context) core.Response {
		id := c.Param("id")
		return c.JSON(map[string]string{"id": id})
	})

	// Sub-router for products
	productsRouter := core.NewRouter()
	productsRouter.GET("/list", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"type": "products"})
	})

	// Main app
	app := core.New()
	app.Mount("/api/users", usersRouter)
	app.Mount("/api/products", productsRouter)

	server := httptest.NewServer(http.HandlerFunc(app.ServeHTTP))
	defer server.Close()

	// Test users endpoints
	resp, err := http.Get(server.URL + "/api/users/list")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for /api/users/list, got %d", resp.StatusCode)
	}

	resp, err = http.Get(server.URL + "/api/users/123")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for /api/users/123, got %d", resp.StatusCode)
	}

	// Test products endpoints
	resp, err = http.Get(server.URL + "/api/products/list")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for /api/products/list, got %d", resp.StatusCode)
	}
}

func TestRouterMountMiddleware(t *testing.T) {
	middlewareCalled := false

	// Sub-router
	r := core.NewRouter()
	r.GET("/test", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"status": "ok"})
	})

	// App with mount middleware
	app := core.New()
	app.Mount("/api", r, func(c *core.Context, next core.Handler) core.Response {
		middlewareCalled = true
		return next(c)
	})

	server := httptest.NewServer(http.HandlerFunc(app.ServeHTTP))
	defer server.Close()

	http.Get(server.URL + "/api/test")

	if !middlewareCalled {
		t.Error("Mount middleware should be called")
	}
}

func TestRouterMountNested(t *testing.T) {
	// Inner-most router
	innerRouter := core.NewRouter()
	innerRouter.GET("", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"level": "inner"})
	})

	// Middle router
	middleRouter := core.NewRouter()
	middleRouter.Mount("/inner", innerRouter)

	// App with outer mount
	app := core.New()
	app.Mount("/api/middle", middleRouter)

	server := httptest.NewServer(http.HandlerFunc(app.ServeHTTP))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/middle/inner")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for nested mount, got %d", resp.StatusCode)
	}
}

func TestRouterHTTPMethods(t *testing.T) {
	r := core.NewRouter()

	r.GET("/resource", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"method": "GET"})
	})

	r.POST("/resource", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"method": "POST"})
	})

	r.PUT("/resource", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"method": "PUT"})
	})

	r.PATCH("/resource", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"method": "PATCH"})
	})

	r.DELETE("/resource", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"method": "DELETE"})
	})

	r.OPTIONS("/resource", func(c *core.Context) core.Response {
		return c.NoContent()
	})

	r.HEAD("/resource", func(c *core.Context) core.Response {
		return c.NoContent()
	})

	app := core.New()
	app.Mount("", r)

	server := httptest.NewServer(http.HandlerFunc(app.ServeHTTP))
	defer server.Close()

	tests := []struct {
		method string
		path   string
		code   int
	}{
		{http.MethodGet, "/resource", http.StatusOK},
		{http.MethodPost, "/resource", http.StatusOK},
		{http.MethodPut, "/resource", http.StatusOK},
		{http.MethodPatch, "/resource", http.StatusOK},
		{http.MethodDelete, "/resource", http.StatusOK},
		{http.MethodOptions, "/resource", http.StatusNoContent},
		{http.MethodHead, "/resource", http.StatusNoContent},
	}

	for _, test := range tests {
		req, _ := http.NewRequest(test.method, server.URL+test.path, nil)
		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != test.code {
			t.Errorf("%s %s: expected %d, got %d", test.method, test.path, test.code, resp.StatusCode)
		}
	}
}

func TestRouterAny(t *testing.T) {
	r := core.NewRouter()

	r.Any("/multi", func(c *core.Context) core.Response {
		return c.JSON(map[string]string{"matched": "any"})
	})

	app := core.New()
	app.Mount("", r)

	server := httptest.NewServer(http.HandlerFunc(app.ServeHTTP))
	defer server.Close()

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
	}

	for _, method := range methods {
		req, _ := http.NewRequest(method, server.URL+"/multi", nil)
		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s /multi: expected 200, got %d", method, resp.StatusCode)
		}
	}
}

func TestRouterChaining(t *testing.T) {
	r := core.NewRouter()

	// Test fluent API - should return *Router for chaining
	result := r.
		GET("/a", func(c *core.Context) core.Response { return c.JSON(map[string]string{"status": "ok"}) }).
		POST("/b", func(c *core.Context) core.Response { return c.JSON(map[string]string{"status": "ok"}) }).
		PUT("/c", func(c *core.Context) core.Response { return c.JSON(map[string]string{"status": "ok"}) })

	if result != r {
		t.Error("Route registration methods should return *Router for chaining")
	}

	app := core.New()
	app.Mount("", r)

	server := httptest.NewServer(http.HandlerFunc(app.ServeHTTP))
	defer server.Close()

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/a"},
		{http.MethodPost, "/b"},
		{http.MethodPut, "/c"},
	}
	for _, tt := range tests {
		req, _ := http.NewRequest(tt.method, server.URL+tt.path, nil)
		resp, _ := http.DefaultClient.Do(req)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s %s: expected 200, got %d", tt.method, tt.path, resp.StatusCode)
		}
	}
}

func TestRouterParams(t *testing.T) {
	r := core.NewRouter()

	r.GET("/users/:id/posts/:postId", func(c *core.Context) core.Response {
		userId := c.Param("id")
		postId := c.Param("postId")
		return c.JSON(map[string]string{
			"userId": userId,
			"postId": postId,
		})
	})

	app := core.New()
	app.Mount("", r)

	server := httptest.NewServer(http.HandlerFunc(app.ServeHTTP))
	defer server.Close()

	resp, _ := http.Get(server.URL + "/users/123/posts/456")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}
