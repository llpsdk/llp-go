package llp

import (
	"container/list"
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTransporter struct {
	t      *testing.T
	data   *list.List
	readCh chan *wsMessage
}

type wsMessage struct {
	messageType int
	data        []byte
	err         error
}

func (ft *fakeTransporter) Close() error {
	return nil
}

// pump a message into the read loop
func (ft *fakeTransporter) sendMessage(buf []byte) {
	dst := make([]byte, len(buf))
	copy(dst, buf)
	msg := &wsMessage{
		messageType: 1,
		data:        dst,
		err:         nil,
	}
	select {
	case <-ft.t.Context().Done():
		return
	case ft.readCh <- msg:
		return
	}
}

func (ft *fakeTransporter) sendClose() {
	msg := &wsMessage{
		messageType: 0,
		data:        []byte{},
		err:         websocket.ErrCloseSent,
	}
	select {
	case <-ft.t.Context().Done():
		return
	case ft.readCh <- msg:
		return
	}
}

func (ft *fakeTransporter) ReadMessage() (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ft.t.Context(), 1000*time.Millisecond)
	defer cancel()

	select {
	case <-ctx.Done():
		return 0, []byte{}, websocket.ErrCloseSent
	case msg := <-ft.readCh:
		return msg.messageType, msg.data, msg.err
	}
}

func (ft *fakeTransporter) WriteMessage(messageType int, data []byte) error {
	if ft.data.Len() == 0 {
		close := &wsMessage{
			messageType: 1,
			data:        []byte{},
			err:         websocket.ErrCloseSent,
		}
		ft.readCh <- close
		return nil
	}

	message := ft.data.Front()
	ft.data.Remove(message)
	reply := message.Value.([]byte)
	dst := make([]byte, len(reply))
	copy(dst, reply)
	msg := &wsMessage{
		messageType: 1,
		data:        dst,
		err:         nil,
	}

	select {
	case <-ft.t.Context().Done():
		return ft.t.Context().Err()
	case ft.readCh <- msg:
		return nil
	}
}

func (ft *fakeTransporter) WriteControl(messageTpye int, data []byte, deadline time.Time) error {
	return nil
}

func (ft *fakeTransporter) SetWriteDeadline(time.Time) error {
	return nil
}

func newFakeTransporter(t *testing.T, messages [][]byte) (*fakeTransporter, func(ctx context.Context, urlStr string) (Transporter, error)) {
	data := list.New()
	for _, m := range messages {
		data.PushBack(m)
	}
	fake := &fakeTransporter{t: t, data: data, readCh: make(chan *wsMessage, 1)}

	return fake, func(ctx context.Context, urlStr string) (Transporter, error) {
		return fake, nil
	}
}

func TestClientAuthentication(t *testing.T) {
	slog.SetLogLoggerLevel(slog.LevelError)
	messages := [][]byte{
		[]byte(`{"type":"authenticated","id":"auth","data":{"session_id":"testabc123"}}`),
		[]byte(`{"type":"ack","id":"online"}`),
	}
	_, callback := newFakeTransporter(t, messages)
	client, err := NewClient("test", "testkey").
		WithTransporter(callback).
		Connect(t.Context())

	require.NoError(t, err)
	assert.Equal(t, client.Status(), Authenticated)
	assert.Equal(t, client.Presence(), Available)
	assert.Equal(t, client.SessionID(), "testabc123")
}

func TestClientConnectFailure(t *testing.T) {
	slog.SetLogLoggerLevel(slog.LevelError)
	testCases := []struct {
		desc     string
		messages [][]byte
		err      error
	}{
		{
			desc: "Wrong API key",
			messages: [][]byte{
				[]byte(`{"type":"error","id":"auth","code":100,"message":"Invalid Key"}`),
			},
			err: &PlatformError{
				ID:      "auth",
				Message: "Invalid Key",
				Code:    100,
			},
		},
		{
			desc: "Invalid presence",
			messages: [][]byte{
				[]byte(`{"type":"authenticated","id":"auth","data":{"session_id":"testabc123"}}`),
				[]byte(`{"type":"error","id":"online","code":3,"message":"Invalid presence schema"}`),
			},
			err: &PlatformError{
				ID:      "online",
				Message: "Invalid presence schema",
				Code:    3,
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			_, callback := newFakeTransporter(t, tC.messages)
			_, err := NewClient("test", "testkey").
				WithTransporter(callback).
				Connect(t.Context())
			assert.ErrorIs(t, err, tC.err)
		})
	}
}

func TestClientReceivesPresence(t *testing.T) {
	slog.SetLogLoggerLevel(slog.LevelError)
	messages := [][]byte{
		[]byte(`{"type":"authenticated","id":"auth","data":{"session_id":"testabc123"}}`),
		[]byte(`{"type":"ack","id":"online"}`),
	}

	presCh := make(chan PresenceMessage, 1)

	fake, callback := newFakeTransporter(t, messages)
	_, err := NewClient("test", "testkey").
		WithTransporter(callback).
		OnPresence(func(ctx context.Context, pres PresenceMessage) {
			presCh <- pres
		}).
		Connect(t.Context())

	require.NoError(t, err)
	fake.sendMessage([]byte(`{"type":"presence","id":"chaosagent","from":"chaos-agent-1","data":{"status":"available"}}`))
	pres := <-presCh
	assert.Equal(t, pres.Sender, "chaos-agent-1")
	assert.Equal(t, pres.Status, Available)
}

func TestClientReceivesMessage(t *testing.T) {
	slog.SetLogLoggerLevel(slog.LevelError)
	messages := [][]byte{
		[]byte(`{"type":"authenticated","id":"auth","data":{"session_id":"testabc123"}}`),
		[]byte(`{"type":"ack","id":"online"}`),
	}

	msgCh := make(chan TextMessage, 1)

	fake, callback := newFakeTransporter(t, messages)
	_, err := NewClient("test", "testkey").
		WithTransporter(callback).
		OnMessage(func(ctx context.Context, telemetry Annotater, msg TextMessage) (TextMessage, error) {
			msgCh <- msg
			return msg.Reply("ok"), nil
		}).
		Connect(t.Context())

	require.NoError(t, err)
	fake.sendMessage([]byte(`{"type":"message","id":"chaosagent","from":"chaos-agent-1","data":{"to": "test","prompt":"aGVsbG8="}}`))
	fake.sendMessage([]byte(`{"type":"ack","id":"chaosagent"}`))
	msg := <-msgCh
	assert.Equal(t, msg.Prompt, "hello")
	assert.Equal(t, msg.ID, "chaosagent")
}

func TestClientMessageFailure(t *testing.T) {
	testCases := []struct {
		desc     string
		message  []byte
		err      error
		replyErr error
	}{
		{
			desc:    "Receives error message",
			message: []byte(`{"type":"error","id":"foo","code":5,"message":"General server error, try again"}`),
			err: &PlatformError{
				ID:      "foo",
				Message: "General server error, try again",
				Code:    5,
			},
			replyErr: nil,
		},
		{
			desc:     "Message reply error",
			message:  []byte(`{"type":"message","id":"foo","from":"chaos-agent-1","data":{"to":"test","prompt":"aGVsbG8="}}`),
			err:      fmt.Errorf("message handler: %w", fmt.Errorf("agent failed")),
			replyErr: fmt.Errorf("agent failed"),
		},
	}
	slog.SetLogLoggerLevel(slog.LevelError)
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			messages := [][]byte{
				[]byte(`{"type":"authenticated","id":"auth","data":{"session_id":"testabc123"}}`),
				[]byte(`{"type":"ack","id":"online"}`),
			}

			fake, callback := newFakeTransporter(t, messages)

			client, err := NewClient("test", "testkey").
				WithTransporter(callback).
				OnMessage(func(ctx context.Context, telemetry Annotater, msg TextMessage) (TextMessage, error) {
					return msg.Reply("ok"), tC.replyErr
				}).
				Connect(t.Context())

			fake.sendMessage(tC.message)
			require.NoError(t, err)
			e := client.AwaitResult(t.Context())
			assert.Contains(t, e.Error(), tC.err.Error())
			assert.Equal(t, client.Status(), Disconnected)
		})
	}
}
