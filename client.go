package llp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectionStatus represents the current connection state
type ConnectionStatus int

const (
	Disconnected ConnectionStatus = iota
	Connecting
	Connected
	Authenticated
	Disconnecting
	Closed
)

// String returns the string representation of the connection status
func (c ConnectionStatus) String() string {
	switch c {
	case Disconnected:
		return "disconnected"
	case Connecting:
		return "connecting"
	case Connected:
		return "connected"
	case Authenticated:
		return "authenticated"
	case Closed:
		return "closed"
	default:
		return "unknown"
	}
}

// Config holds the configuration for the client
type Config struct {
	PlatformURL  string
	PingInterval time.Duration
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() Config {
	return Config{
		PlatformURL:  "wss://llphq.com/agent/websocket",
		PingInterval: 5 * time.Second,
	}
}

// Client is the main SDK client
type Client struct {
	// Connection
	conn   Transporter
	connMu sync.RWMutex

	// Session state
	apiKey     string
	name       string
	sessionID  string
	status     ConnectionStatus
	statusMu   sync.RWMutex
	presence   PresenceStatus
	presenceMu sync.RWMutex

	// Presence tracking
	presenceAnnounced   bool
	presenceAnnouncedMu sync.Mutex

	// Message handling
	handlers *HandlerRegistry
	outbound chan []byte
	inbound  chan []byte
	auth     chan []byte
	result   chan error

	// Lifecycle
	cancel func()
	wg     sync.WaitGroup

	// Config
	config Config
	logger *slog.Logger
}

// SessionID returns the current session ID
func (c *Client) SessionID() string {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.sessionID
}

// Status returns the current connection status
func (c *Client) Status() ConnectionStatus {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()
	return c.status
}

// Presence returns the current presence status
func (c *Client) Presence() PresenceStatus {
	c.presenceMu.RLock()
	defer c.presenceMu.RUnlock()
	return c.presence
}

func (c *Client) AwaitResult(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-c.result:
		return result
	}
}

// Connect establishes a Websocket connection to the server
func (c *Client) connect(ctx context.Context, dialCallback func(context.Context, string) (Transporter, error)) error {
	status := c.Status()

	if status != Disconnected {
		return fmt.Errorf("cannot connect from status %s", status)
	}

	c.setStatus(Connecting)
	c.logger.Info("connecting to server", "address", c.config.PlatformURL)

	url, err := url.Parse(c.config.PlatformURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	conn, err := dialCallback(ctx, url.String())
	if err != nil {
		c.setStatus(Disconnected)
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	c.setStatus(Connected)
	c.logger.Info("connected to server", "address", c.config.PlatformURL)

	if err = c.authenticate(); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	if err := c.setPresence(Available); err != nil {
		return fmt.Errorf("set presence: %w", err)
	}
	c.presenceAnnouncedMu.Lock()
	c.presenceAnnounced = true
	c.presenceAnnouncedMu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.wg.Add(3)
	go c.readLoop(ctx)
	go c.writeLoop(ctx)
	go c.dispatcher(ctx)

	return nil
}

// Close gracefully shuts down the client
func (c *Client) Close() error {
	c.statusMu.RLock()
	status := c.status
	c.statusMu.RUnlock()

	if status == Closed {
		return ErrAlreadyClosed
	}

	c.logger.Debug("closing client")

	// Auto-presence: send unavailable if we announced presence
	c.presenceAnnouncedMu.Lock()
	shouldSendUnavailable := c.presenceAnnounced
	c.presenceAnnouncedMu.Unlock()

	if shouldSendUnavailable {
		if err := c.setPresence(Unavailable); err != nil {
			c.logger.Warn("auto-unavailable failed during close, continuing with shutdown anyway", "error", err)
			// Continue with shutdown anyway
		}
	}

	c.cancel()
	c.wg.Wait()

	c.setStatus(Closed)
	c.logger.Debug("client closed")

	return nil
}

func (c *Client) AnnotateToolCall(ctx context.Context, tc ToolCall) error {
	if c.Status() != Authenticated {
		return ErrNotAuthed
	}
	td := ToolCallData{
		To:             tc.Recipient,
		Name:           tc.Name,
		Parameters:     tc.Parameters,
		ThrewException: tc.ThrewException,
		Result:         tc.Result,
		Duration:       tc.Duration.Milliseconds(),
	}
	t := ToolCallJSON{
		Type: "tool_call",
		ID:   tc.ID,
		Data: td,
	}

	tJSON, err := json.Marshal(t)

	if err != nil {
		return fmt.Errorf("annotate json marshal: %w", err)
	}

	select {
	case c.outbound <- tJSON:
	case <-ctx.Done():
		return fmt.Errorf("annotate outbound: %w", ctx.Err())
	}

	c.logger.Debug("sending tool call", "to", tc.Recipient, "id", tc.ID, "name", tc.Name, "json", string(tJSON))
	return nil
}

func (c *Client) sendAsyncMessage(ctx context.Context, request TextMessage) error {
	if c.Status() != Authenticated {
		return ErrNotAuthed
	}

	msg := []byte(request.Prompt)

	td := TextMessageData{
		To:     request.Recipient,
		Prompt: msg,
	}

	tm := TextMessageJSON{
		Type: "message",
		ID:   request.ID,
		Data: td,
	}

	tmJSON, err := json.Marshal(tm)
	if err != nil {
		return err
	}

	select {
	case c.outbound <- tmJSON:
	case <-ctx.Done():
		return ctx.Err()
	}

	c.logger.Debug("sending message", "to", request.Recipient, "id", request.ID)

	return nil
}

// authenticate sends an authentication message and waits for response
func (c *Client) authenticate() error {
	if c.Status() != Connected {
		return ErrNotConnected
	}

	authMsg := AuthenticateMessage{
		Type: "authenticate",
		ID:   "auth",
		Name: c.name,
		Key:  c.apiKey,
	}

	data, err := json.Marshal(authMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal auth message: %w", err)
	}

	// Send the message
	if err = c.writeMessage(data); err != nil {
		return fmt.Errorf("auth message write: %w", err)
	}

	reply, err := c.readMessage()
	if err != nil {
		c.handleDisconnect(err)
		return fmt.Errorf("auth message read: %w", err)
	}

	var msg AuthenticatedResponse
	var base baseMessage
	if err := json.Unmarshal(reply, &base); err != nil {
		return fmt.Errorf("base message unmarshal: %w", err)
	}

	if base.Type == "error" {
		var errMsg ErrorMessageJSON
		json.Unmarshal(reply, &errMsg)
		pErr := PlatformError{
			Code:    ErrorCode(errMsg.Code),
			ID:      errMsg.ID,
			Message: errMsg.Message,
		}
		return &pErr
	}

	if err := json.Unmarshal(reply, &msg); err != nil {
		return fmt.Errorf("authenticated message unmarshal: %w", err)
	}

	c.connMu.Lock()
	c.sessionID = msg.Data.SessionID
	c.connMu.Unlock()

	c.setStatus(Authenticated)

	return nil
}

func (c *Client) setStatus(status ConnectionStatus) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.logger.Debug("status change", "from", c.status, "to", status)
	c.status = status
}

func (c *Client) setPresenceStatus(status PresenceStatus) {
	c.presenceMu.Lock()
	defer c.presenceMu.Unlock()
	c.presence = status
}

func (c *Client) setPresence(status PresenceStatus) error {
	if c.Status() != Authenticated {
		return ErrNotAuthed
	}

	c.logger.Debug("sending presence", "status", status.String())

	p := PresenceMessageJSON{
		Type: "presence",
		ID:   "online",
		Data: PresenceData{
			Status: status.String(),
		},
	}

	data, _ := json.Marshal(p)

	if err := c.writeMessage(data); err != nil {
		return fmt.Errorf("write presence: %w", err)
	}

	if err := c.awaitAck("online"); err != nil {
		return fmt.Errorf("ack: %w", err)
	}

	c.setPresenceStatus(status)
	return nil
}

func (c *Client) awaitAck(id string) error {
	buf, err := c.readMessage()
	if err != nil {
		return fmt.Errorf("read message: %w", err)
	}
	var bm baseMessage
	err = json.Unmarshal(buf, &bm)
	if err != nil {
		c.logger.Error("unmarshal error", "buffer", string(buf))
		return fmt.Errorf("unmarshal: %w", err)
	}

	if bm.Type == "ack" {
		if bm.ID == id {
			return nil
		} else {
			return fmt.Errorf("non-matching id")
		}
	}

	if bm.Type == "error" {
		var errMsg ErrorMessageJSON
		json.Unmarshal(buf, &errMsg)
		pErr := PlatformError{
			Code:    ErrorCode(errMsg.Code),
			ID:      errMsg.ID,
			Message: errMsg.Message,
		}
		return &pErr

	} else {
		return fmt.Errorf("unknown message type: %v", bm.Type)
	}
}

func (c *Client) readLoop(ctx context.Context) {
	defer c.wg.Done()

	c.logger.Debug("readLoop started")
	defer c.logger.Debug("readLoop stopped")

	for {
		select {
		case <-ctx.Done():
			c.handleDisconnect(ctx.Err())
			return
		default:
		}

		buffer, err := c.readMessage()
		if err != nil {
			c.logger.Error("Unrecoverable message read error, disconnecting", "error", err)
			c.handleDisconnect(err)
			return
		}

		if len(buffer) > 0 {
			data := make([]byte, len(buffer))
			copy(data, buffer)

			select {
			case c.inbound <- data:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (c *Client) writeLoop(ctx context.Context) {
	defer c.wg.Done()

	c.logger.Debug("writeLoop started")
	defer c.logger.Debug("writeLoop stopped")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case data := <-c.outbound:
			if err := c.writeMessage(data); err != nil {
				c.logger.Error("write error", "error", err)
				c.handleDisconnect(err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(3*time.Second)); err != nil {
				c.logger.Error("ping error", "error", err)
				c.handleDisconnect(err)
				return
			}
		case <-ctx.Done():
			c.conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(3*time.Second))
			c.conn.Close()
			return
		}
	}
}

func (c *Client) writeMessage(buf []byte) error {
	c.conn.SetWriteDeadline(time.Now().Add(c.config.PingInterval))

	c.logger.Debug("writing message")
	if err := c.conn.WriteMessage(websocket.TextMessage, buf); err != nil {
		return fmt.Errorf("write error: %w", err)
	}
	return nil
}

func (c *Client) readMessage() ([]byte, error) {
	_, buffer, err := c.conn.ReadMessage()
	if err != nil {
		if websocket.IsCloseError(err,
			websocket.CloseNormalClosure,
			websocket.CloseAbnormalClosure,
			websocket.CloseGoingAway,
			websocket.CloseProtocolError) {
			err = fmt.Errorf("websocket close: %w", err)
			c.handleDisconnect(err)
			return []byte{}, err
		}
		err = fmt.Errorf("read error: %w", err)
		c.handleDisconnect(err)
		return []byte{}, err
	}

	return buffer, nil
}

// dispatcher routes incoming messages to handlers
func (c *Client) dispatcher(ctx context.Context) {
	defer c.wg.Done()

	c.logger.Debug("dispatcher started")
	defer c.logger.Debug("dispatcher stopped")

	for {
		select {
		case data := <-c.inbound:
			c.logger.Debug("received inbound message")
			if err := c.handleMessage(ctx, data); err != nil {
				c.logger.Error("handle message failure, shutting down.", "error", err)
				c.handleDisconnect(err)
			}

		case <-ctx.Done():
			return
		}
	}
}

// handleMessage processes a single incoming message
func (c *Client) handleMessage(ctx context.Context, data []byte) error {
	var base baseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return fmt.Errorf("base message decode: %w", err)
	}

	// Route based on type
	switch base.Type {
	case "error":
		var errMsg ErrorMessageJSON
		if err := json.Unmarshal(data, &errMsg); err != nil {
			return fmt.Errorf("error message decode: %w", err)
		}

		err := &PlatformError{
			Code:    ErrorCode(errMsg.Code),
			ID:      errMsg.ID,
			Message: errMsg.Message,
		}
		return err

	case "presence":
		var msg PresenceMessageJSON
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("presence message decode: %w", err)
		}

		c.logger.Debug("received presence update", "from", msg.From, "status", msg.Data.Status)

		status, err := ParsePresenceStatus(msg.Data.Status)
		if err != nil {
			return fmt.Errorf("presence status '%v': %w", msg.Data.Status, err)
		}

		p := PresenceMessage{
			Sender:             msg.From,
			Status:             status,
			SupportsEncryption: msg.Data.SupportsEncryption,
		}
		c.handlers.callPresence(ctx, p)

	case "message":
		return c.handleMessageCallback(ctx, data)

	case "ack":
		return nil

	default:
		c.logger.Warn("unknown message type", "type", base.Type)
	}
	return nil
}

func (c *Client) handleMessageCallback(ctx context.Context, data []byte) error {
	var msg TextMessageJSON
	err := json.Unmarshal(data, &msg)
	if err != nil {
		return fmt.Errorf("message decode: %w", err)
	}

	m := msg.Data.Prompt

	tm := TextMessage{
		Sender:     msg.From,
		Recipient:  msg.Data.To,
		Prompt:     string(m),
		ID:         msg.ID,
		Attachment: msg.Data.Attachment,
	}

	c.logger.Debug("routing to message callback")
	resp, err := c.handlers.callMessage(ctx, tm)
	if errors.Is(err, ErrHandlerNotSet) {
		return nil
	}

	if err != nil {
		// TODO: send an error response back to the platform
		return fmt.Errorf("message handler: %w", err)
	}

	err = c.sendAsyncMessage(ctx, resp)
	if err != nil {
		c.logger.Error("async message response failed", "error", err)
		return fmt.Errorf("send async message: %w", err)
	}
	return nil
}

// handleDisconnect handles disconnection events
func (c *Client) handleDisconnect(err error) {
	if c.Status() == Disconnected || c.Status() == Disconnecting {
		return
	}
	c.setStatus(Disconnecting)

	c.logger.Info("disconnected from server")

	// Auto-presence: send unavailable if we announced presence
	c.presenceAnnouncedMu.Lock()
	shouldSendUnavailable := c.presenceAnnounced
	c.presenceAnnounced = false // Reset state
	c.presenceAnnouncedMu.Unlock()

	if shouldSendUnavailable {
		if err := c.setPresence(Unavailable); err != nil {
			c.logger.Debug("auto-unavailable failed during disconnect", "error", err)
			// Connection is dead anyway, this is expected
		}
	}

	c.setStatus(Disconnected)
	c.result <- err
	c.cancel()
}
