package proxy

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/MarcelBecker1/reverseproxy/internal/logger"
)

// Can check that we listen on port with netstat -ano | findstr ":8080"

// Should use raw tcp socket connections

type ProxyServer struct {
	host        string
	port        string
	connections int
	mu          sync.Mutex
}

type Config struct {
	Host string
	Port string
}

func New(conf *Config) *ProxyServer {
	logger.NewWithComponent("proxy")
	return &ProxyServer{
		host:        conf.Host,
		port:        conf.Port,
		connections: 0,
	}
}

func (p *ProxyServer) Start(errorC chan error) {
	hostAdress := net.JoinHostPort(p.host, p.port)
	slog.Info("listening for tcp connections", "address", hostAdress)

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
				slog.Warn("connection error but no receiver reading", "error", err)
			}
			continue
		}
		go p.handleConnection(conn)
	}
}

func (p *ProxyServer) handleConnection(conn net.Conn) {
	p.incConnections()
	conn.Close()
	// add logic later
}

func (p *ProxyServer) incConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connections++
	slog.Info("connection count increased", "connections", p.connections)
}
