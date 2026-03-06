package gx

import (
	"context"
	"sync"
	"time"

	"github.com/s-anzie/gx/core"
)

// HealthStatus represents the health status of a dependency
type HealthStatus string

const (
	StatusOK       HealthStatus = "ok"
	StatusDegraded HealthStatus = "degraded"
	StatusDown     HealthStatus = "down"
)

// HealthCheck represents a health check for a dependency
type HealthCheck struct {
	name     string
	check    func(context.Context) error
	interval time.Duration
	timeout  time.Duration
	critical bool

	// State
	mu         sync.RWMutex
	lastStatus HealthStatus
	lastError  error
	lastCheck  time.Time
	latencyMs  int64
}

// HealthCheckOption configures a health check
type HealthCheckOption func(*HealthCheck)

// HealthInterval sets the check interval (default: 30s)
func HealthInterval(interval time.Duration) HealthCheckOption {
	return func(h *HealthCheck) {
		h.interval = interval
	}
}

// HealthTimeout sets the check timeout (default: 5s)
func HealthTimeout(timeout time.Duration) HealthCheckOption {
	return func(h *HealthCheck) {
		h.timeout = timeout
	}
}

// HealthCritical marks the check as critical (default: true)
// Non-critical checks don't fail the /health/ready endpoint
func HealthCritical(critical bool) HealthCheckOption {
	return func(h *HealthCheck) {
		h.critical = critical
	}
}

// Health registers a health check for a dependency
func (app *App) Health(name string, check func(context.Context) error, opts ...HealthCheckOption) {
	h := &HealthCheck{
		name:       name,
		check:      check,
		interval:   30 * time.Second,
		timeout:    5 * time.Second,
		critical:   true,
		lastStatus: StatusOK,
	}

	// Apply options
	for _, opt := range opts {
		opt(h)
	}

	app.healthChecks[name] = h
}

// runHealthCheck executes a single health check
func (h *HealthCheck) run(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	start := time.Now()

	// Create context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	// Run the check
	err := h.check(checkCtx)
	latency := time.Since(start)

	// Update state
	h.lastCheck = time.Now()
	h.latencyMs = latency.Milliseconds()
	h.lastError = err

	if err != nil {
		h.lastStatus = StatusDown
	} else {
		h.lastStatus = StatusOK
	}
}

// status returns the current health status (thread-safe)
func (h *HealthCheck) status() (HealthStatus, error, int64, time.Time) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastStatus, h.lastError, h.latencyMs, h.lastCheck
}

// HealthResult represents the result of a health check
type HealthResult struct {
	Status    HealthStatus `json:"status"`
	LatencyMs int64        `json:"latency_ms,omitempty"`
	Error     string       `json:"error,omitempty"`
	LastCheck time.Time    `json:"last_check,omitempty"`
}

// HealthResponse is the JSON response for health endpoints
type HealthResponse struct {
	Status HealthStatus            `json:"status"`
	Checks map[string]HealthResult `json:"checks,omitempty"`
}

// RegisterHealthEndpoints adds health check endpoints to the app
func (app *App) RegisterHealthEndpoints() {
	// Global health - all checks
	app.GET("/health", func(c *core.Context) core.Response {
		allOk := true
		checks := make(map[string]HealthResult)

		for name, check := range app.healthChecks {
			status, err, latency, lastCheck := check.status()
			result := HealthResult{
				Status:    status,
				LatencyMs: latency,
				LastCheck: lastCheck,
			}

			if err != nil {
				result.Error = err.Error()
				if check.critical {
					allOk = false
				}
			}

			checks[name] = result

			if status != StatusOK {
				allOk = false
			}
		}

		globalStatus := StatusOK
		if !allOk {
			globalStatus = StatusDegraded
		}

		response := HealthResponse{
			Status: globalStatus,
			Checks: checks,
		}

		if globalStatus == StatusOK {
			return c.JSON(response)
		}
		return core.WithStatus(c.JSON(response), 503)
	})

	// Liveness - app is running
	app.GET("/health/live", func(c *core.Context) core.Response {
		return c.JSON(HealthResponse{Status: StatusOK})
	})

	// Readiness - all critical dependencies are healthy
	app.GET("/health/ready", func(c *core.Context) core.Response {
		allCriticalOk := true
		checks := make(map[string]HealthResult)

		for name, check := range app.healthChecks {
			if !check.critical {
				continue
			}

			status, err, latency, lastCheck := check.status()
			result := HealthResult{
				Status:    status,
				LatencyMs: latency,
				LastCheck: lastCheck,
			}

			if err != nil {
				result.Error = err.Error()
				allCriticalOk = false
			}

			checks[name] = result

			if status != StatusOK {
				allCriticalOk = false
			}
		}

		globalStatus := StatusOK
		if !allCriticalOk {
			globalStatus = StatusDown
		}

		response := HealthResponse{
			Status: globalStatus,
			Checks: checks,
		}

		if globalStatus == StatusOK {
			return c.JSON(response)
		}
		return core.WithStatus(c.JSON(response), 503)
	})
}

// StartHealthChecks begins periodic health check execution
func (app *App) StartHealthChecks(ctx context.Context) {
	for _, check := range app.healthChecks {
		// Run initial check immediately
		check.run(ctx)

		// Start periodic checks
		go func(h *HealthCheck) {
			ticker := time.NewTicker(h.interval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					h.run(ctx)
				case <-ctx.Done():
					return
				}
			}
		}(check)
	}
}
