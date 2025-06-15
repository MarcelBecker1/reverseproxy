package client

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/MarcelBecker1/reverseproxy/internal/framing"
	"github.com/MarcelBecker1/reverseproxy/internal/logger"
)

/*
	TODO:
		1. first send message with auth from client to server
		2. then wait for response
		3. then send more data

		General: Need simulation to test properly
*/

var log *slog.Logger

type Client struct {
	name string
}

type Config struct {
	Name string
}

func New(conf *Config) *Client {
	log = logger.NewWithComponent("client")
	return &Client{
		name: conf.Name,
	}
}

func (c *Client) Connect(host, port string, errorC chan error) {
	address := net.JoinHostPort(host, port)
	log.Info("connecting client", "address", address)

	conn, err := net.Dial("tcp", address)
	if err != nil {
		errorC <- fmt.Errorf("error connecting to server %w", err)
		return
	}
	defer conn.Close()
	errorC <- nil

	dummyMessage := "[1] This is a client and i want to send some message. "
	dummyMessage2 := "[2] Second smaller message, which will be combined with the first. "
	largeDummyMessage := "[3] This is a client and i want to send some message, but this is a large message that should be split into multiple packets. " +
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. " +
		"Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. " +
		"Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. " +
		"Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum."

	go c.Send(conn, dummyMessage2)
	c.Send(conn, dummyMessage)
	c.Send(conn, largeDummyMessage+largeDummyMessage)

	log.Info("finished sending - closing connection")
}

func (c *Client) Send(conn net.Conn, msg string) error {
	// deadline := time.Duration(10) * time.Second
	// conn.SetWriteDeadline(time.Now().Add(deadline))

	err := framing.SendMessage(conn, msg, log)
	if err != nil {
		return err
	}
	return nil
}
