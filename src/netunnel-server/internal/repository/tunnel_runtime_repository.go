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

func (r *TunnelRuntimeRepository) ValidateAgentSession(ctx context.Context, agentID, secretKey string) (bool, error) {
	const query = `
select exists (
    select 1
    from agents a
    where a.id = $1 and a.secret_key = $2
)`

	var ok bool
	err := r.db.QueryRowContext(ctx, query, agentID, secretKey).Scan(&ok)
	return ok, err
}
