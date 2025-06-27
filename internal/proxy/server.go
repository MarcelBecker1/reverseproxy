package proxy

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/MarcelBecker1/reverseproxy/internal/connection"
	"github.com/MarcelBecker1/reverseproxy/internal/framing"
	"github.com/MarcelBecker1/reverseproxy/internal/gameserver"
	"github.com/MarcelBecker1/reverseproxy/internal/logger"
	"github.com/MarcelBecker1/reverseproxy/internal/tcpserver"
)

// Can check that we listen on port with netstat -ano | findstr ":8080"

// Should use raw tcp socket connections

/*
	TODO:
		1. Can we split some functions, create new files to seperate concerns better?
		2. Can we test it like this? Need interfaces?
		3. Error handling more consistent?
		4. Better cleanup / shutdown logic
		5. Connection pooling for gs?
		6. Healthchecks for gs?
		7. Performance considerations?

	What we need here:
		- Bidirectional data flow, now we only get data from client and forward to gs but we need to also receive from gs and forward to client as mitm


	Does it make sense to switch to all tcp functions net functions?
*/

// Maybe use embeddigs

type GameServerInfo struct {
	server   *gameserver.Server
	connMngr *connection.Manager
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
	tcpServer   *tcpserver.Server
	connMngr    *connection.Manager
	gameServers map[string]*GameServerInfo
	clients     map[string]*ClientInfo
	deadline    time.Duration
	logger      *slog.Logger
	mu          sync.RWMutex
}

type Config struct {
	Host     string
	Port     string
	Deadline time.Duration
}

const (
	defaultCap uint16 = 10
	maxGsPort  uint16 = 9000
)

func New(c *Config) *ProxyServer {
	log := logger.NewWithComponent("proxy")
	server := tcpserver.New(c.Host, c.Port, log)
	connMngr := connection.NewManager(100) // proxy connection capacity

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
		p.logger.Error("failed initial auth", "error", err)
		return
	}
	gsInfo := p.gameServers[client.gsKey]

	gsInfo.connMngr.Increment()
	defer gsInfo.connMngr.Decrement()

	p.startBidirectionalProxy(client)
}

func (p *ProxyServer) handleInitAuth(client *ClientInfo) error {
	for !client.authenticated {
		if err := client.conn.SetReadDeadline(time.Now().Add(p.deadline)); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}

		msg, bytes, err := framing.ReadMessage(client.conn, p.logger)
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

// Currently its all over the place, we need to
// 1. Auth from client to proxy
// 2. Forward to gs
// 3. Gs checks again and returns to proxy
// 4. Proxy returns to client
// 5. Proxy keeps listening
// In general we just need 2 listeners as goroutines that for now run in while loops
// one for messages from client to gs
// one for messages from gs to client

func (p *ProxyServer) startBidirectionalProxy(client *ClientInfo, msg string) {
	// can be done in goroutines, forwarding the data to both
	if err := framing.SendMessage(client.gsConn, msg, p.logger); err != nil {
		p.logger.Error("sending message to gs failed", "error", err)
		return
	}

	for {
		if err := client.conn.SetReadDeadline(time.Now().Add(p.deadline)); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}

		msg, bytes, err := framing.ReadMessage(client.conn, p.logger)
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

	go func() {
		// read
	}()

	// start listener
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
		return framing.SendMessage(c.conn, "AUTH_OK", p.logger)
	}

	p.logger.Warn("auth failed", "client", c.id)
	framing.SendMessage(c.conn, "AUTH_FAILED", p.logger)
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

	gsConfig := &gameserver.Config{
		Host:     "localhost",
		Port:     port,
		Capacity: defaultCap,
	}
	gs := gameserver.New(gsConfig)

	connMngr := connection.NewManager(defaultCap)

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
