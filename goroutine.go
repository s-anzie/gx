package gx

import (
	"context"
	"log/slog"
	"runtime/debug"
)

// Go runs fn in a new goroutine with panic recovery.
// A panic in fn is caught, logged as an error with a stack trace,
// and the server continues operating normally.
//
// Always prefer gx.Go over bare `go func()` in handlers.
// Never store *gx.Context or *core.Context in the goroutine — extract needed
// values before launching.
//
//	id := c.Param("id")
//	gx.Go(func() {
//	    sendEmail(id)
//	})
func Go(fn func()) {
	go func() {
		defer recoverGoroutine()
		fn()
	}()
}

// GoCtx runs fn in a new goroutine with context propagation and panic recovery.
// Use this when the goroutine needs to respect request cancellation.
//
//	ctx := c.GoContext()
//	gx.GoCtx(ctx, func(ctx context.Context) {
//	    doAsyncWork(ctx)
//	})
func GoCtx(ctx context.Context, fn func(context.Context)) {
	go func() {
		defer recoverGoroutine()
		fn(ctx)
	}()
}

// recoverGoroutine is the deferred panic handler injected by Go/GoCtx.
func recoverGoroutine() {
	if r := recover(); r != nil {
		slog.Error("goroutine panic recovered",
			"panic", r,
			"stack", string(debug.Stack()),
		)
	}
}
