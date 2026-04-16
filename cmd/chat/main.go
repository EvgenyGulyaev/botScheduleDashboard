package main

import (
	"botDashboard/internal/chat"
	"botDashboard/internal/config"
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg := config.LoadConfig()

	port := cfg.Env["CHAT_PORT"]
	if port == "" {
		port = "8083"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := chat.NewServer()
	if err := srv.Start(ctx, ":"+port); err != nil {
		log.Print(err)
	}
}
