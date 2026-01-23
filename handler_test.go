package llp

import (
	"context"
	"sync"
	"testing"
)

func TestHandlerRegistry_SetAndCall(t *testing.T) {
	registry := newHandlerRegistry()
	ctx := context.Background()

	t.Run("presence handler", func(t *testing.T) {
		var called bool
		var receivedMsg PresenceMessage

		registry.setPresence(func(ctx context.Context, client *Client, msg PresenceMessage) {
			called = true
			receivedMsg = msg
		})

		msg := PresenceMessage{
			Sender: "456",
			Status: Available,
		}

		registry.callPresence(ctx, nil, msg)

		if !called {
			t.Error("handler was not called")
		}
		if receivedMsg != msg {
			t.Error("handler received wrong message")
		}
	})

	t.Run("message handler", func(t *testing.T) {
		var called bool
		var receivedMsg TextMessage

		registry.setMessage(func(ctx context.Context, msg TextMessage) (TextMessage, error) {
			called = true
			receivedMsg = msg
			return msg, nil
		})

		msg := TextMessage{
			Recipient: "789",
		}

		registry.callMessage(ctx, msg)

		if !called {
			t.Error("handler was not called")
		}
		if receivedMsg != msg {
			t.Error("handler received wrong message")
		}
	})
}

func TestHandlerRegistry_NoHandlerSet(t *testing.T) {
	registry := newHandlerRegistry()
	ctx := context.Background()

	// Should not panic when handlers are not set
	registry.callPresence(ctx, nil, PresenceMessage{})
	registry.callMessage(ctx, TextMessage{})
}

func TestHandlerRegistry_ConcurrentAccess(t *testing.T) {
	registry := newHandlerRegistry()
	ctx := context.Background()

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent writes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range iterations {
			registry.setPresence(func(ctx context.Context, client *Client, msg PresenceMessage) {})
		}
	}()

	// Concurrent reads/calls
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range iterations {
			registry.callPresence(ctx, nil, PresenceMessage{})
		}
	}()

	wg.Wait()
}
