package server

import (
	"log/slog"
	"net"
)

type Handler interface {
	HandleConnection(conn net.Conn)
}

type TCPServer struct {
	host     string
	port     string
	listener net.Listener
	logger   *slog.Logger
}

func NewTCPServer(host, port string, logger *slog.Logger) *TCPServer {
	return &TCPServer{
		host:   host,
		port:   port,
		logger: logger,
	}
}

func (s *TCPServer) Start(handler Handler) error {
	hostAdress := net.JoinHostPort(s.host, s.port)
	listener, err := net.Listen("tcp", hostAdress)
	if err != nil {
		s.logger.Error("failed to create tcp listener", "error", err)
		return err
	}
	s.listener = listener
	s.logger.Info("listening for tcp connections", "address", hostAdress)

	go func() {
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				s.logger.Error("failed to accept connection", "error", err)
				continue // TODO: better handling of errors for permanent ones
			}
			go handler.HandleConnection(conn)
		}
	}()

	return nil
}

func (s *TCPServer) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
