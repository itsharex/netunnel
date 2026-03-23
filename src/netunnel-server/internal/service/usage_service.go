package service

import (
	"context"
	"fmt"
	"strings"

	"netunnel/server/internal/domain"
	"netunnel/server/internal/repository"
)

type UsageService struct {
	usage *repository.UsageRepository
}

func NewUsageService(usage *repository.UsageRepository) *UsageService {
	return &UsageService{usage: usage}
}

func (s *UsageService) ListConnections(ctx context.Context, userID, tunnelID string, limit int) ([]domain.TunnelConnection, error) {
	userID = strings.TrimSpace(userID)
	tunnelID = strings.TrimSpace(tunnelID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 200", ErrInvalidArgument)
	}

	return s.usage.ListTunnelConnections(ctx, repository.ListTunnelConnectionsParams{
		UserID:   userID,
		TunnelID: tunnelID,
		Limit:    limit,
	})
}

func (s *UsageService) ListTraffic(ctx context.Context, userID, tunnelID string, hours int) ([]domain.TrafficUsage, error) {
	userID = strings.TrimSpace(userID)
	tunnelID = strings.TrimSpace(tunnelID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}
	if hours <= 0 {
		hours = 24
	}
	if hours > 24*30 {
		return nil, fmt.Errorf("%w: hours must be between 1 and 720", ErrInvalidArgument)
	}

	return s.usage.ListTrafficUsages(ctx, repository.ListTrafficUsagesParams{
		UserID:   userID,
		TunnelID: tunnelID,
		Hours:    hours,
	})
}
