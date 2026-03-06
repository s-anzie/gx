package core

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// Handler is the fundamental function signature for handling requests.
// It receives a Context and returns a Response.
type Handler func(*Context) Response

// Context provides access to request data and response builders.
// It is pooled via sync.Pool for zero allocation on the hot path.
type Context struct {
	// Raw access to underlying HTTP primitives
	Request *http.Request
	Writer  http.ResponseWriter

	// Internal state - not exported
	params   Params
	handlers []Handler
	index    int
	store    map[string]interface{}
	app      *App
	written  bool
}

var contextPool = sync.Pool{
	New: func() interface{} {
		return &Context{
			store: make(map[string]interface{}),
		}
	},
}

// acquireContext gets a Context from the pool
func acquireContext(w http.ResponseWriter, r *http.Request, app *App) *Context {
	c := contextPool.Get().(*Context)
	c.reset(w, r, app)
	return c
}

// releaseContext returns a Context to the pool
func releaseContext(c *Context) {
	c.reset(nil, nil, nil)
	contextPool.Put(c)
}

// reset prepares the context for reuse
func (c *Context) reset(w http.ResponseWriter, r *http.Request, app *App) {
	c.Writer = w
	c.Request = r
	c.app = app
	c.params = nil
	c.handlers = nil
	c.index = -1
	c.written = false

	// Clear the store map without reallocating
	for k := range c.store {
		delete(c.store, k)
	}
}

// ── Routing Parameters ───────────────────────────────────────────────────────

// Param returns the value of a URL parameter by name
func (c *Context) Param(name string) string {
	if c.params == nil {
		return ""
	}
	return c.params.Get(name)
}

// ── Query Parameters ─────────────────────────────────────────────────────────

// Query returns a query string parameter by name
func (c *Context) Query(name string) string {
	return c.Request.URL.Query().Get(name)
}

// QueryDefault returns a query string parameter with a default fallback
func (c *Context) QueryDefault(name, defaultValue string) string {
	value := c.Query(name)
	if value == "" {
		return defaultValue
	}
	return value
}

// ── Request Body ─────────────────────────────────────────────────────────────

// BindJSON deserializes the request body as JSON into the given value
func (c *Context) BindJSON(v interface{}) error {
	if c.Request.Body == nil {
		return fmt.Errorf("request body is empty")
	}
	defer c.Request.Body.Close()
	return json.NewDecoder(c.Request.Body).Decode(v)
}

// BindXML deserializes the request body as XML into the given value
func (c *Context) BindXML(v interface{}) error {
	if c.Request.Body == nil {
		return fmt.Errorf("request body is empty")
	}
	defer c.Request.Body.Close()
	return xml.NewDecoder(c.Request.Body).Decode(v)
}

// Body returns the raw request body as bytes
func (c *Context) Body() ([]byte, error) {
	if c.Request.Body == nil {
		return nil, nil
	}
	defer c.Request.Body.Close()
	return io.ReadAll(c.Request.Body)
}

// ── Form Values ──────────────────────────────────────────────────────────────

// FormValue returns a form field value
func (c *Context) FormValue(name string) string {
	return c.Request.FormValue(name)
}

// ── Request Metadata ─────────────────────────────────────────────────────────

// Header returns a request header value
func (c *Context) Header(name string) string {
	return c.Request.Header.Get(name)
}

// ClientIP extracts the real client IP, respecting X-Forwarded-For
func (c *Context) ClientIP() string {
	// Check X-Forwarded-For header first
	xff := c.Header("X-Forwarded-For")
	if xff != "" {
		// Take the first IP in the list
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	xri := c.Header("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return ip
}

// Method returns the HTTP method
func (c *Context) Method() string {
	return c.Request.Method
}

// Path returns the request path
func (c *Context) Path() string {
	return c.Request.URL.Path
}

// Proto returns the HTTP protocol version
func (c *Context) Proto() string {
	return c.Request.Proto
}

// IsHTTP2 returns true if the request is HTTP/2
func (c *Context) IsHTTP2() bool {
	return c.Request.ProtoMajor == 2
}

// IsHTTP3 returns true if the request is HTTP/3
func (c *Context) IsHTTP3() bool {
	return c.Request.ProtoMajor == 3
}

// ── Context Propagation ──────────────────────────────────────────────────────

// GoContext returns the underlying Go context for propagation to third-party libraries
func (c *Context) GoContext() context.Context {
	return c.Request.Context()
}

// ── Response Builders ────────────────────────────────────────────────────────

// JSON creates a JSON response with 200 status
func (c *Context) JSON(data interface{}) Response {
	return JSON(data)
}

// Created creates a JSON response with 201 status
func (c *Context) Created(data interface{}) Response {
	return Created(data)
}

// NoContent creates a 204 No Content response
func (c *Context) NoContent() Response {
	return NoContent()
}

// Text creates a plain text response
func (c *Context) Text(format string, args ...interface{}) Response {
	return Text(format, args...)
}

// HTML creates an HTML response
func (c *Context) HTML(html string) Response {
	return HTML(html)
}

// XML creates an XML response
func (c *Context) XML(data interface{}) Response {
	return XML(data)
}

// File creates a file response
func (c *Context) File(path string) Response {
	return File(path)
}

// Stream creates a streaming response
func (c *Context) Stream(contentType string, reader io.Reader) Response {
	return Stream(contentType, reader)
}

// Redirect creates a redirect response
func (c *Context) Redirect(url string) Response {
	return Redirect(url)
}

// ── Per-Request Store ────────────────────────────────────────────────────────

// Set stores a value in the context for the duration of the request
func (c *Context) Set(key string, value interface{}) {
	if c.store == nil {
		c.store = make(map[string]interface{})
	}
	c.store[key] = value
}

// Get retrieves a value from the context store
func (c *Context) Get(key string) (interface{}, bool) {
	if c.store == nil {
		return nil, false
	}
	value, exists := c.store[key]
	return value, exists
}

// MustGet retrieves a value from the context store, panicking if not found
func (c *Context) MustGet(key string) interface{} {
	value, exists := c.Get(key)
	if !exists {
		panic(fmt.Sprintf("key '%s' does not exist in context store", key))
	}
	return value
}

// ── Chain Control ────────────────────────────────────────────────────────────

// Next executes the next handler in the chain
func (c *Context) Next() Response {
	c.index++
	if c.index < len(c.handlers) {
		return c.handlers[c.index](c)
	}
	return nil
}

// Abort stops the handler chain without writing a response
func (c *Context) Abort() {
	c.index = len(c.handlers)
}

// AbortWithStatus stops the handler chain and writes a status code
func (c *Context) AbortWithStatus(code int) Response {
	c.Abort()
	return WithStatus(NoContent(), code)
}
