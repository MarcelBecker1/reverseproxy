package db

import (
	"time"
)

// TODO: generally update models and repos, remove some unnecessary fields
// TODO: update my entities to support different statuses

type DBGameServer struct {
	ID          int       `json:"id"`
	Host        string    `json:"host"`
	Port        string    `json:"port"`
	Capacity    uint16    `json:"capacity"`
	CurrentLoad uint16    `json:"current_load"`
	Status      string    `json:"status"` // TODO: would be nice to have "running", "stopped", "error"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DBClient struct {
	ID            string    `json:"id"`
	GameServerID  *int      `json:"game_server_id,omitempty"`
	Status        string    `json:"status"` // TODO: would be nice to have "connected", "authenticated", "proxying", "disconnected", currently we have auth errors
	ConnectedAt   time.Time `json:"connected_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	RemoteAddr    string    `json:"remote_addr"`
	Authenticated bool      `json:"authenticated"`
}

type DBProxySession struct {
	ID           int        `json:"id"`
	ClientID     string     `json:"client_id"`
	GameServerID int        `json:"game_server_id"`
	StartedAt    time.Time  `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	Status       string     `json:"status"` // TODO: would be nice to have "active", "ended", "error"
	BytesUp      int64      `json:"bytes_up"`
	BytesDown    int64      `json:"bytes_down"`
	MessagesUp   int64      `json:"messages_up"`
	MessagesDown int64      `json:"messages_down"`
}

type DBProxyStats struct {
	TotalConnections  int64     `json:"total_connections"`
	ActiveConnections int       `json:"active_connections"`
	ActiveGameServers int       `json:"active_game_servers"`
	TotalGameServers  int       `json:"total_game_servers"`
	TotalBytesUp      int64     `json:"total_bytes_up"`
	TotalBytesDown    int64     `json:"total_bytes_down"`
	TotalMessagesUp   int64     `json:"total_messages_up"`
	TotalMessagesDown int64     `json:"total_messages_down"`
	UpdatedAt         time.Time `json:"updated_at"`
}
