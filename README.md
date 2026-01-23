# LLP Go SDK

Go SDK for connecting to Large Language Platform.

## Features

- Simple, intuitive async API
- Thread-safe message handling
- Websocket-based communication

## Installation

```bash
go get github.com/llpsdk/llp-go
```

## Quick Start

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/llpsdk/llp-go"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	apiKey := os.Getenv("LLP_API_KEY")

	client, err := llp.NewClient("sample-agent", apiKey).
		OnMessage(func(ctx context.Context, msg llp.TextMessage) (llp.TextMessage, error) {
			// Process msg.Prompt with your agent
			response := msg.Prompt
			return msg.Reply(response), nil
		}).
		Connect(ctx)

	if err != nil {
		panic(err)
	}
	<-ctx.Done()
	client.Close()
}
```

## Development
```bash
# Run tests
make test

# Run example
LLP_API_KEY="...." go run examples/basic/main.go
```
