package llp

import (
	"context"
	"fmt"
	"log/slog"
)

type ClientBuilder struct {
	name       string
	apiKey     string
	logger     *slog.Logger
	config     Config
	msgHandler MessageHandler
	pHandler   PresenceHandler
}

func NewClient(name, apiKey string) ClientBuilder {
	return ClientBuilder{
		name:   name,
		apiKey: apiKey,
		logger: slog.Default(),
		config: DefaultConfig(),
	}
}

func (cb ClientBuilder) WithConfig(cfg Config) ClientBuilder {
	cb.config = cfg
	return cb
}

func (cb ClientBuilder) WithLogger(logger *slog.Logger) ClientBuilder {
	cb.logger = logger
	return cb
}

func (cb ClientBuilder) OnMessage(handler MessageHandler) ClientBuilder {
	cb.msgHandler = handler
	return cb
}

func (cb ClientBuilder) OnPresence(handler PresenceHandler) ClientBuilder {
	cb.pHandler = handler
	return cb
}

func (cb ClientBuilder) Connect(ctx context.Context) (*Client, error) {
	c := &Client{
		name:            cb.name,
		apiKey:          cb.apiKey,
		status:          Disconnected,
		presence:        Unavailable,
		handlers:        newHandlerRegistry(),
		outbound:        make(chan []byte, 32),
		inbound:         make(chan []byte, 32),
		auth:            make(chan []byte, 1),
		config:          cb.config,
		logger:          cb.logger,
		pendingMessages: make(map[string]*PendingResponse),
	}

	if cb.msgHandler != nil {
		c.handlers.setMessage(cb.msgHandler)
	} else {
		cb.logger.WarnContext(ctx, "OnMessage handler was not set")
	}

	if cb.pHandler != nil {
		c.handlers.setPresence(cb.pHandler)
	}

	err := c.connect(ctx)

	if err != nil {
		return nil, fmt.Errorf("client builder connect: %w", err)
	}

	return c, nil
}
