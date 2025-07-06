package db

import (
	"fmt"
	"log/slog"
	"time"
)

type StateManager struct {
	db                   *Database
	gameServerRepo       *GameServerRepository
	clientRepo           *ClientRepository
	proxySessionRepo     *ProxySessionRepository
	statsRepo            *StatsRepository
	logger               *slog.Logger
	statsRefreshInterval time.Duration
	stopChan             chan struct{}
}

func NewStateManager(dbPath string, logger *slog.Logger) (*StateManager, error) {
	db, err := NewDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	sm := &StateManager{
		db:                   db,
		gameServerRepo:       NewGameServerRepository(db),
		clientRepo:           NewClientRepository(db),
		proxySessionRepo:     NewProxySessionRepository(db),
		statsRepo:            NewStatsRepository(db),
		logger:               logger,
		statsRefreshInterval: 30 * time.Second,
		stopChan:             make(chan struct{}),
	}

	return sm, nil
}

func (sm *StateManager) Start() {
	go sm.statsRefreshLoop()
	sm.logger.Info("state manager started")
}

func (sm *StateManager) Stop() {
	close(sm.stopChan)
	sm.db.Close()
	sm.logger.Info("state manager stopped")
}

func (sm *StateManager) statsRefreshLoop() {
	ticker := time.NewTicker(sm.statsRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := sm.statsRepo.RefreshStats(); err != nil {
				sm.logger.Error("failed to refresh stats", "error", err)
			}
		case <-sm.stopChan:
			return
		}
	}
}

// Client operations
func (sm *StateManager) CreateClient(clientID, remoteAddr string) error {
	client := &DBClient{
		ID:            clientID,
		Status:        "connected",
		RemoteAddr:    remoteAddr,
		Authenticated: false,
	}

	if err := sm.clientRepo.Create(client); err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	// Increment total connections
	if err := sm.statsRepo.IncrementTotalConnections(); err != nil {
		sm.logger.Error("failed to increment total connections", "error", err)
	}

	return nil
}

func (sm *StateManager) GetClient(clientID string) (*DBClient, error) {
	return sm.clientRepo.GetByID(clientID)
}

func (sm *StateManager) UpdateClientStatus(clientID, status string) error {
	return sm.clientRepo.UpdateStatus(clientID, status)
}

func (sm *StateManager) SetClientAuthenticated(clientID string, authenticated bool) error {
	return sm.clientRepo.UpdateAuthenticated(clientID, authenticated)
}

func (sm *StateManager) AssignClientToGameServer(clientID string, gameServerID int) error {
	return sm.clientRepo.AssignToGameServer(clientID, gameServerID)
}

func (sm *StateManager) UpdateClientActivity(clientID string) error {
	return sm.clientRepo.UpdateActivity(clientID)
}

func (sm *StateManager) RemoveClient(clientID string) error {
	return sm.clientRepo.Delete(clientID)
}

func (sm *StateManager) GetAllClients() ([]*DBClient, error) {
	return sm.clientRepo.GetAll()
}

func (sm *StateManager) GetActiveClients() ([]*DBClient, error) {
	return sm.clientRepo.GetByStatus("proxying")
}

// Game Server operations
func (sm *StateManager) CreateGameServer(host, port, status string, capacity, currentLoad uint16) (*DBGameServer, error) {
	gs := &DBGameServer{
		Host:        host,
		Port:        port,
		Capacity:    capacity,
		CurrentLoad: currentLoad,
		Status:      status,
	}

	if err := sm.gameServerRepo.Create(gs); err != nil {
		return nil, fmt.Errorf("failed to create game server: %w", err)
	}

	return gs, nil
}

func (sm *StateManager) GetGameServer(id int) (*DBGameServer, error) {
	return sm.gameServerRepo.GetByID(id)
}

func (sm *StateManager) GetGameServerByHostPort(host, port string) (*DBGameServer, error) {
	return sm.gameServerRepo.GetByHostPort(host, port)
}

func (sm *StateManager) GetAvailableGameServer() (*DBGameServer, error) {
	return sm.gameServerRepo.GetAvailable()
}

func (sm *StateManager) UpdateGameServerStatus(id int, status string) error {
	return sm.gameServerRepo.UpdateStatus(id, status)
}

func (sm *StateManager) IncrementGameServerLoad(id int) error {
	return sm.gameServerRepo.IncrementLoad(id)
}

func (sm *StateManager) DecrementGameServerLoad(id int) error {
	return sm.gameServerRepo.DecrementLoad(id)
}

func (sm *StateManager) RemoveGameServer(id int) error {
	return sm.gameServerRepo.Delete(id)
}

func (sm *StateManager) GetAllGameServers() ([]*DBGameServer, error) {
	return sm.gameServerRepo.GetAll()
}

func (sm *StateManager) GetActiveGameServers() ([]*DBGameServer, error) {
	servers, err := sm.gameServerRepo.GetAll()
	if err != nil {
		return nil, err
	}

	var activeServers []*DBGameServer
	for _, server := range servers {
		if server.Status == "running" {
			activeServers = append(activeServers, server)
		}
	}

	return activeServers, nil
}

// Proxy Session operations
func (sm *StateManager) CreateProxySession(clientID string, gameServerID int) (*DBProxySession, error) {
	session := &DBProxySession{
		ClientID:     clientID,
		GameServerID: gameServerID,
		Status:       "active",
		BytesUp:      0,
		BytesDown:    0,
		MessagesUp:   0,
		MessagesDown: 0,
	}

	if err := sm.proxySessionRepo.Create(session); err != nil {
		return nil, fmt.Errorf("failed to create proxy session: %w", err)
	}

	return session, nil
}

func (sm *StateManager) GetProxySession(id int) (*DBProxySession, error) {
	return sm.proxySessionRepo.GetByID(id)
}

func (sm *StateManager) GetProxySessionByClient(clientID string) (*DBProxySession, error) {
	return sm.proxySessionRepo.GetByClientID(clientID)
}

func (sm *StateManager) UpdateProxySessionTraffic(id int, bytesUp, bytesDown, messagesUp, messagesDown int64) error {
	return sm.proxySessionRepo.UpdateTraffic(id, bytesUp, bytesDown, messagesUp, messagesDown)
}

func (sm *StateManager) EndProxySession(id int, status string) error {
	return sm.proxySessionRepo.End(id, status)
}

func (sm *StateManager) GetActiveProxySessions() ([]*DBProxySession, error) {
	return sm.proxySessionRepo.GetActive()
}

func (sm *StateManager) GetProxySessionsByGameServer(gameServerID int) ([]*DBProxySession, error) {
	return sm.proxySessionRepo.GetByGameServerID(gameServerID)
}

// Stats operations
func (sm *StateManager) GetStats() (*DBProxyStats, error) {
	return sm.statsRepo.GetStats()
}

func (sm *StateManager) RefreshStats() error {
	return sm.statsRepo.RefreshStats()
}

// State introspection and management for testing
func (sm *StateManager) GetFullState() (*ProxyState, error) {
	clients, err := sm.clientRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to get clients: %w", err)
	}

	gameServers, err := sm.gameServerRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to get game servers: %w", err)
	}

	sessions, err := sm.proxySessionRepo.GetActive()
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}

	stats, err := sm.statsRepo.GetStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return &ProxyState{
		Clients:     clients,
		GameServers: gameServers,
		Sessions:    sessions,
		Stats:       stats,
		Timestamp:   time.Now(),
	}, nil
}

func (sm *StateManager) ClearAllState() error {
	return sm.db.ClearAll()
}

func (sm *StateManager) HealthCheck() error {
	return sm.db.HealthCheck()
}

// ProxyState represents the complete state of the proxy
type ProxyState struct {
	Clients     []*DBClient       `json:"clients"`
	GameServers []*DBGameServer   `json:"game_servers"`
	Sessions    []*DBProxySession `json:"sessions"`
	Stats       *DBProxyStats     `json:"stats"`
	Timestamp   time.Time         `json:"timestamp"`
}
