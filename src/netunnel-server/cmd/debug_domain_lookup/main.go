package main

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	db, err := sql.Open("pgx", "postgresql://ai_ssh_user:ai_ssh_password@127.0.0.1:5432/netunnel")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	rows, err := db.Query(`
select
	dr.domain,
	dr.scheme,
	t.id,
	t.name,
	t.enabled,
	t.local_host,
	t.local_port,
	a.id,
	a.name,
	a.status,
	a.last_heartbeat_at
from domain_routes dr
join tunnels t on t.id = dr.tunnel_id
join agents a on a.id = t.agent_id
where dr.domain in ('demo1.localtest.me', 'demo2.localtest.me')
order by dr.domain`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var domain string
		var scheme string
		var tunnelID string
		var tunnelName string
		var enabled bool
		var localHost string
		var localPort int
		var agentID string
		var agentName string
		var agentStatus string
		var lastHeartbeat sql.NullTime

		if err := rows.Scan(
			&domain,
			&scheme,
			&tunnelID,
			&tunnelName,
			&enabled,
			&localHost,
			&localPort,
			&agentID,
			&agentName,
			&agentStatus,
			&lastHeartbeat,
		); err != nil {
			panic(err)
		}

		fmt.Printf(
			"domain=%s scheme=%s tunnel=%s(%s) enabled=%v local=%s:%d agent=%s(%s) status=%s heartbeat=%v\n",
			domain,
			scheme,
			tunnelName,
			tunnelID,
			enabled,
			localHost,
			localPort,
			agentName,
			agentID,
			agentStatus,
			lastHeartbeat,
		)
	}
}
