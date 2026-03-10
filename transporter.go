package llp

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
)

type Transporter interface {
	Close() error
	ReadMessage() (int, []byte, error)
	WriteMessage(int, []byte) error
	WriteControl(int, []byte, time.Time) error
	SetWriteDeadline(time.Time) error
}

func NewTransporter(ctx context.Context, urlStr string) (Transporter, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, urlStr, nil)
	return conn, err
}
