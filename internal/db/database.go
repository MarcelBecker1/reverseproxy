package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// TODO: add retry to logic to other repos

// Executes a database operation with retry logic for SQLITE_BUSY errors
func RetryDBOperation(operation func() error, maxRetries int, retryDelay time.Duration) error {
	var err error
	for i := 0; i <= maxRetries; i++ {
		err = operation()
		if err == nil {
			return nil
		}

		if i < maxRetries && (err.Error() == "database is locked (5) (SQLITE_BUSY)" ||
			err.Error() == "database is locked" ||
			err.Error() == "database is busy") {
			time.Sleep(retryDelay)
			continue
		}

		return err
	}
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS game_servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    host TEXT NOT NULL,
    port TEXT NOT NULL,
    capacity INTEGER NOT NULL,
    current_load INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'starting',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_ping DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(host, port)
);

CREATE TABLE IF NOT EXISTS clients (
    id TEXT PRIMARY KEY,
    game_server_id INTEGER,
    status TEXT NOT NULL DEFAULT 'connected',
    connected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_activity DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    remote_addr TEXT NOT NULL,
    authenticated BOOLEAN NOT NULL DEFAULT 0,
    FOREIGN KEY (game_server_id) REFERENCES game_servers(id)
);

CREATE TABLE IF NOT EXISTS proxy_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id TEXT NOT NULL,
    game_server_id INTEGER NOT NULL,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at DATETIME,
    status TEXT NOT NULL DEFAULT 'active',
    bytes_up INTEGER NOT NULL DEFAULT 0,
    bytes_down INTEGER NOT NULL DEFAULT 0,
    messages_up INTEGER NOT NULL DEFAULT 0,
    messages_down INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (client_id) REFERENCES clients(id),
    FOREIGN KEY (game_server_id) REFERENCES game_servers(id)
);

CREATE TABLE IF NOT EXISTS proxy_stats (
    id INTEGER PRIMARY KEY,
    total_connections INTEGER NOT NULL DEFAULT 0,
    active_connections INTEGER NOT NULL DEFAULT 0,
    active_game_servers INTEGER NOT NULL DEFAULT 0,
    total_game_servers INTEGER NOT NULL DEFAULT 0,
    total_bytes_up INTEGER NOT NULL DEFAULT 0,
    total_bytes_down INTEGER NOT NULL DEFAULT 0,
    total_messages_up INTEGER NOT NULL DEFAULT 0,
    total_messages_down INTEGER NOT NULL DEFAULT 0,
    last_updated DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Insert initial stats row
INSERT OR IGNORE INTO proxy_stats (id) VALUES (1);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_clients_status ON clients(status);
CREATE INDEX IF NOT EXISTS idx_clients_game_server_id ON clients(game_server_id);
CREATE INDEX IF NOT EXISTS idx_game_servers_status ON game_servers(status);
CREATE INDEX IF NOT EXISTS idx_proxy_sessions_client_id ON proxy_sessions(client_id);
CREATE INDEX IF NOT EXISTS idx_proxy_sessions_game_server_id ON proxy_sessions(game_server_id);
CREATE INDEX IF NOT EXISTS idx_proxy_sessions_status ON proxy_sessions(status);

-- Triggers to update timestamps
CREATE TRIGGER IF NOT EXISTS update_game_servers_updated_at
    AFTER UPDATE ON game_servers
    FOR EACH ROW
    BEGIN
        UPDATE game_servers SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
    END;

CREATE TRIGGER IF NOT EXISTS update_clients_updated_at
    AFTER UPDATE ON clients
    FOR EACH ROW
    BEGIN
        UPDATE clients SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
    END;

CREATE TRIGGER IF NOT EXISTS update_proxy_stats_last_updated
    AFTER UPDATE ON proxy_stats
    FOR EACH ROW
    BEGIN
        UPDATE proxy_stats SET last_updated = CURRENT_TIMESTAMP WHERE id = NEW.id;
    END;
`

type Database struct {
	db *sql.DB
}

func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_timeout=30000&_foreign_keys=ON&_busy_timeout=30000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	database := &Database{db: db}

	if err := database.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return database, nil
}

func (d *Database) initSchema() error {
	_, err := d.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}
	return nil
}

func (d *Database) Close() error {
	return d.db.Close()
}

func (d *Database) HealthCheck() error {
	return d.db.Ping()
}

func (d *Database) BeginTx() (*sql.Tx, error) {
	return d.db.Begin()
}

// Useful for testing
func (d *Database) ClearAll() error {
	tx, err := d.BeginTx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tables := []string{"proxy_sessions", "clients", "game_servers"}
	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("failed to clear table %s: %w", table, err)
		}
	}

	if _, err := tx.Exec(`UPDATE proxy_stats SET 
		total_connections = 0,
		active_connections = 0,
		active_game_servers = 0,
		total_game_servers = 0,
		total_bytes_up = 0,
		total_bytes_down = 0,
		total_messages_up = 0,
		total_messages_down = 0,
		last_updated = CURRENT_TIMESTAMP
		WHERE id = 1`); err != nil {
		return fmt.Errorf("failed to reset stats: %w", err)
	}

	return tx.Commit()
}
