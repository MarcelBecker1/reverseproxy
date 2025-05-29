package client

import (
	"fmt"
	"log"
	"net"
	"os"
)

type Client struct {
	name   string
	logger *log.Logger
}

type Config struct {
	Name string
}

func New(conf *Config) *Client {
	return &Client{
		name:   conf.Name,
		logger: log.New(os.Stdout, "[CLIENT] ", log.LstdFlags),
	}
}

func (c *Client) Connect(host, port string, errorC chan error) {
	hostAdress := net.JoinHostPort(host, port)
	c.logger.Printf("connecting client to %s", hostAdress)

	conn, err := net.Dial("tcp", hostAdress)
	if err != nil {
		errorC <- fmt.Errorf("error connecting to server %w", err)
		return
	}
	defer conn.Close()
	errorC <- nil

	c.logger.Printf("connected client to server")
}
