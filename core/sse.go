package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SSEEvent represents a single Server-Sent Event
type SSEEvent struct {
	ID    string // Event ID (optional)
	Event string // Event type (optional)
	Data  string // Event data (required)
	Retry int    // Retry timeout in milliseconds (optional)
}

// SSEStream represents an active SSE connection
type SSEStream struct {
	writer  http.ResponseWriter
	flusher http.Flusher
	ctx     context.Context
	closed  bool
}

// NewSSEStream creates a new SSE stream from a ResponseWriter
func NewSSEStream(w http.ResponseWriter, ctx context.Context) (*SSEStream, error) {
	// Check if ResponseWriter supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	return &SSEStream{
		writer:  w,
		flusher: flusher,
		ctx:     ctx,
		closed:  false,
	}, nil
}

// Send sends an SSE event to the client
func (s *SSEStream) Send(event SSEEvent) error {
	if s.closed {
		return fmt.Errorf("stream is closed")
	}

	// Check if context is cancelled
	select {
	case <-s.ctx.Done():
		s.closed = true
		return s.ctx.Err()
	default:
	}

	// Write event ID
	if event.ID != "" {
		if _, err := fmt.Fprintf(s.writer, "id: %s\n", event.ID); err != nil {
			s.closed = true
			return err
		}
	}

	// Write event type
	if event.Event != "" {
		if _, err := fmt.Fprintf(s.writer, "event: %s\n", event.Event); err != nil {
			s.closed = true
			return err
		}
	}

	// Write retry
	if event.Retry > 0 {
		if _, err := fmt.Fprintf(s.writer, "retry: %d\n", event.Retry); err != nil {
			s.closed = true
			return err
		}
	}

	// Write data (can be multiline)
	if _, err := fmt.Fprintf(s.writer, "data: %s\n\n", event.Data); err != nil {
		s.closed = true
		return err
	}

	// Flush to send immediately
	s.flusher.Flush()

	return nil
}

// SendData is a convenience method to send just data
func (s *SSEStream) SendData(data string) error {
	return s.Send(SSEEvent{Data: data})
}

// SendJSON sends JSON data as an SSE event
func (s *SSEStream) SendJSON(data interface{}) error {
	// Convert to JSON string
	jsonData := fmt.Sprintf("%v", data)
	return s.SendData(jsonData)
}

// Close closes the SSE stream
func (s *SSEStream) Close() {
	s.closed = true
}

// IsClosed returns true if the stream is closed
func (s *SSEStream) IsClosed() bool {
	return s.closed
}

// Context returns the stream's context
func (s *SSEStream) Context() context.Context {
	return s.ctx
}

// ── SSE Response ─────────────────────────────────────────────────────────────

// sseResponse implements Response for SSE streaming
type sseResponse struct {
	baseResponse
	handler func(*SSEStream) error
}

func (r *sseResponse) Write(w http.ResponseWriter) error {
	// Create SSE stream
	stream, err := NewSSEStream(w, context.Background())
	if err != nil {
		return err
	}

	// Execute handler
	return r.handler(stream)
}

// SSE creates an SSE streaming response
func SSE(handler func(*SSEStream) error) Response {
	return &sseResponse{
		baseResponse: newBaseResponse(http.StatusOK),
		handler:      handler,
	}
}

// ── SSE Helper Functions ─────────────────────────────────────────────────────

// SSEKeepAlive sends periodic keep-alive comments to prevent connection timeout
func SSEKeepAlive(stream *SSEStream, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stream.Context().Done():
			return
		case <-ticker.C:
			// Send comment as keep-alive
			io.WriteString(stream.writer, ": keep-alive\n\n")
			stream.flusher.Flush()
		}
	}
}
