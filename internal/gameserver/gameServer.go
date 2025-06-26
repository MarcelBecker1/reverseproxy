package gameserver

import (
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/MarcelBecker1/reverseproxy/internal/connection"
	"github.com/MarcelBecker1/reverseproxy/internal/framing"
	"github.com/MarcelBecker1/reverseproxy/internal/logger"
	"github.com/MarcelBecker1/reverseproxy/internal/tcpserver"
)

/*
	TODO:
		Create some dummy data that we want to send back to clients
*/

type Server struct {
	host      string
	port      string
	tcpServer *tcpserver.Server
	connMgr   *connection.Manager
	mu        sync.Mutex
	logger    *slog.Logger
}

type Config struct {
	Host     string
	Port     string
	Capacity uint16
}

func New(c *Config) *Server {
	log := logger.NewWithComponent("gameserver")
	server := tcpserver.New(c.Host, c.Port, log)
	connMngr := connection.NewManager(c.Capacity)

	return &Server{
		host:      c.Host,
		port:      c.Port,
		tcpServer: server,
		connMgr:   connMngr,
		logger:    log,
	}
}

// Both functions are currently almost the same is in my proxy server
// Maybe we can reduce duplications

func (s *Server) Start() error {
	return s.tcpServer.Start(s)
}

func (s *Server) HandleConnection(conn net.Conn) {
	defer conn.Close()
	s.connMgr.Increment()
	defer s.connMgr.Decrement()

	for {
		// we are not setting read deadling atm, maybe change? -> be consistent
		msg, bytes, err := framing.ReadMessage(conn, s.logger)
		if err != nil {
			if err == io.EOF {
				s.logger.Info("client disconnected")
				return
			}
			s.logger.Error("failed to read from connection", "error", err)
			return
		}

		s.logger.Info("received data",
			"bytes", bytes,
			"data", msg,
		)

		// TODO: Do something with it, for now, just echo back
		if err := framing.SendMessage(conn, "ACK: "+msg, s.logger); err != nil {
			s.logger.Error("failed to send response", "error", err)
			return
		}
	}
}

func (s *Server) Host() string {
	return s.host
}

func (s *Server) Port() string {
	return s.port
}

func (s *Server) HasCapacity() bool {
	return s.connMgr.HasCapacity()
}

func (s *Server) ConnectionCount() uint16 {
	return s.connMgr.Count()
}
