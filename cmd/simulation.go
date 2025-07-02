package main

import (
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/MarcelBecker1/reverseproxy/internal/client"
	"github.com/MarcelBecker1/reverseproxy/internal/logger"
	"github.com/MarcelBecker1/reverseproxy/internal/server"
)

/*
	1) Start proxy
	2) spawn clients that connect to proxy
	3) send data from clients periodically

	Maybe have different bursts of connections
*/

const (
	host           = "localhost"
	port           = "8080"
	defaultTimeout = time.Duration(30) * time.Second
)

var log *slog.Logger

func RunSimulation() {
	prettyLogger := slog.New(logger.NewHandler(&slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(prettyLogger)

	log = logger.NewWithComponent("main")

	// start proxy
	proxyConfig := &server.ProxyServerConfig{
		Host:    host,
		Port:    port,
		Timeout: defaultTimeout,
	}
	proxy := server.NewProxyServer(proxyConfig)
	errorChan := make(chan error, 1)
	go proxy.Start(errorChan)
	if err := <-errorChan; err != nil {
		log.Error("can't start proxy")
		return
	}

	time.Sleep(1 * time.Second)
	spawnClients(100)
}

func spawnClients(amount int) {
	var wg sync.WaitGroup
	// idk get something meaningful as config
	conf := &client.Config{
		Name: "John Smith",
	}

	errorReports := []*ErrorReport{}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	batchSize := 50
	for batch := 0; batch < amount; batch += batchSize {
		currentBatch := min(batchSize, amount-batch)
		wg.Add(currentBatch)

		for i := range batchSize {
			go func(idx int) {
				defer wg.Done()

				randomOffset := r.Intn(100)
				time.Sleep(time.Duration(randomOffset) * time.Millisecond)

				if errReport := runClient(idx, conf, r); errReport != nil {
					errorReports = append(errorReports, errReport)
				}
			}(i)
		}
	}

	wg.Wait()

	if len(errorReports) > 0 {
		log.Warn("finished with client errors", "errors", errorReports)
	} else {
		log.Info("finished with no client errors")
	}
}

type ErrorReport struct {
	client int
	err    error
}

func (e ErrorReport) String() string {
	return fmt.Sprintf("Client %d had error %v", e.client, e.err)
}

func runClient(id int, conf *client.Config, r *rand.Rand) *ErrorReport {
	errChan := make(chan error, 1)
	client := client.New(conf)

	go client.Connect(host, port, errChan)
	if err := <-errChan; err != nil {
		log.Error("client connection failed", "id", id)
	}

	messages := []string{
		fmt.Sprintf("Hello from client %d", id),
		fmt.Sprintf("Message 2 from client %d", id),
		fmt.Sprintf("Final message from client %d", id),
	}

	for _, msg := range messages {
		randomOffset := 500 + r.Intn(2000)
		time.Sleep(time.Duration(randomOffset) * time.Millisecond)

		if err := client.Send(msg); err != nil {
			log.Error("client failed to send message", "id", id, "message", msg)
			return &ErrorReport{client: id, err: err}
		}
	}

	time.Sleep(time.Duration(rand.Intn(3000)+5000) * time.Millisecond)
	client.Close()
	log.Info("client gracefully shut down", "id", id)

	return nil
}
