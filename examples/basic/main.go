package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/llpsdk/llp-go"
)

func main() {
	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	var name string
	flag.StringVar(&name, "n", "sample-agent", "specify agent name")
	flag.Parse()

	apiKey := os.Getenv("LLP_API_KEY")
	cfg := llp.Config{
		PlatformURL:  "ws://localhost:4000/agent/websocket",
		PingInterval: 5 * time.Second,
	}
	slog.SetLogLoggerLevel(slog.LevelDebug)
	client, err := llp.NewClient(name, apiKey).
		WithConfig(cfg).
		OnMessage(func(ctx context.Context, telemetry llp.Annotater, msg llp.TextMessage) (llp.TextMessage, error) {
			log.Printf("Received message: %v", msg.Prompt)
			// process prompt with your agent
			tc := msg.ToolCall("get_foo", `{"city":"San Francisco"}`, "foggy", 1000*time.Millisecond)
			if err := telemetry.AnnotateToolCall(ctx, tc); err != nil {
				return llp.TextMessage{}, err
			}
			return msg.Reply("hello"), nil
		}).
		Connect(ctx)

	if err != nil {
		panic(err)
	}
	log.Printf("client is connected and authenticated")

	client.AwaitResult(ctx)
	log.Printf("waiting for ctrl+c")
	if err = client.AwaitResult(ctx); err != nil {
		log.Printf("test failed")
		os.Exit(1)
	} else {
		log.Printf("test passed")
	}
}
