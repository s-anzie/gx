package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/s-anzie/gx"
	"github.com/s-anzie/gx/core"
)

// UIType represents the type of API documentation UI
type UIType string

const (
	Scalar  UIType = "scalar"
	Swagger UIType = "swagger"
	Redoc   UIType = "redoc"
)

// Config holds OpenAPI plugin configuration
type Config struct {
	Title       string
	Version     string
	Description string
	DocsPath    string
	SpecPath    string
	UI          UIType
	Servers     []Server
	Disabled    bool
}

// Server represents an OpenAPI server entry
type Server struct {
	URL         string
	Description string
}

// openapiPlugin implements the OpenAPI documentation plugin
type openapiPlugin struct {
	config *Config
	spec   *OpenAPISpec
}

// New creates a new OpenAPI plugin
func New(opts ...Option) gx.Plugin {
	config := &Config{
		Title:       "API Documentation",
		Version:     "1.0.0",
		Description: "",
		DocsPath:    "/docs",
		SpecPath:    "/openapi.json",
		UI:          Scalar,
		Servers:     []Server{{URL: "http://localhost:8080", Description: "Development server"}},
		Disabled:    false,
	}

	for _, opt := range opts {
		opt(config)
	}

	return &openapiPlugin{
		config: config,
	}
}

// Name returns the plugin name
func (p *openapiPlugin) Name() string {
	return "openapi"
}

// OnBoot generates the OpenAPI specification from registered contracts
func (p *openapiPlugin) OnBoot(app *gx.App) error {
	if p.config.Disabled {
		return nil
	}

	// Generate spec from contracts
	p.spec = p.generateSpec(app)

	// Register routes for spec and docs
	app.App.GET(p.config.SpecPath, func(c *core.Context) core.Response {
		return c.JSON(p.spec)
	})

	app.App.GET(p.config.DocsPath, func(c *core.Context) core.Response {
		html := p.generateDocsHTML()
		return core.HTML(html)
	})

	return nil
}

// OnShutdown is called when the application shuts down
func (p *openapiPlugin) OnShutdown(ctx context.Context) error {
	return nil
}

// generateSpec creates the OpenAPI 3.1 specification
func (p *openapiPlugin) generateSpec(app *gx.App) *OpenAPISpec {
	spec := &OpenAPISpec{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:       p.config.Title,
			Version:     p.config.Version,
			Description: p.config.Description,
		},
		Servers: make([]ServerSpec, len(p.config.Servers)),
		Paths:   make(map[string]PathItem),
		Components: Components{
			Schemas: make(map[string]Schema),
		},
	}

	for i, srv := range p.config.Servers {
		spec.Servers[i] = ServerSpec{
			URL:         srv.URL,
			Description: srv.Description,
		}
	}

	contracts := app.GetContracts()
	for _, rc := range contracts {
		pathKey := rc.Path

		pathItem, exists := spec.Paths[pathKey]
		if !exists {
			pathItem = PathItem{}
		}

		operation := p.createOperation(rc.Contract, spec)
		method := strings.ToLower(rc.Method)

		switch method {
		case "get":
			pathItem.Get = &operation
		case "post":
			pathItem.Post = &operation
		case "put":
			pathItem.Put = &operation
		case "patch":
			pathItem.Patch = &operation
		case "delete":
			pathItem.Delete = &operation
		}

		spec.Paths[pathKey] = pathItem
	}

	return spec
}

// createOperation creates an OpenAPI operation from a contract
func (p *openapiPlugin) createOperation(contract gx.Contract, spec *OpenAPISpec) Operation {
	op := Operation{
		Summary:     contract.Summary,
		Description: contract.Description,
		Tags:        contract.Tags,
		Deprecated:  contract.Deprecated,
		Responses:   make(map[string]Response),
	}

	if !contract.Body.IsZero() {
		schema := p.typeToSchema(contract.Body.Type(), spec)
		op.RequestBody = &RequestBody{
			Required: true,
			Content: map[string]MediaType{
				"application/json": {Schema: schema},
			},
		}
	}

	if !contract.Query.IsZero() {
		op.Parameters = append(op.Parameters, p.typeToParameters(contract.Query.Type(), "query")...)
	}

	if !contract.Params.IsZero() {
		op.Parameters = append(op.Parameters, p.typeToParameters(contract.Params.Type(), "path")...)
	}

	if !contract.Output.IsZero() {
		schema := p.typeToSchema(contract.Output.Type(), spec)
		op.Responses["200"] = Response{
			Description: "Successful response",
			Content: map[string]MediaType{
				"application/json": {Schema: schema},
			},
		}
	} else {
		op.Responses["200"] = Response{Description: "Successful response"}
	}

	for _, err := range contract.Errors {
		statusCode := fmt.Sprintf("%d", err.Status)
		op.Responses[statusCode] = Response{
			Description: err.Message,
			Content: map[string]MediaType{
				"application/json": {
					Schema: SchemaRef{
						Type: "object",
						Properties: map[string]Schema{
							"error":   {Type: "string"},
							"code":    {Type: "string"},
							"message": {Type: "string"},
						},
					},
				},
			},
		}
	}

	return op
}

// typeToSchema converts a reflect.Type to an OpenAPI schema
func (p *openapiPlugin) typeToSchema(typ reflect.Type, spec *OpenAPISpec) SchemaRef {
	if typ == nil {
		return SchemaRef{Type: "object"}
	}

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.String:
		return SchemaRef{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return SchemaRef{Type: "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return SchemaRef{Type: "integer", Format: "uint"}
	case reflect.Float32, reflect.Float64:
		return SchemaRef{Type: "number"}
	case reflect.Bool:
		return SchemaRef{Type: "boolean"}
	case reflect.Slice, reflect.Array:
		itemSchema := p.typeToSchema(typ.Elem(), spec)
		return SchemaRef{
			Type:  "array",
			Items: &itemSchema,
		}
	case reflect.Struct:
		return p.structToSchema(typ, spec)
	}

	return SchemaRef{Type: "object"}
}

// structToSchema converts a struct type to an OpenAPI schema
func (p *openapiPlugin) structToSchema(typ reflect.Type, spec *OpenAPISpec) SchemaRef {
	schemaName := typ.Name()

	if _, exists := spec.Components.Schemas[schemaName]; exists {
		return SchemaRef{Ref: "#/components/schemas/" + schemaName}
	}

	schema := Schema{
		Type:       "object",
		Properties: make(map[string]Schema),
		Required:   []string{},
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		tagParts := strings.Split(jsonTag, ",")
		fieldName := tagParts[0]

		fieldSchema := p.reflectTypeToSchema(field.Type, spec)

		if desc := field.Tag.Get("description"); desc != "" {
			fieldSchema.Description = desc
		}

		if example := field.Tag.Get("example"); example != "" {
			fieldSchema.Example = example
		}

		schema.Properties[fieldName] = fieldSchema

		validateTag := field.Tag.Get("validate")
		if strings.Contains(validateTag, "required") {
			schema.Required = append(schema.Required, fieldName)
		}
	}

	if schemaName != "" {
		spec.Components.Schemas[schemaName] = schema
		return SchemaRef{Ref: "#/components/schemas/" + schemaName}
	}

	return SchemaRef{
		Type:       schema.Type,
		Properties: schema.Properties,
		Required:   schema.Required,
	}
}

// reflectTypeToSchema converts a reflect.Type to a Schema (without refs)
func (p *openapiPlugin) reflectTypeToSchema(typ reflect.Type, spec *OpenAPISpec) Schema {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.String:
		return Schema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Schema{Type: "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Schema{Type: "integer", Format: "uint"}
	case reflect.Float32, reflect.Float64:
		return Schema{Type: "number"}
	case reflect.Bool:
		return Schema{Type: "boolean"}
	case reflect.Slice, reflect.Array:
		itemSchema := p.reflectTypeToSchema(typ.Elem(), spec)
		return Schema{
			Type:  "array",
			Items: &SchemaRef{Type: itemSchema.Type},
		}
	case reflect.Struct:
		ref := p.structToSchema(typ, spec)
		if ref.Ref != "" {
			return Schema{Ref: ref.Ref}
		}
		return Schema{
			Type:       "object",
			Properties: ref.Properties,
			Required:   ref.Required,
		}
	}

	return Schema{Type: "object"}
}

// typeToParameters converts a struct type to OpenAPI parameters
func (p *openapiPlugin) typeToParameters(typ reflect.Type, in string) []Parameter {
	if typ == nil {
		return nil
	}

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return nil
	}

	params := []Parameter{}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}

		tagParts := strings.Split(tag, ",")
		paramName := tagParts[0]

		param := Parameter{
			Name:        paramName,
			In:          in,
			Description: field.Tag.Get("description"),
			Required:    strings.Contains(field.Tag.Get("validate"), "required") || in == "path",
			Schema:      p.fieldTypeToSimpleSchema(field.Type),
		}

		params = append(params, param)
	}

	return params
}

// fieldTypeToSimpleSchema converts a field type to a simple schema
func (p *openapiPlugin) fieldTypeToSimpleSchema(typ reflect.Type) SchemaRef {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	switch typ.Kind() {
	case reflect.String:
		return SchemaRef{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return SchemaRef{Type: "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return SchemaRef{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return SchemaRef{Type: "number"}
	case reflect.Bool:
		return SchemaRef{Type: "boolean"}
	default:
		return SchemaRef{Type: "string"}
	}
}

// generateDocsHTML generates the HTML for the API documentation UI
func (p *openapiPlugin) generateDocsHTML() string {
	switch p.config.UI {
	case Scalar:
		return p.generateScalarHTML()
	case Swagger:
		return p.generateSwaggerHTML()
	case Redoc:
		return p.generateRedocHTML()
	default:
		return p.generateScalarHTML()
	}
}

// generateScalarHTML generates Scalar UI HTML
func (p *openapiPlugin) generateScalarHTML() string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>%s - API Documentation</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
</head>
<body>
    <script id="api-reference" data-url="%s"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.24.0"></script>
</body>
</html>`, p.config.Title, p.config.SpecPath)
}

// generateSwaggerHTML generates Swagger UI HTML
func (p *openapiPlugin) generateSwaggerHTML() string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>%s - API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui.css" />
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: "%s",
                dom_id: '#swagger-ui',
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                layout: "BaseLayout"
            });
        };
    </script>
</body>
</html>`, p.config.Title, p.config.SpecPath)
}

// generateRedocHTML generates Redoc UI HTML
func (p *openapiPlugin) generateRedocHTML() string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>%s - API Documentation</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
    <style>
        body {
            margin: 0;
            padding: 0;
        }
    </style>
</head>
<body>
    <redoc spec-url='%s'></redoc>
    <script src="https://cdn.jsdelivr.net/npm/redoc@latest/bundles/redoc.standalone.js"></script>
</body>
</html>`, p.config.Title, p.config.SpecPath)
}

// Option is a functional option for OpenAPI configuration
type Option func(*Config)

// Title sets the API title
func Title(title string) Option {
	return func(c *Config) {
		c.Title = title
	}
}

// Version sets the API version
func Version(version string) Option {
	return func(c *Config) {
		c.Version = version
	}
}

// Description sets the API description
func Description(desc string) Option {
	return func(c *Config) {
		c.Description = desc
	}
}

// DocsPath sets the documentation UI path
func DocsPath(path string) Option {
	return func(c *Config) {
		c.DocsPath = path
	}
}

// SpecPath sets the OpenAPI spec JSON path
func SpecPath(path string) Option {
	return func(c *Config) {
		c.SpecPath = path
	}
}

// UI sets the documentation UI type
func UI(ui UIType) Option {
	return func(c *Config) {
		c.UI = ui
	}
}

// AddServer adds a server to the OpenAPI spec
func AddServer(url, description string) Option {
	return func(c *Config) {
		c.Servers = append(c.Servers, Server{
			URL:         url,
			Description: description,
		})
	}
}

// DisableInProduction disables docs in production
func DisableInProduction(app *gx.App) Option {
	return func(c *Config) {
		c.Disabled = app.IsProduction()
	}
}

// OpenAPISpec represents the OpenAPI 3.1 specification
type OpenAPISpec struct {
	OpenAPI    string              `json:"openapi"`
	Info       Info                `json:"info"`
	Servers    []ServerSpec        `json:"servers,omitempty"`
	Paths      map[string]PathItem `json:"paths"`
	Components Components          `json:"components,omitempty"`
}

type Info struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

type ServerSpec struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type PathItem struct {
	Get    *Operation `json:"get,omitempty"`
	Post   *Operation `json:"post,omitempty"`
	Put    *Operation `json:"put,omitempty"`
	Patch  *Operation `json:"patch,omitempty"`
	Delete *Operation `json:"delete,omitempty"`
}

type Operation struct {
	Summary     string              `json:"summary,omitempty"`
	Description string              `json:"description,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	Parameters  []Parameter         `json:"parameters,omitempty"`
	RequestBody *RequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]Response `json:"responses"`
	Deprecated  bool                `json:"deprecated,omitempty"`
}

type Parameter struct {
	Name        string    `json:"name"`
	In          string    `json:"in"`
	Description string    `json:"description,omitempty"`
	Required    bool      `json:"required,omitempty"`
	Schema      SchemaRef `json:"schema"`
}

type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required,omitempty"`
	Content     map[string]MediaType `json:"content"`
}

type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema SchemaRef `json:"schema"`
}

type Components struct {
	Schemas map[string]Schema `json:"schemas,omitempty"`
}

type Schema struct {
	Type        string            `json:"type,omitempty"`
	Format      string            `json:"format,omitempty"`
	Description string            `json:"description,omitempty"`
	Properties  map[string]Schema `json:"properties,omitempty"`
	Required    []string          `json:"required,omitempty"`
	Items       *SchemaRef        `json:"items,omitempty"`
	Example     any               `json:"example,omitempty"`
	Ref         string            `json:"$ref,omitempty"`
}

type SchemaRef struct {
	Ref         string            `json:"$ref,omitempty"`
	Type        string            `json:"type,omitempty"`
	Format      string            `json:"format,omitempty"`
	Description string            `json:"description,omitempty"`
	Properties  map[string]Schema `json:"properties,omitempty"`
	Required    []string          `json:"required,omitempty"`
	Items       *SchemaRef        `json:"items,omitempty"`
	Example     any               `json:"example,omitempty"`
}

// MarshalJSON custom marshaller to handle schema refs
func (s SchemaRef) MarshalJSON() ([]byte, error) {
	if s.Ref != "" {
		return json.Marshal(map[string]string{"$ref": s.Ref})
	}

	type Alias SchemaRef
	return json.Marshal((Alias)(s))
}
