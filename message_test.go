package llp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticateMessage_Marshal(t *testing.T) {
	msg := AuthenticateMessage{
		Type: "authenticate",
		ID:   "1",
		Name: "weather",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded map[string]any
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "authenticate", decoded["type"])
	assert.Equal(t, "1", decoded["id"])
	assert.Equal(t, "weather", decoded["name"])
}

func TestPresenceMessage_Marshal(t *testing.T) {
	msg := PresenceMessageJSON{
		Type: "presence",
		Data: PresenceData{
			Status: "available",
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded map[string]any
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "presence", decoded["type"])
	dataField, ok := decoded["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "available", dataField["status"])
}

func TestAuthenticatedResponse_Unmarshal(t *testing.T) {
	jsonData := []byte(`{
		"type": "authenticated",
		"id": "msg-1",
		"data": {
			"session_id": "42"
		}
	}`)

	var msg AuthenticatedResponse
	err := json.Unmarshal(jsonData, &msg)
	require.NoError(t, err)
	assert.Equal(t, "authenticated", msg.Type)
	assert.Equal(t, "msg-1", msg.ID)
	assert.Equal(t, "42", msg.Data.SessionID)
}

func TestPresenceUpdate_Unmarshal(t *testing.T) {
	jsonData := []byte(`{
		"type": "presence",
		"from": "123",
		"data": {
			"status": "unavailable"
		}
	}`)

	var msg PresenceMessageJSON
	err := json.Unmarshal(jsonData, &msg)
	require.NoError(t, err)
	assert.Equal(t, "presence", msg.Type)
	assert.Equal(t, "123", msg.From)
	assert.Equal(t, "unavailable", msg.Data.Status)
}

func TestMessage_Unmarshal(t *testing.T) {
	jsonData := []byte(`{
		"type": "message",
		"from": "456",
		"id": "req-1",
		"data": {
			"to": "789",
            "encrypted": false,
			"prompt": "aGVsbG8="
		}
	}`)

	var msg TextMessageJSON
	err := json.Unmarshal(jsonData, &msg)
	require.NoError(t, err)
	assert.Equal(t, "message", msg.Type)
	assert.Equal(t, "456", msg.From)
	assert.Equal(t, "789", msg.Data.To)
	assert.Equal(t, []byte("hello"), msg.Data.Prompt)
	assert.False(t, msg.Data.Encrypted)
}

func TestErrorMessage_Unmarshal(t *testing.T) {
	jsonData := []byte(`{
		"type": "error",
		"id": "err-1",
		"code": 102,
		"message": "missing recipient"
	}`)

	var msg ErrorMessageJSON
	err := json.Unmarshal(jsonData, &msg)
	require.NoError(t, err)
	assert.Equal(t, "error", msg.Type)
	assert.Equal(t, 102, msg.Code)
	assert.Equal(t, "missing recipient", msg.Message)
}
