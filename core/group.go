package core

import "strings"

// Group represents a route group with a common prefix and middleware
type Group struct {
	prefix      string
	middlewares []Middleware
	app         *App
}

// Use adds middleware to the group
func (g *Group) Use(middlewares ...Middleware) {
	g.middlewares = append(g.middlewares, middlewares...)
}

// Group creates a sub-group with an additional prefix
func (g *Group) Group(prefix string, middlewares ...Middleware) *Group {
	return &Group{
		prefix:      g.prefix + prefix,
		middlewares: append(append([]Middleware{}, g.middlewares...), middlewares...),
		app:         g.app,
	}
}

// GET registers a GET route
func (g *Group) GET(path string, handlers ...Handler) {
	g.handle("GET", path, handlers...)
}

// POST registers a POST route
func (g *Group) POST(path string, handlers ...Handler) {
	g.handle("POST", path, handlers...)
}

// PUT registers a PUT route
func (g *Group) PUT(path string, handlers ...Handler) {
	g.handle("PUT", path, handlers...)
}

// PATCH registers a PATCH route
func (g *Group) PATCH(path string, handlers ...Handler) {
	g.handle("PATCH", path, handlers...)
}

// DELETE registers a DELETE route
func (g *Group) DELETE(path string, handlers ...Handler) {
	g.handle("DELETE", path, handlers...)
}

// OPTIONS registers an OPTIONS route
func (g *Group) OPTIONS(path string, handlers ...Handler) {
	g.handle("OPTIONS", path, handlers...)
}

// HEAD registers a HEAD route
func (g *Group) HEAD(path string, handlers ...Handler) {
	g.handle("HEAD", path, handlers...)
}

// Any registers a route for all HTTP methods
func (g *Group) Any(path string, handlers ...Handler) {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}
	for _, method := range methods {
		g.handle(method, path, handlers...)
	}
}

// handle registers a route with the given method
func (g *Group) handle(method, path string, handlers ...Handler) {
	if len(handlers) == 0 {
		panic("at least one handler is required")
	}

	// Build full path
	fullPath := g.buildPath(path)

	// Last handler is the actual handler, others are treated as route-level middleware
	var routeMiddlewares []Middleware
	var handler Handler

	if len(handlers) > 1 {
		// Convert all but last handler to middleware
		for i := 0; i < len(handlers)-1; i++ {
			h := handlers[i]
			routeMiddlewares = append(routeMiddlewares, func(c *Context, next Handler) Response {
				return h(c)
			})
		}
		handler = handlers[len(handlers)-1]
	} else {
		handler = handlers[0]
	}

	// Combine group middleware, route middleware, and handler
	allMiddlewares := append(append([]Middleware{}, g.middlewares...), routeMiddlewares...)
	finalHandler := Chain(allMiddlewares, handler)

	// Register with router
	g.app.router.Add(method, fullPath, finalHandler)
}

// buildPath constructs the full path from group prefix and route path
func (g *Group) buildPath(path string) string {
	if path == "" {
		if g.prefix == "" {
			return "/"
		}
		return g.prefix
	}

	// Ensure path starts with /
	if path[0] != '/' {
		path = "/" + path
	}

	fullPath := g.prefix + path

	// Remove trailing slash (except for root)
	if len(fullPath) > 1 && strings.HasSuffix(fullPath, "/") {
		fullPath = fullPath[:len(fullPath)-1]
	}

	return fullPath
}
