package db

import (
	"database/sql"
	"fmt"
	"time"
)

type GameServerRepository struct {
	db *Database
}

func NewGameServerRepository(db *Database) *GameServerRepository {
	return &GameServerRepository{db: db}
}

func (r *GameServerRepository) Create(gs *DBGameServer) error {
	query := `INSERT INTO game_servers (host, port, capacity, current_load, status, created_at, updated_at, last_ping)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	result, err := r.db.db.Exec(query, gs.Host, gs.Port, gs.Capacity, gs.CurrentLoad, gs.Status, now, now, now)
	if err != nil {
		return fmt.Errorf("failed to create game server: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	gs.ID = int(id)
	gs.CreatedAt = now
	gs.UpdatedAt = now

	return nil
}

func (r *GameServerRepository) GetByID(id int) (*DBGameServer, error) {
	query := `SELECT id, host, port, capacity, current_load, status, created_at, updated_at, last_ping
              FROM game_servers WHERE id = ?`

	gs := &DBGameServer{}
	err := r.db.db.QueryRow(query, id).Scan(
		&gs.ID, &gs.Host, &gs.Port, &gs.Capacity, &gs.CurrentLoad, &gs.Status,
		&gs.CreatedAt, &gs.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get game server: %w", err)
	}

	return gs, nil
}

func (r *GameServerRepository) GetByHostPort(host, port string) (*DBGameServer, error) {
	query := `SELECT id, host, port, capacity, current_load, status, created_at, updated_at, last_ping
              FROM game_servers WHERE host = ? AND port = ?`

	gs := &DBGameServer{}
	err := r.db.db.QueryRow(query, host, port).Scan(
		&gs.ID, &gs.Host, &gs.Port, &gs.Capacity, &gs.CurrentLoad, &gs.Status,
		&gs.CreatedAt, &gs.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get game server: %w", err)
	}

	return gs, nil
}

func (r *GameServerRepository) GetAvailable() (*DBGameServer, error) {
	query := `SELECT id, host, port, capacity, current_load, status, created_at, updated_at
              FROM game_servers 
              WHERE status = 'running' AND current_load < capacity 
              ORDER BY current_load ASC
              LIMIT 1`

	gs := &DBGameServer{}
	err := r.db.db.QueryRow(query).Scan(
		&gs.ID, &gs.Host, &gs.Port, &gs.Capacity, &gs.CurrentLoad, &gs.Status,
		&gs.CreatedAt, &gs.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get available game server: %w", err)
	}

	return gs, nil
}

func (r *GameServerRepository) GetAll() ([]*DBGameServer, error) {
	query := `SELECT id, host, port, capacity, current_load, status, created_at, updated_at, last_ping
              FROM game_servers ORDER BY created_at DESC`

	rows, err := r.db.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query game servers: %w", err)
	}
	defer rows.Close()

	var servers []*DBGameServer
	for rows.Next() {
		gs := &DBGameServer{}
		err := rows.Scan(
			&gs.ID, &gs.Host, &gs.Port, &gs.Capacity, &gs.CurrentLoad, &gs.Status,
			&gs.CreatedAt, &gs.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game server: %w", err)
		}
		servers = append(servers, gs)
	}

	return servers, nil
}

func (r *GameServerRepository) Update(gs *DBGameServer) error {
	query := `UPDATE game_servers 
              SET host = ?, port = ?, capacity = ?, current_load = ?, status = ?, last_ping = ?
              WHERE id = ?`

	_, err := r.db.db.Exec(query, gs.Host, gs.Port, gs.Capacity, gs.CurrentLoad, gs.Status, gs.ID)
	if err != nil {
		return fmt.Errorf("failed to update game server: %w", err)
	}

	return nil
}

func (r *GameServerRepository) UpdateStatus(id int, status string) error {
	query := `UPDATE game_servers SET status = ?, last_ping = ? WHERE id = ?`
	_, err := r.db.db.Exec(query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update game server status: %w", err)
	}
	return nil
}

func (r *GameServerRepository) UpdateLoad(id int, load uint16) error {
	query := `UPDATE game_servers SET current_load = ?, last_ping = ? WHERE id = ?`
	_, err := r.db.db.Exec(query, load, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update game server load: %w", err)
	}
	return nil
}

func (r *GameServerRepository) IncrementLoad(id int) error {
	query := `UPDATE game_servers SET current_load = current_load + 1, last_ping = ? WHERE id = ?`
	_, err := r.db.db.Exec(query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to increment game server load: %w", err)
	}
	return nil
}

func (r *GameServerRepository) DecrementLoad(id int) error {
	query := `UPDATE game_servers SET current_load = CASE 
                WHEN current_load > 0 THEN current_load - 1 
                ELSE 0 
              END, last_ping = ? WHERE id = ?`
	_, err := r.db.db.Exec(query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to decrement game server load: %w", err)
	}
	return nil
}

func (r *GameServerRepository) Delete(id int) error {
	query := `DELETE FROM game_servers WHERE id = ?`
	_, err := r.db.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete game server: %w", err)
	}
	return nil
}
