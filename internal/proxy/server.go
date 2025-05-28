package proxy

import (
	"fmt"
)

// Should use raw tcp socket connections

type ProxyServer struct {
	host string
	port uint16
}

type Config struct {
	Host string
	Port uint16
}

// Actually should be singleton, we have one reverse proxy
func New(config *Config) *ProxyServer {
	return &ProxyServer{
		host: config.Host,
		port: config.Port,
	}
}

func (p *ProxyServer) Start() error {
	// Starting server listening for connections
	fmt.Println("Listenting for connections")
	return nil
}

func (p *ProxyServer) Connect() error {
	// Client connecting
	fmt.Println("Initiating connection")
	return nil
}
