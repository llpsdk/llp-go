package llp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlatformError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *PlatformError
		contains []string
	}{
		{
			name: "with ID",
			err: &PlatformError{
				Code:    ErrCodeUnauthenticated,
				Message: "not authenticated",
				ID:      "msg-123",
			},
			contains: []string{"platform error", "1", "id=msg-123", "not authenticated"},
		},
		{
			name: "without ID",
			err: &PlatformError{
				Code:    ErrCodeMissingRecipient,
				Message: "missing recipient",
			},
			contains: []string{"platform error", "102", "missing recipient"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()

			for _, substr := range tt.contains {
				assert.Contains(t, got, substr)
			}
		})
	}
}

func TestErrorCodes(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected int
	}{
		{ErrCodeDeserialize, 0},
		{ErrCodeUnauthenticated, 1},
		{ErrCodeProtocolSchema, 2},
		{ErrCodePresenceSchema, 3},
		{ErrCodeMessageSchema, 4},
		{ErrCodeInvalidKey, 100},
		{ErrCodeNameRegistered, 101},
		{ErrCodeMissingRecipient, 102},
		{ErrCodeUnrecognizedType, 104},
		{ErrCodeEncryptionUnsupported, 105},
	}

	for _, tt := range tests {
		t.Run("error code", func(t *testing.T) {
			assert.Equal(t, tt.expected, int(tt.code))
		})
	}
}

func TestSentinelErrors(t *testing.T) {
	// Ensure sentinel errors exist and have messages
	errors := []error{
		ErrNotConnected,
		ErrNotAuthed,
		ErrAlreadyClosed,
		ErrTimeout,
		ErrInvalidStatus,
	}

	for _, err := range errors {
		assert.Error(t, err)
		assert.NotEmpty(t, err.Error())
	}
}
