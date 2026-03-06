package gx

import (
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/s-anzie/gx/core"
)

var (
	validatorOnce     sync.Once
	defaultValidator  *validator.Validate
	validatorMu       sync.RWMutex
	customValidations []customValidation
)

type customValidation struct {
	tag string
	fn  validator.Func
}

func getValidator() *validator.Validate {
	validatorOnce.Do(func() {
		defaultValidator = validator.New(validator.WithRequiredStructEnabled())

		// Register any custom validators accumulated before first use
		validatorMu.RLock()
		defer validatorMu.RUnlock()
		for _, cv := range customValidations {
			_ = defaultValidator.RegisterValidation(cv.tag, cv.fn)
		}
	})
	return defaultValidator
}

// Validate validates a struct against its `validate` tags.
// Returns nil if valid, or a ValidationError with field-level details.
//
//	if err := gx.Validate(req); err != nil {
//	    return c.Fail(gx.ErrValidation).With("fields", err)
//	}
func Validate(v any) error {
	err := getValidator().Struct(v)
	if err == nil {
		return nil
	}

	var validationErrors validator.ValidationErrors
	if !isValidationErrors(err, &validationErrors) {
		return err
	}

	fields := make([]FieldError, 0, len(validationErrors))
	for _, fe := range validationErrors {
		fields = append(fields, FieldError{
			Field:   fe.Field(),
			Rule:    fe.Tag(),
			Message: humanizeValidationError(fe),
		})
	}

	return ValidationError(fields...)
}

// ValidateOrFail validates v and returns (response, false) with a 422 error body
// if validation fails. Returns (nil, true) if valid.
//
//	if res, ok := gx.ValidateOrFail(c, &req); !ok {
//	    return res
//	}
func ValidateOrFail(c *core.Context, v any) (core.Response, bool) {
	err := Validate(v)
	if err == nil {
		return nil, true
	}

	if appErr, ok := err.(AppError); ok {
		return appErr.ToResponse(), false
	}

	return ErrValidation.With("error", err.Error()).ToResponse(), false
}

// ValidateVar validates a single value against a tag string.
//
//	if err := gx.ValidateVar(email, "required,email"); err != nil {
//	    return c.Fail(ErrInvalidEmail)
//	}
func ValidateVar(v any, tag string) error {
	return getValidator().Var(v, tag)
}

// RegisterValidator registers a custom validation rule, available globally.
// Must be called before the first use of Validate/ValidateOrFail.
//
//	gx.RegisterValidator("slug", func(fl validator.FieldLevel) bool {
//	    return slugRegex.MatchString(fl.Field().String())
//	})
func RegisterValidator(tag string, fn validator.Func) {
	validatorMu.Lock()
	customValidations = append(customValidations, customValidation{tag: tag, fn: fn})
	validatorMu.Unlock()

	// If validator is already initialized, register immediately
	if defaultValidator != nil {
		_ = defaultValidator.RegisterValidation(tag, fn)
	}
}

// RegisterValidatorAlias registers a mapping from a custom tag to an existing
// combination of tags.
//
//	gx.RegisterValidatorAlias("password", "required,min=8,max=72")
func RegisterValidatorAlias(alias, tags string) {
	getValidator().RegisterAlias(alias, tags)
}

func isValidationErrors(err error, out *validator.ValidationErrors) bool {
	ve, ok := err.(validator.ValidationErrors)
	if ok {
		*out = ve
	}
	return ok
}

func humanizeValidationError(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "this field is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return "must be at least " + fe.Param() + " characters"
	case "max":
		return "cannot exceed " + fe.Param() + " characters"
	case "url":
		return "must be a valid URL"
	case "oneof":
		return "must be one of: " + fe.Param()
	case "uuid":
		return "must be a valid UUID"
	case "numeric":
		return "must be a numeric value"
	case "alphanum":
		return "must contain only alphanumeric characters"
	}
	return "failed validation rule: " + fe.Tag()
}
