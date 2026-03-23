-- Server ports management table for TCP tunnel auto-allocation
CREATE TABLE IF NOT EXISTS server_ports (
    port INT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'unused',
    tunnel_id UUID REFERENCES tunnels(id) ON DELETE SET NULL,
    allocated_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_server_ports_status ON server_ports(status);
