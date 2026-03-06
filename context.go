package gx

import (
	"context"

	"github.com/s-anzie/gx/core"
)

// Context extends core.Context with GX-specific functionality
type Context struct {
	*core.Context
	gxApp  *App
	span   Span
	logger Logger
}

// Fail creates an error response from an AppError
func (c *Context) Fail(err AppError) core.Response {
	return err.ToResponse()
}

// Span returns the current request's span
// Returns a no-op span if tracing is not enabled
func (c *Context) Span() Span {
	if c.span != nil {
		return c.span
	}
	// Return no-op span
	return &noopSpan{ctx: c.GoContext()}
}

// Trace creates a new child span with the given name
func (c *Context) Trace(name string) Span {
	if c.gxApp != nil && c.gxApp.Tracer() != nil {
		return c.gxApp.Tracer().Start(c.GoContext(), name)
	}
	// Return no-op span
	return &noopSpan{ctx: c.GoContext()}
}

// Log returns a logger correlated with the current request
// The logger includes trace ID, span ID, request ID, route, and method
func (c *Context) Log() Logger {
	if c.logger != nil {
		return c.logger
	}
	if c.gxApp != nil {
		return c.gxApp.Logger()
	}
	// Fallback to no-op logger
	return &slogLogger{}
}

// Handler is the GX handler signature (uses gx.Context instead of core.Context)
type Handler func(*Context) core.Response

// WrapHandler converts a GX Handler to a core.Handler
// This allows GX handlers to work with the core routing system
func WrapHandler(h Handler) core.Handler {
	return func(c *core.Context) core.Response {
		gxCtx := &Context{Context: c}
		return h(gxCtx)
	}
}

// WrapHandlerWithApp converts a GX Handler to a core.Handler with app context
// This version includes the GX app for observability access
func WrapHandlerWithApp(app *App, h Handler) core.Handler {
	return func(c *core.Context) core.Response {
		gxCtx := &Context{
			Context: c,
			gxApp:   app,
		}

		// Create root span if tracing is enabled
		if app.enableTracing && app.observability != nil {
			span := app.Tracer().StartWithAttributes(
				context.Background(),
				c.Request.Method+" "+c.Request.URL.Path,
				map[string]any{
					"http.method":         c.Method(),
					"http.route":          c.Path(),
					"http.protocol":       c.Proto(),
					"net.peer.ip":         c.ClientIP(),
					"user_agent.original": c.Header("User-Agent"),
				},
			)
			defer span.End()
			gxCtx.span = span
		}

		// Create request-scoped logger if logging is enabled
		if app.enableLogs && app.observability != nil {
			logger := app.Logger().With(
				"method", c.Method(),
				"path", c.Path(),
				"client_ip", c.ClientIP(),
			)
			gxCtx.logger = logger
		}

		return h(gxCtx)
	}
}
