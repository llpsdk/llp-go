package llp

import (
	"errors"
	"fmt"
)

// ErrorCode represents platform error codes
type ErrorCode int

const (
	ErrCodeDeserialize           ErrorCode = 0
	ErrCodeUnauthenticated       ErrorCode = 1
	ErrCodeProtocolSchema        ErrorCode = 2
	ErrCodePresenceSchema        ErrorCode = 3
	ErrCodeMessageSchema         ErrorCode = 4
	ErrCodeGeneralServer         ErrorCode = 5
	ErrCodeInvalidKey            ErrorCode = 100
	ErrCodeNameRegistered        ErrorCode = 101
	ErrCodeMissingRecipient      ErrorCode = 102
	ErrCodeUnrecognizedType      ErrorCode = 104
	ErrCodeEncryptionUnsupported ErrorCode = 105
	ErrCodeAgentNotFound         ErrorCode = 106
)

// PlatformError represents an error returned by the platform server
type PlatformError struct {
	Code    ErrorCode
	Message string
	ID      string
}

func (e *PlatformError) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("platform error %d (id=%s): %s", e.Code, e.ID, e.Message)
	}
	return fmt.Sprintf("platform error %d: %s", e.Code, e.Message)
}

// Sentinel errors for client-side conditions
var (
	ErrNotConnected  = errors.New("not connected")
	ErrNotAuthed     = errors.New("not authenticated")
	ErrAlreadyClosed = errors.New("client already closed")
	ErrTimeout       = errors.New("operation timeout")
	ErrInvalidStatus = errors.New("invalid presence status")
)
