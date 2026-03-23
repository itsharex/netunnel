package repository

import (
	"context"
	"database/sql"
)

type TunnelRuntimeRepository struct {
	db *sql.DB
}

func NewTunnelRuntimeRepository(db *sql.DB) *TunnelRuntimeRepository {
	return &TunnelRuntimeRepository{db: db}
}

func (r *TunnelRuntimeRepository) ValidateAgentTunnel(ctx context.Context, tunnelID, secretKey string) (bool, error) {
	const query = `
select exists (
    select 1
    from tunnels t
    join agents a on a.id = t.agent_id
    where t.id = $1 and a.secret_key = $2 and t.enabled = true and t.type in ('tcp', 'http_host')
)`

	var ok bool
	err := r.db.QueryRowContext(ctx, query, tunnelID, secretKey).Scan(&ok)
	return ok, err
}
