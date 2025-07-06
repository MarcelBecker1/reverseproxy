package server

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/MarcelBecker1/reverseproxy/internal/netutils"
)

const (
	defaultTimeout = 30
)

type AuthResult struct {
	Success        bool
	ClientID       string
	Reason         string
	ProxyAuth      bool
	GameServerAuth bool
}

func HandleInitAuth(client *ClientInfo, authTimeout, readTimeout time.Duration, logger *slog.Logger) (*AuthResult, error) {
	if authTimeout <= 0 {
		authTimeout = defaultTimeout * time.Second
	}
	authTimeoutChan := time.After(authTimeout)

	for {
		select {
		case <-authTimeoutChan:
			return &AuthResult{
				Success:   false,
				ClientID:  client.id,
				Reason:    "authentication timeout",
				ProxyAuth: false,
			}, fmt.Errorf("authentication timeout")
		default:
		}

		if readTimeout > 0 {
			if err := client.conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
				return &AuthResult{
					Success:   false,
					ClientID:  client.id,
					Reason:    "failed to set read deadline",
					ProxyAuth: false,
				}, fmt.Errorf("failed to set read deadline: %w", err)
			}
		}

		msg, bytes, err := netutils.ReadMessage(client.conn, logger)
		if err != nil {
			if err == io.EOF {
				return &AuthResult{
					Success:   false,
					ClientID:  client.id,
					Reason:    "client disconnected during auth",
					ProxyAuth: false,
				}, fmt.Errorf("client disconnected during auth")
			}
			// For other errors, log and continue the loop to allow retry
			logger.Warn("failed to read from connection, continuing auth loop", "error", err)
			continue
		}

		logger.Info("received auth data", "bytes", bytes, "data", msg)

		proxyAuthResult := authenticateAtProxy(client, msg, logger)
		if !proxyAuthResult.Success {
			netutils.SendMessage(client.conn, "AUTH_FAILED", logger)
			return proxyAuthResult, fmt.Errorf("proxy authentication failed: %s", proxyAuthResult.Reason)
		}

		authorizeResult := authorizeClient(client, logger)
		if !authorizeResult.Success {
			netutils.SendMessage(client.conn, "AUTH_FAILED", logger)
			return authorizeResult, fmt.Errorf("authorization failed: %s", authorizeResult.Reason)
		}

		logger.Info("proxy authentication and authorization successful", "client", client.id)

		return &AuthResult{
			Success:   true,
			ClientID:  client.id,
			Reason:    "proxy auth and authorization successful",
			ProxyAuth: true,
		}, nil
	}
}

func HandleGameServerAuth(client *ClientInfo, gsConn net.Conn, timeout time.Duration, logger *slog.Logger) (*AuthResult, error) {
	authMsg := fmt.Sprintf("CLIENT_AUTH:%s", client.id)
	if err := netutils.SendMessage(gsConn, authMsg, logger); err != nil {
		return &AuthResult{
			Success:        false,
			ClientID:       client.id,
			Reason:         "failed to send auth to game server",
			ProxyAuth:      true,
			GameServerAuth: false,
		}, fmt.Errorf("failed to send client auth to game server: %w", err)
	}

	if timeout > 0 {
		if err := gsConn.SetReadDeadline((time.Now().Add(timeout))); err != nil {
			return &AuthResult{
				Success:        false,
				ClientID:       client.id,
				Reason:         "failed to set read deadline for game server",
				ProxyAuth:      true,
				GameServerAuth: false,
			}, fmt.Errorf("failed to set read deadline for game server: %w", err)
		}
	}

	response, _, err := netutils.ReadMessage(gsConn, logger)
	if err != nil {
		return &AuthResult{
			Success:        false,
			ClientID:       client.id,
			Reason:         "failed to read game server auth response",
			ProxyAuth:      true,
			GameServerAuth: false,
		}, fmt.Errorf("failed to read game server auth response: %w", err)
	}

	if response != "AUTH_ACK" {
		return &AuthResult{
			Success:        false,
			ClientID:       client.id,
			Reason:         fmt.Sprintf("game server rejected auth: %s", response),
			ProxyAuth:      true,
			GameServerAuth: false,
		}, fmt.Errorf("game server auth failed: got %s instead of AUTH_ACK", response)
	}

	logger.Info("game server authentication successful", "client", client.id)

	return &AuthResult{
		Success:        true,
		ClientID:       client.id,
		Reason:         "full authentication successful",
		ProxyAuth:      true,
		GameServerAuth: true,
	}, nil
}

func CompleteAuthentication(client *ClientInfo, logger *slog.Logger) error {
	return netutils.SendMessage(client.conn, "AUTH_OK", logger)
}

// Maybe add more sophisticated logic with jwt tokens, user db, whitelisting
func authenticateAtProxy(client *ClientInfo, msg string, logger *slog.Logger) *AuthResult {
	if !containsAuthToken(msg) {
		logger.Warn("proxy auth failed - missing auth token", "client", client.id)
		return &AuthResult{
			Success:   false,
			ClientID:  client.id,
			Reason:    "missing auth token",
			ProxyAuth: false,
		}
	}

	logger.Info("proxy authentication successful", "client", client.id)
	return &AuthResult{
		Success:   true,
		ClientID:  client.id,
		Reason:    "proxy auth successful",
		ProxyAuth: true,
	}
}

// For now we just always authorize authenticated clients, can add more sophisticated checks with user permissions
func authorizeClient(client *ClientInfo, logger *slog.Logger) *AuthResult {
	logger.Info("proxy authorization successful", "client", client.id)
	return &AuthResult{
		Success:   true,
		ClientID:  client.id,
		Reason:    "authorization successful",
		ProxyAuth: true,
	}
}

// very simple auth check
func containsAuthToken(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "auth")
}
