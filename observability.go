package gx

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"
)

// ── Tracing ──────────────────────────────────────────────────────────────────

// Span represents a single operation within a trace
type Span interface {
	// SetAttr sets an attribute on the span
	SetAttr(key string, value any)

	// End completes the span
	End()

	// Context returns a context.Context with this span
	Context() context.Context
}

// Tracer creates and manages spans
type Tracer interface {
	// Start begins a new span with the given name
	Start(ctx context.Context, name string) Span

	// StartWithAttributes begins a new span with initial attributes
	StartWithAttributes(ctx context.Context, name string, attrs map[string]any) Span
}

// noopSpan is a no-op implementation of Span
type noopSpan struct {
	ctx context.Context
}

func (s *noopSpan) SetAttr(key string, value any) {}
func (s *noopSpan) End()                          {}
func (s *noopSpan) Context() context.Context      { return s.ctx }

// noopTracer is a no-op implementation of Tracer
type noopTracer struct{}

func (t *noopTracer) Start(ctx context.Context, name string) Span {
	return &noopSpan{ctx: ctx}
}

func (t *noopTracer) StartWithAttributes(ctx context.Context, name string, attrs map[string]any) Span {
	return &noopSpan{ctx: ctx}
}

// ── Metrics ──────────────────────────────────────────────────────────────────

// Counter is a monotonically increasing metric
type Counter interface {
	// Inc increments the counter by 1
	Inc()

	// Add increments the counter by the given value
	Add(value float64)

	// With returns a counter with the given label values
	With(labelValues ...string) Counter
}

// Histogram observes values and counts them in buckets
type Histogram interface {
	// Observe adds a single observation
	Observe(value float64)

	// With returns a histogram with the given label values
	With(labelValues ...string) Histogram
}

// Gauge is a metric that can go up and down
type Gauge interface {
	// Set sets the gauge to the given value
	Set(value float64)

	// Inc increments the gauge by 1
	Inc()

	// Dec decrements the gauge by 1
	Dec()

	// Add adds the given value to the gauge
	Add(value float64)

	// Sub subtracts the given value from the gauge
	Sub(value float64)

	// With returns a gauge with the given label values
	With(labelValues ...string) Gauge
}

// MetricsRegistry manages all metrics
type MetricsRegistry interface {
	// Counter creates or retrieves a counter
	Counter(name, help string, labels ...string) Counter

	// Histogram creates or retrieves a histogram
	Histogram(name, help string, labels ...string) Histogram

	// Gauge creates or retrieves a gauge
	Gauge(name, help string, labels ...string) Gauge
}

// noopCounter is a no-op implementation of Counter
type noopCounter struct{}

func (c *noopCounter) Inc()                               {}
func (c *noopCounter) Add(value float64)                  {}
func (c *noopCounter) With(labelValues ...string) Counter { return c }

// noopHistogram is a no-op implementation of Histogram
type noopHistogram struct{}

func (h *noopHistogram) Observe(value float64)                {}
func (h *noopHistogram) With(labelValues ...string) Histogram { return h }

// noopGauge is a no-op implementation of Gauge
type noopGauge struct{}

func (g *noopGauge) Set(value float64)                {}
func (g *noopGauge) Inc()                             {}
func (g *noopGauge) Dec()                             {}
func (g *noopGauge) Add(value float64)                {}
func (g *noopGauge) Sub(value float64)                {}
func (g *noopGauge) With(labelValues ...string) Gauge { return g }

// noopMetricsRegistry is a no-op implementation of MetricsRegistry
type noopMetricsRegistry struct{}

func (r *noopMetricsRegistry) Counter(name, help string, labels ...string) Counter {
	return &noopCounter{}
}

func (r *noopMetricsRegistry) Histogram(name, help string, labels ...string) Histogram {
	return &noopHistogram{}
}

func (r *noopMetricsRegistry) Gauge(name, help string, labels ...string) Gauge {
	return &noopGauge{}
}

// ── Logging ──────────────────────────────────────────────────────────────────

// Logger is a structured logger interface
type Logger interface {
	// Info logs an informational message
	Info(msg string, args ...any)

	// Warn logs a warning message
	Warn(msg string, args ...any)

	// Error logs an error message
	Error(msg string, args ...any)

	// Debug logs a debug message
	Debug(msg string, args ...any)

	// With returns a logger with the given attributes pre-populated
	With(args ...any) Logger
}

// slogLogger wraps slog.Logger to implement our Logger interface
type slogLogger struct {
	logger *slog.Logger
}

func (l *slogLogger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

func (l *slogLogger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

func (l *slogLogger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

func (l *slogLogger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

func (l *slogLogger) With(args ...any) Logger {
	return &slogLogger{logger: l.logger.With(args...)}
}

// ── Observability Configuration ──────────────────────────────────────────────

// LogFormat defines the log output format
type LogFormat string

const (
	LogText LogFormat = "text"
	LogJSON LogFormat = "json"
)

// observability holds the observability components
type observability struct {
	tracer          Tracer
	metricsRegistry MetricsRegistry
	logger          *slog.Logger
	logLevel        slog.Level
	logFormat       LogFormat

	// HTTP metrics
	requestsTotal   Counter
	requestDuration Histogram
	requestSize     Histogram
	responseSize    Histogram
	activeRequests  Gauge

	mu sync.RWMutex
}

// newObservability creates a new observability instance with no-op implementations
func newObservability() *observability {
	return &observability{
		tracer:          &noopTracer{},
		metricsRegistry: &noopMetricsRegistry{},
		logger:          slog.New(slog.NewTextHandler(os.Stdout, nil)),
		logLevel:        slog.LevelInfo,
		logFormat:       LogText,
	}
}

// initializeMetrics sets up the standard HTTP metrics
func (o *observability) initializeMetrics() {
	o.requestsTotal = o.metricsRegistry.Counter(
		"http_requests_total",
		"Total number of HTTP requests",
		"method", "route", "status", "protocol",
	)

	o.requestDuration = o.metricsRegistry.Histogram(
		"http_request_duration_seconds",
		"HTTP request duration in seconds",
		"method", "route", "protocol",
	)

	o.requestSize = o.metricsRegistry.Histogram(
		"http_request_size_bytes",
		"HTTP request size in bytes",
		"method", "route",
	)

	o.responseSize = o.metricsRegistry.Histogram(
		"http_response_size_bytes",
		"HTTP response size in bytes",
		"method", "route",
	)

	o.activeRequests = o.metricsRegistry.Gauge(
		"http_active_requests",
		"Number of active HTTP requests",
		"protocol",
	)
}

// ── Configuration Types ──────────────────────────────────────────────────────

// tracingConfig holds tracing configuration
type tracingConfig struct {
	serviceName  string
	endpoint     string
	samplingRate float64
}

// metricsConfig holds metrics configuration
type metricsConfig struct {
	prometheusAddr string
	buckets        []float64
}

// logConfig holds logging configuration
type logConfig struct {
	level  slog.Level
	format LogFormat
	output *os.File
}

// Option types for functional configuration
type TracingOption func(*tracingConfig)
type MetricsOption func(*metricsConfig)
type LogOption func(*logConfig)

// ── Tracing Options ──────────────────────────────────────────────────────────

// OTELExporter configures the OpenTelemetry exporter endpoint
func OTELExporter(endpoint string) TracingOption {
	return func(cfg *tracingConfig) {
		cfg.endpoint = endpoint
	}
}

// TracingSampler sets the sampling rate (0.0 to 1.0)
func TracingSampler(rate float64) TracingOption {
	return func(cfg *tracingConfig) {
		cfg.samplingRate = rate
	}
}

// ── Metrics Options ──────────────────────────────────────────────────────────

// PrometheusExporter sets the Prometheus metrics endpoint address
func PrometheusExporter(addr string) MetricsOption {
	return func(cfg *metricsConfig) {
		cfg.prometheusAddr = addr
	}
}

// MetricsBuckets sets custom histogram buckets
func MetricsBuckets(buckets []float64) MetricsOption {
	return func(cfg *metricsConfig) {
		cfg.buckets = buckets
	}
}

// ── Logging Options ──────────────────────────────────────────────────────────

// LogLevel sets the minimum log level
func LogLevel(level slog.Level) LogOption {
	return func(cfg *logConfig) {
		cfg.level = level
	}
}

// WithLogFormat sets the log output format (text or JSON)
func WithLogFormat(format LogFormat) LogOption {
	return func(cfg *logConfig) {
		cfg.format = format
	}
}

// LogOutput sets the log output destination
func LogOutput(output *os.File) LogOption {
	return func(cfg *logConfig) {
		cfg.output = output
	}
}

// ── App Integration ──────────────────────────────────────────────────────────

// initializeObservability sets up tracing, metrics, and logging
func (app *App) initializeObservability() {
	if app.observability == nil {
		app.observability = newObservability()
	}

	// Initialize HTTP metrics if metrics are enabled
	if app.enableMetrics {
		app.observability.initializeMetrics()
	}
}

// Tracer returns the app's tracer
func (app *App) Tracer() Tracer {
	if app.observability == nil {
		app.initializeObservability()
	}
	return app.observability.tracer
}

// Metrics returns the app's metrics registry
func (app *App) Metrics() MetricsRegistry {
	if app.observability == nil {
		app.initializeObservability()
	}
	return app.observability.metricsRegistry
}

// Logger returns the app's logger
func (app *App) Logger() Logger {
	if app.observability == nil {
		app.initializeObservability()
	}
	return &slogLogger{logger: app.observability.logger}
}

// ── Request Instrumentation ──────────────────────────────────────────────────

// requestMetrics tracks metrics for a single request
type requestMetrics struct {
	start    time.Time
	protocol string
	method   string
	route    string
}

// startRequestMetrics begins tracking a request
func (app *App) startRequestMetrics(protocol, method, route string) *requestMetrics {
	if !app.enableMetrics || app.observability == nil {
		return nil
	}

	rm := &requestMetrics{
		start:    time.Now(),
		protocol: protocol,
		method:   method,
		route:    route,
	}

	// Increment active requests
	app.observability.activeRequests.With(protocol).Inc()

	return rm
}

// endRequestMetrics records metrics at the end of a request
func (app *App) endRequestMetrics(rm *requestMetrics, status int, requestSize, responseSize int64) {
	if rm == nil || !app.enableMetrics || app.observability == nil {
		return
	}

	// Decrement active requests
	app.observability.activeRequests.With(rm.protocol).Dec()

	// Record request duration
	duration := time.Since(rm.start).Seconds()
	app.observability.requestDuration.With(rm.method, rm.route, rm.protocol).Observe(duration)

	// Record request total
	statusStr := statusCodeToString(status)
	app.observability.requestsTotal.With(rm.method, rm.route, statusStr, rm.protocol).Inc()

	// Record sizes
	if requestSize > 0 {
		app.observability.requestSize.With(rm.method, rm.route).Observe(float64(requestSize))
	}
	if responseSize > 0 {
		app.observability.responseSize.With(rm.method, rm.route).Observe(float64(responseSize))
	}
}

// statusCodeToString converts HTTP status code to string
func statusCodeToString(code int) string {
	switch code / 100 {
	case 1:
		return "1xx"
	case 2:
		return "2xx"
	case 3:
		return "3xx"
	case 4:
		return "4xx"
	case 5:
		return "5xx"
	default:
		return "unknown"
	}
}
