package llp

import (
	"context"
	"errors"
	"sync"
)

var ErrHandlerNotSet = errors.New("message callback handler not set")

// Specific handler types - fully type-safe, no interface{} usage
type PresenceHandler func(ctx context.Context, client *Client, msg PresenceMessage)
type MessageHandler func(ctx context.Context, msg TextMessage) (TextMessage, error)
type ErrorHandler func(ctx context.Context, err *PlatformError)
type DisconnectedHandler func(ctx context.Context)

// HandlerRegistry stores all event handlers
type HandlerRegistry struct {
	mu         sync.RWMutex
	onPresence PresenceHandler
	onMessage  MessageHandler
}

// newHandlerRegistry creates a new handler registry
func newHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{}
}

// setPresence sets the presence handler
func (h *HandlerRegistry) setPresence(handler PresenceHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onPresence = handler
}

// setMessage sets the message handler
func (h *HandlerRegistry) setMessage(handler MessageHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onMessage = handler
}

// callPresence calls the presence handler if set
func (h *HandlerRegistry) callPresence(ctx context.Context, client *Client, msg PresenceMessage) {
	h.mu.RLock()
	handler := h.onPresence
	h.mu.RUnlock()

	if handler != nil {
		handler(ctx, client, msg)
	}
}

// callMessage calls the message handler if set
func (h *HandlerRegistry) callMessage(ctx context.Context, msg TextMessage) (TextMessage, error) {
	h.mu.RLock()
	handler := h.onMessage
	h.mu.RUnlock()

	if handler != nil {
		return handler(ctx, msg)
	}

	return TextMessage{}, ErrHandlerNotSet
}
