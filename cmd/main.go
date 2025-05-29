package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/MarcelBecker1/reverseproxy/internal/client"
	"github.com/MarcelBecker1/reverseproxy/internal/proxy"
)

func main() {
	const host string = "localhost"
	const port string = "8080"

	logger := log.New(os.Stdout, "[MAIN]   ", log.LstdFlags)

	// Currently we create simple channel for os interrupts with buffer size 1, thus we are blocking the main thread after starting
	// the goroutines until we get the interrupt and can finish gracefully
	// TODO: Add cleanup for the other components
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Println("starting reverse proxy")
	server := proxy.New(&proxy.Config{
		Host: host,
		Port: port,
	})

	serverChan := make(chan error)
	go server.Start(serverChan)

	if err := <-serverChan; err != nil {
		logger.Fatal("failed to start server: ", err)
	}

	go func() {
		for err := range serverChan {
			logger.Printf("server error: %v", err)
		}
	}()

	client := client.New(&client.Config{
		Name: "John Smith",
	})

	clientChan := make(chan error)
	go client.Connect(host, port, clientChan)

	if err := <-clientChan; err != nil {
		logger.Fatal("failed to connect client to server: ", err)
	}

	sig := <-sigChan
	logger.Printf("received signal %v, shutting down", sig)
}
