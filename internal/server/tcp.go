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
	defer listener.Close()
	s.logger.Info("listening for tcp connections", "address", hostAdress)

	for {
		conn, err := listener.Accept()
		if err != nil {
			s.logger.Error("failed to accept connection", "error", err)
			continue
		}
		go handler.HandleConnection(conn)
	}
}

func (s *TCPServer) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
