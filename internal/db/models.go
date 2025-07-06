package db

import (
	"time"
)

// TODO: update my entities to support different statuses

type DBGameServer struct {
	ID          int       `json:"id"`
	Host        string    `json:"host"`
	Port        string    `json:"port"`
	Capacity    uint16    `json:"capacity"`
	CurrentLoad uint16    `json:"current_load"`
	Status      string    `json:"status"` // "running", "stopped", "error"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastPing    time.Time `json:"last_ping"`
}

// DBClient represents a client connection in the database
type DBClient struct {
	ID            string    `json:"id"`
	GameServerID  *int      `json:"game_server_id,omitempty"`
	Status        string    `json:"status"` // "connected", "authenticated", "proxying", "disconnected"
	ConnectedAt   time.Time `json:"connected_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	LastActivity  time.Time `json:"last_activity"`
	RemoteAddr    string    `json:"remote_addr"`
	Authenticated bool      `json:"authenticated"`
}

// DBProxySession represents an active proxy session
type DBProxySession struct {
	ID           int        `json:"id"`
	ClientID     string     `json:"client_id"`
	GameServerID int        `json:"game_server_id"`
	StartedAt    time.Time  `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	Status       string     `json:"status"` // "active", "ended", "error"
	BytesUp      int64      `json:"bytes_up"`
	BytesDown    int64      `json:"bytes_down"`
	MessagesUp   int64      `json:"messages_up"`
	MessagesDown int64      `json:"messages_down"`
}

// DBProxyStats represents overall proxy statistics
type DBProxyStats struct {
	TotalConnections  int64     `json:"total_connections"`
	ActiveConnections int       `json:"active_connections"`
	ActiveGameServers int       `json:"active_game_servers"`
	TotalGameServers  int       `json:"total_game_servers"`
	TotalBytesUp      int64     `json:"total_bytes_up"`
	TotalBytesDown    int64     `json:"total_bytes_down"`
	TotalMessagesUp   int64     `json:"total_messages_up"`
	TotalMessagesDown int64     `json:"total_messages_down"`
	LastUpdated       time.Time `json:"last_updated"`
}
