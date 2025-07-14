package db

import (
	"fmt"
	"time"
)

type ClientRepository struct {
	db *Database
}

func NewClientRepository(db *Database) *ClientRepository {
	return &ClientRepository{db: db}
}

func (r *ClientRepository) Create(client *DBClient) error {
	query := `INSERT INTO clients (id, game_server_id, status, connected_at, updated_at, remote_addr, authenticated)
              VALUES (?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()

	err := RetryDBOperation(func() error {
		_, err := r.db.db.Exec(query, client.ID, client.GameServerID, client.Status, now, now, client.RemoteAddr, client.Authenticated)
		return err
	}, 3, 50*time.Millisecond)

	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	client.ConnectedAt = now
	client.UpdatedAt = now

	return nil
}

func (r *ClientRepository) GetByStatus(status string) ([]*DBClient, error) {
	query := `SELECT id, game_server_id, status, connected_at, updated_at, remote_addr, authenticated
              FROM clients WHERE status = ?`

	rows, err := r.db.db.Query(query, status)
	if err != nil {
		return nil, fmt.Errorf("failed to query clients by status: %w", err)
	}
	defer rows.Close()

	var clients []*DBClient
	for rows.Next() {
		client := &DBClient{}
		err := rows.Scan(
			&client.ID, &client.GameServerID, &client.Status, &client.ConnectedAt,
			&client.UpdatedAt, &client.RemoteAddr, &client.Authenticated,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan client: %w", err)
		}
		clients = append(clients, client)
	}

	return clients, nil
}

func (r *ClientRepository) GetAll() ([]*DBClient, error) {
	query := `SELECT id, game_server_id, status, connected_at, updated_at, remote_addr, authenticated
              FROM clients ORDER BY connected_at DESC`

	rows, err := r.db.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all clients: %w", err)
	}
	defer rows.Close()

	var clients []*DBClient
	for rows.Next() {
		client := &DBClient{}
		err := rows.Scan(
			&client.ID, &client.GameServerID, &client.Status, &client.ConnectedAt,
			&client.UpdatedAt, &client.RemoteAddr, &client.Authenticated,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan client: %w", err)
		}
		clients = append(clients, client)
	}

	return clients, nil
}

func (r *ClientRepository) Update(client *DBClient) error {
	query := `UPDATE clients 
              SET game_server_id = ?, status = ?, updated_at = ?, authenticated = ?
              WHERE id = ?`

	client.UpdatedAt = time.Now()
	_, err := r.db.db.Exec(query, client.GameServerID, client.Status, client.UpdatedAt, client.Authenticated, client.ID)
	if err != nil {
		return fmt.Errorf("failed to update client: %w", err)
	}

	return nil
}

func (r *ClientRepository) UpdateStatus(id string, status string) error {
	query := `UPDATE clients SET status = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.db.Exec(query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update client status: %w", err)
	}
	return nil
}

func (r *ClientRepository) UpdateAuthenticated(id string, authenticated bool) error {
	query := `UPDATE clients SET authenticated = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.db.Exec(query, authenticated, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update client authentication: %w", err)
	}
	return nil
}

func (r *ClientRepository) AssignToGameServer(clientID string, gameServerID int) error {
	query := `UPDATE clients SET game_server_id = ?, status = 'proxying', updated_at = ? WHERE id = ?`
	_, err := r.db.db.Exec(query, gameServerID, time.Now(), clientID)
	if err != nil {
		return fmt.Errorf("failed to assign client to game server: %w", err)
	}
	return nil
}

func (r *ClientRepository) UpdateActivity(id string) error {
	query := `UPDATE clients SET updated_at = ? WHERE id = ?`
	_, err := r.db.db.Exec(query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update client activity: %w", err)
	}
	return nil
}

func (r *ClientRepository) Delete(id string) error {
	query := `DELETE FROM clients WHERE id = ?`

	err := RetryDBOperation(func() error {
		_, err := r.db.db.Exec(query, id)
		return err
	}, 3, 50*time.Millisecond)

	if err != nil {
		return fmt.Errorf("failed to delete client: %w", err)
	}
	return nil
}

func (r *ClientRepository) CountByGameServer(gameServerID int) (int, error) {
	query := `SELECT COUNT(*) FROM clients WHERE game_server_id = ? AND status IN ('authenticated', 'proxying')`
	var count int
	err := r.db.db.QueryRow(query, gameServerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count clients by game server: %w", err)
	}
	return count, nil
}
