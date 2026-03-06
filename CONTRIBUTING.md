# Contributing to GX

Thank you for your interest in contributing to GX! This document provides guidelines and information for contributors.

## Project Status

GX is in active development. The Core layer is complete and stable. The GX layer (contracts, errors, plugins) is the current focus.

See [STATUS.md](./STATUS.md) for detailed implementation status.

## Development Setup

### Prerequisites

- Go 1.22 or later
- Git
- A text editor or IDE with Go support

### Getting Started

1. Fork the repository on GitHub
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/gx.git
   cd gx
   ```

3. Add the upstream repository:
   ```bash
   git remote add upstream https://github.com/s-anzie/gx.git
   ```

4. Create a branch for your work:
   ```bash
   git checkout -b feature/my-feature
   ```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./gxtest -v
```

### Building Examples

```bash
# Build all examples
go build ./examples/basic
go build ./examples/sse
go build ./examples/tls

# Run an example
go run examples/basic/main.go
```

## Project Structure

```
gx/
├── core/              # Core layer - transport and routing
│   ├── app.go         # Application server
│   ├── context.go     # Request context
│   ├── response.go    # Response interface and builders
│   ├── router.go      # Radix-tree router
│   ├── middleware.go  # Middleware system
│   ├── group.go       # Route grouping
│   ├── params.go      # Route parameters
│   ├── sse.go         # Server-Sent Events
│   └── tls.go         # TLS utilities
├── gxtest/            # Test utilities and tests
├── examples/          # Example applications
│   ├── basic/         # Basic routing and middleware
│   ├── sse/           # Server-Sent Events demo
│   └── tls/           # HTTPS with self-signed cert
├── specs/             # Design specifications
│   ├── core.md        # Core layer spec
│   └── init.md        # Overall framework spec
├── plugins/           # Standard plugins (future)
└── README.md          # Main documentation
```

## Design Principles

When contributing, please keep these principles in mind:

### 1. Explicit over Magic

- No hidden behaviors or implicit state
- Avoid reflection at request time (compile time is OK)
- Clear, documented APIs

### 2. Composable over Monolithic

- Each component should be independently testable
- Avoid tight coupling between layers
- The Core should work without GX layer

### 3. Framework Works, Developer Expresses

- Automate what can be inferred from declarations
- Don't require boilerplate for common patterns
- Let the compiler catch errors when possible

## Code Style

### Go Conventions

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable names
- Write documentation comments for exported types and functions
- Keep functions focused and reasonably sized

### Comments

- Document the "why", not just the "what"
- Include examples for non-trivial APIs
- Reference design decisions when applicable

Example:

```go
// Response is the core response interface that all response types must implement.
// It's designed to be chainable and immutable - each method returns a new Response.
//
// Unlike traditional web frameworks where responses are written via side effects,
// GX handlers return Response values. This enables:
//
//   - Compile-time enforcement (forgot return = compile error)
//   - Unit testing without HTTP infrastructure
//   - Unified error and success handling
//
// Example:
//
//   func getUser(c *Context) Response {
//       user, err := db.Find(id)
//       if err != nil {
//           return c.Fail(ErrNotFound)  // error is a Response
//       }
//       return c.JSON(user)  // success is a Response
//   }
type Response interface {
    Status() int
    Headers() http.Header
    Write(w http.ResponseWriter) error
}
```

### Error Handling

- Return errors from functions that can fail
- Don't panic except for programming errors (e.g., invalid configuration at boot)
- Provide context in error messages

```go
// Good
if file == nil {
    return fmt.Errorf("failed to open config file: %w", err)
}

// Bad
if file == nil {
    panic(err)  // Don't panic for runtime errors
}
```

## Testing Guidelines

### Unit Tests

- Test files end with `_test.go`
- Use table-driven tests for multiple cases
- Test both success and error paths
- Avoid external dependencies (network, filesystem, databases)

Example:

```go
func TestResponse(t *testing.T) {
    tests := []struct {
        name     string
        response Response
        wantCode int
    }{
        {
            name:     "JSON response",
            response: JSON(map[string]string{"key": "value"}),
            wantCode: 200,
        },
        {
            name:     "Created response",
            response: Created(user),
            wantCode: 201,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := tt.response.Status(); got != tt.wantCode {
                t.Errorf("Status() = %v, want %v", got, tt.wantCode)
            }
        })
    }
}
```

### Integration Tests

- Use `httptest` for HTTP-level testing
- Create minimal reproducible scenarios
- Clean up resources in `defer` statements

### Benchmarks

- Benchmark files end with `_test.go`
- Focus on hot paths (request handling, routing)
- Include baseline comparisons when optimizing

```go
func BenchmarkRouter(b *testing.B) {
    router := NewRouter()
    router.Add("GET", "/users/:id", handler)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        router.Find("GET", "/users/123")
    }
}
```

## Pull Request Process

1. **Create an Issue First** (for significant changes)
   - Describe the problem or feature
   - Discuss the approach before coding
   - Get feedback from maintainers

2. **Write Code**
   - Follow the code style guidelines
   - Add tests for new functionality
   - Update documentation as needed

3. **Commit Messages**
   - Use clear, descriptive commit messages
   - Reference issue numbers when applicable
   - Format: `component: brief description`
   
   Examples:
   ```
   core/router: add support for optional parameters
   docs: update README with middleware examples
   test: add integration tests for SSE
   ```

4. **Push and Create PR**
   - Push to your fork
   - Create a PR from your branch to `main`
   - Fill out the PR template
   - Link related issues

5. **Code Review**
   - Respond to feedback
   - Make requested changes
   - Ask questions if something is unclear

6. **Merge**
   - Once approved, a maintainer will merge
   - Your contribution is now part of GX!

## What to Contribute

### High Priority

The GX layer is the current focus. Key areas:

- **Error Taxonomy** - AppError, error codes, error handling
- **Contract System** - Schema definition, validation integration
- **OpenAPI Generation** - Contract-to-OpenAPI conversion
- **Standard Plugins** - Auth, CORS, rate limiting, etc.

### Always Welcome

- **Bug fixes** - Found a bug? Please report and/or fix it!
- **Documentation** - Examples, guides, API docs
- **Tests** - Improve coverage, add edge cases
- **Performance** - Benchmarks, optimizations
- **Examples** - Real-world usage demonstrations

### Discussion Needed

For larger changes, please open an issue first:

- New features not in the specs
- Breaking changes to existing APIs
- Major refactoring
- Architectural decisions

## Design Specification

Before contributing to core features, read the design specs:

- [`specs/core.md`](./specs/core.md) - Core layer design
- [`specs/init.md`](./specs/init.md) - Overall framework design

These documents explain the "why" behind design decisions and provide context for implementation choices.

## License

By contributing to GX, you agree that your contributions will be licensed under the MIT License.

## Questions?

- Open an issue for bugs or feature requests
- Start a discussion for questions about usage or design
- Check existing issues and discussions first

## Code of Conduct

- Be respectful and constructive
- Welcome newcomers
- Focus on what's best for the project
- Assume good intentions

---

Thank you for contributing to GX! 🚀
