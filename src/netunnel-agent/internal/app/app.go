package app

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"netunnel/agent/internal/config"
	"netunnel/agent/internal/control"
	"netunnel/agent/internal/forwarder"
)

type App struct {
	cfg                config.Config
	client             *control.Client
	dataSession        *forwarder.DataSessionClient
	lastConfigSnapshot string
}

func Bootstrap(cfg config.Config) (*App, error) {
	return &App{
		cfg:         cfg,
		client:      control.NewClient(cfg.ServerURL),
		dataSession: forwarder.NewDataSessionClient(cfg.BridgeAddr),
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	registerResp, err := a.client.Register(ctx, control.RegisterRequest{
		UserID:        a.cfg.UserID,
		Name:          a.cfg.AgentName,
		MachineCode:   a.cfg.MachineCode,
		ClientVersion: a.cfg.ClientVersion,
		OSType:        a.cfg.OSType,
	})
	if err != nil {
		return err
	}

	log.Printf("agent registered: id=%s created=%t machine_code=%s", registerResp.Agent.ID, registerResp.Created, registerResp.Agent.MachineCode)

	ticker := time.NewTicker(time.Duration(a.cfg.SyncIntervalS) * time.Second)
	defer ticker.Stop()

	if err := a.syncOnce(ctx, registerResp.Agent); err != nil {
		log.Printf("initial config sync failed: %v", err)
	}
	go a.dataSession.Run(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := a.syncOnce(ctx, registerResp.Agent); err != nil {
				log.Printf("config sync failed: %v", err)
			}
		}
	}
}

func (a *App) syncOnce(ctx context.Context, agent control.Agent) error {
	resp, err := a.client.LoadConfig(ctx, control.HeartbeatRequest{
		AgentID:       agent.ID,
		SecretKey:     agent.SecretKey,
		Status:        "online",
		ClientVersion: a.cfg.ClientVersion,
		OSType:        a.cfg.OSType,
	})
	if err != nil {
		return err
	}

	a.dataSession.Update(resp.Config.Agent, resp.Config.Tunnels)

	snapshotPayload := struct {
		AgentID      string                           `json:"agent_id"`
		Tunnels      []control.Tunnel                 `json:"tunnels"`
		DomainRoutes map[string][]control.DomainRoute `json:"domain_routes"`
	}{
		AgentID:      resp.Config.Agent.ID,
		Tunnels:      resp.Config.Tunnels,
		DomainRoutes: resp.Config.DomainRoutes,
	}

	snapshotBytes, err := json.Marshal(snapshotPayload)
	if err != nil {
		log.Printf("config synced: agent=%s tunnels=%d", resp.Config.Agent.ID, len(resp.Config.Tunnels))
		return nil
	}

	snapshot := string(snapshotBytes)
	if snapshot == a.lastConfigSnapshot {
		return nil
	}

	a.lastConfigSnapshot = snapshot
	log.Printf("config synced: agent=%s tunnels=%d", resp.Config.Agent.ID, len(resp.Config.Tunnels))
	for _, tunnel := range resp.Config.Tunnels {
		remotePort := 0
		if tunnel.RemotePort != nil {
			remotePort = *tunnel.RemotePort
		}
		log.Printf(
			"tunnel loaded: id=%s type=%s local=%s:%d remote_port=%d enabled=%t",
			tunnel.ID,
			tunnel.Type,
			tunnel.LocalHost,
			tunnel.LocalPort,
			remotePort,
			tunnel.Enabled,
		)
		if routes := resp.Config.DomainRoutes[tunnel.ID]; len(routes) > 0 {
			for _, route := range routes {
				log.Printf(
					"domain route loaded: tunnel=%s domain=%s scheme=%s cert_source=%s",
					tunnel.ID,
					route.Domain,
					route.Scheme,
					route.CertSource,
				)
			}
		}
	}
	return nil
}
