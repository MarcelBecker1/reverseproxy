package server

import (
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/MarcelBecker1/reverseproxy/internal/logger"
	"github.com/MarcelBecker1/reverseproxy/internal/netutils"
)

/*
	TODO:
		Create some dummy data that we want to send back to clients
*/

type GameServer struct {
	host      string
	port      string
	tcpServer *TCPServer
	connMgr   *netutils.Manager
	mu        sync.Mutex
	logger    *slog.Logger
}

type GameServerConfig struct {
	Host     string
	Port     string
	Capacity uint16
}

func NewGameServer(c *GameServerConfig) *GameServer {
	log := logger.NewWithComponent("gameserver")
	server := NewTCPServer(c.Host, c.Port, log)
	connMngr := netutils.NewManager(c.Capacity)

	return &GameServer{
		host:      c.Host,
		port:      c.Port,
		tcpServer: server,
		connMgr:   connMngr,
		logger:    log,
	}
}

func (s *GameServer) Start() error {
	return s.tcpServer.Start(s)
}

func (s *GameServer) HandleConnection(conn net.Conn) {
	defer conn.Close()
	s.connMgr.Increment()
	defer s.connMgr.Decrement()

	for {
		// we are not setting read deadling atm, maybe change? -> be consistent
		msg, bytes, err := netutils.ReadMessage(conn, s.logger)
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
		if err := netutils.SendMessage(conn, "ACK: "+msg, s.logger); err != nil {
			s.logger.Error("failed to send response", "error", err)
			return
		}
	}
}

func (s *GameServer) Host() string {
	return s.host
}

func (s *GameServer) Port() string {
	return s.port
}

func (s *GameServer) HasCapacity() bool {
	return s.connMgr.HasCapacity()
}

func (s *GameServer) ConnectionCount() uint16 {
	return s.connMgr.Count()
}
