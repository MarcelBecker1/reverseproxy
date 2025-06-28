package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/MarcelBecker1/reverseproxy/internal/logger"
	"github.com/MarcelBecker1/reverseproxy/internal/netutils"
)

/*
	Does it make sense to switch to all tcp functions net functions?
*/

// Maybe use embeddigs ?

type GameServerInfo struct {
	server   *GameServer
	connMngr *netutils.Manager
}

type ClientInfo struct {
	id            string
	conn          net.Conn
	gsConn        net.Conn
	gsKey         string
	authenticated bool
}

type ProxyServer struct {
	host        string
	port        string
	tcpServer   *TCPServer
	connMngr    *netutils.Manager
	gameServers map[string]*GameServerInfo
	clients     map[string]*ClientInfo
	deadline    time.Duration
	logger      *slog.Logger
	mu          sync.RWMutex
}

type ProxyServerConfig struct {
	Host     string
	Port     string
	Deadline time.Duration
}

const (
	defaultCap uint16 = 10
	maxGsPort  uint16 = 9000
)

func NewProxyServer(c *ProxyServerConfig) *ProxyServer {
	log := logger.NewWithComponent("proxy")
	server := NewTCPServer(c.Host, c.Port, log)
	connMngr := netutils.NewManager(100) // proxy connection capacity

	return &ProxyServer{
		host:        c.Host,
		port:        c.Port,
		tcpServer:   server,
		connMngr:    connMngr,
		gameServers: make(map[string]*GameServerInfo),
		clients:     make(map[string]*ClientInfo),
		deadline:    c.Deadline,
		logger:      log,
	}
}

func (p *ProxyServer) Start(errorC chan error) {
	if err := p.tcpServer.Start(p); err != nil {
		errorC <- fmt.Errorf("failed to start tcp server: %w", err)
		return
	}
	errorC <- nil
}

func (p *ProxyServer) HandleConnection(conn net.Conn) {
	defer conn.Close()

	p.connMngr.Increment()
	defer p.connMngr.Decrement()

	clientId := uuid.New().String()
	client := &ClientInfo{
		id:   clientId,
		conn: conn,
	}

	p.mu.Lock()
	p.clients[clientId] = client
	p.mu.Unlock()

	defer func() {
		p.logger.Info("closing connections")
		p.mu.Lock()
		delete(p.clients, clientId)
		if client.gsConn != nil {
			client.gsConn.Close()
		}
		p.mu.Unlock()
	}()

	if err := p.handleInitAuth(client); err != nil {
		p.logger.Error("failed initial auth", "error", err)
		return
	}

	if err := p.establishGSConnection(client); err != nil {
		p.logger.Error("failed to establish game server connection", "error", err)
		return
	}
	gsInfo := p.gameServers[client.gsKey]

	gsInfo.connMngr.Increment()
	defer gsInfo.connMngr.Decrement()

	p.startBidirectionalProxy(client)
}

func (p *ProxyServer) handleInitAuth(client *ClientInfo) error {
	authTimeout := time.After(30 * time.Second)

	for !client.authenticated {
		select {
		case <-authTimeout:
			return fmt.Errorf("authentication timeout")
		default:

		}

		if err := client.conn.SetReadDeadline(time.Now().Add(p.deadline)); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}

		msg, bytes, err := netutils.ReadMessage(client.conn, p.logger)
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("client disconnected during auth")
			}
			return fmt.Errorf("failed to read from connection %w", err)
		}

		p.logger.Info("received auth data", "bytes", bytes, "data", msg)

		if err := p.handleAuth(client, msg); err != nil {
			return err
		}
	}
	return nil
}

func (p *ProxyServer) establishGSConnection(client *ClientInfo) error {
	gsInfo := p.findAvailableGameServer()
	if gsInfo == nil {
		var err error
		gsInfo, err = p.startNewGameServer()
		if err != nil {
			return fmt.Errorf("failed to start new game server %w", err)
		}
	}
	gsKey := net.JoinHostPort(gsInfo.server.Host(), gsInfo.server.Port())
	gsConn, err := net.Dial("tcp", gsKey)
	if err != nil {
		return fmt.Errorf("failed to connect to game server %w", err)
	}
	client.gsConn = gsConn
	client.gsKey = gsKey
	return nil
}

func (p *ProxyServer) startBidirectionalProxy(client *ClientInfo) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer cancel()
		p.handleClientCommunication(ctx, client)
	}()

	go func() {
		defer wg.Done()
		defer cancel()
		p.handleGSCommunication(ctx, client)
	}()

	wg.Wait()
	p.logger.Info("proxy session ended", "client", client.id)
}

// TOOD: Implement retry with exp backoff (maybe 3 times) before closing connection

// Currently pretty much the same but they might diverge more so we keep them as 2 seperate functions for now

func (p *ProxyServer) handleClientCommunication(ctx context.Context, client *ClientInfo) {
	clientMsgChan := make(chan string, 10)
	go netutils.ListenForMessages(ctx, client.conn, clientMsgChan, p.deadline, p.logger)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("client communication stopped", "reason", "context cancelled")
			return
		case msg, ok := <-clientMsgChan:
			if !ok || msg == "" {
				p.logger.Info("client channel closed")
				p.forwardMsg(client.gsConn, "client connection closed")
				return
			}
			if err := p.forwardMsg(client.gsConn, msg); err != nil {
				// send client abort message
				p.logger.Error("failed to forward to gs", "error", err)
				p.forwardMsg(client.conn, "failed to forward to gs - aborting connection")
				return
			}
		}
	}
}

func (p *ProxyServer) handleGSCommunication(ctx context.Context, client *ClientInfo) {
	gsMsgChan := make(chan string, 20)
	go netutils.ListenForMessages(ctx, client.gsConn, gsMsgChan, p.deadline, p.logger)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("gs communication stopped", "reason", "context cancelled")
			return
		case msg, ok := <-gsMsgChan:
			if !ok || msg == "" {
				p.logger.Info("gs channel closed")
				p.forwardMsg(client.conn, "gs connection closed")
				return
			}
			if err := p.forwardMsg(client.conn, msg); err != nil {
				// send gs abort message
				p.logger.Error("failed to forward to client", "error", err)
				p.forwardMsg(client.gsConn, "failed forwarding msg to client - aborting connection")
				return
			}
		}
	}
}

func (p *ProxyServer) forwardMsg(conn net.Conn, msg string) error {
	if err := conn.SetWriteDeadline(time.Now().Add(p.deadline)); err != nil {
		return fmt.Errorf("failed to set write deadling %w", err)
	}

	if err := netutils.SendMessage(conn, msg, p.logger); err != nil {
		return fmt.Errorf("failed sending messages to gs %w", err)
	}

	return nil
}

func (p *ProxyServer) authClient(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "auth")
}

func (p *ProxyServer) handleAuth(c *ClientInfo, msg string) error {
	if c.authenticated {
		return nil
	}

	if p.authClient(msg) {
		c.authenticated = true
		p.logger.Info("client authenticated", "id", c.id)
		return netutils.SendMessage(c.conn, "AUTH_OK", p.logger)
	}

	p.logger.Warn("auth failed", "client", c.id)
	netutils.SendMessage(c.conn, "AUTH_FAILED", p.logger)
	return fmt.Errorf("missing auth in message")
}

func (p *ProxyServer) findAvailableGameServer() *GameServerInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, gs := range p.gameServers {
		if gs.connMngr.HasCapacity() {
			return gs
		}
	}
	return nil
}

func (p *ProxyServer) startNewGameServer() (*GameServerInfo, error) {
	port, err := p.getFreePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get free port: %w", err)
	}

	gsConfig := &GameServerConfig{
		Host:     "localhost",
		Port:     port,
		Capacity: defaultCap,
	}
	gs := NewGameServer(gsConfig)

	connMngr := netutils.NewManager(defaultCap)

	info := &GameServerInfo{
		server:   gs,
		connMngr: connMngr,
	}

	p.mu.Lock()
	p.gameServers[net.JoinHostPort(gsConfig.Host, gsConfig.Port)] = info
	p.mu.Unlock()

	go gs.Start()
	return info, nil
}

func (p *ProxyServer) getFreePort() (string, error) {
	startPort, err := strconv.Atoi(p.port)
	if err != nil {
		return "", err
	}

	for port := startPort + 1; port <= int(maxGsPort); port++ {
		addr := net.JoinHostPort(p.host, strconv.Itoa(port))

		l, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		l.Close()

		portStr := strconv.Itoa(port)
		p.logger.Info("found free port", "port", portStr)
		return portStr, nil
	}
	return "", fmt.Errorf("no port available in the range %d to %d", startPort+1, maxGsPort)
}
