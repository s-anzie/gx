package gx

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/s-anzie/gx/core"
)

// Channel is the interface for bidirectional real-time communication
// It abstracts WebSocket (HTTP/1.1, HTTP/2) and QUIC streams (HTTP/3)
type Channel interface {
	// Send serializes and sends a value (JSON by default)
	Send(v any) error

	// Receive deserializes the next message into v
	Receive(v any) error

	// SendRaw sends raw bytes
	SendRaw(b []byte) error

	// ReceiveRaw receives the next raw message
	ReceiveRaw() ([]byte, error)

	// Done returns a channel that's closed when the client disconnects
	Done() <-chan struct{}

	// Proto returns the protocol actually used ("websocket" or "quic")
	Proto() string

	// Close closes the channel
	Close() error
}

// ChannelHandler is a handler function for channel-based endpoints
// It receives both the request context and the bidirectional channel
type ChannelHandler func(*Context, Channel) error

// ChannelContract describes a channel endpoint's schema
type ChannelContract struct {
	Summary     string
	Description string
	Tags        []string

	// Message schemas
	InputMessage  SchemaRef // Messages from client to server
	OutputMessage SchemaRef // Messages from server to client

	// Possible errors
	Errors []AppError
}

// ── No-op Channel Implementation (placeholder) ──────────────────────────────

// noopChannel is a placeholder channel implementation
// Real implementations will be provided by WebSocket and QUIC adapters
type noopChannel struct {
	done   chan struct{}
	closed bool
}

// NewNoopChannel creates a new no-op channel for testing
func NewNoopChannel() Channel {
	return &noopChannel{
		done: make(chan struct{}),
	}
}

func newNoopChannel() *noopChannel {
	return &noopChannel{
		done: make(chan struct{}),
	}
}

func (ch *noopChannel) Send(v any) error {
	if ch.closed {
		return errors.New("channel closed")
	}
	// No-op: would serialize to JSON and send over transport
	return nil
}

func (ch *noopChannel) Receive(v any) error {
	if ch.closed {
		return io.EOF
	}
	// No-op: would read from transport and deserialize
	return errors.New("no-op channel: receive not implemented")
}

func (ch *noopChannel) SendRaw(b []byte) error {
	if ch.closed {
		return errors.New("channel closed")
	}
	// No-op: would send raw bytes over transport
	return nil
}

func (ch *noopChannel) ReceiveRaw() ([]byte, error) {
	if ch.closed {
		return nil, io.EOF
	}
	// No-op: would read raw bytes from transport
	return nil, errors.New("no-op channel: receive not implemented")
}

func (ch *noopChannel) Done() <-chan struct{} {
	return ch.done
}

func (ch *noopChannel) Proto() string {
	return "noop"
}

func (ch *noopChannel) Close() error {
	if !ch.closed {
		ch.closed = true
		close(ch.done)
	}
	return nil
}

// ── SSE-based Channel (Server-to-Client only) ────────────────────────────────

// sseChannel wraps SSEStream to implement one-way Channel (server→client only)
// This is useful for server-push scenarios that don't need client→server messages
type sseChannel struct {
	stream *core.SSEStream
	done   chan struct{}
	closed bool
}

func newSSEChannel(stream *core.SSEStream) *sseChannel {
	return &sseChannel{
		stream: stream,
		done:   make(chan struct{}),
	}
}

func (ch *sseChannel) Send(v any) error {
	if ch.closed {
		return errors.New("channel closed")
	}

	// Serialize to JSON
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	// Send as SSE event
	return ch.stream.SendData(string(data))
}

func (ch *sseChannel) Receive(v any) error {
	// SSE is server→client only
	return errors.New("SSE channels do not support receiving messages")
}

func (ch *sseChannel) SendRaw(b []byte) error {
	if ch.closed {
		return errors.New("channel closed")
	}
	return ch.stream.SendData(string(b))
}

func (ch *sseChannel) ReceiveRaw() ([]byte, error) {
	// SSE is server→client only
	return nil, errors.New("SSE channels do not support receiving messages")
}

func (ch *sseChannel) Done() <-chan struct{} {
	return ch.done
}

func (ch *sseChannel) Proto() string {
	return "sse"
}

func (ch *sseChannel) Close() error {
	if !ch.closed {
		ch.closed = true
		close(ch.done)
		ch.stream.Close()
	}
	return nil
}

// ── Channel Registration ─────────────────────────────────────────────────────

// ChannelRoute represents a registered channel endpoint
type ChannelRoute struct {
	Path     string
	Contract ChannelContract
	Handler  ChannelHandler
}

// Channel registers a bidirectional channel endpoint
// The actual transport (WebSocket or QUIC) is chosen based on the client's protocol
func (app *App) Channel(path string, contract ChannelContract, handler ChannelHandler) {
	// For now, this is a placeholder
	// Full implementation would:
	// 1. Register both WebSocket upgrade handler (for HTTP/1.1 and HTTP/2)
	// 2. Register QUIC stream handler (for HTTP/3)
	// 3. Detect client protocol and choose appropriate transport

	// Store channel route for documentation/introspection
	if app.channelRoutes == nil {
		app.channelRoutes = make(map[string]ChannelRoute)
	}

	app.channelRoutes[path] = ChannelRoute{
		Path:     path,
		Contract: contract,
		Handler:  handler,
	}

	// TODO: Register actual WebSocket and QUIC handlers
}

// SSEChannel creates a server-to-client channel using Server-Sent Events
// This is a simpler alternative for use cases that don't need client→server messages
func (c *Context) SSEChannel() (Channel, error) {
	stream, err := core.NewSSEStream(c.Writer, c.GoContext())
	if err != nil {
		return nil, err
	}
	return newSSEChannel(stream), nil
}

// ── Migration Events (QUIC-specific) ─────────────────────────────────────────

// MigrateEvent represents a QUIC connection migration event
type MigrateEvent struct {
	OldAddr   string
	NewAddr   string
	SessionID string
}

// OnMigrate registers a callback for QUIC connection migration events
// Called when a client changes network (e.g., WiFi → 4G)
func (app *App) OnMigrate(callback func(MigrateEvent)) {
	// Placeholder for QUIC migration hook
	// Full implementation would register this with the QUIC transport
	if app.migrationCallbacks == nil {
		app.migrationCallbacks = make([]func(MigrateEvent), 0)
	}
	app.migrationCallbacks = append(app.migrationCallbacks, callback)
}
