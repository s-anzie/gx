package gxtest

import (
	"net/http/httptest"
	"testing"

	"github.com/s-anzie/gx/core"
)

// TestBasicResponse tests basic response builders
func TestBasicResponse(t *testing.T) {
	tests := []struct {
		name           string
		response       core.Response
		expectedStatus int
	}{
		{
			name:           "JSON response",
			response:       core.JSON(map[string]string{"message": "hello"}),
			expectedStatus: 200,
		},
		{
			name:           "Created response",
			response:       core.Created(map[string]string{"id": "123"}),
			expectedStatus: 201,
		},
		{
			name:           "NoContent response",
			response:       core.NoContent(),
			expectedStatus: 204,
		},
		{
			name:           "Text response",
			response:       core.Text("Hello, %s!", "World"),
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.response.Status() != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, tt.response.Status())
			}
		})
	}
}

// TestChainableResponse tests chainable response modifications
func TestChainableResponse(t *testing.T) {
	res := core.JSON(map[string]string{"message": "hello"})
	res = core.WithStatus(res, 201)
	res = core.WithHeader(res, "X-Custom", "value")

	if res.Status() != 201 {
		t.Errorf("expected status 201, got %d", res.Status())
	}

	if res.Headers().Get("X-Custom") != "value" {
		t.Errorf("expected header X-Custom to be 'value', got '%s'", res.Headers().Get("X-Custom"))
	}
}

// TestRouter tests basic routing functionality
func TestRouter(t *testing.T) {
	router := core.NewRouter()

	handler := func(c *core.Context) core.Response {
		return core.JSON(map[string]string{"status": "ok"})
	}

	// Add routes
	router.Add("GET", "/users", handler)
	router.Add("GET", "/users/:id", handler)
	router.Add("GET", "/files/*path", handler)

	tests := []struct {
		name       string
		method     string
		path       string
		shouldFind bool
		params     map[string]string
	}{
		{
			name:       "static route",
			method:     "GET",
			path:       "/users",
			shouldFind: true,
			params:     map[string]string{},
		},
		{
			name:       "param route",
			method:     "GET",
			path:       "/users/123",
			shouldFind: true,
			params:     map[string]string{"id": "123"},
		},
		{
			name:       "wildcard route",
			method:     "GET",
			path:       "/files/a/b/c.txt",
			shouldFind: true,
			params:     map[string]string{"path": "a/b/c.txt"},
		},
		{
			name:       "not found",
			method:     "GET",
			path:       "/notfound",
			shouldFind: false,
			params:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, params := router.Find(tt.method, tt.path)

			if tt.shouldFind && h == nil {
				t.Errorf("expected to find handler for %s %s", tt.method, tt.path)
			}

			if !tt.shouldFind && h != nil {
				t.Errorf("expected not to find handler for %s %s", tt.method, tt.path)
			}

			if tt.shouldFind {
				for k, v := range tt.params {
					if params.Get(k) != v {
						t.Errorf("expected param %s to be %s, got %s", k, v, params.Get(k))
					}
				}
			}
		})
	}
}

// TestApp tests full application flow
func TestApp(t *testing.T) {
	app := core.New()

	app.GET("/hello", func(c *core.Context) core.Response {
		return core.JSON(map[string]string{"message": "hello"})
	})

	app.GET("/hello/:name", func(c *core.Context) core.Response {
		name := c.Param("name")
		return core.JSON(map[string]string{"message": "hello " + name})
	})

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "static route",
			method:         "GET",
			path:           "/hello",
			expectedStatus: 200,
			expectedBody:   `{"message":"hello"}`,
		},
		{
			name:           "param route",
			method:         "GET",
			path:           "/hello/world",
			expectedStatus: 200,
			expectedBody:   `{"message":"hello world"}`,
		},
		{
			name:           "not found",
			method:         "GET",
			path:           "/notfound",
			expectedStatus: 404,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			app.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedBody != "" {
				body := w.Body.String()
				// Remove newline that json encoder adds
				if len(body) > 0 && body[len(body)-1] == '\n' {
					body = body[:len(body)-1]
				}
				if body != tt.expectedBody {
					t.Errorf("expected body %s, got %s", tt.expectedBody, body)
				}
			}
		})
	}
}

// TestMiddleware tests middleware functionality
func TestMiddleware(t *testing.T) {
	app := core.New()

	// Global middleware that adds a header
	app.Use(func(c *core.Context, next core.Handler) core.Response {
		res := next(c)
		return core.WithHeader(res, "X-Global", "true")
	})

	app.GET("/test", func(c *core.Context) core.Response {
		return core.JSON(map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Header().Get("X-Global") != "true" {
		t.Errorf("expected X-Global header to be 'true', got '%s'", w.Header().Get("X-Global"))
	}
}

// TestGroups tests route grouping
func TestGroups(t *testing.T) {
	app := core.New()

	api := app.Group("/api")
	v1 := api.Group("/v1")

	v1.GET("/users", func(c *core.Context) core.Response {
		return core.JSON(map[string]string{"version": "v1"})
	})

	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
