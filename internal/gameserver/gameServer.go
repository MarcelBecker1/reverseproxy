package gameserver

import (
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/MarcelBecker1/reverseproxy/internal/framing"
	"github.com/MarcelBecker1/reverseproxy/internal/logger"
)

/*
	TODO:
		1. Receive and send back data should be supported
		2. Create some dummy data that we want to send back to clients
*/

type Server struct {
	host        string
	port        string
	capacity    uint8
	connections uint8
	mu          sync.Mutex
}

type Config struct {
	Host     string
	Port     string
	Capacity uint8
}

var log *slog.Logger

func New(c *Config) *Server {
	log = logger.NewWithComponent("gameserver")
	return &Server{
		host:        c.Host,
		port:        c.Port,
		capacity:    c.Capacity,
		connections: 0,
	}
}

// Both functions are currently almost the same is in my proxy server
// Maybe we can reduce duplications

func (s *Server) Start() {
	hostAdress := net.JoinHostPort(s.host, s.port)
	log.Info("listening for tcp connections", "address", hostAdress)

	listener, err := net.Listen("tcp", hostAdress)
	if err != nil {
		log.Error("failed to create tcp listener", "error", err)
		return
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Error("failed to accept connection", "error", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	s.incConnections()

	for {
		// we are not setting read deadling atm, maybe change? -> be consistent
		msg, bytes, err := framing.ReadMessage(conn, log)
		if err != nil {
			if err == io.EOF {
				log.Info("client disconnected")
				s.handleDisconnect()
				return
			}
			log.Error("failed to read from connection", "error", err)
			return
		}

		log.Info("received data",
			"bytes", bytes,
			"data", msg,
		)
	}
}

func (s *Server) incConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections++
	log.Info("received new user", "connections", s.connections)
}

func (s *Server) handleDisconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections--
	log.Info("user disconnected", "connections", s.connections)
}

func (s *Server) Host() string {
	return s.host
}

func (s *Server) Port() string {
	return s.port
}
