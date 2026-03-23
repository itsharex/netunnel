package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"netunnel/agent/internal/app"
	"netunnel/agent/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.Bootstrap(cfg)
	if err != nil {
		log.Fatalf("bootstrap app: %v", err)
	}

	if err := application.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("run app: %v", err)
	}
}
