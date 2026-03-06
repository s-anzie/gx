package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

// Config holds JWT authentication configuration
type Config struct {
	// Secret is the signing key for JWT validation
	Secret []byte

	// OnInvalid is called when token validation fails
	OnInvalid func(*gx.Context) core.Response

	// TokenExtractor extracts the token from the request (default: from Authorization header)
	TokenExtractor func(*gx.Context) (string, error)

	// claimsType stores the type information for claims parsing
	claimsType any
}

// jwtPlugin implements the JWT authentication plugin
type jwtPlugin struct {
	config Config
}

// JWT creates a new JWT authentication plugin
func JWT(options ...Option) gx.Plugin {
	config := Config{
		Secret: []byte(""),
		OnInvalid: func(c *gx.Context) core.Response {
			return c.Fail(gx.ErrUnauthorized)
		},
		TokenExtractor: defaultTokenExtractor,
		claimsType:     &jwt.MapClaims{},
	}

	// Apply options
	for _, opt := range options {
		opt(&config)
	}

	return &jwtPlugin{config: config}
}

// Name returns the plugin name
func (p *jwtPlugin) Name() string {
	return "auth"
}

// OnBoot is called when the application boots
func (p *jwtPlugin) OnBoot(app *gx.App) error {
	if len(p.config.Secret) == 0 {
		return errors.New("JWT secret is required")
	}
	return nil
}

// OnRequest processes each request for JWT authentication
func (p *jwtPlugin) OnRequest(c *gx.Context, next core.Handler) core.Response {
	// Extract token from request
	tokenString, err := p.config.TokenExtractor(c)
	if err != nil {
		return p.config.OnInvalid(c)
	}

	// Parse and validate token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return p.config.Secret, nil
	})

	if err != nil || !token.Valid {
		return p.config.OnInvalid(c)
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return p.config.OnInvalid(c)
	}

	// Store claims in context
	// If custom claims type was specified, convert to it
	if p.config.claimsType != nil {
		// Store the claims map in the context
		// The user can access it via gx.Typed[CustomType](c)
		c.Set("jwt.claims", claims)

		// Also store individual standard claims for easy access
		if sub, ok := claims["sub"].(string); ok {
			c.Set("jwt.subject", sub)
		}
		if userId, ok := claims["user_id"].(string); ok {
			c.Set("user_id", userId)
		}
	}

	return next(c.Context)
}

// OnShutdown is called when the application shuts down
func (p *jwtPlugin) OnShutdown(ctx context.Context) error {
	return nil
}

// ── Options ──────────────────────────────────────────────────────────────────

// Option is a functional option for JWT configuration
type Option func(*Config)

// Secret sets the JWT signing secret
func Secret(secret string) Option {
	return func(c *Config) {
		c.Secret = []byte(secret)
	}
}

// SecretBytes sets the JWT signing secret as bytes
func SecretBytes(secret []byte) Option {
	return func(c *Config) {
		c.Secret = secret
	}
}

// OnInvalid sets the callback for invalid tokens
func OnInvalid(fn func(*gx.Context) core.Response) Option {
	return func(c *Config) {
		c.OnInvalid = fn
	}
}

// ClaimsType sets the expected claims type for parsing
func ClaimsType[T any]() Option {
	return func(c *Config) {
		var zero T
		c.claimsType = zero
	}
}

// WithTokenExtractor sets a custom token extractor
func WithTokenExtractor(fn func(*gx.Context) (string, error)) Option {
	return func(c *Config) {
		c.TokenExtractor = fn
	}
}

// ── Token Extraction ─────────────────────────────────────────────────────────

// defaultTokenExtractor extracts token from Authorization header
func defaultTokenExtractor(c *gx.Context) (string, error) {
	auth := c.Request.Header.Get("Authorization")
	if auth == "" {
		return "", errors.New("missing authorization header")
	}

	// Check for Bearer token
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", errors.New("invalid authorization header format")
	}

	return parts[1], nil
}

// ── Helper Functions ─────────────────────────────────────────────────────────

// GetClaims retrieves JWT claims from context
func GetClaims(c *gx.Context) (jwt.MapClaims, bool) {
	val, exists := c.Get("jwt.claims")
	if !exists {
		return nil, false
	}
	claims, ok := val.(jwt.MapClaims)
	return claims, ok
}

// GetSubject retrieves the JWT subject claim
func GetSubject(c *gx.Context) (string, bool) {
	val, exists := c.Get("jwt.subject")
	if !exists {
		return "", false
	}
	sub, ok := val.(string)
	return sub, ok
}

// GenerateToken generates a new JWT token with the given claims
func GenerateToken(secret string, claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
