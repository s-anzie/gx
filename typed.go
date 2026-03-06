package gx

import (
	"fmt"
	"reflect"

	"github.com/s-anzie/gx/core"
)

// Typed retrieves a value from the context store and casts it to type T
// Panics if the value doesn't exist or cannot be cast to T
// This is intended for use in handlers where the Contract has validated the presence
func Typed[T any](c *core.Context) *T {
	var zero T
	typeName := reflect.TypeOf(zero).String()

	// Try to get the value from the store
	val, exists := c.Get(typeName)
	if !exists {
		panic(fmt.Sprintf("typed: value of type %s not found in context - ensure Contract declares this type", typeName))
	}

	// Type assert
	typed, ok := val.(*T)
	if !ok {
		panic(fmt.Sprintf("typed: value in context is %T, expected *%s", val, typeName))
	}

	return typed
}

// TryTyped retrieves a value from the context store and casts it to type T
// Returns (value, true) if found and castable, (nil, false) otherwise
// Use this when the value might not be present (e.g., optional middleware data)
func TryTyped[T any](c *core.Context) (*T, bool) {
	var zero T
	typeName := reflect.TypeOf(zero).String()

	// Try to get the value from the store
	val, exists := c.Get(typeName)
	if !exists {
		return nil, false
	}

	// Type assert
	typed, ok := val.(*T)
	if !ok {
		return nil, false
	}

	return typed, true
}

// SetTyped stores a value in the context using its type name as the key
// This is the counterpart to Typed/TryTyped - middleware and validators use this
func SetTyped[T any](c *core.Context, value *T) {
	var zero T
	typeName := reflect.TypeOf(zero).String()
	c.Set(typeName, value)
}
