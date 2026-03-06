package core

// Params represents URL parameters extracted from dynamic routes.
// It maps parameter names to their string values.
type Params map[string]string

// Get retrieves a parameter by name. Returns empty string if not found.
func (p Params) Get(name string) string {
	if p == nil {
		return ""
	}
	return p[name]
}
