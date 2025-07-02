package client

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"net"
	"sync"
	"time"

	"github.com/MarcelBecker1/reverseproxy/internal/logger"
	"github.com/MarcelBecker1/reverseproxy/internal/netutils"
)

/*
	Maybe add message queue?
*/

type Client struct {
	conf   *Config
	conn   net.Conn
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	logger *slog.Logger
}

// TODO: think about client config
type Config struct {
	Name           string
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
}

func New(conf *Config) *Client {
	log := logger.NewWithComponent("client")

	// TODO: not sure about the timeouts
	if conf.ConnectTimeout == 0 {
		conf.ConnectTimeout = 30 * time.Second
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
		logger: log,
	}
}

func (c *Client) Connect(host, port string, errorC chan error) {
	address := net.JoinHostPort(host, port)
	c.logger.Info("connecting client", "address", address)

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

	c.logger.Info("connected and authenticated")
	errorC <- nil

	c.handleProxyConnection()
	c.Close()
}

func (c *Client) handleProxyConnection() {
	msgChan := make(chan string, 20)
	go netutils.ListenForMessages(c.ctx, c.conn, msgChan, c.conf.ReadTimeout, c.logger)

	for {
		select {
		case <-c.ctx.Done():
			c.logger.Info("proxy communication stopped", "reason", "context cancelled")
			return
		case msg, ok := <-msgChan:
			if !ok {
				c.logger.Info("proxy channel closed")
				return
			}
			if msg == "" {
				c.logger.Warn("received emptpy messages from proxy")
				continue
			}
			// TODO: not optimal to look for this strings, should have better signaling here
			if strings.HasSuffix(msg, "aborting connection") || strings.Contains(msg, "gs connection closed") {
				c.logger.Info("abort received from proxy")
				return
			}

			c.logger.Info("received message", "message", msg)
		}
	}
}

func (c *Client) Send(msg string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("client not connected")
	}

	select {
	case <-c.ctx.Done():
		return fmt.Errorf("can't send message - reason: context cancelled")
	default:
		c.conn.SetWriteDeadline(time.Now().Add(c.conf.WriteTimeout))
		return netutils.SendMessage(c.conn, msg, c.logger)
	}
}

func (c *Client) auth() error {
	c.logger.Info("authenticating client")
	authString := "auth: someUserToken" // dont really want to auth like this
	err := c.Send(authString)
	if err != nil {
		return err
	}

	msg, _, err := netutils.ReadMessage(c.conn, c.logger)
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
		c.logger.Info("successful authenticated client")
		return nil
	}

	return fmt.Errorf("unknown auth return")
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
	}

	var err error
	if c.conn != nil {
		c.logger.Info("closing connection")
		err = c.conn.Close()
		c.conn = nil
	}

	return err
}
