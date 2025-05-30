package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/MarcelBecker1/reverseproxy/internal/client"
	"github.com/MarcelBecker1/reverseproxy/internal/logger"
	"github.com/MarcelBecker1/reverseproxy/internal/proxy"
)

func main() {
	const host string = "localhost"
	const port string = "8080"

	prettyLogger := slog.New(logger.NewHandler(&slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(prettyLogger)
	logger.NewWithComponent("main")

	// Currently we create simple channel for os interrupts with buffer size 1, thus we are blocking the main thread after starting
	// the goroutines until we get the interrupt and can finish gracefully
	// TODO: Add cleanup for the other components
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("starting reverse proxy")
	server := proxy.New(&proxy.Config{
		Host: host,
		Port: port,
	})

	serverChan := make(chan error)
	go server.Start(serverChan)

	if err := <-serverChan; err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}

	go func() {
		for err := range serverChan {
			slog.Error("server error", "error", err)
		}
	}()

	client := client.New(&client.Config{
		Name: "John Smith",
	})

	clientChan := make(chan error)
	go client.Connect(host, port, clientChan)

	if err := <-clientChan; err != nil {
		slog.Error("failed to connect client to server", "error", err)
	}

	sig := <-sigChan
	slog.Info("received signal shutting down", "signal", sig)
}
