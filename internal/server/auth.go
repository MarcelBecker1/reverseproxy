package server

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/MarcelBecker1/reverseproxy/internal/netutils"
)

const (
	defaultTimeout = 30
)

func HandleInitAuth(client *ClientInfo, authTimeout, readTimeout time.Duration, logger *slog.Logger) error {
	if authTimeout <= 0 {
		authTimeout = defaultTimeout
	}
	authTimeoutChan := time.After(authTimeout * time.Second)

	for !client.authenticated {
		select {
		case <-authTimeoutChan:
			return fmt.Errorf("authentication timeout")
		default:
		}

		if readTimeout > 0 {
			if err := client.conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
				return fmt.Errorf("failed to set read deadline: %w", err)
			}
		}

		msg, bytes, err := netutils.ReadMessage(client.conn, logger)
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("client disconnected during auth")
			}
			return fmt.Errorf("failed to read from connection %w", err)
		}

		logger.Info("received auth data", "bytes", bytes, "data", msg)

		if err := handleClientAuth(client, msg, logger); err != nil {
			return err
		}
	}
	return nil
}

func authClient(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "auth")
}

func handleClientAuth(c *ClientInfo, msg string, logger *slog.Logger) error {
	if c.authenticated {
		return nil
	}

	if authClient(msg) {
		c.authenticated = true
		logger.Info("client authenticated", "id", c.id)
		return netutils.SendMessage(c.conn, "AUTH_OK", logger)
	}

	logger.Warn("auth failed", "client", c.id)
	netutils.SendMessage(c.conn, "AUTH_FAILED", logger)
	return fmt.Errorf("missing auth in message")
}
