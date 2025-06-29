package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
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
	timeout     time.Duration
	authTimeout time.Duration
	mu          *sync.RWMutex
	logger      *slog.Logger
}

type ProxyServerConfig struct {
	Host    string
	Port    string
	Timeout time.Duration
}

const (
	defaultCap uint16 = 10
	maxGsPort  uint16 = 9000
)

func NewProxyServer(c *ProxyServerConfig) *ProxyServer {
	log := logger.NewWithComponent("proxy")
	server := NewTCPServer(c.Host, c.Port, log)
	connMngr := netutils.NewManager(100) // proxy connection capacity
	authTimeout := time.Duration(30)

	return &ProxyServer{
		host:        c.Host,
		port:        c.Port,
		tcpServer:   server,
		connMngr:    connMngr,
		gameServers: make(map[string]*GameServerInfo),
		clients:     make(map[string]*ClientInfo),
		timeout:     c.Timeout,
		authTimeout: authTimeout,
		mu:          &sync.RWMutex{},
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

	if err := HandleInitAuth(client, p.authTimeout, p.timeout, p.logger); err != nil {
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

func (p *ProxyServer) establishGSConnection(client *ClientInfo) error {
	gsInfo := p.findAvailableGameServer()
	if gsInfo == nil {
		var err error
		gsInfo, err = p.startNewGameServer()
		if err != nil {
			return fmt.Errorf("failed to start new game server %w", err)
		}
	}
	p.logger.Info("game server is ready")
	gsKey := net.JoinHostPort(gsInfo.server.Host(), gsInfo.server.Port())
	gsConn, err := net.Dial("tcp", gsKey)
	if err != nil {
		return fmt.Errorf("failed to connect to game server %w", err)
	}

	// TODO: for now we just forward the id after we authenticated at the proxy. I think we should also check at the gs itself.

	authMsg := fmt.Sprintf("CLIENT_AUTH:%s", client.id)
	if err := netutils.SendMessage(gsConn, authMsg, p.logger); err != nil {
		gsConn.Close()
		return fmt.Errorf("failed to send client auth to game server: %w", err)
	}

	// TODO: i want to auth at gs and return the ack only if it also passes, then we return to client not before from proxy

	response, _, err := netutils.ReadMessage(gsConn, p.logger)
	if err != nil || response != "AUTH_ACK" {
		gsConn.Close()
		return fmt.Errorf("game server auth failed: %w", err)
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
	go netutils.ListenForMessages(ctx, client.conn, clientMsgChan, p.timeout, p.logger)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("client communication stopped", "reason", "context cancelled")
			return
		case msg, ok := <-clientMsgChan:
			if !ok {
				p.logger.Info("client channel closed")
				netutils.ForwardMsg(client.gsConn, "client connection closed", p.timeout, p.logger)
				return
			}
			if msg == "" {
				p.logger.Warn("received emptpy messages from client")
				continue
			}
			if err := netutils.ForwardMsg(client.gsConn, msg, p.timeout, p.logger); err != nil {
				// send client abort message
				p.logger.Error("failed to forward to gs", "error", err)
				netutils.ForwardMsg(client.conn, "failed to forward to gs - aborting connection", p.timeout, p.logger)
				return
			}
		}
	}
}

func (p *ProxyServer) handleGSCommunication(ctx context.Context, client *ClientInfo) {
	gsMsgChan := make(chan string, 20)
	go netutils.ListenForMessages(ctx, client.gsConn, gsMsgChan, p.timeout, p.logger)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("gs communication stopped", "reason", "context cancelled")
			return
		case msg, ok := <-gsMsgChan:
			if !ok {
				p.logger.Info("gs channel closed")
				netutils.ForwardMsg(client.conn, "gs connection closed", p.timeout, p.logger)
				return
			}
			if msg == "" {
				p.logger.Warn("received emptpy messages from game server")
				continue
			}
			if err := netutils.ForwardMsg(client.conn, msg, p.timeout, p.logger); err != nil {
				// send gs abort message
				p.logger.Error("failed to forward to client", "error", err)
				netutils.ForwardMsg(client.gsConn, "failed forwarding msg to client - aborting connection", p.timeout, p.logger)
				return
			}
		}
	}
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
	p.logger.Info("starting new game server")
	port, err := p.getFreePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get free port: %w", err)
	}

	gsConfig := &GameServerConfig{
		Host:     "localhost",
		Port:     port,
		Capacity: defaultCap,
		Timeout:  p.timeout,
	}
	gs := NewGameServer(gsConfig)

	connMngr := netutils.NewManager(defaultCap)

	info := &GameServerInfo{
		server:   gs,
		connMngr: connMngr,
	}

	errorChan := make(chan error, 1)
	go gs.Start(errorChan)
	select {
	case err := <-errorChan:
		if err != nil {
			return nil, err
		}
	case <-time.After(10 * time.Second):
		// we need to singal gs that we stopped, else it might start but takes too long and now we dont use it
		return nil, fmt.Errorf("timeout waiting for game server to start")
	}
	p.mu.Lock()
	p.gameServers[net.JoinHostPort(gsConfig.Host, gsConfig.Port)] = info
	p.mu.Unlock()
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
