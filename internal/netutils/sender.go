package netutils

import (
	"fmt"
	"log/slog"
	"net"
	"time"
)

func ForwardMsg(conn net.Conn, msg string, timeout time.Duration, logger *slog.Logger) error {
	if timeout > 0 {
		if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			return fmt.Errorf("failed to set write deadling %w", err)
		}
	}

	if err := SendMessage(conn, msg, logger); err != nil {
		return fmt.Errorf("failed sending message %w", err)
	}

	return nil
}
