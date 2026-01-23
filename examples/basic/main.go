package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

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
	client, err := llp.NewClient(name, apiKey).
		OnMessage(func(ctx context.Context, msg llp.TextMessage) (llp.TextMessage, error) {
			log.Printf("Received message: %v", msg.Prompt)
			// process prompt with your agent
			return msg.Reply("hello"), nil
		}).
		Connect(ctx)

	if err != nil {
		panic(err)
	}
	log.Printf("client is connected and authenticated")

	log.Printf("waiting for ctrl+c")
	<-ctx.Done()
	log.Printf("shutting down")
	err = client.Close()
	if err != nil {
		log.Printf("error shutting down: %v", err)
	}
}
