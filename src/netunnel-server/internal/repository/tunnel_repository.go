package repository

import (
	"context"
	"database/sql"
	"fmt"

	"netunnel/server/internal/domain"
)

type TunnelRepository struct {
	db *sql.DB
}

func NewTunnelRepository(db *sql.DB) *TunnelRepository {
	return &TunnelRepository{db: db}
}

type CreateTunnelParams struct {
	UserID     string
	AgentID    string
	Name       string
	Type       string
	LocalHost  string
	LocalPort  int
	RemotePort *int
}

type UpdateTunnelParams struct {
	TunnelID  string
	UserID    string
	AgentID   string
	Name      string
	LocalHost string
	LocalPort int
}

func (r *TunnelRepository) CreateTCP(ctx context.Context, params CreateTunnelParams) (*domain.Tunnel, error) {
	const query = `
insert into tunnels (user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port)
values ($1, $2, $3, 'tcp', 'active', true, $4, $5, $6)
returning id, user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port, created_at, updated_at`

	tunnel, err := r.scanTunnelRow(ctx, query, params.UserID, params.AgentID, params.Name, params.LocalHost, params.LocalPort, params.RemotePort)
	if err != nil {
		return nil, fmt.Errorf("create tcp tunnel: %w", err)
	}
	return tunnel, nil
}

func (r *TunnelRepository) CreateHTTPHost(ctx context.Context, params CreateTunnelParams) (*domain.Tunnel, error) {
	const query = `
insert into tunnels (user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port)
values ($1, $2, $3, 'http_host', 'active', true, $4, $5, null)
returning id, user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port, created_at, updated_at`

	tunnel, err := r.scanTunnelRow(ctx, query, params.UserID, params.AgentID, params.Name, params.LocalHost, params.LocalPort)
	if err != nil {
		return nil, fmt.Errorf("create http_host tunnel: %w", err)
	}
	return tunnel, nil
}

func (r *TunnelRepository) ListByUser(ctx context.Context, userID string) ([]domain.Tunnel, error) {
	const query = `
select id, user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port, created_at, updated_at
from tunnels
where user_id = $1
order by created_at desc`

	return r.list(ctx, query, userID)
}

func (r *TunnelRepository) ListByAgent(ctx context.Context, agentID string) ([]domain.Tunnel, error) {
	const query = `
select id, user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port, created_at, updated_at
from tunnels
where agent_id = $1 and enabled = true
order by created_at desc`

	return r.list(ctx, query, agentID)
}

func (r *TunnelRepository) ListActiveTCP(ctx context.Context) ([]domain.Tunnel, error) {
	const query = `
select id, user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port, created_at, updated_at
from tunnels
where enabled = true and type = 'tcp' and remote_port is not null
order by created_at desc`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tunnels []domain.Tunnel
	for rows.Next() {
		var tunnel domain.Tunnel
		if err := rows.Scan(
			&tunnel.ID,
			&tunnel.UserID,
			&tunnel.AgentID,
			&tunnel.Name,
			&tunnel.Type,
			&tunnel.Status,
			&tunnel.Enabled,
			&tunnel.LocalHost,
			&tunnel.LocalPort,
			&tunnel.RemotePort,
			&tunnel.CreatedAt,
			&tunnel.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tunnels = append(tunnels, tunnel)
	}
	return tunnels, rows.Err()
}

func (r *TunnelRepository) GetByIDAndUser(ctx context.Context, tunnelID, userID string) (*domain.Tunnel, error) {
	const query = `
select id, user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port, created_at, updated_at
from tunnels
where id = $1 and user_id = $2`

	return r.scanTunnelRow(ctx, query, tunnelID, userID)
}

func (r *TunnelRepository) UpdateEnabled(ctx context.Context, tunnelID, userID string, enabled bool) (*domain.Tunnel, error) {
	status := "active"

	const query = `
update tunnels
set enabled = $3, status = $4, updated_at = now()
where id = $1 and user_id = $2
returning id, user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port, created_at, updated_at`

	return r.scanTunnelRow(ctx, query, tunnelID, userID, enabled, status)
}

func (r *TunnelRepository) UpdateEnabledAndAgent(ctx context.Context, tunnelID, userID string, enabled bool, agentID string) (*domain.Tunnel, error) {
	status := "active"

	const query = `
update tunnels
set enabled = $3,
    status = $4,
    agent_id = case when $5 <> '' then $5 else agent_id end,
    updated_at = now()
where id = $1 and user_id = $2
returning id, user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port, created_at, updated_at`

	return r.scanTunnelRow(ctx, query, tunnelID, userID, enabled, status, agentID)
}

func (r *TunnelRepository) Update(ctx context.Context, params UpdateTunnelParams) (*domain.Tunnel, error) {
	const query = `
update tunnels
set agent_id = $3,
    name = $4,
    local_host = $5,
    local_port = $6,
    status = case when status = 'disabled_billing' then status else 'active' end,
    updated_at = now()
where id = $1 and user_id = $2
returning id, user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port, created_at, updated_at`

	return r.scanTunnelRow(ctx, query, params.TunnelID, params.UserID, params.AgentID, params.Name, params.LocalHost, params.LocalPort)
}

func (r *TunnelRepository) ListEnabledByUser(ctx context.Context, userID string) ([]domain.Tunnel, error) {
	const query = `
select id, user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port, created_at, updated_at
from tunnels
where user_id = $1 and enabled = true
order by created_at desc`

	return r.list(ctx, query, userID)
}

func (r *TunnelRepository) ListDisabledByUserAndStatus(ctx context.Context, userID, status string) ([]domain.Tunnel, error) {
	const query = `
select id, user_id, agent_id, name, type, status, enabled, local_host, local_port, remote_port, created_at, updated_at
from tunnels
where user_id = $1 and enabled = false and status = $2
order by created_at desc`

	rows, err := r.db.QueryContext(ctx, query, userID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tunnels := make([]domain.Tunnel, 0)
	for rows.Next() {
		var tunnel domain.Tunnel
		if err := rows.Scan(
			&tunnel.ID,
			&tunnel.UserID,
			&tunnel.AgentID,
			&tunnel.Name,
			&tunnel.Type,
			&tunnel.Status,
			&tunnel.Enabled,
			&tunnel.LocalHost,
			&tunnel.LocalPort,
			&tunnel.RemotePort,
			&tunnel.CreatedAt,
			&tunnel.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tunnels = append(tunnels, tunnel)
	}
	return tunnels, rows.Err()
}

func (r *TunnelRepository) DisableAllByUser(ctx context.Context, userID string) error {
	const query = `
update tunnels
set enabled = false, status = 'disabled_billing', updated_at = now()
where user_id = $1 and enabled = true`

	if _, err := r.db.ExecContext(ctx, query, userID); err != nil {
		return fmt.Errorf("disable user tunnels: %w", err)
	}
	return nil
}

func (r *TunnelRepository) RestoreBillingDisabledByUser(ctx context.Context, userID string) error {
	const query = `
update tunnels
set enabled = true, status = 'active', updated_at = now()
where user_id = $1 and enabled = false and status = 'disabled_billing'`

	if _, err := r.db.ExecContext(ctx, query, userID); err != nil {
		return fmt.Errorf("restore billing disabled tunnels: %w", err)
	}
	return nil
}

func (r *TunnelRepository) DeleteByIDAndUser(ctx context.Context, tunnelID, userID string) error {
	const query = `
delete from tunnels
where id = $1 and user_id = $2`

	result, err := r.db.ExecContext(ctx, query, tunnelID, userID)
	if err != nil {
		return fmt.Errorf("delete tunnel: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete tunnel rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *TunnelRepository) list(ctx context.Context, query string, arg string) ([]domain.Tunnel, error) {
	rows, err := r.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tunnels := make([]domain.Tunnel, 0)
	for rows.Next() {
		var tunnel domain.Tunnel
		if err := rows.Scan(
			&tunnel.ID,
			&tunnel.UserID,
			&tunnel.AgentID,
			&tunnel.Name,
			&tunnel.Type,
			&tunnel.Status,
			&tunnel.Enabled,
			&tunnel.LocalHost,
			&tunnel.LocalPort,
			&tunnel.RemotePort,
			&tunnel.CreatedAt,
			&tunnel.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tunnels = append(tunnels, tunnel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tunnels, nil
}

func (r *TunnelRepository) scanTunnelRow(ctx context.Context, query string, args ...any) (*domain.Tunnel, error) {
	var tunnel domain.Tunnel
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&tunnel.ID,
		&tunnel.UserID,
		&tunnel.AgentID,
		&tunnel.Name,
		&tunnel.Type,
		&tunnel.Status,
		&tunnel.Enabled,
		&tunnel.LocalHost,
		&tunnel.LocalPort,
		&tunnel.RemotePort,
		&tunnel.CreatedAt,
		&tunnel.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &tunnel, nil
}
