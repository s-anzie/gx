package gx

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/s-anzie/gx/core"
)

// Public type aliases for easier API usage
type Response = core.Response

// Environment represents the application environment
type Environment string

const (
	Development Environment = "development"
	Staging     Environment = "staging"
	Production  Environment = "production"
)

// RouteContract stores a route with its contract
type RouteContract struct {
	Method   string
	Path     string
	Contract Contract
}

// App is the GX application instance that extends core.App
type App struct {
	*core.App

	// GX-specific fields
	environment   Environment
	pluginEntries []pluginEntry
	bootHooks     []BootHook
	shutdown      []ShutdownHook
	healthChecks  map[string]*HealthCheck
	observability *observability

	// Channel support
	channelRoutes      map[string]ChannelRoute
	migrationCallbacks []func(MigrateEvent)

	// Contract storage for OpenAPI generation
	contracts []RouteContract

	// Configuration
	maxBodySize    int64
	trustedProxies []string
	enableTracing  bool
	enableMetrics  bool
	enableLogs     bool
	ignoreEnv      bool

	// Lazy middleware setup (sorted by priority on first use)
	middlewareOnce sync.Once
}

// BootHook is a function called during application boot
type BootHook func(context.Context) error

// ShutdownHook is a function called during application shutdown
type ShutdownHook func(context.Context) error

// New creates a new GX application with the given options
func New(opts ...Option) *App {
	app := &App{
		App:          core.New(),
		environment:  Development,
		healthChecks: make(map[string]*HealthCheck),
		maxBodySize:  10 << 20, // 10 MB default
	}

	// Apply options
	for _, opt := range opts {
		opt(app)
	}

	// Apply environment variable overrides (after options, lower priority)
	app.applyEnvConfig()

	return app
}

// Option is a functional option for configuring the GX App
type Option func(*App)

// ── Environment Options ──────────────────────────────────────────────────────

// WithEnvironment sets the application environment
func WithEnvironment(env Environment) Option {
	return func(app *App) {
		app.environment = env
	}
}

// IgnoreEnv disables reading of GX_* environment variables.
// When set, only explicit Go options determine the configuration.
func IgnoreEnv() Option {
	return func(app *App) {
		app.ignoreEnv = true
	}
}

// Environment returns the current application environment
func (app *App) Environment() Environment {
	return app.environment
}

// IsDevelopment returns true if the app is in development mode
func (app *App) IsDevelopment() bool {
	return app.environment == Development
}

// IsProduction returns true if the app is in production mode
func (app *App) IsProduction() bool {
	return app.environment == Production
}

// ── Protocol Options ─────────────────────────────────────────────────────────

// WithHTTP2 enables HTTP/2 support (enabled by default with TLS)
func WithHTTP2() Option {
	return func(app *App) {
		// HTTP/2 is automatically enabled by Go's stdlib when TLS is present
		// This is a no-op placeholder for API consistency
	}
}

// WithHTTP3 enables HTTP/3 support (QUIC)
func WithHTTP3() Option {
	return func(app *App) {
		app.EnableHTTP3()
	}
}

// WithTLS configures TLS using certificate and key files
func WithTLS(certFile, keyFile string) Option {
	return func(app *App) {
		app.App.WithTLS(certFile, keyFile)
	}
}

// WithDevTLS configures development TLS with self-signed certificate
func WithDevTLS(hosts ...string) Option {
	return func(app *App) {
		tlsConfig, err := core.GenerateDevTLS(hosts...)
		if err != nil {
			panic("failed to generate dev TLS: " + err.Error())
		}
		app.App.WithTLSConfig(tlsConfig)
	}
}

// ── Timeout Options ──────────────────────────────────────────────────────────

// ReadTimeout sets the maximum duration for reading the entire request
func ReadTimeout(timeout time.Duration) Option {
	return func(app *App) {
		app.App.WithReadTimeout(timeout)
	}
}

// WriteTimeout sets the maximum duration before timing out writes of the response
func WriteTimeout(timeout time.Duration) Option {
	return func(app *App) {
		app.App.WithWriteTimeout(timeout)
	}
}

// ShutdownTimeout sets the maximum duration for graceful shutdown
func ShutdownTimeout(timeout time.Duration) Option {
	return func(app *App) {
		app.App.WithShutdownTimeout(timeout)
	}
}

// ── Security Options ─────────────────────────────────────────────────────────

// TrustedProxies sets the list of trusted proxy IP addresses/CIDR ranges
func TrustedProxies(proxies ...string) Option {
	return func(app *App) {
		app.trustedProxies = proxies
	}
}

// MaxBodySize sets the maximum allowed size for request bodies
func MaxBodySize(size int64) Option {
	return func(app *App) {
		app.maxBodySize = size
	}
}

// ── Observability Options ────────────────────────────────────────────────────

// WithTracing enables distributed tracing with OpenTelemetry
func WithTracing(serviceName string, opts ...TracingOption) Option {
	return func(app *App) {
		app.enableTracing = true
		if app.observability == nil {
			app.observability = newObservability()
		}
		// Apply tracing config options
		cfg := &tracingConfig{serviceName: serviceName, samplingRate: 1.0}
		for _, opt := range opts {
			opt(cfg)
		}
		// Tracing will be initialized with real OTEL implementation in the future
	}
}

// WithMetrics enables Prometheus metrics
func WithMetrics(opts ...MetricsOption) Option {
	return func(app *App) {
		app.enableMetrics = true
		if app.observability == nil {
			app.observability = newObservability()
		}
		// Apply metrics config options
		cfg := &metricsConfig{}
		for _, opt := range opts {
			opt(cfg)
		}
		app.observability.initializeMetrics()
	}
}

// WithStructuredLogs enables structured logging
func WithStructuredLogs(opts ...LogOption) Option {
	return func(app *App) {
		app.enableLogs = true
		if app.observability == nil {
			app.observability = newObservability()
		}
		// Apply logging config options
		cfg := &logConfig{
			level:  app.observability.logLevel,
			format: app.observability.logFormat,
			output: nil,
		}
		for _, opt := range opts {
			opt(cfg)
		}
		app.observability.logLevel = cfg.level
		app.observability.logFormat = cfg.format
	}
}

// ── Plugin Management ────────────────────────────────────────────────────────

// Install adds a plugin to the application with optional install options.
//
//	app.Install(auth.New())
//	app.Install(recovery.New(), gx.PluginPriority(1000))  // execute first
func (app *App) Install(plugin Plugin, opts ...PluginInstallOption) *App {
	entry := pluginEntry{plugin: plugin}
	for _, opt := range opts {
		opt(&entry)
	}
	app.pluginEntries = append(app.pluginEntries, entry)
	return app
}

// plugins returns the ordered plugin list (by priority desc, then insertion order)
func (app *App) plugins() []Plugin {
	sorted := make([]pluginEntry, len(app.pluginEntries))
	copy(sorted, app.pluginEntries)
	sortPluginEntries(sorted)

	result := make([]Plugin, len(sorted))
	for i, e := range sorted {
		result[i] = e.plugin
	}
	return result
}

// ensureMiddlewares registers RequestPlugin middlewares in priority order.
// Called lazily on first ServeHTTP or explicitly during Boot.
func (app *App) ensureMiddlewares() {
	app.middlewareOnce.Do(func() {
		sorted := make([]pluginEntry, len(app.pluginEntries))
		copy(sorted, app.pluginEntries)
		sortPluginEntries(sorted)

		for _, entry := range sorted {
			if reqPlugin, ok := entry.plugin.(RequestPlugin); ok {
				p := reqPlugin // capture loop variable
				app.App.Use(func(c *core.Context, next core.Handler) core.Response {
					gxCtx := &Context{Context: c, gxApp: app}
					return p.OnRequest(gxCtx, next)
				})
			}
		}
	})
}

// ServeHTTP implements http.Handler, ensuring plugin middlewares are registered
// in priority order before the first request is processed.
func (app *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	app.ensureMiddlewares()
	app.App.ServeHTTP(w, r)
}

// ── Lifecycle Management ─────────────────────────────────────────────────────

// OnBoot registers a hook to be called during application boot
func (app *App) OnBoot(hook BootHook) {
	app.bootHooks = append(app.bootHooks, hook)
}

// OnShutdown registers a hook to be called during application shutdown
func (app *App) OnShutdown(hook ShutdownHook) {
	app.shutdown = append(app.shutdown, hook)
}

// Boot executes all boot hooks in order
func (app *App) Boot(ctx context.Context) error {
	// Register plugin middlewares in priority order (no-op if already done)
	app.ensureMiddlewares()

	orderedPlugins := app.plugins()

	// Boot plugins
	for _, plugin := range orderedPlugins {
		if err := plugin.OnBoot(app); err != nil {
			return err
		}
	}

	// Then execute user boot hooks
	for _, hook := range app.bootHooks {
		if err := hook(ctx); err != nil {
			return err
		}
	}

	return nil
}

// ShutdownGracefully executes all shutdown hooks in reverse order
func (app *App) ShutdownGracefully(ctx context.Context) error {
	// Shutdown user hooks first (reverse order)
	for i := len(app.shutdown) - 1; i >= 0; i-- {
		if err := app.shutdown[i](ctx); err != nil {
			return err
		}
	}

	// Then shutdown plugins in reverse priority order
	orderedPlugins := app.plugins()
	for i := len(orderedPlugins) - 1; i >= 0; i-- {
		if err := orderedPlugins[i].OnShutdown(ctx); err != nil {
			return err
		}
	}

	// Finally shutdown the core app
	return app.App.Shutdown(ctx)
}

// ── Contract-Aware Route Registration ───────────────────────────────────────

// GET registers a GET route with optional contract
func (app *App) GET(path string, contractOrHandler any, handlers ...any) {
	app.handleWithContract("GET", path, contractOrHandler, handlers)
}

// POST registers a POST route with optional contract
func (app *App) POST(path string, contractOrHandler any, handlers ...any) {
	app.handleWithContract("POST", path, contractOrHandler, handlers)
}

// PUT registers a PUT route with optional contract
func (app *App) PUT(path string, contractOrHandler any, handlers ...any) {
	app.handleWithContract("PUT", path, contractOrHandler, handlers)
}

// PATCH registers a PATCH route with optional contract
func (app *App) PATCH(path string, contractOrHandler any, handlers ...any) {
	app.handleWithContract("PATCH", path, contractOrHandler, handlers)
}

// DELETE registers a DELETE route with optional contract
func (app *App) DELETE(path string, contractOrHandler any, handlers ...any) {
	app.handleWithContract("DELETE", path, contractOrHandler, handlers)
}

// handleWithContract handles route registration with optional contract
func (app *App) handleWithContract(method, path string, contractOrHandler any, handlers []any) {
	var contract *Contract
	var coreHandlers []core.Handler

	// Check if first argument is a Contract
	if c, ok := contractOrHandler.(Contract); ok {
		// Contract provided - next handlers are gx.Context handlers
		contract = &c

		// Convert handlers to core handlers
		for _, h := range handlers {
			if gxHandler, ok := h.(func(*Context) core.Response); ok {
				// Wrap gx.Context handler as core.Handler
				coreHandlers = append(coreHandlers, func(c *core.Context) core.Response {
					return gxHandler(&Context{Context: c, gxApp: app})
				})
			} else if coreHandler, ok := h.(core.Handler); ok {
				coreHandlers = append(coreHandlers, coreHandler)
			}
		}
	} else if h, ok := contractOrHandler.(func(*Context) core.Response); ok {
		// No contract, just gx handler
		contract = nil
		coreHandlers = append(coreHandlers, func(c *core.Context) core.Response {
			return h(&Context{Context: c, gxApp: app})
		})
		// Add remaining handlers
		for _, handler := range handlers {
			if coreHandler, ok := handler.(core.Handler); ok {
				coreHandlers = append(coreHandlers, coreHandler)
			}
		}
	} else if h, ok := contractOrHandler.(func(*core.Context) core.Response); ok {
		// Core handler (unnamed function type)
		contract = nil
		coreHandlers = append(coreHandlers, core.Handler(h))
		// Add remaining handlers
		for _, handler := range handlers {
			if coreHandler, ok := handler.(func(*core.Context) core.Response); ok {
				coreHandlers = append(coreHandlers, core.Handler(coreHandler))
			} else if coreHandler, ok := handler.(core.Handler); ok {
				coreHandlers = append(coreHandlers, coreHandler)
			}
		}
	} else if h, ok := contractOrHandler.(core.Handler); ok {
		// Core handler (named type)
		contract = nil
		coreHandlers = append(coreHandlers, h)
		// Add remaining handlers
		for _, handler := range handlers {
			if coreHandler, ok := handler.(core.Handler); ok {
				coreHandlers = append(coreHandlers, coreHandler)
			}
		}
	}

	// Store contract if provided
	if contract != nil {
		app.contracts = append(app.contracts, RouteContract{
			Method:   method,
			Path:     path,
			Contract: *contract,
		})
	}

	// Register with core App
	switch method {
	case "GET":
		app.App.GET(path, coreHandlers...)
	case "POST":
		app.App.POST(path, coreHandlers...)
	case "PUT":
		app.App.PUT(path, coreHandlers...)
	case "PATCH":
		app.App.PATCH(path, coreHandlers...)
	case "DELETE":
		app.App.DELETE(path, coreHandlers...)
	}
}

// GetContracts returns all registered contracts
func (app *App) GetContracts() []RouteContract {
	return app.contracts
}
