package gx

import (
	"context"
	"sort"

	"github.com/s-anzie/gx/core"
)

// Plugin is the base interface for all GX plugins
type Plugin interface {
	// Name returns the unique name of the plugin
	Name() string

	// OnBoot is called during application boot, before the server starts
	OnBoot(app *App) error

	// OnShutdown is called during graceful shutdown
	OnShutdown(ctx context.Context) error
}

// RequestPlugin is an optional interface for plugins that need to intercept requests
type RequestPlugin interface {
	Plugin

	// OnRequest is called for each incoming request
	// It can act as middleware by calling next and potentially modifying the response
	OnRequest(c *Context, next core.Handler) core.Response
}

// ErrorPlugin is an optional interface for plugins that want to handle errors
type ErrorPlugin interface {
	Plugin

	// OnError is called when an error occurs
	// Return nil to let the next error handler process it
	// Return a Response to handle the error and stop propagation
	OnError(c *Context, err AppError) core.Response
}

// RoutePlugin is an optional interface for plugins that need to know about registered routes
type RoutePlugin interface {
	Plugin

	// OnRoute is called once for each route registered during boot
	OnRoute(route RouteInfo)
}

// RouteInfo contains metadata about a registered route
type RouteInfo struct {
	Method  string
	Path    string
	Handler core.Handler
	Tags    []string
}

// ── Plugin Install Options ───────────────────────────────────────────────────

// PluginEntry wraps a Plugin with its install-time metadata.
type pluginEntry struct {
	plugin   Plugin
	priority int
}

// PluginInstallOption configures how a plugin is installed.
type PluginInstallOption func(*pluginEntry)

// PluginPriority sets the execution priority for a plugin.
// Higher values execute earlier (first in the OnRequest chain).
// Default priority is 0 (last custom plugin).
//
// Standard framework plugin priorities:
//
//	recovery:    1000
//	requestid:    950
//	logger:       900
//	cors:         850
//	ratelimit:    800
//	auth:         750
//	cache:        700
//	compress:     100
func PluginPriority(n int) PluginInstallOption {
	return func(e *pluginEntry) {
		e.priority = n
	}
}

// sortPluginEntries sorts plugin entries by priority descending (highest first).
// Plugins with the same priority maintain their insertion order.
func sortPluginEntries(entries []pluginEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})
}
