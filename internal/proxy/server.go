package proxy

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/MarcelBecker1/reverseproxy/internal/logger"
)

// Can check that we listen on port with netstat -ano | findstr ":8080"

// Should use raw tcp socket connections

type ProxyServer struct {
	host        string
	port        string
	deadline    time.Duration
	connections int
	mu          sync.Mutex
}

type Config struct {
	Host     string
	Port     string
	Deadline time.Duration
}

func New(conf *Config) *ProxyServer {
	logger.NewWithComponent("proxy")
	return &ProxyServer{
		host:        conf.Host,
		port:        conf.Port,
		connections: 0,
		deadline:    conf.Deadline,
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
	defer conn.Close()
	p.incConnections()

	buffer := make([]byte, 1024) // Idk yet about the size, do we want to take something larger?

	for {
		if err := conn.SetReadDeadline(time.Now().Add(p.deadline)); err != nil {
			slog.Error("failed to set read deadline", "error", err)
			return
		}
		n, err := conn.Read(buffer)
		if err != nil {
			slog.Error("failed to read from connection", "error", err)
			return
		}
		slog.Info("received data", "bytes", n)
	}

}

func (p *ProxyServer) incConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connections++
	slog.Info("connection count increased", "connections", p.connections)
}
