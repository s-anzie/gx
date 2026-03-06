package compress

import (
	"bufio"
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

// Algorithm represents a compression algorithm
type Algorithm int

const (
	// Gzip compression
	Gzip Algorithm = iota
	// Brotli compression
	Brotli
)

// Config holds compression configuration
type Config struct {
	// Algorithms is the list of allowed compression algorithms in order of preference
	Algorithms []Algorithm

	// MinSize is the minimum response size to compress (in bytes)
	MinSize int

	// Level is the compression level (-1 = default, 0 = no compression, 1-9 for gzip)
	Level int
}

// DefaultConfig returns the default compression configuration
func DefaultConfig() Config {
	return Config{
		Algorithms: []Algorithm{Brotli, Gzip},
		MinSize:    1024, // 1 KB
		Level:      -1,   // Default compression
	}
}

// compressPlugin implements the compression plugin
type compressPlugin struct {
	config Config
}

// New creates a new compression plugin with the given configuration
func New(config Config) gx.Plugin {
	// Set defaults if not provided
	if len(config.Algorithms) == 0 {
		config.Algorithms = []Algorithm{Brotli, Gzip}
	}
	if config.MinSize == 0 {
		config.MinSize = 1024
	}
	if config.Level == 0 {
		config.Level = -1
	}

	return &compressPlugin{config: config}
}

// Name returns the plugin name
func (p *compressPlugin) Name() string {
	return "compress"
}

// OnBoot is called when the application boots
func (p *compressPlugin) OnBoot(app *gx.App) error {
	return nil
}

// OnRequest processes each request to enable compression
func (p *compressPlugin) OnRequest(c *gx.Context, next core.Handler) core.Response {
	// Get the Accept-Encoding header
	acceptEncoding := c.Header("Accept-Encoding")
	if acceptEncoding == "" {
		// Client doesn't support compression
		return next(c.Context)
	}

	// Determine which compression algorithm to use
	var algorithm Algorithm
	var encoding string
	found := false

	for _, alg := range p.config.Algorithms {
		switch alg {
		case Brotli:
			if strings.Contains(acceptEncoding, "br") {
				algorithm = Brotli
				encoding = "br"
				found = true
			}
		case Gzip:
			if strings.Contains(acceptEncoding, "gzip") {
				algorithm = Gzip
				encoding = "gzip"
				found = true
			}
		}

		if found {
			break
		}
	}

	if !found {
		// No supported compression algorithm
		return next(c.Context)
	}

	// Wrap the ResponseWriter with a compression writer
	crw := &compressResponseWriter{
		ResponseWriter: c.Writer,
		algorithm:      algorithm,
		encoding:       encoding,
		minSize:        p.config.MinSize,
		level:          p.config.Level,
		headerWritten:  false,
		statusWritten:  false,
		statusCode:     200,
	}

	// Replace the writer
	c.Writer = crw

	// Call the next handler
	resp := next(c.Context)

	// Note: Close() will be called by ServeHTTP after writing the response
	return resp
}

// OnShutdown is called when the application shuts down
func (p *compressPlugin) OnShutdown(ctx context.Context) error {
	return nil
}

// compressResponseWriter wraps http.ResponseWriter to compress response data
type compressResponseWriter struct {
	http.ResponseWriter
	algorithm     Algorithm
	encoding      string
	minSize       int
	level         int
	writer        io.WriteCloser
	headerWritten bool
	statusWritten bool
	statusCode    int
	buffer        []byte
}

// Write compresses and writes data
func (w *compressResponseWriter) Write(data []byte) (int, error) {
	// If headers written and compression active, write through compressor
	if w.headerWritten {
		if w.writer != nil {
			return w.writer.Write(data)
		}
		// No compression, write directly
		return w.ResponseWriter.Write(data)
	}

	// Still buffering - accumulate data
	w.buffer = append(w.buffer, data...)

	// Check if we've buffered enough to make a compression decision
	if len(w.buffer) >= w.minSize {
		// Enough data - enable compression
		w.initCompression()
		return len(data), nil
	}

	// Not enough data yet, keep buffering
	return len(data), nil
}

// WriteHeader captures the status code
func (w *compressResponseWriter) WriteHeader(statusCode int) {
	if w.statusWritten {
		return
	}
	w.statusCode = statusCode
	w.statusWritten = true
	// Don't actually write headers yet - wait for Write()
}

// initCompression sets up compression and writes buffered data
func (w *compressResponseWriter) initCompression() {
	if w.headerWritten {
		return
	}
	w.headerWritten = true

	// Write status code if it was set
	if !w.statusWritten {
		w.statusCode = 200
	}

	// Set compression headers
	w.ResponseWriter.Header().Set("Content-Encoding", w.encoding)
	w.ResponseWriter.Header().Del("Content-Length")

	// Write the status
	w.ResponseWriter.WriteHeader(w.statusCode)

	// Create compression writer
	switch w.algorithm {
	case Brotli:
		if w.level == -1 {
			w.writer = brotli.NewWriter(w.ResponseWriter)
		} else {
			w.writer = brotli.NewWriterLevel(w.ResponseWriter, w.level)
		}
	case Gzip:
		if w.level == -1 {
			w.writer, _ = gzip.NewWriterLevel(w.ResponseWriter, gzip.DefaultCompression)
		} else {
			w.writer, _ = gzip.NewWriterLevel(w.ResponseWriter, w.level)
		}
	}

	// Write buffered data through compressor
	if len(w.buffer) > 0 && w.writer != nil {
		w.writer.Write(w.buffer)
		w.buffer = nil
	}
}

// flushUncompressed writes buffered data without compression
func (w *compressResponseWriter) flushUncompressed() {
	if w.headerWritten {
		return
	}
	w.headerWritten = true

	// Write status code
	if !w.statusWritten {
		w.statusCode = 200
	}
	w.ResponseWriter.WriteHeader(w.statusCode)

	// Write buffered data directly
	if len(w.buffer) > 0 {
		w.ResponseWriter.Write(w.buffer)
		w.buffer = nil
	}
}

// Close flushes and closes the compression writer
func (w *compressResponseWriter) Close() error {
	// If we haven't written headers yet and have buffered data,
	// decide whether to compress or not
	if !w.headerWritten {
		if len(w.buffer) >= w.minSize {
			// Enough data to compress
			w.initCompression()
		} else {
			// Not enough data, write without compression
			w.flushUncompressed()
			return nil
		}
	}

	// Close the compression writer if it exists
	if w.writer != nil {
		return w.writer.Close()
	}

	return nil
}

// Flush implements http.Flusher
func (w *compressResponseWriter) Flush() {
	if fw, ok := w.writer.(interface{ Flush() error }); ok {
		fw.Flush()
	}

	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements http.Hijacker
func (w *compressResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}
