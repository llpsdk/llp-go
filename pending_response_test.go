package llp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func PendingResponseTest(t *testing.T) {
	t.Run("Message on completion", func(t *testing.T) {
		p := PendingResponse{}
		p.setDone(TextMessage{})

		tm, err := p.Message()
		assert.NoError(t, err)
		assert.Equal(t, TextMessage{}, tm)
	})

	t.Run("Error on completion", func(t *testing.T) {
		p := PendingResponse{}
		e := PlatformError{}
		p.setError(e)

		tm, err := p.Message()
		assert.Nil(t, tm)
		assert.Error(t, err)
		assert.ErrorIs(t, err, &PlatformError{})
	})

	t.Run("Done on completion", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		p := PendingResponse{}
		go func() {
			p.setDone(TextMessage{})
		}()

		select {
		case <-p.Done():
		case <-ctx.Done():
			assert.Fail(t, "Pending response never finished")
		}
	})

	t.Run("Done twice on completion", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		p := PendingResponse{}
		go func() {
			p.setDone(TextMessage{})
		}()

		select {
		case <-p.Done():
		case <-ctx.Done():
			assert.Fail(t, "Pending response never finished")
		}

		select {
		case <-p.Done():
		case <-ctx.Done():
			assert.Fail(t, "Pending response never finished")
		}
	})

	t.Run("Can only be set once", func(t *testing.T) {
		p := PendingResponse{}
		p.setDone(TextMessage{})

		e := PlatformError{}
		p.setError(e)

		tm, err := p.Message()
		assert.NoError(t, err)
		assert.Equal(t, TextMessage{}, tm)
	})
}
