package proxy

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

// Can check that we listen on port with netstat -ano | findstr ":8080"

// Should use raw tcp socket connections

type ProxyServer struct {
	host        string
	port        string
	logger      *log.Logger
	connections int
	mu          sync.Mutex
}

type Config struct {
	Host string
	Port string
}

func New(conf *Config) *ProxyServer {
	return &ProxyServer{
		host:        conf.Host,
		port:        conf.Port,
		logger:      log.New(os.Stdout, "[PROXY]  ", log.LstdFlags),
		connections: 0,
	}
}

func (p *ProxyServer) Start(errorC chan error) {
	hostAdress := net.JoinHostPort(p.host, p.port)
	p.logger.Printf("listening on %s", hostAdress)

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
				p.logger.Printf("warning: connection error but no receiver reading: %v", err)
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
	p.logger.Printf("current connection count: %d", p.connections)
}
