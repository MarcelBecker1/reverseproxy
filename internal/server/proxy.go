package server

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/MarcelBecker1/reverseproxy/internal/db"
	"github.com/MarcelBecker1/reverseproxy/internal/logger"
	"github.com/MarcelBecker1/reverseproxy/internal/netutils"
)

/*
	Does it make sense to switch to all tcp functions net functions?
*/

type ClientInfo struct {
	id           string
	conn         net.Conn
	gsConn       net.Conn
	gsID         int
	sessionID    int
	trafficMutex sync.Mutex
	bytesUp      int64
	bytesDown    int64
	messagesUp   int64
	messagesDown int64
	lastUpdate   time.Time
}

type ProxyServer struct {
	host         string
	port         string
	tcpServer    *TCPServer
	clients      map[string]*ClientInfo // runtime client connections
	stateManager *db.StateManager       // state of proxy with clients and gameservers
	timeout      time.Duration
	authTimeout  time.Duration
	mu           *sync.RWMutex
	logger       *slog.Logger
	dbUpdateChan chan dbUpdateJob
	dbWorkerWg   sync.WaitGroup
}

type ProxyServerConfig struct {
	Host         string
	Port         string
	Timeout      time.Duration
	DatabasePath string
}

type dbUpdateJob struct {
	sessionID    int
	bytesUp      int64
	bytesDown    int64
	messagesUp   int64
	messagesDown int64
}

const (
	defaultCap uint16 = 10
	maxGsPort  uint16 = 9000
)

func NewProxyServer(c *ProxyServerConfig) *ProxyServer {
	log := logger.NewWithComponent("proxy")
	server := NewTCPServer(c.Host, c.Port, log)
	authTimeout := time.Duration(30) * time.Second

	dbPath := c.DatabasePath
	if dbPath == "" {
		dbPath = fmt.Sprintf("./proxydatabases/proxy_%d.db", time.Now().Unix())
	}

	stateManager, err := db.NewStateManager(dbPath, log)
	if err != nil {
		log.Error("failed to create state manager", "error", err)
		return nil
	}

	proxy := &ProxyServer{
		host:         c.Host,
		port:         c.Port,
		tcpServer:    server,
		clients:      make(map[string]*ClientInfo),
		stateManager: stateManager,
		timeout:      c.Timeout,
		authTimeout:  authTimeout,
		mu:           &sync.RWMutex{},
		logger:       log,
		dbUpdateChan: make(chan dbUpdateJob, 100),
	}

	// Let worker pool handle db stat updates to prevent db overloading
	proxy.startDBWorkers()
	return proxy
}

func (p *ProxyServer) Start(errorC chan error) {
	p.stateManager.Start()

	if err := p.tcpServer.Start(p); err != nil {
		errorC <- fmt.Errorf("failed to start tcp server: %w", err)
		return
	}
	errorC <- nil
}

func (p *ProxyServer) Stop() {
	close(p.dbUpdateChan)
	p.dbWorkerWg.Wait()
	p.stateManager.Stop()
}

func (p *ProxyServer) HandleConnection(conn net.Conn) {
	defer conn.Close()

	clientId := uuid.New().String()
	client := &ClientInfo{
		id:   clientId,
		conn: conn,
	}

	if err := p.stateManager.CreateClient(clientId, conn.RemoteAddr().String()); err != nil {
		p.logger.Error("failed to create client in database", "error", err)
		return
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

		stats, err := p.stateManager.GetStats()
		if err != nil {
			p.logger.Debug("cannot get stats")
		}
		p.logger.Info("proxy stats", "stats", stats)

		if err := p.stateManager.RemoveClient(clientId); err != nil {
			p.logger.Error("failed to remove client from database", "error", err)
		}
	}()

	proxyAuthResult, err := HandleInitAuth(client, p.authTimeout, p.timeout, p.logger)
	if err != nil {
		p.logger.Error("failed proxy authentication", "error", err, "result", proxyAuthResult)
		p.stateManager.UpdateClientStatus(clientId, "auth_failed")
		return
	}

	if err := p.stateManager.SetClientAuthenticated(clientId, true); err != nil {
		p.logger.Error("failed to update client authentication status", "error", err)
		return
	}

	if err := p.establishGSConnection(client); err != nil {
		p.logger.Error("failed to establish game server connection", "error", err)
		p.stateManager.UpdateClientStatus(clientId, "gs_connection_failed")
		return
	}

	gsAuthResult, err := HandleGameServerAuth(client, client.gsConn, p.timeout, p.logger)
	if err != nil {
		p.logger.Error("failed game server authentication", "error", err, "result", gsAuthResult)
		p.stateManager.UpdateClientStatus(clientId, "gs_auth_failed")
		return
	}

	if err := CompleteAuthentication(client, p.logger); err != nil {
		p.logger.Error("failed to complete authentication", "error", err)
		return
	}

	p.logger.Info("full authentication successful", "client", clientId,
		"proxyAuth", proxyAuthResult.Success, "gsAuth", gsAuthResult.Success)

	session, err := p.stateManager.CreateProxySession(clientId, client.gsID)
	if err != nil {
		p.logger.Error("failed to create proxy session", "error", err)
		return
	}
	client.sessionID = session.ID

	if err := p.stateManager.IncrementGameServerLoad(client.gsID); err != nil {
		p.logger.Error("failed to increment game server load", "error", err)
	}

	defer func() {
		if err := p.stateManager.DecrementGameServerLoad(client.gsID); err != nil {
			p.logger.Error("failed to decrement game server load", "error", err)
		}

		if err := p.stateManager.EndProxySession(client.sessionID, "ended"); err != nil {
			p.logger.Error("failed to end proxy session", "error", err)
		}
	}()

	p.startBidirectionalProxy(client)
}

func (p *ProxyServer) establishGSConnection(client *ClientInfo) error {
	gsInfo, err := p.stateManager.GetAvailableGameServer()
	if err != nil {
		return fmt.Errorf("failed to query available game servers: %w", err)
	}

	if gsInfo == nil {
		gsInfo, err = p.startNewGameServer()
		if err != nil {
			return fmt.Errorf("failed to start new game server: %w", err)
		}
	}

	p.logger.Info("game server is ready", "id", gsInfo.ID, "host", gsInfo.Host, "port", gsInfo.Port)

	gsKey := net.JoinHostPort(gsInfo.Host, gsInfo.Port)
	gsConn, err := net.Dial("tcp", gsKey)
	if err != nil {
		return fmt.Errorf("failed to connect to game server: %w", err)
	}

	client.gsConn = gsConn
	client.gsID = gsInfo.ID

	if err := p.stateManager.AssignClientToGameServer(client.id, gsInfo.ID); err != nil {
		return fmt.Errorf("failed to assign client to game server: %w", err)
	}

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

// Update database every 10 messages or every x seconds with x in [5,15] as we are creating too many connections to the database
func (p *ProxyServer) updateTrafficBatched(client *ClientInfo, bytesUp, bytesDown, messagesUp, messagesDown int64) {
	client.trafficMutex.Lock()
	defer client.trafficMutex.Unlock()

	client.bytesUp += bytesUp
	client.bytesDown += bytesDown
	client.messagesUp += messagesUp
	client.messagesDown += messagesDown

	now := time.Now()

	if client.lastUpdate.IsZero() {
		client.lastUpdate = now
	}

	r := rand.New(rand.NewSource(now.UnixNano()))
	jitter := 5000 + r.Intn(10000) // range from 5 to 15 seconds
	shouldUpdate := client.messagesUp+client.messagesDown >= 3 || now.Sub(client.lastUpdate) >= time.Duration(jitter)*time.Millisecond

	if shouldUpdate {
		up, down, msgUp, msgDown := client.bytesUp, client.bytesDown, client.messagesUp, client.messagesDown
		client.bytesUp, client.bytesDown, client.messagesUp, client.messagesDown = 0, 0, 0, 0
		client.lastUpdate = now

		// Non blocking, just log if we can't update the databse atm
		select {
		case p.dbUpdateChan <- dbUpdateJob{
			sessionID:    client.sessionID,
			bytesUp:      up,
			bytesDown:    down,
			messagesUp:   msgUp,
			messagesDown: msgDown,
		}:
		default:
			p.logger.Warn("database update queue full, skipping update")
		}
	}
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
				p.logger.Warn("received empty messages from client")
				continue
			}

			if err := p.stateManager.UpdateClientActivity(client.id); err != nil {
				p.logger.Error("failed to update client activity", "error", err)
			}

			if err := netutils.ForwardMsg(client.gsConn, msg, p.timeout, p.logger); err != nil {
				p.logger.Error("failed to forward to gs", "error", err)
				netutils.ForwardMsg(client.conn, "failed to forward to gs - aborting connection", p.timeout, p.logger)
				return
			}

			msgSize := int64(len(msg))
			p.updateTrafficBatched(client, msgSize, 0, 1, 0)
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
				p.logger.Warn("received empty messages from game server")
				continue
			}

			if err := netutils.ForwardMsg(client.conn, msg, p.timeout, p.logger); err != nil {
				p.logger.Error("failed to forward to client", "error", err)
				netutils.ForwardMsg(client.gsConn, "failed forwarding msg to client - aborting connection", p.timeout, p.logger)
				return
			}

			msgSize := int64(len(msg))
			p.updateTrafficBatched(client, 0, msgSize, 0, 1)
		}
	}
}

func (p *ProxyServer) startNewGameServer() (*db.DBGameServer, error) {
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
	errorChan := make(chan error, 1)
	go gs.Start(errorChan)

	select {
	case err := <-errorChan:
		if err != nil {
			return nil, fmt.Errorf("failed to start game server: %w", err)
		}
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("timeout waiting for game server to start")
	}

	gsRecord, err := p.stateManager.CreateGameServer(gs.host, gs.port, "running", gs.capacity, gs.CurrentLoad())
	if err != nil {
		return nil, fmt.Errorf("failed to create game server record: %w", err)
	}

	return gsRecord, nil
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

// Methods for testing

func (p *ProxyServer) GetProxyState() (*db.ProxyState, error) {
	return p.stateManager.GetFullState()
}

func (p *ProxyServer) GetStats() (*db.DBProxyStats, error) {
	return p.stateManager.GetStats()
}

func (p *ProxyServer) GetActiveClients() ([]*db.DBClient, error) {
	return p.stateManager.GetActiveClients()
}

func (p *ProxyServer) GetGameServers() ([]*db.DBGameServer, error) {
	return p.stateManager.GetAllGameServers()
}

func (p *ProxyServer) RefreshStats() error {
	return p.stateManager.RefreshStats()
}

func (p *ProxyServer) HealthCheck() error {
	return p.stateManager.HealthCheck()
}

func (p *ProxyServer) startDBWorkers() {
	for range 3 {
		p.dbWorkerWg.Add(1)
		go p.dbUpdateWorker()
	}
}

// TODO: stat updating not working as i want it
func (p *ProxyServer) dbUpdateWorker() {
	defer p.dbWorkerWg.Done()

	for job := range p.dbUpdateChan {
		if err := p.stateManager.UpdateProxySessionTraffic(job.sessionID, job.bytesUp, job.bytesDown, job.messagesUp, job.messagesDown); err != nil {
			p.logger.Error("failed to update session traffic", "error", err)
		}
	}
}
