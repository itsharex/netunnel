package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"netunnel/server/internal/domain"
)

type AgentRepository struct {
	db *sql.DB
}

func NewAgentRepository(db *sql.DB) *AgentRepository {
	return &AgentRepository{db: db}
}

type RegisterAgentParams struct {
	UserID        string
	Name          string
	MachineCode   string
	SecretKey     string
	ClientVersion string
	OSType        string
}

type HeartbeatAgentParams struct {
	AgentID       string
	SecretKey     string
	Status        string
	ClientVersion string
	OSType        string
}

func (r *AgentRepository) Register(ctx context.Context, params RegisterAgentParams) (*domain.Agent, bool, error) {
	existingAgent, err := r.findByUserAndMachine(ctx, params.UserID, params.MachineCode)
	if err == nil {
		return existingAgent, false, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return nil, false, fmt.Errorf("find existing agent before insert: %w", err)
	}

	const insertQuery = `
with payload as (
	select
		$1::uuid as user_id,
		$2::text as name,
		$3::text as machine_code,
		$4::text as secret_key,
		$5::text as client_version,
		$6::text as os_type
)
insert into agents (user_id, name, machine_code, secret_key, status, client_version, os_type, last_heartbeat_at)
select payload.user_id, payload.name, payload.machine_code, payload.secret_key, 'online', payload.client_version, payload.os_type, now()
from payload
returning id, user_id, name, machine_code, secret_key, status, client_version, os_type, last_heartbeat_at, created_at, updated_at`

	agent, err := r.scanAgentRow(ctx, insertQuery, params.UserID, params.Name, params.MachineCode, params.SecretKey, params.ClientVersion, params.OSType)
	if err == nil {
		return agent, true, nil
	}
	if !isUniqueViolation(err) {
		return nil, false, fmt.Errorf("insert agent: %w", err)
	}

	agent, err = r.findByUserAndMachine(ctx, params.UserID, params.MachineCode)
	if err != nil {
		return nil, false, fmt.Errorf("load existing agent: %w", err)
	}
	return agent, false, nil
}

func (r *AgentRepository) Heartbeat(ctx context.Context, params HeartbeatAgentParams) (*domain.Agent, error) {
	const query = `
with payload as (
	select
		$1::uuid as id,
		$2::text as secret_key,
		$3::text as status,
		nullif($4::text, '') as client_version,
		nullif($5::text, '') as os_type
)
update agents
set status = payload.status,
    client_version = coalesce(payload.client_version, agents.client_version),
    os_type = coalesce(payload.os_type, agents.os_type),
    last_heartbeat_at = now(),
    updated_at = now()
from payload
where agents.id = payload.id and agents.secret_key = payload.secret_key
returning agents.id, agents.user_id, agents.name, agents.machine_code, agents.secret_key, agents.status, agents.client_version, agents.os_type, agents.last_heartbeat_at, agents.created_at, agents.updated_at`

	agent, err := r.scanAgentRow(ctx, query, params.AgentID, params.SecretKey, params.Status, params.ClientVersion, params.OSType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("heartbeat agent: %w", err)
	}
	return agent, nil
}

func (r *AgentRepository) CountByUser(ctx context.Context, userID string) (int, int, error) {
	const query = `
select
	count(1) as total_agents,
	count(1) filter (where status = 'online') as online_agents
from agents
where user_id = $1`

	var totalAgents int
	var onlineAgents int
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&totalAgents, &onlineAgents); err != nil {
		return 0, 0, err
	}
	return totalAgents, onlineAgents, nil
}

func (r *AgentRepository) scanAgentRow(ctx context.Context, query string, args ...any) (*domain.Agent, error) {
	var agent domain.Agent
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&agent.ID,
		&agent.UserID,
		&agent.Name,
		&agent.MachineCode,
		&agent.SecretKey,
		&agent.Status,
		&agent.ClientVersion,
		&agent.OSType,
		&agent.LastHeartbeatAt,
		&agent.CreatedAt,
		&agent.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func (r *AgentRepository) findByUserAndMachine(ctx context.Context, userID, machineCode string) (*domain.Agent, error) {
	const query = `
select id, user_id, name, machine_code, secret_key, status, client_version, os_type, last_heartbeat_at, created_at, updated_at
from agents
where user_id = $1 and machine_code = $2`

	return r.scanAgentRow(ctx, query, userID, machineCode)
}

func isUniqueViolation(err error) bool {
	pgErr, ok := err.(*pgconn.PgError)
	return ok && pgErr.Code == "23505"
}
