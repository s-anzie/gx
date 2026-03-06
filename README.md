# GX - Go Web Framework

> **Version**: 0.1.0-alpha  
> **Status**: Active Development  
> **Go Version**: 1.22+

GX is a modern, high-performance web framework for Go that combines Express.js-like simplicity with Go's type safety and performance.

## Philosophy

1. **Explicit over Magic** - No hidden behaviors, no reflection at runtime
2. **Composable over Monolithic** - Independent, testable components
3. **Framework Works, Developer Expresses** - Automation through correct declarations

## Core Features

### ✓ Implemented (Core Layer)

- [x] **Response Interface** - Chainable, immutable response builders
- [x] **Context with Pooling** - Zero-allocation request context via `sync.Pool`
- [x] **Radix-Tree Router** - O(log n) route lookups with conflict detection at boot
- [x] **Middleware System** - Unique design where middleware can observe and modify responses
- [x] **Route Groups** - Nested routing with scoped middleware
- [x] **Server-Sent Events** - First-class SSE support
- [x] **TLS Utilities** - Auto-generated self-signed certificates for development
- [x] **Graceful Shutdown** - Clean shutdown with configurable timeouts

### 🚧 Planned (GX Layer)

- [ ] Contract & Schema system
- [ ] Error taxonomy
- [ ] Plugin system
- [ ] OpenAPI generation
- [ ] Observability (tracing, metrics, structured logging)
- [ ] Lifecycle hooks
- [ ] HTTP/3 support

## Installation

```bash
go get github.com/s-anzie/gx
```

## Quick Start

```go
package main

import (
    "github.com/s-anzie/gx/core"
    "log"
)

func main() {
    app := core.New()

    app.GET("/", func(c *core.Context) core.Response {
        return c.JSON(map[string]string{
            "message": "Hello, GX!",
        })
    })

    log.Fatal(app.Listen(":8080"))
}
```

## Core Concepts

### Handler Signature

The fundamental decision of GX: handlers return `Response` values.

```go
type Handler func(*Context) Response
```

**Why this matters:**

- ✓ Compiler-enforced - forgetting `return` is a compile error
- ✓ Testable without HTTP - unit test handlers directly
- ✓ Unified error handling - errors are responses, not separate code paths

Compare with other frameworks:

| Framework | Signature | Problem |
|-----------|-----------|---------|
| Gin | `func(*Context)` | Forgot `return`? Silent bug |
| Echo | `func(*Context) error` | Error vs response = two code paths |
| **GX** | `func(*Context) Response` | Unified, testable, enforced ✓ |

### Context

Pooled request context with rich API:

```go
func handler(c *core.Context) core.Response {
    // Routing
    id := c.Param("id")
    page := c.QueryDefault("page", "1")
    
    // Request body
    var data MyStruct
    c.BindJSON(&data)
    
    // Headers & metadata
    token := c.Header("Authorization")
    ip := c.ClientIP()
    
    // Protocol detection
    if c.IsHTTP2() {
        // handle HTTP/2
    }
    
    // Store per-request values
    c.Set("user", user)
    
    return c.JSON(data)
}
```

### Middleware

Unlike Express, GX middleware **receives the response** from the next handler:

```go
func timer(c *core.Context, next core.Handler) core.Response {
    start := time.Now()
    
    res := next(c)  // handler executes here
    
    // After - full access to the Response
    duration := time.Since(start)
    return core.WithHeader(res, "X-Duration", duration.String())
}
```

This is structurally impossible in Express where `next()` is fire-and-forget.

### Responses

All response types are chainable and immutable:

```go
// JSON response
return c.JSON(user)

// with custom status and headers
return c.JSON(user).
    Status(201).
    Header("Location", "/users/"+user.ID).
    Cache(5 * time.Minute)

// Text response
return c.Text("Hello, %s!", name)

// HTML response
return c.HTML("<h1>Welcome</h1>")

// File response
return c.File("/path/to/file.pdf")

// Stream response
return c.Stream("video/mp4", videoReader)

// Redirect
return c.Redirect("/login").Permanent()

// No content
return c.NoContent()
```

### Router

Radix-tree with deterministic conflict resolution:

```go
app := core.New()

// Static routes
app.GET("/users/me", getCurrentUser)

// Parameters
app.GET("/users/:id", getUser)

// Wildcards
app.GET("/files/*path", serveFile)
```

Priority: `static > parameter > wildcard`

Conflicts detected at **boot time**, not runtime:

```go
app.GET("/users/:id", handler1)
app.GET("/users/:uid", handler2)  // PANIC: conflicting param names
```

### Groups

Organize routes with shared prefixes and middleware:

```go
api := app.Group("/api/v1")
api.Use(auth, rateLimit)

users := api.Group("/users")
users.GET("", listUsers)           // GET /api/v1/users
users.GET("/:id", getUser)         // GET /api/v1/users/:id
users.POST("", createUser)         // POST /api/v1/users
users.PATCH("/:id", updateUser)    // PATCH /api/v1/users/:id
users.DELETE("/:id", deleteUser)   // DELETE /api/v1/users/:id
```

### Server-Sent Events

First-class SSE support:

```go
app.GET("/events", func(c *core.Context) core.Response {
    return core.SSE(func(stream *core.SSEStream) error {
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()
        
        for {
            select {
            case <-stream.Context().Done():
                return nil
            case t := <-ticker.C:
                stream.Send(core.SSEEvent{
                    Event: "time",
                    Data:  t.Format(time.RFC3339),
                })
            }
        }
    })
})
```

### TLS

Development TLS made easy:

```go
app := core.New()

// Auto-generate self-signed certificate
app.WithDevTLS("localhost", "127.0.0.1")

app.Listen(":8443")  // HTTPS enabled
```

Or use your own certificates:

```go
app.WithTLS("cert.pem", "key.pem")
```

### Graceful Shutdown

Built-in support for clean shutdowns:

```go
app := core.New()

// ... register routes ...

// Handles SIGTERM/SIGINT automatically
log.Fatal(app.ListenWithGracefulShutdown(":8080"))
```

## Examples

See the [`examples/`](./examples) directory:

- [`examples/basic/`](./examples/basic) - Basic routing and middleware
- [`examples/sse/`](./examples/sse) - Server-Sent Events demo
- [`examples/tls/`](./examples/tls) - HTTPS with self-signed certificate

Run an example:

```bash
go run examples/basic/main.go
```

## Architecture

```
┌──────────────────────────────────────┐
│         Application Code             │
├──────────────────────────────────────┤
│         GX Layer (planned)           │
│  Contracts · Schema · Errors         │
│  Plugins · Observability             │
├──────────────────────────────────────┤
│         Core Layer (✓)               │
│  Router · Context · Response         │
│  Middleware · Groups · SSE           │
├──────────────┬───────────────────────┤
│  net/http    │  (Future: HTTP/2)     │
└──────────────┴───────────────────────┘
```

## Performance

- **Zero allocations** on hot path via context pooling
- **O(log n)** route lookups with radix-tree
- **No reflection** at request time
- **Deterministic** - conflicts detected at boot, not runtime

## Roadmap

### Phase 1: Core (Current)
- [x] Response system
- [x] Context & pooling
- [x] Router with radix-tree
- [x] Middleware
- [x] Groups
- [x] SSE
- [x] TLS utilities

### Phase 2: GX Layer (Next)
- [ ] Contract & Schema system
- [ ] Error taxonomy
- [ ] Plugin architecture
- [ ] OpenAPI generation
- [ ] Validation

### Phase 3: Observability
- [ ] Structured logging
- [ ] Tracing (OpenTelemetry)
- [ ] Metrics (Prometheus)

### Phase 4: Advanced
- [ ] HTTP/3 support
- [ ] WebSocket channels
- [ ] QUIC primitives

## Design Principles

### Handler Return Values

```go
// ✓ Good - explicit return, testable
func getUser(c *core.Context) core.Response {
    return c.JSON(user)
}

// ✗ Bad (other frameworks) - side effect, easy to forget return
func getUser(c SomeContext) {
    c.JSON(user)  // forgot return? double send!
}
```

### Middleware & Response Access

```go
// GX - middleware sees the response
func log(c *core.Context, next core.Handler) core.Response {
    res := next(c)
    log.Printf("Status: %d", res.Status())  // ✓ can observe
    return res
}

// Express - fire and forget
function log(req, res, next) {
    next()  // can't see what happened after
}
```

### Error Handling

In GX, errors ARE responses:

```go
// Unified path
if err != nil {
    return c.Fail(ErrNotFound)  // returns Response
}
return c.JSON(data)  // also returns Response
```

No special error handling, no double code paths.

## Testing

Handlers are pure functions - easy to test:

```go
func TestGetUser(t *testing.T) {
    c := makeMockContext("id", "123")
    
    res := getUser(c)
    
    assert.Equal(t, 200, res.Status())
    // no HTTP server needed!
}
```

## Contributing

GX is in active development. Contributions welcome!

1. Read the [design specs](./specs)
2. Fork and create a feature branch
3. Ensure tests pass
4. Submit a PR

## License

MIT License - see [LICENSE](./LICENSE)

## Credits

Inspired by:
- **Express.js** - minimalism and developer experience
- **Gin** - performance focus
- **Echo** - clean API design
- But taking full advantage of Go's strengths

---

**Status**: Core layer complete, GX layer in progress.  
**Next**: Contract system, Error taxonomy, Plugin architecture
