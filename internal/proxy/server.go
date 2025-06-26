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

type GameServerInfo struct {
	server   *gameserver.Server
	connMngr *connection.Manager
}

type ClientInfo struct {
	id            string
	conn          net.Conn
	gsConn        net.Conn
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
	mu          sync.Mutex
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

// Unsure if i want to start it in a new goroutine here
func (p *ProxyServer) Start(errorC chan error) {
	go func() {
		if err := p.tcpServer.Start(p); err != nil {
			errorC <- fmt.Errorf("failed to start tcp server: %w", err)
		}
	}()
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

	for {
		if err := conn.SetReadDeadline(time.Now().Add(p.deadline)); err != nil { // Do we even need the timeout?
			p.logger.Error("failed to set read deadline", "error", err)
			return
		}

		msg, bytes, err := framing.ReadMessage(conn, p.logger)
		if err != nil {
			if err == io.EOF {
				p.logger.Info("client disconnected")
				return
			}
			p.logger.Error("failed to read from connection", "error", err)
			return
		}

		p.logger.Info("received data",
			"bytes", bytes,
			"data", msg,
		)

		if !client.authenticated {
			if err := p.handleAuth(client, msg); err != nil {
				p.logger.Error("authentication failed for client", "error", err)
				return
			}
		}

		if client.gsConn == nil {
			gsInfo := p.findAvailableGameServer()
			if gsInfo == nil {
				gsInfo, err = p.startNewGameServer()
				if err != nil {
					p.logger.Error("failed to start new game server", "error", err)
					return
				}
			}

			gsConn, err := net.Dial("tcp", net.JoinHostPort(gsInfo.server.Host(), gsInfo.server.Port()))
			if err != nil {
				p.logger.Error("failed to connect to game server", "error", err)
				return
			}
			client.gsConn = gsConn
			gsInfo.connMngr.Increment()
			defer gsInfo.connMngr.Decrement()
		}

		p.startBidirectionalProxy(client, msg)
	}
}

func (p *ProxyServer) startBidirectionalProxy(client *ClientInfo, msg string) {
	// can be done in goroutines, forwarding the data to both
	if err := framing.SendMessage(client.gsConn, msg, p.logger); err != nil {
		p.logger.Error("sending message to gs failed", "error", err)
		return
	}
}

// TODO: Split the Handle Connection function into handleInitAuth / establishGsConn

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
	return framing.SendMessage(c.conn, "AUTH_FAILED", p.logger)
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
