package proxy

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/MarcelBecker1/reverseproxy/internal/framing"
	"github.com/MarcelBecker1/reverseproxy/internal/gameserver"
	"github.com/MarcelBecker1/reverseproxy/internal/logger"
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
*/

const (
	defaultCap uint8 = 10
)

var log *slog.Logger

type GameServerInfo struct {
	server      *gameserver.Server
	capacity    uint8
	connections uint8
	mu          sync.Mutex
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
	deadline    time.Duration
	gameServers map[string]*GameServerInfo
	clients     map[string]*ClientInfo
	mu          sync.Mutex
}

type Config struct {
	Host     string
	Port     string
	Deadline time.Duration
}

func New(conf *Config) *ProxyServer {
	log = logger.NewWithComponent("proxy")
	return &ProxyServer{
		host:        conf.Host,
		port:        conf.Port,
		gameServers: make(map[string]*GameServerInfo),
		clients:     make(map[string]*ClientInfo),
		deadline:    conf.Deadline,
	}
}

func (p *ProxyServer) Start(errorC chan error) {
	hostAdress := net.JoinHostPort(p.host, p.port)
	log.Info("listening for tcp connections", "address", hostAdress)

	listener, err := net.Listen("tcp", hostAdress)
	if err != nil {
		errorC <- fmt.Errorf("failed to create tcp listener: %w", err)
		return
	}
	defer listener.Close()
	errorC <- nil

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case errorC <- fmt.Errorf("failed to accept connection: %w", err):
			default:
				log.Warn("connection error but no receiver reading", "error", err)
			}
			continue
		}
		go p.handleConnection(conn)
	}
}

func (p *ProxyServer) handleConnection(conn net.Conn) {
	defer conn.Close()

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
			log.Error("failed to set read deadline", "error", err)
			return
		}

		msg, length, err := framing.ReadMessage(conn, log)
		if err != nil {
			if err == io.EOF {
				log.Info("client disconnected")
				return
			}
			log.Error("failed to read from connection", "error", err)
			return
		}

		log.Info("received data",
			"bytes", length,
			"data", msg,
		)

		if !client.authenticated {
			if err := p.handleAuth(client, msg); err != nil {
				log.Error("authentication failed for client", "error", err)
				return
			}
		}

		if client.gsConn == nil {
			gsInfo := p.findAvailableGameServer()
			if gsInfo == nil {
				gsInfo, err = p.startNewGameServer()
				if err != nil {
					log.Error("failed to start new game server", "error", err)
					return
				}
			}

			gsConn, err := net.Dial("tcp", net.JoinHostPort(gsInfo.server.Host(), gsInfo.server.Port()))
			if err != nil {
				log.Error("failed to connect to game server", "error", err)
				return
			}
			client.gsConn = gsConn
			gsInfo.incrementConnections()
			defer gsInfo.decrementConnections()
		}

		if err := framing.SendMessage(client.gsConn, msg, log); err != nil {
			log.Error("sending message to gs failed", "error", err)
			return
		}
	}
}

func (p *ProxyServer) findAvailableGameServer() *GameServerInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, gs := range p.gameServers {
		if gs.hasCapacity() {
			return gs
		}
	}
	return nil
}

func (p *ProxyServer) startNewGameServer() (*GameServerInfo, error) {
	cap := defaultCap
	port, err := getFreePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get free port: %w", err)
	}

	gsConfig := &gameserver.Config{
		Host:     "localhost",
		Port:     port,
		Capacity: defaultCap,
	}
	gs := gameserver.New(gsConfig)

	info := &GameServerInfo{
		server:      gs,
		capacity:    cap,
		connections: 0,
	}

	p.mu.Lock()
	p.gameServers[net.JoinHostPort(gsConfig.Host, gsConfig.Port)] = info
	p.mu.Unlock()

	go gs.Start()
	return info, nil
}

func (p *ProxyServer) authClient(msg string) bool {
	return strings.Contains(strings.ToLower(msg), ("auth"))
}

func (p *ProxyServer) handleAuth(c *ClientInfo, msg string) error {
	if c.authenticated {
		return nil
	}

	if p.authClient(msg) {
		c.authenticated = true
		log.Info("client authenticated", "id", c.id)
		return framing.SendMessage(c.conn, "AUTH_OK", log)
	}

	return framing.SendMessage(c.conn, "AUTH_FAILED", log)
}

func (gs *GameServerInfo) incrementConnections() {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.connections++
}

func (gs *GameServerInfo) decrementConnections() {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if gs.connections > 0 {
		gs.connections--
	}
}

func (gs *GameServerInfo) hasCapacity() bool {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.connections < gs.capacity
}

// TODO: Do i want to switch in general to all tcp functions?
func getFreePort() (string, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:8000")
	if err != nil {
		return "", err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return "", err
	}
	defer l.Close()

	return fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port), nil
}

// I need to keep track of the clients that are connected and the gameservers
// is sql lite here a good idea?
// What i want to do is sort of matchmaking:
// 		if gameserver available -> forward connection to gameserver by creating new tcp socket and basically just be the man in the middle
// 		if not available (no capacity left or non started at all) -> i want to start a new one and forward connection
