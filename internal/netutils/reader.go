package netutils

import (
	"context"
	"io"
	"log/slog"
	"net"
	"time"
)

type MessageReader struct {
	conn    net.Conn
	timeout time.Duration
	logger  *slog.Logger
}

func NewMessageReader(conn net.Conn, timeout time.Duration, logger *slog.Logger) *MessageReader {
	return &MessageReader{
		conn:    conn,
		timeout: timeout,
		logger:  logger,
	}
}

func (mr *MessageReader) Listen(ctx context.Context, msgChan chan string) {
	defer close(msgChan)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if mr.timeout > 0 {
			if err := mr.conn.SetReadDeadline(time.Now().Add(mr.timeout)); err != nil {
				mr.logger.Error("failed to set read deadline", "error", err)
				return
			}
		}

		msg, bytes, err := ReadMessage(mr.conn, mr.logger)
		if err != nil {
			if err == io.EOF {
				mr.logger.Info("connection disconnected", "error", err)
			} else {
				mr.logger.Error("failed to read from connection", "error", err)
			}
			return
		}

		mr.logger.Info("received data", "bytes", bytes, "data", msg)

		select {
		case msgChan <- msg:
		case <-ctx.Done(): // need this again to prevent deadlocks if channel is full and context cancelled
			return
		}
	}
}

func ListenForMessages(ctx context.Context, conn net.Conn, msgChan chan string, timeout time.Duration, logger *slog.Logger) {
	reader := NewMessageReader(conn, timeout, logger)
	reader.Listen(ctx, msgChan)
}
