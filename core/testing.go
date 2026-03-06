package core

import (
	"net/http"
	"net/http/httptest"
)

// NewTestContext creates a Context for use in unit tests (without a real server).
// It accepts an optional http.ResponseWriter; if nil, a fresh ResponseRecorder is used.
func NewTestContext(w http.ResponseWriter, r *http.Request) *Context {
	if w == nil {
		w = httptest.NewRecorder()
	}
	if r == nil {
		r, _ = http.NewRequest(http.MethodGet, "/", nil)
	}
	c := &Context{
		Writer:  w,
		Request: r,
		params:  Params{},
		store:   make(map[string]interface{}),
	}
	return c
}

// SetTestContextParams sets path parameters on a test context.
// Used internally by gxtest when simulating routed requests.
func SetTestContextParams(c *Context, params Params) {
	c.params = params
}
