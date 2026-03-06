// Package gxtest provides utilities for testing GX handlers, routers, and apps
// without starting a real HTTP server.
//
//	res := gxtest.GET(app, "/users").
//	    Header("Authorization", "Bearer token").
//	    Do(t)
//	res.AssertStatus(t, 200)
package gxtest

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/s-anzie/gx/core"
)

// Target is anything that can handle an HTTP request.
// Supports *core.App, *core.Router, or core.Handler.
type Target interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// handlerTarget wraps a core.Handler as an http.Handler for testing.
type handlerTarget struct {
	handler core.Handler
	app     *core.App
}

func (h *handlerTarget) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := core.NewTestContext(w, r)
	res := h.handler(c)
	if res != nil {
		_ = res.Write(w)
	}
}

// routerTarget wraps a *core.Router as an http.Handler for testing.
type routerTarget struct {
	router *core.Router
}

func (h *routerTarget) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler, params := h.router.Find(r.Method, r.URL.Path)
	if handler == nil {
		http.NotFound(w, r)
		return
	}
	c := core.NewTestContext(w, r)
	core.SetTestContextParams(c, params)
	res := handler(c)
	if res != nil {
		_ = res.Write(w)
	}
}

// toTarget normalizes v into a http.Handler target.
func toTarget(v any) http.Handler {
	switch t := v.(type) {
	case http.Handler:
		return t
	case core.Handler:
		return &handlerTarget{handler: t}
	case *core.Router:
		return &routerTarget{router: t}
	default:
		panic(fmt.Sprintf("gxtest: unsupported target type %T", v))
	}
}

// ── Request Builder ──────────────────────────────────────────────────────────

// RequestBuilder builds a test request with a fluent API.
type RequestBuilder struct {
	target      http.Handler
	method      string
	path        string
	headers     map[string]string
	cookies     []http.Cookie
	queryParams url.Values
	body        io.Reader
	contentType string
}

func newRequest(target any, method, path string) *RequestBuilder {
	return &RequestBuilder{
		target:      toTarget(target),
		method:      method,
		path:        path,
		headers:     make(map[string]string),
		queryParams: make(url.Values),
	}
}

// GET creates a GET request builder.
func GET(target any, path string) *RequestBuilder {
	return newRequest(target, http.MethodGet, path)
}

// POST creates a POST request builder.
func POST(target any, path string) *RequestBuilder {
	return newRequest(target, http.MethodPost, path)
}

// PUT creates a PUT request builder.
func PUT(target any, path string) *RequestBuilder {
	return newRequest(target, http.MethodPut, path)
}

// PATCH creates a PATCH request builder.
func PATCH(target any, path string) *RequestBuilder {
	return newRequest(target, http.MethodPatch, path)
}

// DELETE creates a DELETE request builder.
func DELETE(target any, path string) *RequestBuilder {
	return newRequest(target, http.MethodDelete, path)
}

// Header adds a request header.
func (rb *RequestBuilder) Header(key, value string) *RequestBuilder {
	rb.headers[key] = value
	return rb
}

// Cookie adds a cookie to the request.
func (rb *RequestBuilder) Cookie(name, value string) *RequestBuilder {
	rb.cookies = append(rb.cookies, http.Cookie{Name: name, Value: value})
	return rb
}

// Query adds a query parameter.
func (rb *RequestBuilder) Query(key, value string) *RequestBuilder {
	rb.queryParams.Set(key, value)
	return rb
}

// JSON sets the request body as JSON and the Content-Type header.
func (rb *RequestBuilder) JSON(v any) *RequestBuilder {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("gxtest.JSON: marshal error: %v", err))
	}
	rb.body = bytes.NewReader(data)
	rb.contentType = "application/json"
	return rb
}

// XML sets the request body as XML and the Content-Type header.
func (rb *RequestBuilder) XML(v any) *RequestBuilder {
	data, err := xml.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("gxtest.XML: marshal error: %v", err))
	}
	rb.body = bytes.NewReader(data)
	rb.contentType = "application/xml"
	return rb
}

// Body sets a raw body with the given content type.
func (rb *RequestBuilder) Body(r io.Reader, contentType string) *RequestBuilder {
	rb.body = r
	rb.contentType = contentType
	return rb
}

// BodyString sets a raw string body with the given content type.
func (rb *RequestBuilder) BodyString(s, contentType string) *RequestBuilder {
	return rb.Body(strings.NewReader(s), contentType)
}

// Do executes the request and returns the response.
func (rb *RequestBuilder) Do(t *testing.T) *RecordedResponse {
	t.Helper()

	// Build URL
	target := rb.path
	if len(rb.queryParams) > 0 {
		target += "?" + rb.queryParams.Encode()
	}

	// Create request
	req := httptest.NewRequest(rb.method, target, rb.body)

	// Apply headers
	for k, v := range rb.headers {
		req.Header.Set(k, v)
	}

	// Apply content type
	if rb.contentType != "" {
		req.Header.Set("Content-Type", rb.contentType)
	}

	// Apply cookies
	for _, c := range rb.cookies {
		req.AddCookie(&c)
	}

	// Execute
	w := httptest.NewRecorder()
	rb.target.ServeHTTP(w, req)

	return &RecordedResponse{
		t:        t,
		recorder: w,
		body:     w.Body.String(),
	}
}

// ── Test Response ─────────────────────────────────────────────────────────────

// RecordedResponse wraps the recorded response with assertion helpers.
type RecordedResponse struct {
	t        *testing.T
	recorder *httptest.ResponseRecorder
	body     string
}

// Status returns the response status code.
func (r *RecordedResponse) Status() int {
	return r.recorder.Code
}

// Body returns the response body as a string.
func (r *RecordedResponse) Body() string {
	return r.body
}

// Header returns the value of a response header.
func (r *RecordedResponse) Header(key string) string {
	return r.recorder.Header().Get(key)
}

// AssertStatus asserts the response status code matches expected.
func (r *RecordedResponse) AssertStatus(t *testing.T, expected int) {
	t.Helper()
	if r.recorder.Code != expected {
		t.Errorf("expected status %d, got %d\nbody: %s", expected, r.recorder.Code, r.body)
	}
}

// AssertHeader asserts a response header equals expected.
func (r *RecordedResponse) AssertHeader(t *testing.T, key, expected string) {
	t.Helper()
	got := r.recorder.Header().Get(key)
	if got != expected {
		t.Errorf("header %q: expected %q, got %q", key, expected, got)
	}
}

// AssertHeaderContains asserts a response header contains the substring.
func (r *RecordedResponse) AssertHeaderContains(t *testing.T, key, substr string) {
	t.Helper()
	got := r.recorder.Header().Get(key)
	if !strings.Contains(got, substr) {
		t.Errorf("header %q: expected to contain %q, got %q", key, substr, got)
	}
}

// AssertBodyContains asserts the response body contains the given substring.
func (r *RecordedResponse) AssertBodyContains(t *testing.T, substr string) {
	t.Helper()
	if !strings.Contains(r.body, substr) {
		t.Errorf("expected body to contain %q\nbody: %s", substr, r.body)
	}
}

// AssertBodyEquals asserts the response body equals the exact string.
func (r *RecordedResponse) AssertBodyEquals(t *testing.T, expected string) {
	t.Helper()
	if r.body != expected {
		t.Errorf("expected body %q\ngot: %q", expected, r.body)
	}
}

// AssertJSON unmarshals the response body into out and calls fn for assertions.
//
//	res.AssertJSON(t, &UserResponse{}, func(u *UserResponse) {
//	    if u.Name != "Alice" { t.Error("wrong name") }
//	})
func (r *RecordedResponse) AssertJSON(t *testing.T, out any, fn func()) {
	t.Helper()
	if err := json.Unmarshal([]byte(r.body), out); err != nil {
		t.Fatalf("AssertJSON: failed to unmarshal response body: %v\nbody: %s", err, r.body)
	}
	fn()
}

// AssertError asserts the response contains a GX structured error with the given code.
func (r *RecordedResponse) AssertError(t *testing.T, code string) {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(r.body), &payload); err != nil {
		t.Fatalf("AssertError: failed to unmarshal error response: %v\nbody: %s", err, r.body)
	}
	if payload.Error.Code != code {
		t.Errorf("AssertError: expected error code %q, got %q\nbody: %s", code, payload.Error.Code, r.body)
	}
}

// AssertErrorStatus asserts both error code and status.
func (r *RecordedResponse) AssertErrorStatus(t *testing.T, code string, status int) {
	t.Helper()
	r.AssertStatus(t, status)
	r.AssertError(t, code)
}

// AssertCookie asserts a cookie with the given name is present in the response.
func (r *RecordedResponse) AssertCookie(t *testing.T, name string) {
	t.Helper()
	for _, cookie := range r.recorder.Result().Cookies() {
		if cookie.Name == name {
			return
		}
	}
	t.Errorf("expected cookie %q to be set", name)
}

// AssertNoCookie asserts no cookie with the given name is present.
func (r *RecordedResponse) AssertNoCookie(t *testing.T, name string) {
	t.Helper()
	for _, cookie := range r.recorder.Result().Cookies() {
		if cookie.Name == name {
			t.Errorf("expected cookie %q to not be set", name)
			return
		}
	}
}

// AssertContentType asserts the Content-Type header.
func (r *RecordedResponse) AssertContentType(t *testing.T, expected string) {
	t.Helper()
	r.AssertHeaderContains(t, "Content-Type", expected)
}
