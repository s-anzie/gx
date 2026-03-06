package core

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Response is the core response interface that all response types must implement.
// It's designed to be chainable and immutable - each method returns a new Response.
type Response interface {
	Status() int
	Headers() http.Header
	Write(w http.ResponseWriter) error
}

// ── Response Builders ────────────────────────────────────────────────────────

// baseResponse contains fields common to all response types
type baseResponse struct {
	status  int
	headers http.Header
}

func (r *baseResponse) Status() int {
	return r.status
}

func (r *baseResponse) Headers() http.Header {
	return r.headers
}

func newBaseResponse(status int) baseResponse {
	return baseResponse{
		status:  status,
		headers: make(http.Header),
	}
}

// ── JSON Response ────────────────────────────────────────────────────────────

type jsonResponse struct {
	baseResponse
	data interface{}
}

func (r *jsonResponse) Write(w http.ResponseWriter) error {
	// Set content type if not already set
	if r.headers.Get("Content-Type") == "" {
		r.headers.Set("Content-Type", "application/json; charset=utf-8")
	}

	// Write headers
	for k, v := range r.headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	w.WriteHeader(r.status)

	if r.data == nil {
		return nil
	}

	return json.NewEncoder(w).Encode(r.data)
}

// JSON creates a JSON response with 200 status
func JSON(data interface{}) Response {
	return &jsonResponse{
		baseResponse: newBaseResponse(http.StatusOK),
		data:         data,
	}
}

// Created creates a JSON response with 201 status
func Created(data interface{}) Response {
	return &jsonResponse{
		baseResponse: newBaseResponse(http.StatusCreated),
		data:         data,
	}
}

// ── Text Response ────────────────────────────────────────────────────────────

type textResponse struct {
	baseResponse
	text string
}

func (r *textResponse) Write(w http.ResponseWriter) error {
	// Set content type if not already set
	if r.headers.Get("Content-Type") == "" {
		r.headers.Set("Content-Type", "text/plain; charset=utf-8")
	}

	// Write headers
	for k, v := range r.headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	w.WriteHeader(r.status)
	_, err := w.Write([]byte(r.text))
	return err
}

// Text creates a plain text response with 200 status
func Text(format string, args ...interface{}) Response {
	return &textResponse{
		baseResponse: newBaseResponse(http.StatusOK),
		text:         fmt.Sprintf(format, args...),
	}
}

// ── HTML Response ────────────────────────────────────────────────────────────

type htmlResponse struct {
	baseResponse
	html string
}

func (r *htmlResponse) Write(w http.ResponseWriter) error {
	// Set content type if not already set
	if r.headers.Get("Content-Type") == "" {
		r.headers.Set("Content-Type", "text/html; charset=utf-8")
	}

	// Write headers
	for k, v := range r.headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	w.WriteHeader(r.status)
	_, err := w.Write([]byte(r.html))
	return err
}

// HTML creates an HTML response with 200 status
func HTML(html string) Response {
	return &htmlResponse{
		baseResponse: newBaseResponse(http.StatusOK),
		html:         html,
	}
}

// ── XML Response ─────────────────────────────────────────────────────────────

type xmlResponse struct {
	baseResponse
	data interface{}
}

func (r *xmlResponse) Write(w http.ResponseWriter) error {
	// Set content type if not already set
	if r.headers.Get("Content-Type") == "" {
		r.headers.Set("Content-Type", "application/xml; charset=utf-8")
	}

	// Write headers
	for k, v := range r.headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	w.WriteHeader(r.status)

	if r.data == nil {
		return nil
	}

	return xml.NewEncoder(w).Encode(r.data)
}

// XML creates an XML response with 200 status
func XML(data interface{}) Response {
	return &xmlResponse{
		baseResponse: newBaseResponse(http.StatusOK),
		data:         data,
	}
}

// ── No Content Response ──────────────────────────────────────────────────────

type noContentResponse struct {
	baseResponse
}

func (r *noContentResponse) Write(w http.ResponseWriter) error {
	// Write headers
	for k, v := range r.headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// NoContent creates a 204 No Content response
func NoContent() Response {
	return &noContentResponse{
		baseResponse: newBaseResponse(http.StatusNoContent),
	}
}

// ── Redirect Response ────────────────────────────────────────────────────────

type redirectResponse struct {
	baseResponse
	url string
}

func (r *redirectResponse) Write(w http.ResponseWriter) error {
	// Write headers
	for k, v := range r.headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	http.Redirect(w, nil, r.url, r.status)
	return nil
}

// Redirect creates a 302 Found redirect response
func Redirect(url string) Response {
	return &redirectResponse{
		baseResponse: newBaseResponse(http.StatusFound),
		url:          url,
	}
}

// Permanent changes a redirect from 302 to 301
func (r *redirectResponse) Permanent() Response {
	return &redirectResponse{
		baseResponse: baseResponse{
			status:  http.StatusMovedPermanently,
			headers: r.headers,
		},
		url: r.url,
	}
}

// ── File Response ────────────────────────────────────────────────────────────

type fileResponse struct {
	baseResponse
	path string
}

func (r *fileResponse) Write(w http.ResponseWriter) error {
	// Open file
	file, err := os.Open(r.path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get file info
	info, err := file.Stat()
	if err != nil {
		return err
	}

	// Detect content type if not set
	if r.headers.Get("Content-Type") == "" {
		ext := filepath.Ext(r.path)
		contentType := detectContentType(ext)
		r.headers.Set("Content-Type", contentType)
	}

	// Write headers
	for k, v := range r.headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	// Use ServeContent for proper range support, caching headers, etc.
	http.ServeContent(w, nil, filepath.Base(r.path), info.ModTime(), file)
	return nil
}

// File creates a file response
func File(path string) Response {
	return &fileResponse{
		baseResponse: newBaseResponse(http.StatusOK),
		path:         path,
	}
}

// detectContentType maps file extensions to MIME types
func detectContentType(ext string) string {
	types := map[string]string{
		".html": "text/html; charset=utf-8",
		".css":  "text/css; charset=utf-8",
		".js":   "application/javascript; charset=utf-8",
		".json": "application/json; charset=utf-8",
		".xml":  "application/xml; charset=utf-8",
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".pdf":  "application/pdf",
		".txt":  "text/plain; charset=utf-8",
	}

	if ct, ok := types[ext]; ok {
		return ct
	}

	return "application/octet-stream"
}

// ── Stream Response ──────────────────────────────────────────────────────────

type streamResponse struct {
	baseResponse
	contentType string
	reader      io.Reader
}

func (r *streamResponse) Write(w http.ResponseWriter) error {
	// Set content type
	if r.contentType != "" {
		r.headers.Set("Content-Type", r.contentType)
	}

	// Write headers
	for k, v := range r.headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	w.WriteHeader(r.status)

	_, err := io.Copy(w, r.reader)
	return err
}

// Stream creates a streaming response from an io.Reader
func Stream(contentType string, reader io.Reader) Response {
	return &streamResponse{
		baseResponse: newBaseResponse(http.StatusOK),
		contentType:  contentType,
		reader:       reader,
	}
}

// ── Chainable Methods ────────────────────────────────────────────────────────

// WithStatus returns a new response with the given status code
func WithStatus(r Response, status int) Response {
	switch resp := r.(type) {
	case *jsonResponse:
		return &jsonResponse{
			baseResponse: baseResponse{status: status, headers: resp.headers},
			data:         resp.data,
		}
	case *textResponse:
		return &textResponse{
			baseResponse: baseResponse{status: status, headers: resp.headers},
			text:         resp.text,
		}
	case *htmlResponse:
		return &htmlResponse{
			baseResponse: baseResponse{status: status, headers: resp.headers},
			html:         resp.html,
		}
	case *xmlResponse:
		return &xmlResponse{
			baseResponse: baseResponse{status: status, headers: resp.headers},
			data:         resp.data,
		}
	case *noContentResponse:
		return &noContentResponse{
			baseResponse: baseResponse{status: status, headers: resp.headers},
		}
	case *redirectResponse:
		return &redirectResponse{
			baseResponse: baseResponse{status: status, headers: resp.headers},
			url:          resp.url,
		}
	case *fileResponse:
		return &fileResponse{
			baseResponse: baseResponse{status: status, headers: resp.headers},
			path:         resp.path,
		}
	case *streamResponse:
		return &streamResponse{
			baseResponse: baseResponse{status: status, headers: resp.headers},
			contentType:  resp.contentType,
			reader:       resp.reader,
		}
	default:
		return r
	}
}

// WithHeader returns a new response with an additional header
func WithHeader(r Response, key, value string) Response {
	// Clone headers
	newHeaders := make(http.Header)
	for k, v := range r.Headers() {
		newHeaders[k] = v
	}
	newHeaders.Add(key, value)

	switch resp := r.(type) {
	case *jsonResponse:
		return &jsonResponse{
			baseResponse: baseResponse{status: resp.status, headers: newHeaders},
			data:         resp.data,
		}
	case *textResponse:
		return &textResponse{
			baseResponse: baseResponse{status: resp.status, headers: newHeaders},
			text:         resp.text,
		}
	case *htmlResponse:
		return &htmlResponse{
			baseResponse: baseResponse{status: resp.status, headers: newHeaders},
			html:         resp.html,
		}
	case *xmlResponse:
		return &xmlResponse{
			baseResponse: baseResponse{status: resp.status, headers: newHeaders},
			data:         resp.data,
		}
	case *noContentResponse:
		return &noContentResponse{
			baseResponse: baseResponse{status: resp.status, headers: newHeaders},
		}
	case *redirectResponse:
		return &redirectResponse{
			baseResponse: baseResponse{status: resp.status, headers: newHeaders},
			url:          resp.url,
		}
	case *fileResponse:
		return &fileResponse{
			baseResponse: baseResponse{status: resp.status, headers: newHeaders},
			path:         resp.path,
		}
	case *streamResponse:
		return &streamResponse{
			baseResponse: baseResponse{status: resp.status, headers: newHeaders},
			contentType:  resp.contentType,
			reader:       resp.reader,
		}
	default:
		return r
	}
}

// WithCache adds Cache-Control header with max-age
func WithCache(r Response, duration time.Duration) Response {
	seconds := int(duration.Seconds())
	return WithHeader(r, "Cache-Control", "max-age="+strconv.Itoa(seconds))
}

// WithNoCache adds Cache-Control: no-store header
func WithNoCache(r Response) Response {
	return WithHeader(r, "Cache-Control", "no-store")
}
