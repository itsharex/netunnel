package service

import (
	"context"
	"fmt"
	"strings"

	"netunnel/server/internal/domain"
	"netunnel/server/internal/repository"
)

type DashboardService struct {
	agents  *repository.AgentRepository
	tunnels *repository.TunnelRepository
	billing *BillingService
	usage   *UsageService
}

func NewDashboardService(agents *repository.AgentRepository, tunnels *repository.TunnelRepository, billing *BillingService, usage *UsageService) *DashboardService {
	return &DashboardService{
		agents:  agents,
		tunnels: tunnels,
		billing: billing,
		usage:   usage,
	}
}

func (s *DashboardService) BuildSummary(ctx context.Context, userID string) (*domain.DashboardSummary, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}

	account, err := s.billing.GetAccount(ctx, userID)
	if err != nil {
		return nil, err
	}

	totalAgents, onlineAgents, err := s.agents.CountByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	tunnels, err := s.tunnels.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	usages, err := s.usage.ListTraffic(ctx, userID, "", 24)
	if err != nil {
		return nil, err
	}

	businessRecords, err := s.billing.ListTransactions(ctx, userID, 5)
	if err != nil {
		return nil, err
	}

	summary := &domain.DashboardSummary{
		UserID:                userID,
		Account:               *account,
		TotalAgents:           totalAgents,
		OnlineAgents:          onlineAgents,
		TotalTunnels:          len(tunnels),
		RecentBusinessRecords: businessRecords,
		RecentUsages:          usages,
	}

	for _, tunnel := range tunnels {
		if tunnel.Enabled {
			summary.EnabledTunnels++
		}
		if tunnel.Status == "disabled_billing" {
			summary.DisabledBillingTunnels++
		}
	}

	for _, usage := range usages {
		summary.RecentTrafficBytes24h += usage.TotalBytes
		summary.UnbilledTrafficBytes24h += usage.TotalBytes - usage.BilledBytes
	}

	return summary, nil
}
