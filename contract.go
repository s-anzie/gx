package gx

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/s-anzie/gx/core"
)

// Contract describes what an endpoint accepts and returns
type Contract struct {
	Summary     string
	Description string
	Tags        []string
	Deprecated  bool

	// Input schemas
	Params SchemaRef // Path parameters
	Query  SchemaRef // Query string
	Body   SchemaRef // Request body

	// Output schema
	Output SchemaRef

	// Possible errors
	Errors []AppError
}

// SchemaRef is a reference to a type schema
type SchemaRef struct {
	typeName   string
	typeRef    reflect.Type
	validator  func(any) error
	defaultVal any
}

// Schema creates a SchemaRef for type T
// The schema is inferred from the type structure at compile time
func Schema[T any]() SchemaRef {
	var zero T
	typ := reflect.TypeOf(zero)

	return SchemaRef{
		typeName: typ.String(),
		typeRef:  typ,
	}
}

// TypeName returns the schema's type name
func (s SchemaRef) TypeName() string {
	return s.typeName
}

// Type returns the underlying reflect.Type
func (s SchemaRef) Type() reflect.Type {
	return s.typeRef
}

// IsZero returns true if the schema is not set
func (s SchemaRef) IsZero() bool {
	return s.typeRef == nil
}

// Validate validates a value against the schema
// Returns nil if valid, ValidationError otherwise
func (s SchemaRef) Validate(value any) error {
	if s.validator != nil {
		return s.validator(value)
	}
	// No validator - always valid
	return nil
}

// BindAndValidate binds request data to the schema type and validates it
// Returns the bound value or an error
func (s SchemaRef) BindAndValidate(data []byte) (any, error) {
	if s.typeRef == nil {
		return nil, fmt.Errorf("schema: cannot bind to zero schema")
	}

	// Create new instance of the type
	ptr := reflect.New(s.typeRef)
	instance := ptr.Interface()

	// Unmarshal JSON into instance
	if err := json.Unmarshal(data, instance); err != nil {
		return nil, ValidationError(FieldError{
			Field:   "body",
			Rule:    "json",
			Message: "Invalid JSON: " + err.Error(),
		})
	}

	// Validate if validator is present
	if s.validator != nil {
		if err := s.validator(instance); err != nil {
			return nil, err
		}
	}

	return instance, nil
}

// ── Contract Middleware ──────────────────────────────────────────────────────

// contractMiddleware validates request against contract and stores parsed data
func contractMiddleware(contract Contract) core.Middleware {
	return func(c *core.Context, next core.Handler) core.Response {
		// Validate and bind body if schema is present
		if !contract.Body.IsZero() {
			body, err := c.Body()
			if err != nil {
				return ErrValidation.With("body", "failed to read request body").ToResponse()
			}
			if len(body) == 0 {
				return ErrValidation.With("body", "request body is required").ToResponse()
			}

			// Bind and validate
			instance, err := contract.Body.BindAndValidate(body)
			if err != nil {
				// Check if it's already an AppError
				if appErr, ok := err.(AppError); ok {
					return appErr.ToResponse()
				}
				return ErrValidation.With("body", err.Error()).ToResponse()
			}

			// Store in context using type name as key
			c.Set(contract.Body.TypeName(), instance)
		}

		// TODO: Validate query parameters
		// TODO: Validate path parameters

		// Call next handler
		return next(c)
	}
}

// ── Route Builder with Contract ──────────────────────────────────────────────

// WithContract wraps a handler with contract validation middleware
// This is used internally when contracts are attached to routes
func WithContract(contract Contract, handler core.Handler) core.Handler {
	middleware := contractMiddleware(contract)
	return func(c *core.Context) core.Response {
		return middleware(c, handler)
	}
}

// ── OpenAPI Generation ───────────────────────────────────────────────────────

// OpenAPISpec generates an OpenAPI 3.1 specification from registered contracts
// This is a placeholder for future implementation
type OpenAPISpec struct {
	OpenAPI string                 `json:"openapi"`
	Info    OpenAPIInfo            `json:"info"`
	Paths   map[string]interface{} `json:"paths,omitempty"`
}

type OpenAPIInfo struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

// GenerateOpenAPI creates an OpenAPI specification
// Full implementation will be added with the OpenAPI plugin
func (app *App) GenerateOpenAPI(title, version string) OpenAPISpec {
	return OpenAPISpec{
		OpenAPI: "3.1.0",
		Info: OpenAPIInfo{
			Title:   title,
			Version: version,
		},
		Paths: make(map[string]interface{}),
	}
}
