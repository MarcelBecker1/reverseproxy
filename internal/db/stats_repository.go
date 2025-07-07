package db

import (
	"fmt"
)

type StatsRepository struct {
	db *Database
}

func NewStatsRepository(db *Database) *StatsRepository {
	return &StatsRepository{db: db}
}

func (r *StatsRepository) GetStats() (*DBProxyStats, error) {
	query := `SELECT total_connections, active_connections, active_game_servers, total_game_servers,
              total_bytes_up, total_bytes_down, total_messages_up, total_messages_down, last_updated
              FROM proxy_stats WHERE id = 1`

	stats := &DBProxyStats{}
	err := r.db.db.QueryRow(query).Scan(
		&stats.TotalConnections, &stats.ActiveConnections, &stats.ActiveGameServers, &stats.TotalGameServers,
		&stats.TotalBytesUp, &stats.TotalBytesDown, &stats.TotalMessagesUp, &stats.TotalMessagesDown,
		&stats.LastUpdated,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get proxy stats: %w", err)
	}

	return stats, nil
}

func (r *StatsRepository) UpdateStats(stats *DBProxyStats) error {
	query := `UPDATE proxy_stats 
              SET total_connections = ?, active_connections = ?, active_game_servers = ?, total_game_servers = ?,
                  total_bytes_up = ?, total_bytes_down = ?, total_messages_up = ?, total_messages_down = ?
              WHERE id = 1`

	_, err := r.db.db.Exec(query, stats.TotalConnections, stats.ActiveConnections, stats.ActiveGameServers,
		stats.TotalGameServers, stats.TotalBytesUp, stats.TotalBytesDown, stats.TotalMessagesUp, stats.TotalMessagesDown)

	if err != nil {
		return fmt.Errorf("failed to update proxy stats: %w", err)
	}

	return nil
}

func (r *StatsRepository) IncrementTotalConnections() error {
	query := `UPDATE proxy_stats SET total_connections = total_connections + 1 WHERE id = 1`
	_, err := r.db.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to increment total connections: %w", err)
	}
	return nil
}

func (r *StatsRepository) UpdateActiveConnections(count int) error {
	query := `UPDATE proxy_stats SET active_connections = ? WHERE id = 1`
	_, err := r.db.db.Exec(query, count)
	if err != nil {
		return fmt.Errorf("failed to update active connections: %w", err)
	}
	return nil
}

func (r *StatsRepository) UpdateActiveGameServers(count int) error {
	query := `UPDATE proxy_stats SET active_game_servers = ? WHERE id = 1`
	_, err := r.db.db.Exec(query, count)
	if err != nil {
		return fmt.Errorf("failed to update active game servers: %w", err)
	}
	return nil
}

func (r *StatsRepository) UpdateTotalGameServers(count int) error {
	query := `UPDATE proxy_stats SET total_game_servers = ? WHERE id = 1`
	_, err := r.db.db.Exec(query, count)
	if err != nil {
		return fmt.Errorf("failed to update total game servers: %w", err)
	}
	return nil
}

func (r *StatsRepository) UpdateTrafficStats(bytesUp, bytesDown, messagesUp, messagesDown int64) error {
	query := `UPDATE proxy_stats 
              SET total_bytes_up = ?, total_bytes_down = ?, total_messages_up = ?, total_messages_down = ?
              WHERE id = 1`

	_, err := r.db.db.Exec(query, bytesUp, bytesDown, messagesUp, messagesDown)
	if err != nil {
		return fmt.Errorf("failed to update traffic stats: %w", err)
	}
	return nil
}

func (r *StatsRepository) RefreshStats() error {
	activeConnections, err := r.getActiveConnectionsCount()
	if err != nil {
		return fmt.Errorf("failed to get active connections count: %w", err)
	}

	activeGameServers, err := r.getActiveGameServersCount()
	if err != nil {
		return fmt.Errorf("failed to get active game servers count: %w", err)
	}

	totalGameServers, err := r.getTotalGameServersCount()
	if err != nil {
		return fmt.Errorf("failed to get total game servers count: %w", err)
	}

	bytesUp, bytesDown, messagesUp, messagesDown, err := r.getTrafficStats()
	if err != nil {
		return fmt.Errorf("failed to get traffic stats: %w", err)
	}

	query := `UPDATE proxy_stats 
              SET active_connections = ?, active_game_servers = ?, total_game_servers = ?,
                  total_bytes_up = ?, total_bytes_down = ?, total_messages_up = ?, total_messages_down = ?
              WHERE id = 1`

	_, err = r.db.db.Exec(query, activeConnections, activeGameServers, totalGameServers,
		bytesUp, bytesDown, messagesUp, messagesDown)

	if err != nil {
		return fmt.Errorf("failed to refresh stats: %w", err)
	}

	return nil
}

func (r *StatsRepository) getActiveConnectionsCount() (int, error) {
	query := `SELECT COUNT(*) FROM clients WHERE status IN ('connected', 'authenticated', 'proxying')`
	var count int
	err := r.db.db.QueryRow(query).Scan(&count)
	return count, err
}

func (r *StatsRepository) getActiveGameServersCount() (int, error) {
	query := `SELECT COUNT(*) FROM game_servers WHERE status = 'running'`
	var count int
	err := r.db.db.QueryRow(query).Scan(&count)
	return count, err
}

func (r *StatsRepository) getTotalGameServersCount() (int, error) {
	query := `SELECT COUNT(*) FROM game_servers`
	var count int
	err := r.db.db.QueryRow(query).Scan(&count)
	return count, err
}

func (r *StatsRepository) getTrafficStats() (int64, int64, int64, int64, error) {
	query := `SELECT 
                COALESCE(SUM(bytes_up), 0) as total_bytes_up,
                COALESCE(SUM(bytes_down), 0) as total_bytes_down,
                COALESCE(SUM(messages_up), 0) as total_messages_up,
                COALESCE(SUM(messages_down), 0) as total_messages_down
              FROM proxy_sessions`

	var totalBytesUp, totalBytesDown, totalMessagesUp, totalMessagesDown int64
	err := r.db.db.QueryRow(query).Scan(&totalBytesUp, &totalBytesDown, &totalMessagesUp, &totalMessagesDown)
	return totalBytesUp, totalBytesDown, totalMessagesUp, totalMessagesDown, err
}
