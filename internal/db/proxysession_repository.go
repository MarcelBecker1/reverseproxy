package db

import (
	"database/sql"
	"fmt"
	"time"
)

type ProxySessionRepository struct {
	db *Database
}

func NewProxySessionRepository(db *Database) *ProxySessionRepository {
	return &ProxySessionRepository{db: db}
}

func (r *ProxySessionRepository) Create(session *DBProxySession) error {
	query := `INSERT INTO proxy_sessions (client_id, game_server_id, started_at, status, bytes_up, bytes_down, messages_up, messages_down)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	result, err := r.db.db.Exec(query, session.ClientID, session.GameServerID, now, session.Status,
		session.BytesUp, session.BytesDown, session.MessagesUp, session.MessagesDown)
	if err != nil {
		return fmt.Errorf("failed to create proxy session: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	session.ID = int(id)
	session.StartedAt = now

	return nil
}

func (r *ProxySessionRepository) GetByID(id int) (*DBProxySession, error) {
	query := `SELECT id, client_id, game_server_id, started_at, ended_at, status, bytes_up, bytes_down, messages_up, messages_down
              FROM proxy_sessions WHERE id = ?`

	session := &DBProxySession{}
	err := r.db.db.QueryRow(query, id).Scan(
		&session.ID, &session.ClientID, &session.GameServerID, &session.StartedAt,
		&session.EndedAt, &session.Status, &session.BytesUp, &session.BytesDown,
		&session.MessagesUp, &session.MessagesDown,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get proxy session: %w", err)
	}

	return session, nil
}

func (r *ProxySessionRepository) GetByClientID(clientID string) (*DBProxySession, error) {
	query := `SELECT id, client_id, game_server_id, started_at, ended_at, status, bytes_up, bytes_down, messages_up, messages_down
              FROM proxy_sessions WHERE client_id = ? AND status = 'active' ORDER BY started_at DESC LIMIT 1`

	session := &DBProxySession{}
	err := r.db.db.QueryRow(query, clientID).Scan(
		&session.ID, &session.ClientID, &session.GameServerID, &session.StartedAt,
		&session.EndedAt, &session.Status, &session.BytesUp, &session.BytesDown,
		&session.MessagesUp, &session.MessagesDown,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get proxy session by client: %w", err)
	}

	return session, nil
}

func (r *ProxySessionRepository) GetByGameServerID(gameServerID int) ([]*DBProxySession, error) {
	query := `SELECT id, client_id, game_server_id, started_at, ended_at, status, bytes_up, bytes_down, messages_up, messages_down
              FROM proxy_sessions WHERE game_server_id = ? ORDER BY started_at DESC`

	rows, err := r.db.db.Query(query, gameServerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query proxy sessions by game server: %w", err)
	}
	defer rows.Close()

	var sessions []*DBProxySession
	for rows.Next() {
		session := &DBProxySession{}
		err := rows.Scan(
			&session.ID, &session.ClientID, &session.GameServerID, &session.StartedAt,
			&session.EndedAt, &session.Status, &session.BytesUp, &session.BytesDown,
			&session.MessagesUp, &session.MessagesDown,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan proxy session: %w", err)
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (r *ProxySessionRepository) GetActive() ([]*DBProxySession, error) {
	query := `SELECT id, client_id, game_server_id, started_at, ended_at, status, bytes_up, bytes_down, messages_up, messages_down
              FROM proxy_sessions WHERE status = 'active' ORDER BY started_at DESC`

	rows, err := r.db.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active proxy sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*DBProxySession
	for rows.Next() {
		session := &DBProxySession{}
		err := rows.Scan(
			&session.ID, &session.ClientID, &session.GameServerID, &session.StartedAt,
			&session.EndedAt, &session.Status, &session.BytesUp, &session.BytesDown,
			&session.MessagesUp, &session.MessagesDown,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan proxy session: %w", err)
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (r *ProxySessionRepository) Update(session *DBProxySession) error {
	query := `UPDATE proxy_sessions 
              SET status = ?, bytes_up = ?, bytes_down = ?, messages_up = ?, messages_down = ?
              WHERE id = ?`

	_, err := r.db.db.Exec(query, session.Status, session.BytesUp, session.BytesDown,
		session.MessagesUp, session.MessagesDown, session.ID)
	if err != nil {
		return fmt.Errorf("failed to update proxy session: %w", err)
	}

	return nil
}

func (r *ProxySessionRepository) End(id int, status string) error {
	query := `UPDATE proxy_sessions SET status = ?, ended_at = ? WHERE id = ?`
	now := time.Now()
	_, err := r.db.db.Exec(query, status, now, id)
	if err != nil {
		return fmt.Errorf("failed to end proxy session: %w", err)
	}
	return nil
}

func (r *ProxySessionRepository) UpdateTraffic(id int, bytesUp, bytesDown, messagesUp, messagesDown int64) error {
	query := `UPDATE proxy_sessions 
              SET bytes_up = bytes_up + ?, bytes_down = bytes_down + ?, messages_up = messages_up + ?, messages_down = messages_down + ?
              WHERE id = ?`

	_, err := r.db.db.Exec(query, bytesUp, bytesDown, messagesUp, messagesDown, id)
	if err != nil {
		return fmt.Errorf("failed to update proxy session traffic: %w", err)
	}

	return nil
}

func (r *ProxySessionRepository) Delete(id int) error {
	query := `DELETE FROM proxy_sessions WHERE id = ?`
	_, err := r.db.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete proxy session: %w", err)
	}
	return nil
}

func (r *ProxySessionRepository) CountActive() (int, error) {
	query := `SELECT COUNT(*) FROM proxy_sessions WHERE status = 'active'`
	var count int
	err := r.db.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active proxy sessions: %w", err)
	}
	return count, nil
}

func (r *ProxySessionRepository) GetTrafficStats() (int64, int64, int64, int64, error) {
	query := `SELECT 
                COALESCE(SUM(bytes_up), 0) as total_bytes_up,
                COALESCE(SUM(bytes_down), 0) as total_bytes_down,
                COALESCE(SUM(messages_up), 0) as total_messages_up,
                COALESCE(SUM(messages_down), 0) as total_messages_down
              FROM proxy_sessions`

	var totalBytesUp, totalBytesDown, totalMessagesUp, totalMessagesDown int64
	err := r.db.db.QueryRow(query).Scan(&totalBytesUp, &totalBytesDown, &totalMessagesUp, &totalMessagesDown)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to get traffic stats: %w", err)
	}

	return totalBytesUp, totalBytesDown, totalMessagesUp, totalMessagesDown, nil
}
