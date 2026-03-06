# OpenAPI Plugin

Automatic OpenAPI 3.1 specification generation for GX framework with interactive documentation UIs.

## Features

✅ **Auto-Generated Specs** - Creates OpenAPI 3.1 specs from Contract definitions  
✅ **Multiple UIs** - Scalar, Swagger UI, or Redoc support  
✅ **Type-Safe** - Uses Go types for schema generation  
✅ **Contract-First** - Documentation generated from code contracts  
✅ **Production Ready** - Can disable docs in production  

## Installation

```go
import "github.com/s-anzie/gx/plugins/openapi"
```

## Basic Usage

```go
app := gx.New()

// Install OpenAPI plugin
app.Install(openapi.New(
    openapi.Title("My API"),
    openapi.Version("1.0.0"),
    openapi.UI(openapi.Scalar), // or Swagger, Redoc
))

// Define contract
var CreateUserContract = gx.Contract{
    Summary: "Create a new user",
    Tags:    []string{"users"},
    Body:    gx.Schema[CreateUserRequest](),
    Output:  gx.Schema[UserResponse](),
}

// Register route with contract
app.POST("/users", CreateUserContract, func(c *gx.Context) core.Response {
    // Handler implementation
    return c.JSON(user)
})

// Boot to generate spec
app.Boot(nil)
```

## Configuration Options

### UI Types

```go
openapi.UI(openapi.Scalar)   // Modern, interactive UI (default)
openapi.UI(openapi.Swagger)  // Classic Swagger UI
openapi.UI(openapi.Redoc)    // Documentation-focused UI
```

### API Information

```go
openapi.Title("User Management API")
openapi.Version("2.0.0")
openapi.Description("Comprehensive user management system")
```

### Custom Paths

```go
openapi.DocsPath("/documentation")  // Default: /docs
openapi.SpecPath("/api-spec.json")  // Default: /openapi.json
```

### Multiple Servers

```go
openapi.AddServer("http://localhost:8080", "Development")
openapi.AddServer("https://api.example.com", "Production")
```

### Disable in Production

```go
openapi.DisableInProduction(app)
```

## Contract Definition

Contracts describe what an endpoint accepts and returns:

```go
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required" description:"User's full name" example:"John Doe"`
    Email string `json:"email" validate:"required,email" description:"User email"`
    Age   int    `json:"age" validate:"min=18" description:"User age" example:"25"`
}

var CreateUser = gx.Contract{
    Summary:     "Create a new user",
    Description: "Creates a user account in the system",
    Tags:        []string{"users"},
    Body:        gx.Schema[CreateUserRequest](),
    Output:      gx.Schema[UserResponse](),
    Errors: []gx.AppError{
        {Status: 400, Code: "validation_error", Message: "Invalid data"},
        {Status: 409, Code: "email_exists", Message: "Email taken"},
    },
}
```

## Struct Tags

OpenAPI generation recognizes these struct tags:

- `json:"field_name"` - Field name in JSON/schema
- `validate:"required"` - Marks field as required
- `description:"..."` - Field description in docs
- `example:"value"` - Example value in docs

## Endpoints

After installation, two routes are automatically registered:

- **GET /openapi.json** - OpenAPI 3.1 specification (JSON)
- **GET /docs** - Interactive API documentation UI

## How It Works

1. **Contract Registration** - Routes registered with contracts are stored
2. **Boot Time Generation** - Spec generated once during `app.Boot()`
3. **Type Reflection** - Go types converted to JSON Schema
4. **Spec Serving** - Immutable spec served at `/openapi.json`
5. **UI Rendering** -HTML page loads chosen UI with spec URL

## Schema Generation

The plugin generates OpenAPI schemas from Go types:

```go
type User struct {
    ID    int    `json:"id" description:"Unique identifier"`
    Name  string `json:"name" validate:"required" example:"Jane"`
    Email string `json:"email" validate:"required,email"`
}
```

Becomes:

```json
{
  "type": "object",
  "required": ["name", "email"],
  "properties": {
    "id": {"type": "integer", "description": "Unique identifier"},
    "name": {"type": "string", "example": "Jane"},
    "email": {"type": "string"}
  }
}
```

## Supported Types

- **Primitives**: string, int, float, bool
- **Arrays/Slices**: `[]T` generates array schema
- **Structs**: Converted to object schemas
- **Nested**: Structs referenced via `#/components/schemas/`

## Example

See [examples/openapi/main.go](../../examples/openapi/main.go) for a complete working example with:

- Multiple contracts
- Request/response types
- Validation rules
- Error responses
- Multiple HTTP methods

## Browser Access

```bash
# Start your app
go run main.go

# View documentation
open http://localhost:8080/docs

# Download spec
curl http://localhost:8080/openapi.json > openapi.json
```

## Production Deployment

Disable docs in production while keeping spec generation:

```go
app.Install(openapi.New(
    openapi.Title("My API"),
    openapi.Version("1.0.0"),
    openapi.DisableInProduction(app),
))
```

When `app.Environment()` is `Production`, the /docs and /openapi.json routes won't be registered.

## Limitations

- Only supports JSON request/response bodies
- Path parameters extracted from struct tags (not path template)
- No support for oneOf/anyOf/allOf schemas yet

## Integration with Other Tools

Export the spec and use with:

- **Postman** - Import OpenAPI spec
- **Insomnia** - Load spec for testing
- **OpenAPI Generator** - Generate client SDKs
- **API Testing** - Automated contract testing

## Tests

```bash
cd plugins/openapi
go test -v
```
