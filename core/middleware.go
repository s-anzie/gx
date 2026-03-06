package core

// Middleware represents a function that wraps a Handler.
// Unlike traditional middleware patterns, it receives the next Handler
// as a parameter and can observe (and modify) the Response after execution.
//
// This is fundamentally different from Express-style middleware where next()
// is a fire-and-forget callback. Here, the Response flows back up the chain
// as a value, enabling powerful composition patterns.
//
// Example:
//
//	func timer(c *Context, next Handler) Response {
//	    start := time.Now()
//	    res := next(c)  // handler executes here
//	    // After - full access to the produced Response
//	    return WithHeader(res, "X-Duration-Ms", strconv.Itoa(int(time.Since(start).Milliseconds())))
//	}
type Middleware func(c *Context, next Handler) Response

// WrapHandler converts a Middleware into a Handler by providing the next handler
func WrapHandler(mw Middleware, next Handler) Handler {
	return func(c *Context) Response {
		return mw(c, next)
	}
}

// Chain combines multiple middleware functions with a final handler
func Chain(middlewares []Middleware, handler Handler) Handler {
	// Build the chain from right to left
	// The final handler is wrapped by each middleware in reverse order
	h := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = WrapHandler(middlewares[i], h)
	}
	return h
}
