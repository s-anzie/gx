package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/quic-go/quic-go/http3"
)

// App is the core application instance
type App struct {
	router      *Router
	middlewares []Middleware
	tlsConfig   *tls.Config
	certFile    string
	keyFile     string

	// Server instances
	server     *http.Server
	h3Server   *http3.Server
	altSvcShim *http.Server

	// Configuration
	readTimeout     time.Duration
	writeTimeout    time.Duration
	shutdownTimeout time.Duration
}

// New creates a new App instance
func New() *App {
	return &App{
		router:          NewRouter(),
		middlewares:     []Middleware{},
		readTimeout:     15 * time.Second,
		writeTimeout:    15 * time.Second,
		shutdownTimeout: 10 * time.Second,
	}
}

// Use adds global middleware to the application
func (a *App) Use(middlewares ...Middleware) {
	a.middlewares = append(a.middlewares, middlewares...)
}

// Group creates a new route group
func (a *App) Group(prefix string, middlewares ...Middleware) *Group {
	return &Group{
		prefix:      prefix,
		middlewares: middlewares,
		app:         a,
	}
}

// Mount attaches a Router at the given prefix with optional mounting middleware
func (a *App) Mount(prefix string, router *Router, middlewares ...Middleware) {
	if prefix == "" {
		prefix = "/"
	}

	// Ensure prefix starts with /
	if prefix[0] != '/' {
		panic(fmt.Sprintf("mount prefix must start with '/': %s", prefix))
	}

	// For each method in the router's trees
	for method, root := range router.trees {
		// Mount the router's routes under the prefix
		a.mountNode(method, prefix, root, router.middlewares, middlewares)
	}
}

// mountNode recursively mounts a router's tree
func (a *App) mountNode(method, prefixPath string, node *node, routerMiddlewares []Middleware, mountMiddlewares []Middleware) {
	// Build the full path for this node
	fullPath := prefixPath
	if node.path != "" {
		if prefixPath == "/" {
			fullPath = "/" + node.path
		} else {
			fullPath = prefixPath + "/" + node.path
		}
	}

	// If this node has a handler, register it with global + mount + router middleware
	if node.handler != nil {
		handler := node.handler

		// Apply router-specific middleware
		for i := len(routerMiddlewares) - 1; i >= 0; i-- {
			m := routerMiddlewares[i]
			handler = applyMiddleware(m, handler)
		}

		// Apply mount-specific middleware
		for i := len(mountMiddlewares) - 1; i >= 0; i-- {
			m := mountMiddlewares[i]
			handler = applyMiddleware(m, handler)
		}

		a.handle(method, fullPath, handler)
	}

	// Recursively mount children
	for _, child := range node.children {
		a.mountNode(method, fullPath, child, routerMiddlewares, mountMiddlewares)
	}
}

// GET registers a GET route
func (a *App) GET(path string, handlers ...Handler) {
	a.handle("GET", path, handlers...)
}

// POST registers a POST route
func (a *App) POST(path string, handlers ...Handler) {
	a.handle("POST", path, handlers...)
}

// PUT registers a PUT route
func (a *App) PUT(path string, handlers ...Handler) {
	a.handle("PUT", path, handlers...)
}

// PATCH registers a PATCH route
func (a *App) PATCH(path string, handlers ...Handler) {
	a.handle("PATCH", path, handlers...)
}

// DELETE registers a DELETE route
func (a *App) DELETE(path string, handlers ...Handler) {
	a.handle("DELETE", path, handlers...)
}

// OPTIONS registers an OPTIONS route
func (a *App) OPTIONS(path string, handlers ...Handler) {
	a.handle("OPTIONS", path, handlers...)
}

// HEAD registers a HEAD route
func (a *App) HEAD(path string, handlers ...Handler) {
	a.handle("HEAD", path, handlers...)
}

// Any registers a route for all HTTP methods
func (a *App) Any(path string, handlers ...Handler) {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}
	for _, method := range methods {
		a.handle(method, path, handlers...)
	}
}

// handle is the internal method to register routes.
// Global middleware is NOT baked in here — it is applied dynamically in ServeHTTP.
// Only route-level middleware (extra handlers) is composed here.
func (a *App) handle(method, path string, handlers ...Handler) {
	if len(handlers) == 0 {
		panic("at least one handler is required")
	}

	var handler Handler

	if len(handlers) > 1 {
		// All but last are route-level middleware handlers
		routeMiddlewares := make([]Middleware, 0, len(handlers)-1)
		for _, h := range handlers[:len(handlers)-1] {
			h := h // capture
			routeMiddlewares = append(routeMiddlewares, func(c *Context, next Handler) Response {
				return h(c)
			})
		}
		handler = Chain(routeMiddlewares, handlers[len(handlers)-1])
	} else {
		handler = handlers[0]
	}

	// Register with router (global middleware applied at request time, not here)
	a.router.Add(method, path, handler)
}

// ServeHTTP implements http.Handler interface.
// Global middleware is applied here, allowing it to be registered after routes.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Acquire context from pool
	c := acquireContext(w, r, a)
	defer releaseContext(c)

	// Find handler for this route
	routeHandler, params := a.router.Find(r.Method, r.URL.Path)

	// Set params
	c.params = params

	// Handle 404
	if routeHandler == nil {
		http.NotFound(w, r)
		return
	}

	// Apply global middleware dynamically — allows Use() to work after route registration
	handler := Chain(a.middlewares, routeHandler)

	// Execute handler and get response
	res := handler(c)

	// Write response using the context's Writer (may have been wrapped by middleware)
	if res != nil {
		if err := res.Write(c.Writer); err != nil {
			log.Printf("Error writing response: %v", err)
		}
	}

	// Close the writer if it implements io.Closer (e.g., compression middleware)
	if closer, ok := c.Writer.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			log.Printf("Error closing writer: %v", err)
		}
	}
}

// Listen starts the HTTP server on the given address
func (a *App) Listen(addr string) error {
	a.server = &http.Server{
		Addr:         addr,
		Handler:      a,
		ReadTimeout:  a.readTimeout,
		WriteTimeout: a.writeTimeout,
		TLSConfig:    a.tlsConfig,
	}

	log.Printf("Server listening on %s", addr)

	// Start server
	if a.tlsConfig != nil || (a.certFile != "" && a.keyFile != "") {
		return a.server.ListenAndServeTLS(a.certFile, a.keyFile)
	}

	return a.server.ListenAndServe()
}

// ListenWithGracefulShutdown starts the server and handles graceful shutdown
func (a *App) ListenWithGracefulShutdown(addr string) error {
	a.server = &http.Server{
		Addr:         addr,
		Handler:      a,
		ReadTimeout:  a.readTimeout,
		WriteTimeout: a.writeTimeout,
		TLSConfig:    a.tlsConfig,
	}

	// Channel to listen for errors from the server
	serverErrors := make(chan error, 1)

	// Start server in a goroutine
	go func() {
		log.Printf("Server listening on %s", addr)

		if a.tlsConfig != nil || (a.certFile != "" && a.keyFile != "") {
			serverErrors <- a.server.ListenAndServeTLS(a.certFile, a.keyFile)
		} else {
			serverErrors <- a.server.ListenAndServe()
		}
	}()

	// Channel to listen for interrupt signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Block until we receive a signal or server error
	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		log.Printf("Received signal %v, shutting down gracefully...", sig)

		// Create context with timeout for shutdown
		ctx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
		defer cancel()

		// Attempt graceful shutdown
		if err := a.server.Shutdown(ctx); err != nil {
			// Force close if graceful shutdown fails
			a.server.Close()
			return fmt.Errorf("could not gracefully shutdown server: %w", err)
		}

		log.Println("Server stopped")
	}

	return nil
}

// Shutdown gracefully shuts down the server
func (a *App) Shutdown(ctx context.Context) error {
	var shutdownErr error

	// Shutdown HTTP/1.1/2 server if running
	if a.server != nil {
		if err := a.server.Shutdown(ctx); err != nil {
			shutdownErr = fmt.Errorf("HTTP server shutdown error: %w", err)
		}
	}

	// Shutdown HTTP/3 server if running
	if a.h3Server != nil {
		if err := a.h3Server.Close(); err != nil {
			if shutdownErr == nil {
				shutdownErr = fmt.Errorf("HTTP/3 server shutdown error: %w", err)
			} else {
				shutdownErr = fmt.Errorf("%v; HTTP/3 server shutdown error: %w", shutdownErr, err)
			}
		}
	}

	// Shutdown Alt-Svc shim if running
	if a.altSvcShim != nil {
		if err := a.altSvcShim.Shutdown(ctx); err != nil {
			if shutdownErr == nil {
				shutdownErr = fmt.Errorf("Alt-Svc shim shutdown error: %w", err)
			} else {
				shutdownErr = fmt.Errorf("%v; Alt-Svc shim shutdown error: %w", shutdownErr, err)
			}
		}
	}

	return shutdownErr
}

// ── Configuration Methods ────────────────────────────────────────────────────

// WithTLS configures TLS using certificate and key files
func (a *App) WithTLS(certFile, keyFile string) *App {
	a.certFile = certFile
	a.keyFile = keyFile
	return a
}

// WithTLSConfig configures TLS using a tls.Config
func (a *App) WithTLSConfig(config *tls.Config) *App {
	a.tlsConfig = config
	return a
}

// WithReadTimeout sets the read timeout for the server
func (a *App) WithReadTimeout(timeout time.Duration) *App {
	a.readTimeout = timeout
	return a
}

// WithWriteTimeout sets the write timeout for the server
func (a *App) WithWriteTimeout(timeout time.Duration) *App {
	a.writeTimeout = timeout
	return a
}

// WithShutdownTimeout sets the graceful shutdown timeout
func (a *App) WithShutdownTimeout(timeout time.Duration) *App {
	a.shutdownTimeout = timeout
	return a
}

// ── HTTP/3 Support ───────────────────────────────────────────────────────────

// H3Config holds HTTP/3 configuration options
type H3Config struct {
	altSvcShim   bool
	altSvcMaxAge time.Duration
}

// H3Option is a functional option for HTTP/3 configuration
type H3Option func(*H3Config)

// defaultH3Config returns the default HTTP/3 configuration
func defaultH3Config() *H3Config {
	return &H3Config{
		altSvcShim:   true,
		altSvcMaxAge: 24 * time.Hour, // 86400 seconds
	}
}

// WithoutAltSvcShim disables the Alt-Svc bootstrap shim
// Use this for internal microservices that don't need HTTP/3 discovery
func WithoutAltSvcShim() H3Option {
	return func(cfg *H3Config) {
		cfg.altSvcShim = false
	}
}

// AltSvcMaxAge sets the max-age directive for the Alt-Svc header
func AltSvcMaxAge(duration time.Duration) H3Option {
	return func(cfg *H3Config) {
		cfg.altSvcMaxAge = duration
	}
}

// ListenH3 starts an HTTP/3 server with optional Alt-Svc bootstrap shim
// The shim is a TCP server that announces HTTP/3 availability via Alt-Svc header
func (a *App) ListenH3(addr string, opts ...H3Option) error {
	// Apply configuration options
	cfg := defaultH3Config()
	for _, opt := range opts {
		opt(cfg)
	}

	// Ensure TLS is configured
	if a.tlsConfig == nil && (a.certFile == "" || a.keyFile == "") {
		return fmt.Errorf("HTTP/3 requires TLS configuration")
	}

	// Create HTTP/3 server
	a.h3Server = &http3.Server{
		Addr:      addr,
		Handler:   a,
		TLSConfig: a.tlsConfig,
	}

	// Start Alt-Svc shim if enabled
	if cfg.altSvcShim {
		go a.startAltSvcShim(addr, cfg.altSvcMaxAge)
	}

	log.Printf("HTTP/3 server listening on %s", addr)

	// Start HTTP/3 server
	if a.certFile != "" && a.keyFile != "" {
		return a.h3Server.ListenAndServeTLS(a.certFile, a.keyFile)
	}

	return a.h3Server.ListenAndServe()
}

// startAltSvcShim starts a TCP server that announces HTTP/3 via Alt-Svc header
// and redirects to the same URL. This enables HTTP/3 discovery for new clients.
func (a *App) startAltSvcShim(addr string, maxAge time.Duration) {
	altSvc := fmt.Sprintf(`h3="%s"; ma=%.0f`, addr, maxAge.Seconds())

	a.altSvcShim = &http.Server{
		Addr:         addr,
		TLSConfig:    a.tlsConfig,
		ReadTimeout:  a.readTimeout,
		WriteTimeout: a.writeTimeout,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Announce HTTP/3 availability via Alt-Svc header
			w.Header().Set("Alt-Svc", altSvc)

			// Return a simple response - no redirect needed
			// Modern clients will automatically use HTTP/3 when they see the Alt-Svc header
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("HTTP/3 available via Alt-Svc header"))
		}),
	}

	log.Printf("Alt-Svc shim listening on %s (TCP)", addr)

	// Start shim - errors are non-fatal
	var err error
	if a.certFile != "" && a.keyFile != "" {
		err = a.altSvcShim.ListenAndServeTLS(a.certFile, a.keyFile)
	} else if a.tlsConfig != nil {
		// Use TLS listener for in-memory certificates
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			log.Printf("Alt-Svc shim error (non-fatal): failed to listen: %v", err)
			return
		}

		tlsListener := tls.NewListener(listener, a.tlsConfig)
		err = a.altSvcShim.Serve(tlsListener)
	}

	if err != nil && err != http.ErrServerClosed {
		log.Printf("Alt-Svc shim error (non-fatal): %v", err)
	}
}

// ListenH3WithGracefulShutdown starts HTTP/3 server with graceful shutdown handling
func (a *App) ListenH3WithGracefulShutdown(addr string, opts ...H3Option) error {
	// Apply configuration options
	cfg := defaultH3Config()
	for _, opt := range opts {
		opt(cfg)
	}

	// Ensure TLS is configured
	if a.tlsConfig == nil && (a.certFile == "" || a.keyFile == "") {
		return fmt.Errorf("HTTP/3 requires TLS configuration")
	}

	// Create HTTP/3 server
	a.h3Server = &http3.Server{
		Addr:      addr,
		Handler:   a,
		TLSConfig: a.tlsConfig,
	}

	// Start Alt-Svc shim if enabled
	if cfg.altSvcShim {
		go a.startAltSvcShim(addr, cfg.altSvcMaxAge)
	}

	// Channel to listen for errors from the server
	serverErrors := make(chan error, 1)

	// Start server in a goroutine
	go func() {
		log.Printf("HTTP/3 server listening on %s", addr)

		if a.certFile != "" && a.keyFile != "" {
			serverErrors <- a.h3Server.ListenAndServeTLS(a.certFile, a.keyFile)
		} else {
			serverErrors <- a.h3Server.ListenAndServe()
		}
	}()

	// Channel to listen for interrupt signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Block until we receive a signal or server error
	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		log.Printf("Received signal %v, shutting down gracefully...", sig)

		// Create context with timeout for shutdown
		ctx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
		defer cancel()

		// Shutdown HTTP/3 server
		if err := a.h3Server.Close(); err != nil {
			log.Printf("Error closing HTTP/3 server: %v", err)
		}

		// Shutdown Alt-Svc shim if running
		if a.altSvcShim != nil {
			if err := a.altSvcShim.Shutdown(ctx); err != nil {
				log.Printf("Error shutting down Alt-Svc shim: %v", err)
			}
		}

		log.Println("Server stopped")
	}

	return nil
}
