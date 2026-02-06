package llp

import (
	"fmt"

	"github.com/google/uuid"
)

type TextMessage struct {
	ID         string
	Recipient  string
	Sender     string
	Prompt     string
	Attachment string
}

func (t TextMessage) Reply(message string) TextMessage {
	return TextMessage{Recipient: t.Sender, ID: t.ID, Prompt: message}
}

func (t TextMessage) HasAttachment() bool {
	return t.Attachment != ""
}

func NewTextMessage(to, message string) TextMessage {
	return TextMessage{Recipient: to, Prompt: message, ID: uuid.NewString()}
}

type PresenceMessage struct {
	Sender             string
	Status             PresenceStatus
	SupportsEncryption bool
}

type TextMessageJSON struct {
	Type string          `json:"type"`
	From string          `json:"from,omitempty"`
	ID   string          `json:"id,omitempty"`
	Data TextMessageData `json:"data"`
}

type TextMessageData struct {
	To         string `json:"to"`
	Encrypted  bool   `json:"encrypted"`
	Prompt     []byte `json:"prompt"`
	Attachment string `json:"attachment_url,omitempty"`
}

// Outbound messages (client → server)

// AuthenticateMessage is sent to authenticate with the server
type AuthenticateMessage struct {
	Type string `json:"type"` // "authenticate"
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

// PresenceMessage is sent to update presence status
type PresenceMessageJSON struct {
	Type string       `json:"type"` // "presence"
	From string       `json:"from,omitempty"`
	ID   string       `json:"id,omitempty"`
	Data PresenceData `json:"data"`
}

// PresenceData contains presence status information
type PresenceData struct {
	Status             string `json:"status"` // "available" or "unavailable"
	SupportsEncryption bool   `json:"supports_encryption"`
}

// AuthenticatedResponse is received after successful authentication
type AuthenticatedResponse struct {
	Type string            `json:"type"` // "authenticated"
	ID   string            `json:"id,omitempty"`
	Data AuthenticatedData `json:"data"`
}

// AuthenticatedData contains the assigned session ID
type AuthenticatedData struct {
	SessionID string `json:"session_id"`
}

// ErrorMessageJSON is received when the server returns an error
type ErrorMessageJSON struct {
	Type    string `json:"type"` // "error"
	ID      string `json:"id,omitempty"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e ErrorMessageJSON) Error() string {
	return fmt.Sprintf("llp: %v %v", e.Code, e.Message)
}

// baseMessage is used for initial message type detection
type baseMessage struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}
