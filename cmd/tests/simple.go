package tests

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MarcelBecker1/reverseproxy/internal/client"
	"github.com/MarcelBecker1/reverseproxy/internal/logger"
	"github.com/MarcelBecker1/reverseproxy/internal/server"
)

/*
	Should have better logging, maybe with component (client, proxy, gameserver) and then the file we are in like reader? or have less and more meaningful logs
	-> it is a bit hard to know from what it was caused currently
	Implement real simulation as a way to test
*/

func SimpleTest() {
	const host string = "localhost"
	const port string = "8080"
	const maxSeconds int8 = 30
	const deadline = time.Duration(maxSeconds) * time.Second

	prettyLogger := slog.New(logger.NewHandler(&slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(prettyLogger)

	log := logger.NewWithComponent("main")

	// Currently we create simple channel for os interrupts with buffer size 1, thus we are blocking the main thread after starting
	// the goroutines until we get the interrupt and can finish gracefully
	// TODO: Add cleanup for the other components
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Info("starting reverse proxy")
	server := server.NewProxyServer(&server.ProxyServerConfig{
		Host:    host,
		Port:    port,
		Timeout: deadline,
	})

	serverChan := make(chan error)
	go server.Start(serverChan)

	if err := <-serverChan; err != nil {
		log.Error("failed to start server", "error", err)
		os.Exit(1)
	}

	go func() {
		for err := range serverChan {
			log.Error("server error", "error", err)
		}
	}()

	client := client.New(&client.Config{
		Name: "John Smith",
	})

	clientChan := make(chan error)
	go client.Connect(host, port, clientChan)

	if err := <-clientChan; err != nil {
		log.Error("failed to connect client to server", "error", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		client.Send("Some test data")
		client.Send("Something else")
		time.Sleep(8 * time.Second)
		client.Send("Another one")
		time.Sleep(7 * time.Second)
		client.Send("Last message")
	}()

	time.Sleep(5 * time.Second)
	client.Close()

	sig := <-sigChan
	log.Info("received signal shutting down", "signal", sig)
}
