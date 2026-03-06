package gx

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/s-anzie/gx/core"
)

// Translations is a map of language code → translated message.
// Language codes are normalized lowercase tags like "fr", "es", "de".
type Translations map[string]string

// AppError represents a structured application error
type AppError struct {
	Status       int            `json:"status"`
	Code         string         `json:"code"`
	Message      string         `json:"message"`
	Details      map[string]any `json:"details,omitempty"`
	translations Translations   // nil = no translations registered

	// Internal - not exposed to client
	wrapped error
}

// E creates a new AppError - shorthand constructor.
// An optional Translations map can be provided as the fourth argument:
//
//	var ErrNotFound = gx.E(404, "NOT_FOUND", "Resource not found", gx.Translations{
//	    "fr": "Ressource introuvable",
//	    "es": "Recurso no encontrado",
//	})
func E(status int, code, message string, translations ...Translations) AppError {
	var t Translations
	if len(translations) > 0 {
		t = translations[0]
	}
	return AppError{
		Status:       status,
		Code:         code,
		Message:      message,
		Details:      make(map[string]any),
		translations: t,
	}
}

// MessageFor returns the translated message for lang, falling back to the
// default English message if no translation is available.
func (e AppError) MessageFor(lang string) string {
	if e.translations != nil {
		if msg, ok := e.translations[lang]; ok {
			return msg
		}
	}
	return e.Message
}

// With adds contextual details to the error
// Returns a new error instance (immutable pattern)
func (e AppError) With(key string, value any) AppError {
	newDetails := make(map[string]any, len(e.Details)+1)
	for k, v := range e.Details {
		newDetails[k] = v
	}
	newDetails[key] = value

	return AppError{
		Status:       e.Status,
		Code:         e.Code,
		Message:      e.Message,
		Details:      newDetails,
		translations: e.translations,
		wrapped:      e.wrapped,
	}
}

// Wrap wraps an internal error (logged but not exposed to client)
func (e AppError) Wrap(err error) AppError {
	return AppError{
		Status:       e.Status,
		Code:         e.Code,
		Message:      e.Message,
		Details:      e.Details,
		translations: e.translations,
		wrapped:      err,
	}
}

// Unwrap returns the wrapped error for errors.Is/As compatibility
func (e AppError) Unwrap() error {
	return e.wrapped
}

// Error implements the error interface
func (e AppError) Error() string {
	if e.wrapped != nil {
		return fmt.Sprintf("%s (code=%s, status=%d): %v", e.Message, e.Code, e.Status, e.wrapped)
	}
	return fmt.Sprintf("%s (code=%s, status=%d)", e.Message, e.Code, e.Status)
}

// ToResponse converts the error to a core.Response
func (e AppError) ToResponse() core.Response {
	return &errorResponse{err: e}
}

// errorResponse implements core.Response for errors
type errorResponse struct {
	err AppError
}

func (r *errorResponse) Status() int {
	return r.err.Status
}

func (r *errorResponse) Headers() http.Header {
	return http.Header{
		"Content-Type": []string{"application/json; charset=utf-8"},
	}
}

func (r *errorResponse) Write(w http.ResponseWriter) error {
	// Set headers
	for key, values := range r.Headers() {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set status code
	w.WriteHeader(r.err.Status)

	// Write JSON error body
	response := map[string]any{
		"error": map[string]any{
			"code":    r.err.Code,
			"message": r.err.Message,
		},
	}

	// Add details if present
	if len(r.err.Details) > 0 {
		response["error"].(map[string]any)["details"] = r.err.Details
	}

	return json.NewEncoder(w).Encode(response)
}

// ── Common Errors ────────────────────────────────────────────────────────────

// Standard HTTP errors that applications can use or extend

var (
	// 4xx Client Errors
	ErrBadRequest          = E(400, "BAD_REQUEST", "The request could not be understood")
	ErrUnauthorized        = E(401, "UNAUTHORIZED", "Authentication is required")
	ErrForbidden           = E(403, "FORBIDDEN", "You don't have permission to access this resource")
	ErrNotFound            = E(404, "NOT_FOUND", "The requested resource was not found")
	ErrMethodNotAllowed    = E(405, "METHOD_NOT_ALLOWED", "The HTTP method is not allowed for this resource")
	ErrConflict            = E(409, "CONFLICT", "The request conflicts with the current state")
	ErrGone                = E(410, "GONE", "The resource is no longer available")
	ErrPayloadTooLarge     = E(413, "PAYLOAD_TOO_LARGE", "The request payload is too large")
	ErrUnsupportedMedia    = E(415, "UNSUPPORTED_MEDIA_TYPE", "The media type is not supported")
	ErrUnprocessableEntity = E(422, "UNPROCESSABLE_ENTITY", "The request was well-formed but contains semantic errors")
	ErrTooManyRequests     = E(429, "TOO_MANY_REQUESTS", "Too many requests, please slow down")

	// 5xx Server Errors
	ErrInternal           = E(500, "INTERNAL_ERROR", "An internal server error occurred")
	ErrNotImplemented     = E(501, "NOT_IMPLEMENTED", "This feature is not implemented")
	ErrBadGateway         = E(502, "BAD_GATEWAY", "Invalid response from upstream server")
	ErrServiceUnavailable = E(503, "SERVICE_UNAVAILABLE", "The service is temporarily unavailable")
	ErrGatewayTimeout     = E(504, "GATEWAY_TIMEOUT", "Upstream server timed out")

	// Validation Error (special case with field details)
	ErrValidation = E(422, "VALIDATION_ERROR", "Request validation failed")
)

// ValidationError creates a validation error with field-level details
func ValidationError(fields ...FieldError) AppError {
	err := ErrValidation
	if len(fields) > 0 {
		err.Details = map[string]any{
			"fields": fields,
		}
	}
	return err
}

// FieldError represents a single field validation error
type FieldError struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}
