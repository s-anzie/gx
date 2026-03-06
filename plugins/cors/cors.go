package cors

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

// Config holds CORS configuration
type Config struct {
	// Origins is a list of allowed origins. Use "*" to allow any origin.
	Origins []string

	// Methods is a list of allowed HTTP methods
	Methods []string

	// Headers is a list of allowed request headers
	Headers []string

	// ExposeHeaders is a list of headers to expose to the client
	ExposeHeaders []string

	// Credentials indicates whether credentials are allowed
	Credentials bool

	// MaxAge is how long preflight results can be cached (in seconds)
	MaxAge time.Duration
}

// DefaultConfig returns the default CORS configuration
func DefaultConfig() Config {
	return Config{
		Origins:     []string{"*"},
		Methods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		Headers:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		Credentials: false,
		MaxAge:      12 * time.Hour,
	}
}

// corsPlugin implements the CORS plugin
type corsPlugin struct {
	config Config
}

// New creates a new CORS plugin with the given configuration
func New(config Config) gx.Plugin {
	// Set defaults if not provided
	if len(config.Origins) == 0 {
		config.Origins = []string{"*"}
	}
	if len(config.Methods) == 0 {
		config.Methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	if len(config.Headers) == 0 {
		config.Headers = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	}
	if config.MaxAge == 0 {
		config.MaxAge = 12 * time.Hour
	}

	return &corsPlugin{config: config}
}

// Name returns the plugin name
func (p *corsPlugin) Name() string {
	return "cors"
}

// OnBoot is called when the application boots
func (p *corsPlugin) OnBoot(app *gx.App) error {
	return nil
}

// OnRequest processes each request to add CORS headers
func (p *corsPlugin) OnRequest(c *gx.Context, next core.Handler) core.Response {
	origin := c.Header("Origin")

	// Check if origin is allowed
	allowed := false
	if len(p.config.Origins) > 0 && p.config.Origins[0] == "*" {
		allowed = true
	} else {
		for _, o := range p.config.Origins {
			if o == origin {
				allowed = true
				break
			}
		}
	}

	if !allowed {
		// If origin is not allowed, continue without CORS headers
		return next(c.Context)
	}

	// Set the Access-Control-Allow-Origin header
	if len(p.config.Origins) > 0 && p.config.Origins[0] == "*" {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Vary", "Origin")
	}

	// Set credentials header if enabled
	if p.config.Credentials {
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	// Set expose headers if provided
	if len(p.config.ExposeHeaders) > 0 {
		c.Writer.Header().Set("Access-Control-Expose-Headers", strings.Join(p.config.ExposeHeaders, ", "))
	}

	// Handle preflight requests
	if c.Method() == "OPTIONS" {
		// Set allowed methods
		if len(p.config.Methods) > 0 {
			c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(p.config.Methods, ", "))
		}

		// Set allowed headers
		if len(p.config.Headers) > 0 {
			c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(p.config.Headers, ", "))
		}

		// Set max age
		if p.config.MaxAge > 0 {
			c.Writer.Header().Set("Access-Control-Max-Age", strconv.Itoa(int(p.config.MaxAge.Seconds())))
		}

		// Return 204 No Content for preflight
		c.Writer.WriteHeader(http.StatusNoContent)
		return nil
	}

	// Continue with the request
	return next(c.Context)
}

// OnShutdown is called when the application shuts down
func (p *corsPlugin) OnShutdown(ctx context.Context) error {
	return nil
}
