package gx

import (
	"net"
	"net/http"

	"github.com/s-anzie/gx/core"
)

// HTTPSRedirect returns a middleware that redirects all HTTP requests to HTTPS
// using 308 Permanent Redirect (preserves HTTP method, unlike 301).
// It also sets the HSTS header on responses.
func HTTPSRedirect() core.Middleware {
	return func(c *core.Context, next core.Handler) core.Response {
		if c.Request.TLS == nil && c.Request.Header.Get("X-Forwarded-Proto") != "https" {
			host := c.Request.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			target := "https://" + host + c.Request.RequestURI
			return core.WithStatus(core.WithHeader(core.NoContent(), "Location", target),
				http.StatusPermanentRedirect)
		}

		resp := next(c)

		// Inject HSTS header on HTTPS responses
		if resp != nil {
			resp = core.WithHeader(resp, "Strict-Transport-Security",
				"max-age=63072000; includeSubDomains; preload")
		}
		return resp
	}
}

// ListenHTTPRedirect starts a dedicated HTTP server on httpAddr that permanently
// redirects all requests to the corresponding HTTPS URL on httpsAddr.
// It runs in a background goroutine and returns immediately.
//
//	app.ListenHTTPRedirect(":80", ":443")
func (a *App) ListenHTTPRedirect(httpAddr, httpsAddr string) {
	_, httpsPort, err := net.SplitHostPort(httpsAddr)
	if err != nil {
		httpsPort = "443"
	}

	srv := &http.Server{
		Addr: httpAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			target := "https://" + net.JoinHostPort(host, httpsPort) + r.RequestURI

			// HSTS even on redirect responses
			w.Header().Set("Strict-Transport-Security",
				"max-age=63072000; includeSubDomains; preload")
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
		}),
	}

	go func() {
		_ = srv.ListenAndServe() // ignore error on intentional close
	}()
}
