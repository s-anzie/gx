package core

import (
	"fmt"
	"strings"
)

// Router manages HTTP routes and dispatches requests to handlers.
// It uses a radix-tree for efficient O(log n) lookups and can be mounted
// on another router or app for modular organization.
type Router struct {
	trees       map[string]*node // one tree per HTTP method
	middlewares []Middleware     // middleware specific to this router
	prefix      string           // prefix added by Mount
}

// node represents a node in the radix tree
type node struct {
	path      string  // the path segment of this node
	handler   Handler // handler to execute (nil for intermediate nodes)
	children  []*node // child nodes
	paramName string  // parameter name if this is a param node (:id)
	isParam   bool    // true if this node represents a parameter
	isWild    bool    // true if this is a wildcard node (*path)
	priority  int     // priority for ordering (static > param > wildcard)
}

// nodeType represents the priority of node types
const (
	staticNode   = 0 // static path segment
	paramNode    = 1 // parameter segment (:id)
	wildcardNode = 2 // wildcard segment (*path)
)

// NewRouter creates a new standalone Router instance
func NewRouter() *Router {
	return &Router{
		trees:       make(map[string]*node),
		middlewares: []Middleware{},
	}
}

// Add registers a handler for a given HTTP method and path
func (r *Router) Add(method, path string, handler Handler) {
	if path == "" {
		path = "/"
	}

	// Ensure path starts with /
	if path[0] != '/' {
		panic(fmt.Sprintf("path must start with '/': %s", path))
	}

	// Get or create root node for this method
	root := r.trees[method]
	if root == nil {
		root = &node{}
		r.trees[method] = root
	}

	// Add the route to the tree
	root.addRoute(path, handler)
}

// Find looks up a handler for the given method and path
func (r *Router) Find(method, path string) (Handler, Params) {
	root := r.trees[method]
	if root == nil {
		return nil, nil
	}

	params := make(Params)
	handler := root.findRoute(path, params)
	return handler, params
}

// ── Middleware Management ────────────────────────────────────────────────────

// Use adds middleware to this router
func (r *Router) Use(middlewares ...Middleware) *Router {
	r.middlewares = append(r.middlewares, middlewares...)
	return r
}

// ── Route Registration ────────────────────────────────────────────────────────

// GET registers a GET route
func (r *Router) GET(path string, handler Handler) *Router {
	r.handle("GET", path, handler)
	return r
}

// POST registers a POST route
func (r *Router) POST(path string, handler Handler) *Router {
	r.handle("POST", path, handler)
	return r
}

// PUT registers a PUT route
func (r *Router) PUT(path string, handler Handler) *Router {
	r.handle("PUT", path, handler)
	return r
}

// PATCH registers a PATCH route
func (r *Router) PATCH(path string, handler Handler) *Router {
	r.handle("PATCH", path, handler)
	return r
}

// DELETE registers a DELETE route
func (r *Router) DELETE(path string, handler Handler) *Router {
	r.handle("DELETE", path, handler)
	return r
}

// OPTIONS registers an OPTIONS route
func (r *Router) OPTIONS(path string, handler Handler) *Router {
	r.handle("OPTIONS", path, handler)
	return r
}

// HEAD registers a HEAD route
func (r *Router) HEAD(path string, handler Handler) *Router {
	r.handle("HEAD", path, handler)
	return r
}

// Any registers a route for all HTTP methods
func (r *Router) Any(path string, handler Handler) *Router {
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
		r.handle(method, path, handler)
	}
	return r
}

// handle is the internal method to register routes with middleware wrapping
func (r *Router) handle(method, path string, handler Handler) {
	// Normalize path
	if path == "" {
		path = "/"
	}

	// Wrap handler with router-specific middleware
	wrappedHandler := handler

	// Apply router middleware in reverse order
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		m := r.middlewares[i]
		wrappedHandler = applyMiddleware(m, wrappedHandler)
	}

	r.Add(method, path, wrappedHandler)
}

// applyMiddleware is a helper to properly wrap a handler with middleware
func applyMiddleware(m Middleware, handler Handler) Handler {
	return func(c *Context) Response {
		return m(c, handler)
	}
}

// ── Router Mounting ──────────────────────────────────────────────────────────

// Mount attaches a sub-router at a given prefix
func (r *Router) Mount(prefix string, sub *Router, middlewares ...Middleware) *Router {
	if prefix == "" {
		prefix = "/"
	}

	// Ensure prefix starts with /
	if prefix[0] != '/' {
		panic(fmt.Sprintf("mount prefix must start with '/': %s", prefix))
	}

	// For each method in the sub-router's trees
	for method, subRoot := range sub.trees {
		// Mount the sub-router's routes under the prefix
		r.mountNode(method, prefix, subRoot, sub.middlewares, middlewares)
	}

	return r
}

// mountNode recursively mounts a sub-router's tree
func (r *Router) mountNode(method string, prefixPath string, node *node, subMiddlewares []Middleware, mountMiddlewares []Middleware) {
	// Build the full path for this node
	fullPath := prefixPath
	if node.path != "" {
		fullPath = prefixPath + "/" + node.path
	} else if prefixPath != "/" {
		// Keep the prefix path as-is
		fullPath = prefixPath
	}

	// If this node has a handler, register it with both middleware sets
	if node.handler != nil {
		handler := node.handler

		// Apply sub-router middleware (innermost)
		for i := len(subMiddlewares) - 1; i >= 0; i-- {
			handler = applyMiddleware(subMiddlewares[i], handler)
		}

		// Apply mount middleware (outermost)
		for i := len(mountMiddlewares) - 1; i >= 0; i-- {
			handler = applyMiddleware(mountMiddlewares[i], handler)
		}

		r.Add(method, fullPath, handler)
	}

	// Recursively mount children
	for _, child := range node.children {
		r.mountNode(method, fullPath, child, subMiddlewares, mountMiddlewares)
	}
}

// addRoute adds a route to the node tree
func (n *node) addRoute(path string, handler Handler) {
	// Base case: empty path means we've reached the end
	if path == "" || path == "/" {
		if n.handler != nil {
			panic(fmt.Sprintf("route already registered: %s", path))
		}
		n.handler = handler
		return
	}

	// Remove leading slash
	if path[0] == '/' {
		path = path[1:]
	}

	// Find the next segment
	segment, remaining := splitPath(path)

	// Determine node type
	nodeType, paramName := analyzeSegment(segment)

	// Try to find existing child with matching segment
	var child *node
	for _, c := range n.children {
		if c.isParam && nodeType == paramNode {
			// Conflict: two different param names at same position
			if c.paramName != paramName {
				panic(fmt.Sprintf("route conflict: cannot register :%s, already have :%s", paramName, c.paramName))
			}
			child = c
			break
		} else if c.isWild && nodeType == wildcardNode {
			child = c
			break
		} else if !c.isParam && !c.isWild && c.path == segment {
			child = c
			break
		}
	}

	// Create new child if not found
	if child == nil {
		child = &node{
			path:     segment,
			priority: nodeType,
		}

		if nodeType == paramNode {
			child.isParam = true
			child.paramName = paramName
		} else if nodeType == wildcardNode {
			child.isWild = true
			child.paramName = paramName
		}

		n.children = append(n.children, child)
		// Sort children by priority (static < param < wildcard)
		sortChildren(n.children)
	}

	// Recursively add the remaining path
	child.addRoute(remaining, handler)
}

// findRoute searches for a handler matching the path
func (n *node) findRoute(path string, params Params) Handler {
	// Remove leading slash
	if path != "" && path[0] == '/' {
		path = path[1:]
	}

	// Base case: path is empty, return handler if exists
	if path == "" {
		return n.handler
	}

	// Split path into segment and remaining
	segment, remaining := splitPath(path)

	// Try to match children in priority order
	for _, child := range n.children {
		if child.isWild {
			// Wildcard matches everything
			if child.paramName != "" {
				params[child.paramName] = path
			}
			return child.handler
		} else if child.isParam {
			// Parameter node matches any segment
			if child.paramName != "" {
				params[child.paramName] = segment
			}
			if handler := child.findRoute(remaining, params); handler != nil {
				return handler
			}
		} else if child.path == segment {
			// Static match
			if handler := child.findRoute(remaining, params); handler != nil {
				return handler
			}
		}
	}

	return nil
}

// splitPath splits a path into the first segment and the remaining path
func splitPath(path string) (segment, remaining string) {
	if path == "" {
		return "", ""
	}

	// Find the next slash
	idx := strings.IndexByte(path, '/')
	if idx < 0 {
		return path, ""
	}

	return path[:idx], path[idx:]
}

// analyzeSegment determines the type of a path segment
func analyzeSegment(segment string) (nodeType int, paramName string) {
	if segment == "" {
		return staticNode, ""
	}

	// Check for wildcard (*path)
	if segment[0] == '*' {
		return wildcardNode, segment[1:]
	}

	// Check for parameter (:id)
	if segment[0] == ':' {
		return paramNode, segment[1:]
	}

	return staticNode, ""
}

// sortChildren sorts child nodes by priority
func sortChildren(children []*node) {
	// Simple insertion sort (small arrays)
	for i := 1; i < len(children); i++ {
		for j := i; j > 0 && children[j].priority < children[j-1].priority; j-- {
			children[j], children[j-1] = children[j-1], children[j]
		}
	}
}
