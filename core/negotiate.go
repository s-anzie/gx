package core

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Negotiate returns a response based on the client's Accept header.
// Supports application/json, application/xml, text/plain, text/html.
// Returns 406 Not Acceptable if no format matches.
func (c *Context) Negotiate(v any) Response {
	accepted := c.Accepts("application/json", "application/xml", "text/plain")
	switch accepted {
	case "application/xml":
		return XML(v)
	case "text/plain":
		return Text("%v", v)
	case "application/json", "":
		// JSON is the default (empty = no Accept header or Accept: */*)
		return JSON(v)
	default:
		return WithStatus(JSON(map[string]string{
			"error": fmt.Sprintf("Not Acceptable: supported formats are application/json, application/xml, text/plain"),
		}), http.StatusNotAcceptable)
	}
}

// Accepts evaluates the request's Accept header and returns the best matching
// format from the candidates list, respecting q= quality weights.
// Returns "" if no candidate matches.
func (c *Context) Accepts(candidates ...string) string {
	accept := c.Request.Header.Get("Accept")
	if accept == "" || accept == "*/*" {
		// No preference — first candidate wins (convention: JSON first)
		if len(candidates) > 0 {
			return candidates[0]
		}
		return ""
	}

	// Parse Accept header into weighted entries
	type entry struct {
		mime    string
		quality float64
	}

	parts := strings.Split(accept, ",")
	entries := make([]entry, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		q := 1.0
		segments := strings.Split(part, ";")
		mime := strings.TrimSpace(segments[0])

		for _, seg := range segments[1:] {
			seg = strings.TrimSpace(seg)
			if strings.HasPrefix(seg, "q=") {
				if v, err := strconv.ParseFloat(seg[2:], 64); err == nil {
					q = v
				}
			}
		}
		entries = append(entries, entry{mime: mime, quality: q})
	}

	// Sort by quality descending
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].quality > entries[j].quality
	})

	// Find the best matching candidate
	for _, e := range entries {
		if e.quality == 0 {
			continue
		}
		for _, candidate := range candidates {
			if e.mime == candidate || e.mime == "*/*" {
				return candidate
			}
			// Match wildcard like "text/*"
			if strings.HasSuffix(e.mime, "/*") {
				prefix := strings.TrimSuffix(e.mime, "/*")
				if strings.HasPrefix(candidate, prefix+"/") {
					return candidate
				}
			}
		}
	}

	return ""
}

// AcceptLanguage parses Accept-Language and returns the primary language tag
// in normalized form (e.g., "fr", "en", "de").
func (c *Context) AcceptLanguage() string {
	langs := c.AcceptLanguages()
	if len(langs) == 0 {
		return "en"
	}
	return langs[0]
}

// AcceptLanguages parses Accept-Language and returns all language tags in
// preference order, normalized to the base language (e.g., "fr-FR" → "fr").
func (c *Context) AcceptLanguages() []string {
	header := c.Request.Header.Get("Accept-Language")
	if header == "" {
		return []string{"en"}
	}

	type langQ struct {
		lang    string
		quality float64
	}

	parts := strings.Split(header, ",")
	entries := make([]langQ, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		q := 1.0
		segments := strings.Split(part, ";")
		tag := strings.TrimSpace(segments[0])

		for _, seg := range segments[1:] {
			seg = strings.TrimSpace(seg)
			if strings.HasPrefix(seg, "q=") {
				if v, err := strconv.ParseFloat(seg[2:], 64); err == nil {
					q = v
				}
			}
		}

		// Normalize: "fr-FR" → "fr"
		base := strings.ToLower(strings.SplitN(tag, "-", 2)[0])
		entries = append(entries, langQ{lang: base, quality: q})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].quality > entries[j].quality
	})

	// Deduplicate while preserving order
	seen := make(map[string]bool)
	result := make([]string, 0, len(entries))
	for _, e := range entries {
		if !seen[e.lang] {
			seen[e.lang] = true
			result = append(result, e.lang)
		}
	}

	return result
}
