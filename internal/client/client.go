package client

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/MarcelBecker1/reverseproxy/internal/framing"
	"github.com/MarcelBecker1/reverseproxy/internal/logger"
)

/*
	Maybe add message queue?
*/

var log *slog.Logger

type Client struct {
	conf   *Config
	conn   net.Conn
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

type Config struct {
	Name           string
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
}

func New(conf *Config) *Client {
	log = logger.NewWithComponent("client")

	if conf.ConnectTimeout == 0 {
		conf.ConnectTimeout = 10 * time.Second
	}
	if conf.ReadTimeout == 0 {
		conf.ReadTimeout = 30 * time.Second
	}
	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = 10 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		conf:   conf,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (c *Client) Connect(host, port string, errorC chan error) {
	address := net.JoinHostPort(host, port)
	log.Info("connecting client", "address", address)

	dialer := &net.Dialer{
		Timeout: c.conf.ConnectTimeout,
	}
	conn, err := dialer.DialContext(c.ctx, "tcp", address)

	if err != nil {
		errorC <- fmt.Errorf("error connecting to server %w", err)
		return
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	if err := c.auth(); err != nil {
		c.Close()
		errorC <- fmt.Errorf("auth error %w", err)
		return
	}

	log.Info("connected and authenticated")
	errorC <- nil
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cancel()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		log.Info("connection closed")
		return err
	}
	return nil
}

func (c *Client) auth() error {
	log.Info("authenticating client")
	authString := "auth: someUserToken" // dont really want to auth like this
	err := c.Send(authString)
	if err != nil {
		return err
	}

	msg, _, err := framing.ReadMessage(c.conn, log)
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("proxy aborted connection")
		}
		return fmt.Errorf("failed to read from connection: %w", err)
	}

	if msg == "AUTH_FAILED" {
		return fmt.Errorf("auth failed")
	}
	if msg == "AUTH_OK" {
		log.Info("successful authenticated client")
		return nil
	}

	return fmt.Errorf("unknown auth return")
}

func (c *Client) Send(msg string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("client not connected")
	}

	c.conn.SetWriteDeadline(time.Now().Add(c.conf.WriteTimeout))
	defer c.conn.SetWriteDeadline(time.Time{})

	err := framing.SendMessage(c.conn, msg, log)
	if err != nil {
		return err
	}
	return nil
}
