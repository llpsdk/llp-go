package llp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
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
	PlatformURL string
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() Config {
	return Config{
		PlatformURL: "wss://llphq.com/agent/websocket",
	}
}

// Client is the main SDK client
type Client struct {
	// Connection
	conn   *websocket.Conn
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

	// Lifecycle
	cancel func()
	wg     sync.WaitGroup

	// Config
	config Config
	logger *slog.Logger

	pendingMu       sync.Mutex
	pendingMessages map[string]*PendingResponse
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

// Connect establishes a Websocket connection to the server
func (c *Client) connect(ctx context.Context) error {
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

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url.String(), nil)
	if err != nil {
		c.setStatus(Disconnected)
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	c.setStatus(Connected)
	c.logger.Info("connected to server")

	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.wg.Add(3)
	go c.readLoop(ctx)
	go c.writeLoop(ctx)
	go c.dispatcher(ctx)

	return c.authenticate(ctx)
}

func (c *Client) SendMessage(ctx context.Context, request TextMessage) (*PendingResponse, error) {
	if request.ID == "" {
		return nil, errors.New("send message missing ID")
	}

	c.pendingMu.Lock()
	_, exists := c.pendingMessages[request.ID]
	c.pendingMu.Unlock()
	if exists {
		return nil, errors.New("send message ID exists")
	}

	err := c.sendAsyncMessage(ctx, request)
	if err != nil {
		return nil, err
	}

	c.pendingMu.Lock()
	pr := &PendingResponse{}
	c.pendingMessages[request.ID] = pr
	c.pendingMu.Unlock()

	return pr, nil
}

// Close gracefully shuts down the client
func (c *Client) Close() error {
	c.statusMu.RLock()
	status := c.status
	c.statusMu.RUnlock()

	if status == Closed {
		return ErrAlreadyClosed
	}

	c.logger.Info("closing client")

	// Auto-presence: send unavailable if we announced presence
	c.presenceAnnouncedMu.Lock()
	shouldSendUnavailable := c.presenceAnnounced
	c.presenceAnnouncedMu.Unlock()

	if shouldSendUnavailable {
		// Use short timeout for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if err := c.setPresence(ctx, Unavailable); err != nil {
			c.logger.Warn("auto-unavailable failed during close", "error", err)
			// Continue with shutdown anyway
		}
	}

	c.cancel()
	c.wg.Wait()

	c.setStatus(Closed)
	c.logger.Info("client closed")

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

	c.logger.Debug("sending request", "to", request.Recipient)

	return nil
}

// authenticate sends an authentication message and waits for response
func (c *Client) authenticate(ctx context.Context) error {
	if c.Status() != Connected {
		return ErrNotConnected
	}

	authMsg := AuthenticateMessage{
		Type: "authenticate",
		ID:   "auth",
		Name: c.name,
		Key:  c.apiKey,
	}

	c.logger.Info("authenticating", "name", c.name, "key", c.apiKey)

	data, err := json.Marshal(authMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal auth message: %w", err)
	}

	// Send the message
	select {
	case c.outbound <- data:
	case <-ctx.Done():
		return ctx.Err()
	}

	var msg AuthenticatedResponse

	select {
	case <-ctx.Done():
		return fmt.Errorf("authenticate inbound: %w", ctx.Err())
	case reply := <-c.auth:

		var base baseMessage
		if err := json.Unmarshal(reply, &base); err != nil {
			return fmt.Errorf("authenticated message: %w", err)
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
			return fmt.Errorf("authenticated message: %w", err)
		}
	}

	c.connMu.Lock()
	c.sessionID = msg.Data.SessionID
	c.connMu.Unlock()

	c.setStatus(Authenticated)

	// Auto-presence: send available
	if err := c.setPresence(ctx, Available); err != nil {
		c.logger.Warn("auto-presence failed", "error", err)

		// Degrade gracefully - don't fail authentication
	} else {
		c.presenceAnnouncedMu.Lock()
		c.presenceAnnounced = true
		c.presenceAnnouncedMu.Unlock()
	}

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

func (c *Client) setPresence(ctx context.Context, status PresenceStatus) error {
	if c.Status() != Authenticated {
		return ErrNotAuthed
	}

	c.logger.Info("sending presence", "status", status.String())

	p := PresenceMessageJSON{
		Type: "presence",
		ID:   "online",
		Data: PresenceData{
			Status: status.String(),
		},
	}

	data, _ := json.Marshal(p)

	select {
	case c.outbound <- data:
		c.setPresenceStatus(status)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) readLoop(ctx context.Context) {
	defer c.wg.Done()

	c.logger.Debug("readLoop started")
	defer c.logger.Debug("readLoop stopped")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if c.conn == nil {
			return
		}

		_, buffer, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err,
				websocket.CloseNormalClosure,
				websocket.CloseAbnormalClosure,
				websocket.CloseGoingAway,
				websocket.CloseProtocolError) {
				c.handleDisconnect()
				return
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			c.handleDisconnect()
			return
		}

		if len(buffer) > 0 {
			data := make([]byte, len(buffer))
			copy(data, buffer)

			if c.Status() != Authenticated {
				select {
				case c.auth <- data:
				case <-ctx.Done():
					return
				}
			} else {
				select {
				case c.inbound <- data:
				case <-ctx.Done():
					return
				}
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

			if c.conn == nil {
				return
			}

			c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

			c.logger.Info("writing message")
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				c.logger.Error("write error", "error", err)
				c.handleDisconnect()
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(3*time.Second)); err != nil {
				c.logger.Error("ping error", "error", err)
				c.handleDisconnect()
				return
			}
		case <-ctx.Done():
			c.conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(3*time.Second))
			c.conn.Close()
			return
		}
	}
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
			c.handleMessage(ctx, data)

		case <-ctx.Done():
			return
		}
	}
}

// handleMessage processes a single incoming message
func (c *Client) handleMessage(ctx context.Context, data []byte) {
	// First, determine the message type
	var base baseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		c.logger.Error("failed to decode base message", "error", err)
		return
	}

	// Route based on type
	switch base.Type {
	case "error":
		var errMsg ErrorMessageJSON
		if err := json.Unmarshal(data, &errMsg); err != nil {
			c.logger.Error("failed to decode error message", "error", err)
			return
		}

		c.pendingMu.Lock()
		defer c.pendingMu.Unlock()
		pr, exists := c.pendingMessages[errMsg.ID]

		if exists {
			err := PlatformError{
				Code:    ErrorCode(errMsg.Code),
				ID:      errMsg.ID,
				Message: errMsg.Message,
			}
			pr.setError(err)
		}

	case "presence":
		var msg PresenceMessageJSON
		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Error("failed to decode presence message", "error", err)
			return
		}

		c.logger.Debug("received presence update", "from", msg.From, "status", msg.Data.Status)

		status, err := ParsePresenceStatus(msg.Data.Status)
		if err != nil {
			c.logger.Error("received unknown presence status", "error", err, "status", msg.Data.Status)
			return
		}

		p := PresenceMessage{
			Sender:             msg.From,
			Status:             status,
			SupportsEncryption: msg.Data.SupportsEncryption,
		}
		c.handlers.callPresence(ctx, c, p)

	case "message":
		c.handleMessageCallback(ctx, base.ID, data)

	default:
		c.logger.Warn("unknown message type", "type", base.Type)
	}
}

func (c *Client) handleMessageCallback(ctx context.Context, ID string, data []byte) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	pr, exists := c.pendingMessages[ID]

	var msg TextMessageJSON
	err := json.Unmarshal(data, &msg)
	if err != nil {
		c.logger.Error("failed to decode message", "error", err)
		return
	}

	m := msg.Data.Prompt

	tm := TextMessage{
		Sender:    msg.From,
		Recipient: msg.Data.To,
		Prompt:    string(m),
		ID:        msg.ID,
	}

	if exists {
		c.logger.Info("routing pending message")
		pr.setDone(tm)
		delete(c.pendingMessages, ID)
	} else {
		c.logger.Info("routing to message callback")
		resp, err := c.handlers.callMessage(ctx, tm)
		if errors.Is(err, ErrHandlerNotSet) {
			return
		}

		if err != nil {
			// TODO: send an error response back to the platform
			c.logger.Error("call message handler failure", "error", err)
			return
		}

		err = c.sendAsyncMessage(ctx, resp)
		if err != nil {
			c.logger.Error("async message response failed", "error", err)
			return
		}
	}
}

// handleDisconnect handles disconnection events
func (c *Client) handleDisconnect() {
	c.logger.Info("disconnected from server")

	// Auto-presence: send unavailable if we announced presence
	c.presenceAnnouncedMu.Lock()
	shouldSendUnavailable := c.presenceAnnounced
	c.presenceAnnounced = false // Reset state
	c.presenceAnnouncedMu.Unlock()

	if shouldSendUnavailable {
		// Connection may be dead; use 1s timeout and best-effort send
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		if err := c.setPresence(ctx, Unavailable); err != nil {
			c.logger.Debug("auto-unavailable failed during disconnect", "error", err)
			// Connection is dead anyway, this is expected
		}
	}

	c.setStatus(Disconnected)
	c.cancel()
}
