package proxy

import (
	"fmt"
	"net"
)

// Can check that we listen on port with netstat -ano | findstr ":8080"

// Should use raw tcp socket connections

type ProxyServer struct {
	host string
	port uint16
}

type Config struct {
	Host string
	Port uint16
}

func New(config *Config) *ProxyServer {
	return &ProxyServer{
		host: config.Host,
		port: config.Port,
	}
}

func (p *ProxyServer) Start() error {
	fmt.Println("starting server...")
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", p.host, p.port))
	if err != nil {
		return fmt.Errorf("failed to create tcp listener: %w", err)
	}

	defer ln.Close()

	numConnections := 0 // remove
	for numConnections < 1 {
		conn, err := ln.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept connection: %w", err)
		}
		numConnections++
		go p.handleConnection(conn)
	}

	return nil
}

func (p *ProxyServer) handleConnection(conn net.Conn) {
	fmt.Println("received connection")
	conn.Close()
	// add logic later
}

func (p *ProxyServer) Connect() error {
	// Function for the clients -> move it to client with net.Dial("tcp", "host:port")
	fmt.Println("initiating connection")
	return nil
}
