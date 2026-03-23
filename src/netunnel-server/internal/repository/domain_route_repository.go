package repository

import (
	"context"
	"database/sql"
	"fmt"

	"netunnel/server/internal/domain"
)

type DomainRouteRepository struct {
	db *sql.DB
}

func NewDomainRouteRepository(db *sql.DB) *DomainRouteRepository {
	return &DomainRouteRepository{db: db}
}

type CreateDomainRouteParams struct {
	TunnelID string
	Domain   string
	Scheme   string
}

type UpdateDomainRouteParams struct {
	RouteID string
	UserID  string
	Domain  string
	Scheme  string
}

type HTTPRouteTarget struct {
	Tunnel domain.Tunnel
	Route  domain.DomainRoute
}

func (r *DomainRouteRepository) Create(ctx context.Context, params CreateDomainRouteParams) (*domain.DomainRoute, error) {
	const query = `
insert into domain_routes (tunnel_id, domain, scheme)
values ($1, $2, $3)
returning id, tunnel_id, domain, scheme, created_at, updated_at`

	route, err := r.scanDomainRouteRow(ctx, query, params.TunnelID, params.Domain, params.Scheme)
	if err != nil {
		return nil, fmt.Errorf("create domain route: %w", err)
	}
	return route, nil
}

func (r *DomainRouteRepository) ListByTunnelIDs(ctx context.Context, tunnelIDs []string) (map[string][]domain.DomainRoute, error) {
	result := make(map[string][]domain.DomainRoute)
	if len(tunnelIDs) == 0 {
		return result, nil
	}

	query := `
select id, tunnel_id, domain, scheme, created_at, updated_at
from domain_routes
where tunnel_id = any($1)
order by created_at asc`

	rows, err := r.db.QueryContext(ctx, query, tunnelIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var route domain.DomainRoute
		if err := rows.Scan(
			&route.ID,
			&route.TunnelID,
			&route.Domain,
			&route.Scheme,
			&route.CreatedAt,
			&route.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result[route.TunnelID] = append(result[route.TunnelID], route)
	}
	return result, rows.Err()
}

func (r *DomainRouteRepository) ListByTunnel(ctx context.Context, tunnelID string) ([]domain.DomainRoute, error) {
	const query = `
select id, tunnel_id, domain, scheme, created_at, updated_at
from domain_routes
where tunnel_id = $1
order by created_at asc`

	rows, err := r.db.QueryContext(ctx, query, tunnelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	routes := make([]domain.DomainRoute, 0)
	for rows.Next() {
		var route domain.DomainRoute
		if err := rows.Scan(
			&route.ID,
			&route.TunnelID,
			&route.Domain,
			&route.Scheme,
			&route.CreatedAt,
			&route.UpdatedAt,
		); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

func (r *DomainRouteRepository) DeleteByIDAndUser(ctx context.Context, routeID, userID string) error {
	const query = `
delete from domain_routes dr
using tunnels t
where dr.id = $1 and dr.tunnel_id = t.id and t.user_id = $2`

	result, err := r.db.ExecContext(ctx, query, routeID, userID)
	if err != nil {
		return fmt.Errorf("delete domain route: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete domain route rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *DomainRouteRepository) UpdateByIDAndUser(ctx context.Context, params UpdateDomainRouteParams) (*domain.DomainRoute, error) {
	const query = `
update domain_routes dr
set domain = $3,
    scheme = $4,
    updated_at = now()
from tunnels t
where dr.id = $1 and t.id = dr.tunnel_id and t.user_id = $2
returning dr.id, dr.tunnel_id, dr.domain, dr.scheme, dr.created_at, dr.updated_at`

	return r.scanDomainRouteRow(ctx, query, params.RouteID, params.UserID, params.Domain, params.Scheme)
}

func (r *DomainRouteRepository) FindHTTPRouteByDomain(ctx context.Context, host, scheme string) (*HTTPRouteTarget, error) {
	const query = `
select
	dr.id,
	dr.tunnel_id,
	dr.domain,
	dr.scheme,
	dr.created_at,
	dr.updated_at,
	t.id,
	t.user_id,
	t.agent_id,
	t.name,
	t.type,
	t.status,
	t.enabled,
	t.local_host,
	t.local_port,
	t.remote_port,
	t.created_at,
	t.updated_at
from domain_routes dr
join tunnels t on t.id = dr.tunnel_id
where lower(dr.domain) = lower($1)
  and ($2 = '' or dr.scheme = $2)
  and t.enabled = true
  and t.type = 'http_host'
limit 1`

	var target HTTPRouteTarget
	err := r.db.QueryRowContext(ctx, query, host, scheme).Scan(
		&target.Route.ID,
		&target.Route.TunnelID,
		&target.Route.Domain,
		&target.Route.Scheme,
		&target.Route.CreatedAt,
		&target.Route.UpdatedAt,
		&target.Tunnel.ID,
		&target.Tunnel.UserID,
		&target.Tunnel.AgentID,
		&target.Tunnel.Name,
		&target.Tunnel.Type,
		&target.Tunnel.Status,
		&target.Tunnel.Enabled,
		&target.Tunnel.LocalHost,
		&target.Tunnel.LocalPort,
		&target.Tunnel.RemotePort,
		&target.Tunnel.CreatedAt,
		&target.Tunnel.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func (r *DomainRouteRepository) scanDomainRouteRow(ctx context.Context, query string, args ...any) (*domain.DomainRoute, error) {
	var route domain.DomainRoute
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&route.ID,
		&route.TunnelID,
		&route.Domain,
		&route.Scheme,
		&route.CreatedAt,
		&route.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &route, nil
}
