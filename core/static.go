package core

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// staticConfig holds configuration for static file serving
type staticConfig struct {
	maxAge time.Duration
	index  string
	browse bool
}

// StaticOption configures static file serving behavior
type StaticOption func(*staticConfig)

// StaticMaxAge sets the Cache-Control max-age for served files
func StaticMaxAge(d time.Duration) StaticOption {
	return func(c *staticConfig) { c.maxAge = d }
}

// StaticIndex sets the index file name (default "index.html")
func StaticIndex(name string) StaticOption {
	return func(c *staticConfig) { c.index = name }
}

// StaticBrowse enables or disables directory listing (default disabled)
func StaticBrowse(allow bool) StaticOption {
	return func(c *staticConfig) { c.browse = allow }
}

func newStaticConfig(opts []StaticOption) *staticConfig {
	cfg := &staticConfig{
		index: "index.html",
	}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

// ── App Static Methods ───────────────────────────────────────────────────────

// Static serves files from dir at the given URL prefix.
//
//	app.Static("/assets", "./public")
//	// GET /assets/style.css → ./public/style.css
func (a *App) Static(prefix, dir string, opts ...StaticOption) {
	cfg := newStaticConfig(opts)
	fileServer := http.FileServer(http.Dir(dir))

	handler := func(c *Context) Response {
		// Strip prefix → path relative to dir
		p := strings.TrimPrefix(c.Request.URL.Path, strings.TrimRight(prefix, "/"))
		if p == "" {
			p = "/"
		}

		// Security: block path traversal
		p = path.Clean("/" + p)

		// Check for index file on directory request
		if strings.HasSuffix(p, "/") {
			indexPath := dir + p + cfg.index
			if _, err := os.Stat(indexPath); os.IsNotExist(err) {
				if !cfg.browse {
					return WithStatus(NoContent(), http.StatusNotFound)
				}
			}
		}

		// Check if pre-compressed file exists
		if precompressed := findPrecompressed(dir+p, c.Request.Header.Get("Accept-Encoding")); precompressed != "" {
			enc := "gzip"
			if strings.HasSuffix(precompressed, ".br") {
				enc = "br"
			}
			c.Writer.Header().Set("Content-Encoding", enc)
			c.Writer.Header().Set("Vary", "Accept-Encoding")
		}

		// Apply cache headers
		if cfg.maxAge > 0 {
			seconds := int(cfg.maxAge.Seconds())
			c.Writer.Header().Set("Cache-Control", "public, max-age="+itoa(seconds))
		}

		// Create a response wrapper that serves the file
		return &staticFileResponse{
			fileServer: fileServer,
			path:       p,
			request:    c.Request,
			writer:     c.Writer,
		}
	}

	// Register catch-all under prefix
	a.GET(prefix+"/*filepath", handler)
}

// staticFileResponse serves a static file while respecting existing headers
type staticFileResponse struct {
	fileServer http.Handler
	path       string
	request    *http.Request
	writer     http.ResponseWriter
}

func (r *staticFileResponse) Write(w http.ResponseWriter) error {
	// Temporarily modify the request path for the file server
	originalPath := r.request.URL.Path
	r.request.URL.Path = r.path
	defer func() { r.request.URL.Path = originalPath }()

	// Serve the file - this will write headers and body
	r.fileServer.ServeHTTP(w, r.request)
	return nil
}

func (r *staticFileResponse) Status() int { return 200 }
func (r *staticFileResponse) Headers() http.Header { return make(http.Header) }
func (r *staticFileResponse) ContentType() string { return "" }

// StaticFile serves a single file at path.
//
//	app.StaticFile("/favicon.ico", "./public/favicon.ico")
func (a *App) StaticFile(urlPath, filePath string) {
	a.GET(urlPath, func(c *Context) Response {
		http.ServeFile(c.Writer, c.Request, filePath)
		return nil
	})
}

// StaticFS serves files from an fs.FS (e.g. embed.FS) at the given URL prefix.
//
//	//go:embed public
//	var publicFS embed.FS
//	app.StaticFS("/assets", publicFS)
func (a *App) StaticFS(prefix string, fsys fs.FS, opts ...StaticOption) {
	cfg := newStaticConfig(opts)
	fileServer := http.FileServer(http.FS(fsys))

	handler := func(c *Context) Response {
		p := strings.TrimPrefix(c.Request.URL.Path, strings.TrimRight(prefix, "/"))
		if p == "" {
			p = "/"
		}
		p = path.Clean("/" + p)

		// Check for index on directory
		if strings.HasSuffix(p, "/") {
			indexPath := strings.TrimPrefix(p, "/") + cfg.index
			if _, err := fs.Stat(fsys, indexPath); os.IsNotExist(err) {
				if !cfg.browse {
					return WithStatus(NoContent(), http.StatusNotFound)
				}
			}
		}

		if cfg.maxAge > 0 {
			seconds := int(cfg.maxAge.Seconds())
			c.Writer.Header().Set("Cache-Control", "public, max-age="+itoa(seconds))
		}

		c.Request.URL.Path = p
		fileServer.ServeHTTP(c.Writer, c.Request)
		return nil
	}

	a.GET(prefix+"/*filepath", handler)
	a.GET(prefix, handler)
}

// findPrecompressed looks for a pre-compressed version of the file.
// Returns the compressed path if found and client accepts that encoding.
func findPrecompressed(filePath, acceptEncoding string) string {
	if strings.Contains(acceptEncoding, "br") {
		if _, err := os.Stat(filePath + ".br"); err == nil {
			return filePath + ".br"
		}
	}
	if strings.Contains(acceptEncoding, "gzip") {
		if _, err := os.Stat(filePath + ".gz"); err == nil {
			return filePath + ".gz"
		}
	}
	return ""
}

// itoa is a simple int-to-string helper to avoid importing strconv
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
