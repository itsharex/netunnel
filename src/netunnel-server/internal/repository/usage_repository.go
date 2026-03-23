package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"netunnel/server/internal/domain"
)

type UsageRepository struct {
	db *sql.DB
}

func NewUsageRepository(db *sql.DB) *UsageRepository {
	return &UsageRepository{db: db}
}

func (r *UsageRepository) StartTunnelConnection(ctx context.Context, params domain.TunnelConnectionStart) (string, error) {
	const query = `
insert into tunnel_connections (tunnel_id, agent_id, protocol, source_addr, target_addr)
values ($1, $2, $3, $4, $5)
returning id`

	var connectionID string
	if err := r.db.QueryRowContext(
		ctx,
		query,
		params.TunnelID,
		params.AgentID,
		params.Protocol,
		params.SourceAddr,
		params.TargetAddr,
	).Scan(&connectionID); err != nil {
		return "", fmt.Errorf("start tunnel connection: %w", err)
	}
	return connectionID, nil
}

func (r *UsageRepository) FinishTunnelConnection(ctx context.Context, params domain.TunnelConnectionFinish) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin finish tunnel connection tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	totalBytes := params.IngressBytes + params.EgressBytes

	const updateConnection = `
update tunnel_connections
set ended_at = now(),
    ingress_bytes = $2,
    egress_bytes = $3,
    total_bytes = $4,
    status = $5
where id = $1`

	if _, err = tx.ExecContext(
		ctx,
		updateConnection,
		params.ConnectionID,
		params.IngressBytes,
		params.EgressBytes,
		totalBytes,
		params.Status,
	); err != nil {
		return fmt.Errorf("update tunnel connection: %w", err)
	}

	const upsertUsage = `
insert into traffic_usages (user_id, agent_id, tunnel_id, bucket_time, ingress_bytes, egress_bytes, total_bytes, billed_bytes)
values ($1, $2, $3, date_trunc('hour', now()), $4, $5, $6, 0)
on conflict (tunnel_id, bucket_time) do update
set ingress_bytes = traffic_usages.ingress_bytes + excluded.ingress_bytes,
    egress_bytes = traffic_usages.egress_bytes + excluded.egress_bytes,
    total_bytes = traffic_usages.total_bytes + excluded.total_bytes,
    updated_at = now()`

	if _, err = tx.ExecContext(
		ctx,
		upsertUsage,
		params.UserID,
		params.AgentID,
		params.TunnelID,
		params.IngressBytes,
		params.EgressBytes,
		totalBytes,
	); err != nil {
		return fmt.Errorf("upsert traffic usage: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit finish tunnel connection tx: %w", err)
	}

	log.Printf(
		"usage upsert complete: user=%s tunnel=%s agent=%s ingress=%d egress=%d total=%d status=%s",
		params.UserID,
		params.TunnelID,
		params.AgentID,
		params.IngressBytes,
		params.EgressBytes,
		totalBytes,
		params.Status,
	)
	return nil
}

type ListTunnelConnectionsParams struct {
	UserID   string
	TunnelID string
	Limit    int
}

func (r *UsageRepository) ListTunnelConnections(ctx context.Context, params ListTunnelConnectionsParams) ([]domain.TunnelConnection, error) {
	const query = `
select tc.id, t.user_id, tc.tunnel_id, tc.agent_id, tc.protocol, tc.source_addr, tc.target_addr,
       tc.started_at, tc.ended_at, tc.ingress_bytes, tc.egress_bytes, tc.total_bytes, tc.status
from tunnel_connections tc
join tunnels t on t.id = tc.tunnel_id
where t.user_id = $1 and ($2 = '' or tc.tunnel_id = $2::uuid)
order by tc.started_at desc
limit $3`

	rows, err := r.db.QueryContext(ctx, query, params.UserID, params.TunnelID, params.Limit)
	if err != nil {
		return nil, fmt.Errorf("list tunnel connections: %w", err)
	}
	defer rows.Close()

	connections := make([]domain.TunnelConnection, 0)
	for rows.Next() {
		var item domain.TunnelConnection
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.TunnelID,
			&item.AgentID,
			&item.Protocol,
			&item.SourceAddr,
			&item.TargetAddr,
			&item.StartedAt,
			&item.EndedAt,
			&item.IngressBytes,
			&item.EgressBytes,
			&item.TotalBytes,
			&item.Status,
		); err != nil {
			return nil, err
		}
		connections = append(connections, item)
	}
	return connections, rows.Err()
}

type ListTrafficUsagesParams struct {
	UserID   string
	TunnelID string
	Hours    int
}

func (r *UsageRepository) ListTrafficUsages(ctx context.Context, params ListTrafficUsagesParams) ([]domain.TrafficUsage, error) {
	const query = `
select id, user_id, agent_id, tunnel_id, bucket_time, ingress_bytes, egress_bytes, total_bytes, billed_bytes, created_at, updated_at
from traffic_usages
where user_id = $1
  and ($2 = '' or tunnel_id = $2::uuid)
  and bucket_time >= date_trunc('hour', now()) - make_interval(hours => $3)
order by bucket_time desc, created_at desc`

	rows, err := r.db.QueryContext(ctx, query, params.UserID, params.TunnelID, params.Hours)
	if err != nil {
		return nil, fmt.Errorf("list traffic usages: %w", err)
	}
	defer rows.Close()

	usages := make([]domain.TrafficUsage, 0)
	for rows.Next() {
		var item domain.TrafficUsage
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.AgentID,
			&item.TunnelID,
			&item.BucketTime,
			&item.IngressBytes,
			&item.EgressBytes,
			&item.TotalBytes,
			&item.BilledBytes,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		usages = append(usages, item)
	}
	return usages, rows.Err()
}
