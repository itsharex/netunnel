package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"netunnel/server/internal/config"
)

type PortRepository struct {
	db *sql.DB
}

func NewPortRepository(db *sql.DB) *PortRepository {
	return &PortRepository{db: db}
}

func (r *PortRepository) InitPorts(ctx context.Context, ranges []config.PortRange) error {
	batchSize := 1000
	totalCount := 0
	for _, rng := range ranges {
		for start := rng.Start; start <= rng.End; {
			end := start + batchSize - 1
			if end > rng.End {
				end = rng.End
			}

			var values []string
			for p := start; p <= end; p++ {
				values = append(values, fmt.Sprintf("(%d, 'unused')", p))
			}

			query := fmt.Sprintf(
				`INSERT INTO server_ports (port, status) VALUES %s ON CONFLICT (port) DO NOTHING`,
				strings.Join(values, ","),
			)
			result, err := r.db.ExecContext(ctx, query)
			if err != nil {
				return fmt.Errorf("init ports %d-%d: %w", start, end, err)
			}
			rowsAffected, _ := result.RowsAffected()
			totalCount += int(rowsAffected)
			start = end + 1
		}
	}
	log.Printf("[port_repo] InitPorts inserted %d rows", totalCount)
	return nil
}

func (r *PortRepository) AllocatePort(ctx context.Context, ranges []config.PortRange) (int, error) {
	for _, rng := range ranges {
		var port int
		err := r.db.QueryRowContext(ctx,
			`UPDATE server_ports 
			 SET status='used', allocated_at=NOW(), updated_at=NOW()
			 WHERE port = (
			       SELECT port FROM server_ports 
			       WHERE status='unused' AND port >= $1 AND port <= $2 
			       LIMIT 1 FOR UPDATE
			 )
			 RETURNING port`,
			rng.Start, rng.End,
		).Scan(&port)
		if err == nil {
			return port, nil
		}
		if err != pgx.ErrNoRows {
			return 0, fmt.Errorf("allocate port: %w", err)
		}
	}
	return 0, fmt.Errorf("no available port in ranges")
}

func (r *PortRepository) BindPortToTunnel(ctx context.Context, port int, tunnelID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE server_ports
		 SET tunnel_id=$2, updated_at=NOW()
		 WHERE port=$1`,
		port, tunnelID,
	)
	return err
}

func (r *PortRepository) FreePort(ctx context.Context, port int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE server_ports SET status='unused', tunnel_id=NULL, allocated_at=NULL, updated_at=NOW()
		 WHERE port=$1`,
		port,
	)
	return err
}

func (r *PortRepository) IsPortAvailable(ctx context.Context, port int) (bool, error) {
	var status string
	err := r.db.QueryRowContext(ctx,
		`SELECT status FROM server_ports WHERE port=$1`,
		port,
	).Scan(&status)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "unused", nil
}
