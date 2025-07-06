package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/MarcelBecker1/reverseproxy/internal/logger"
	"github.com/MarcelBecker1/reverseproxy/internal/netutils"
)

/*
	TODO: Create some dummy data that we want to send back to clients
		-> also allows for less chatty logs

	TODO: What we should do is keep some state that we can verify whether it works with the connections and sent messages
*/

type GSClientInfo struct {
	id            string
	authenticated bool
}

type GameServer struct {
	host        string
	port        string
	tcpServer   *TCPServer
	proxyConn   net.Conn
	capacity    uint16
	connMgr     *netutils.Manager
	clients     map[string]*GSClientInfo
	msgCntr     uint16
	timeout     time.Duration
	authTimeout time.Duration
	mu          *sync.Mutex
	logger      *slog.Logger
}

type GameServerConfig struct {
	Host     string
	Port     string
	Capacity uint16
	Timeout  time.Duration
}

func NewGameServer(c *GameServerConfig) *GameServer {
	log := logger.NewWithComponent("gameserver")
	server := NewTCPServer(c.Host, c.Port, log)
	connMngr := netutils.NewManager(c.Capacity)
	authTimeout := time.Duration(30)

	return &GameServer{
		host:        c.Host,
		port:        c.Port,
		tcpServer:   server,
		capacity:    c.Capacity,
		connMgr:     connMngr,
		clients:     make(map[string]*GSClientInfo),
		msgCntr:     0,
		timeout:     c.Timeout,
		authTimeout: authTimeout,
		mu:          &sync.Mutex{},
		logger:      log,
	}
}

func (s *GameServer) Start(errorChan chan error) {
	if err := s.tcpServer.Start(s); err != nil {
		errorChan <- err
		return
	}
	errorChan <- nil
}

func (s *GameServer) HandleConnection(conn net.Conn) {
	defer conn.Close()

	s.connMgr.Increment()
	defer s.connMgr.Decrement()

	s.mu.Lock()
	s.proxyConn = conn
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.proxyConn = nil
		s.mu.Unlock()
		s.logger.Info("proxy connection cleaned up")
	}()

	s.startProxyCommunication()
}

func (s *GameServer) startProxyCommunication() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgChan := make(chan string, 10)
	go netutils.ListenForMessages(ctx, s.proxyConn, msgChan, s.timeout, s.logger)

	var clientId string

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("proxy communication stopped", "reason", "context cancelled")
			return
		case msg, ok := <-msgChan:
			if !ok {
				s.logger.Info("proxy channel closed")
				return
			}
			if msg == "" {
				s.logger.Warn("received emptpy messages from proxy")
				continue
			}
			if client, ok := s.clients[clientId]; !ok || !client.authenticated {
				clientId = s.handleClientAuth(msg)
				continue
			}
			// TODO: not optimal to look for this strings, should have better signaling here
			if strings.HasSuffix(msg, "aborting connection") || strings.Contains(msg, "client connection closed") {
				s.logger.Info("abort received for client", "clientId", clientId)
				s.removeClient(clientId)
				return
			}

			s.handleClientMessage()
		}
	}
}

// TODO: add handling in error cases
// could add some more meaningful returns
func (s *GameServer) handleClientMessage() {
	s.mu.Lock()
	s.msgCntr++
	currMsgs := s.msgCntr
	currClients := len(s.clients)
	s.mu.Unlock()

	response := fmt.Sprintf("GAME_STATE:players_%d,received_messages_%d,timestamp_%d",
		currClients,
		currMsgs,
		time.Now().Unix(),
	)

	if err := netutils.ForwardMsg(s.proxyConn, response, s.timeout, s.logger); err != nil {
		s.logger.Error("failed to forward to proxy", "error", err)
	}
}

func (s *GameServer) handleClientAuth(msg string) string {
	if strings.HasPrefix(msg, "CLIENT_AUTH:") {
		clientId := strings.TrimPrefix(msg, "CLIENT_AUTH:")
		s.logger.Info("received client auth", "clientId", clientId)

		if clientId == "" {
			s.logger.Warn("received client with empty id")
			return ""
		}

		s.mu.Lock()
		s.clients[clientId] = &GSClientInfo{
			id:            clientId,
			authenticated: true,
		}
		s.mu.Unlock()

		if err := netutils.SendMessage(s.proxyConn, "AUTH_ACK", s.logger); err != nil {
			return "" //TODO: retry?
		}

		return clientId
	}

	return ""
}

func (s *GameServer) removeClient(clientId string) {
	s.mu.Lock()
	delete(s.clients, clientId)
	s.mu.Unlock()
	s.logger.Info("client removed", "clientId", clientId)
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

func (s *GameServer) CurrentLoad() uint16 {
	return s.connMgr.Count()
}
