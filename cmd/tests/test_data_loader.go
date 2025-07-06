package tests

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/MarcelBecker1/reverseproxy/internal/db"
)

// JSON test data structure
type TestData struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	GameServers []TestGameServer `json:"game_servers"`
	Clients     []TestClient     `json:"clients"`
	Sessions    []TestSession    `json:"sessions,omitempty"`
}

type TestGameServer struct {
	Host        string `json:"host"`
	Port        string `json:"port"`
	Capacity    uint16 `json:"capacity"`
	CurrentLoad uint16 `json:"current_load"`
	Status      string `json:"status"`
	Load        uint16 `json:"load,omitempty"`
}

type TestClient struct {
	ID            string `json:"id"`
	RemoteAddr    string `json:"remote_addr"`
	Status        string `json:"status"`
	Authenticated bool   `json:"authenticated"`
	GameServerID  *int   `json:"game_server_id,omitempty"`
}

type TestSession struct {
	ClientID     string `json:"client_id"`
	GameServerID int    `json:"game_server_id"`
	Status       string `json:"status"`
	BytesUp      int64  `json:"bytes_up,omitempty"`
	BytesDown    int64  `json:"bytes_down,omitempty"`
	MessagesUp   int64  `json:"messages_up,omitempty"`
	MessagesDown int64  `json:"messages_down,omitempty"`
}

type TestDataLoader struct {
	testDataDir string
}

func NewTestDataLoader(testDataDir string) *TestDataLoader {
	return &TestDataLoader{testDataDir: testDataDir}
}

func (loader *TestDataLoader) LoadTestData(filename string) (*TestData, error) {
	filePath := filepath.Join(loader.testDataDir, filename)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read test data file %s: %w", filePath, err)
	}

	var testData TestData
	if err := json.Unmarshal(data, &testData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal test data: %w", err)
	}

	return &testData, nil
}

func (loader *TestDataLoader) ConvertToDbModels(testData *TestData) ([]*db.DBGameServer, []*db.DBClient, []*db.DBProxySession) {
	var gameServers []*db.DBGameServer
	var clients []*db.DBClient
	var sessions []*db.DBProxySession

	for _, gs := range testData.GameServers {
		gameServers = append(gameServers, &db.DBGameServer{
			Host:        gs.Host,
			Port:        gs.Port,
			Capacity:    gs.Capacity,
			CurrentLoad: gs.CurrentLoad,
			Status:      gs.Status,
		})
	}

	for _, c := range testData.Clients {
		clients = append(clients, &db.DBClient{
			ID:            c.ID,
			RemoteAddr:    c.RemoteAddr,
			Status:        c.Status,
			Authenticated: c.Authenticated,
			GameServerID:  c.GameServerID,
		})
	}

	for _, s := range testData.Sessions {
		sessions = append(sessions, &db.DBProxySession{
			ClientID:     s.ClientID,
			GameServerID: s.GameServerID,
			Status:       s.Status,
			BytesUp:      s.BytesUp,
			BytesDown:    s.BytesDown,
			MessagesUp:   s.MessagesUp,
			MessagesDown: s.MessagesDown,
		})
	}

	return gameServers, clients, sessions
}

// TODO: Extend the generation to handle all properties -> update in state manager

// Creates db for the specified path which we can then use in our simulation tests
func (loader *TestDataLoader) GenerateDbFile(testData *TestData, dbFilePath string) error {
	stateManager, err := db.NewStateManager(dbFilePath, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create state manager: %w", err)
	}
	defer stateManager.Stop()

	if err := stateManager.ClearAllState(); err != nil {
		return fmt.Errorf("failed to clear state: %w", err)
	}

	gameServers, clients, sessions := loader.ConvertToDbModels(testData)

	for _, gs := range gameServers {
		if _, err := stateManager.CreateGameServer(gs.Host, gs.Port, gs.Status, gs.Capacity, gs.CurrentLoad); err != nil {
			return fmt.Errorf("failed to create game server: %w", err)
		}
	}

	for _, client := range clients {
		if err := stateManager.CreateClient(client.ID, client.RemoteAddr); err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
	}

	for _, session := range sessions {
		if _, err := stateManager.CreateProxySession(session.ClientID, session.GameServerID); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	slog.Info("generated seed db file with:",
		"gameservers", len(gameServers),
		"clients", len(clients),
		"sessions", len(sessions),
	)

	return nil
}
