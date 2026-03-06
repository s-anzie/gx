package core

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Cookie represents an HTTP cookie with secure defaults
type Cookie struct {
	Name     string
	Value    string
	Path     string // default "/"
	Domain   string
	MaxAge   int // seconds — 0 = session cookie
	Expires  time.Time
	Secure   bool          // default true for HTTPS
	HttpOnly bool          // default true
	SameSite http.SameSite // default SameSiteLaxMode
}

// ── Context Cookie Methods ───────────────────────────────────────────────────

// Cookie reads the value of a named cookie from the request.
// Returns ("", error) if the cookie doesn't exist.
func (c *Context) Cookie(name string) (string, error) {
	cookie, err := c.Request.Cookie(name)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

// CookieDefault reads a named cookie, returning def if absent or on error.
func (c *Context) CookieDefault(name, def string) string {
	v, err := c.Cookie(name)
	if err != nil {
		return def
	}
	return v
}

// RawCookie returns the raw *http.Cookie for full field access.
// Returns nil and an error if the cookie doesn't exist.
func (c *Context) RawCookie(name string) (*http.Cookie, error) {
	return c.Request.Cookie(name)
}

// SetCookie writes a cookie directly onto the response writer.
// Uses secure defaults: HttpOnly=true, SameSite=Lax.
func (c *Context) SetCookie(name, value string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ── Response Cookie Chainers ─────────────────────────────────────────────────

// WithCookie returns a new Response with the given cookie set in Set-Cookie header.
// Default Path "/" is applied if not set; SameSite defaults to Lax if zero.
// HttpOnly and Secure must be set explicitly — no silent overrides.
func WithCookie(r Response, c Cookie) Response {
	// Apply defaults only for unset fields
	if c.Path == "" {
		c.Path = "/"
	}
	if c.SameSite == 0 {
		c.SameSite = http.SameSiteLaxMode
	}

	return WithHeader(r, "Set-Cookie", formatCookie(c))
}

// WithClearCookie returns a new Response that deletes the named cookie
// by setting its MaxAge to -1 and Expires to the past.
func WithClearCookie(r Response, name string) Response {
	c := Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	return WithHeader(r, "Set-Cookie", formatCookie(c))
}

// formatCookie serializes a Cookie to its Set-Cookie header value
func formatCookie(c Cookie) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s=%s", c.Name, c.Value)

	if c.MaxAge > 0 {
		fmt.Fprintf(&b, "; Max-Age=%d", c.MaxAge)
	} else if c.MaxAge < 0 {
		b.WriteString("; Max-Age=0")
	}

	if !c.Expires.IsZero() {
		fmt.Fprintf(&b, "; Expires=%s", c.Expires.UTC().Format(http.TimeFormat))
	}

	if c.Path != "" {
		fmt.Fprintf(&b, "; Path=%s", c.Path)
	}

	if c.Domain != "" {
		fmt.Fprintf(&b, "; Domain=%s", c.Domain)
	}

	if c.HttpOnly {
		b.WriteString("; HttpOnly")
	}

	if c.Secure {
		b.WriteString("; Secure")
	}

	switch c.SameSite {
	case http.SameSiteStrictMode:
		b.WriteString("; SameSite=Strict")
	case http.SameSiteLaxMode:
		b.WriteString("; SameSite=Lax")
	case http.SameSiteNoneMode:
		b.WriteString("; SameSite=None")
	}

	return b.String()
}
