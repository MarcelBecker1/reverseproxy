package client

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/MarcelBecker1/reverseproxy/internal/logger"
)

/*
TODO: Need a way to authenticate to the proxy server
*/

type Client struct {
	name string
}

type Config struct {
	Name string
}

func New(conf *Config) *Client {
	logger.NewWithComponent("client")
	return &Client{
		name: conf.Name,
	}
}

func (c *Client) Connect(host, port string, errorC chan error) {
	address := net.JoinHostPort(host, port)
	slog.Info("connecting client", "address", address)

	conn, err := net.Dial("tcp", address)
	if err != nil {
		errorC <- fmt.Errorf("error connecting to server %w", err)
		return
	}
	defer conn.Close()
	errorC <- nil

	slog.Info("connected client to server")
}
