package core

import (
	"fmt"
	"net/http"
)

// Link represents a resource hint for Early Hints (RFC 8297).
type Link struct {
	URI         string
	Rel         string // preload | prefetch | preconnect | dns-prefetch
	As          string // style | script | image | font | fetch
	CrossOrigin bool
}

// Preload creates a Link hint for a resource that should be preloaded.
func Preload(uri, as string) Link {
	return Link{URI: uri, Rel: "preload", As: as}
}

// Prefetch creates a Link hint for a resource that should be prefetched
// for likely future navigation.
func Prefetch(uri string) Link {
	return Link{URI: uri, Rel: "prefetch"}
}

// Preconnect creates a Link hint to establish a connection to the origin.
func Preconnect(uri string) Link {
	return Link{URI: uri, Rel: "preconnect"}
}

// DNSPrefetch creates a Link hint to perform DNS resolution ahead of time.
func DNSPrefetch(uri string) Link {
	return Link{URI: uri, Rel: "dns-prefetch"}
}

// EarlyHints sends a 103 Early Hints informational response with the provided
// resource links. This allows the client to start loading sub-resources while
// the server is still processing the main response.
//
// - HTTP/1.1: no-op (103 is not reliably supported)
// - HTTP/2 and HTTP/3: sends a HEADERS frame with 103 status immediately
func (c *Context) EarlyHints(links ...Link) {
	if len(links) == 0 {
		return
	}

	// 103 Early Hints only works on HTTP/2 and HTTP/3
	if c.Request.ProtoMajor < 2 {
		return
	}

	w := c.Writer

	for _, link := range links {
		headerValue := fmt.Sprintf("<%s>; rel=%s", link.URI, link.Rel)
		if link.As != "" {
			headerValue += fmt.Sprintf("; as=%s", link.As)
		}
		if link.CrossOrigin {
			headerValue += "; crossorigin"
		}
		w.Header().Add("Link", headerValue)
	}

	// Use Flush if available (http.Flusher) after writing 103
	if flusher, ok := w.(http.Flusher); ok {
		// Write the 103 status — only works if the ResponseWriter supports it
		// In stdlib, WriteHeader with 103 sends an informational response
		w.WriteHeader(http.StatusEarlyHints)
		flusher.Flush()
		// Reset for the actual response headers
	}
}
