package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"netunnel/server/internal/domain"
	"netunnel/server/internal/repository"
)

var ErrInvalidArgument = errors.New("invalid argument")
var ErrAgentAuthFailed = errors.New("agent auth failed")
var ErrNotFound = errors.New("not found")

type AgentService struct {
	agents       *repository.AgentRepository
	tunnels      *repository.TunnelRepository
	domainRoutes *repository.DomainRouteRepository
}

func NewAgentService(agents *repository.AgentRepository, tunnels *repository.TunnelRepository, domainRoutes *repository.DomainRouteRepository) *AgentService {
	return &AgentService{
		agents:       agents,
		tunnels:      tunnels,
		domainRoutes: domainRoutes,
	}
}

type RegisterAgentInput struct {
	UserID        string `json:"user_id"`
	Name          string `json:"name"`
	MachineCode   string `json:"machine_code"`
	ClientVersion string `json:"client_version"`
	OSType        string `json:"os_type"`
}

type HeartbeatAgentInput struct {
	AgentID       string `json:"agent_id"`
	SecretKey     string `json:"secret_key"`
	Status        string `json:"status"`
	ClientVersion string `json:"client_version"`
	OSType        string `json:"os_type"`
}

type AgentConfig struct {
	Agent        *domain.Agent                   `json:"agent"`
	Tunnels      []domain.Tunnel                 `json:"tunnels"`
	DomainRoutes map[string][]domain.DomainRoute `json:"domain_routes"`
}

func (s *AgentService) Register(ctx context.Context, input RegisterAgentInput) (*domain.Agent, bool, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.Name = strings.TrimSpace(input.Name)
	input.MachineCode = strings.TrimSpace(input.MachineCode)
	input.ClientVersion = strings.TrimSpace(input.ClientVersion)
	input.OSType = strings.TrimSpace(input.OSType)

	if input.UserID == "" || input.Name == "" || input.MachineCode == "" {
		return nil, false, fmt.Errorf("%w: user_id, name, machine_code are required", ErrInvalidArgument)
	}

	secretKey, err := generateSecretKey(16)
	if err != nil {
		return nil, false, err
	}

	agent, created, err := s.agents.Register(ctx, repository.RegisterAgentParams{
		UserID:        input.UserID,
		Name:          input.Name,
		MachineCode:   input.MachineCode,
		SecretKey:     secretKey,
		ClientVersion: input.ClientVersion,
		OSType:        input.OSType,
	})
	if err != nil {
		return nil, false, err
	}
	return agent, created, nil
}

func (s *AgentService) Heartbeat(ctx context.Context, input HeartbeatAgentInput) (*domain.Agent, error) {
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.SecretKey = strings.TrimSpace(input.SecretKey)
	input.Status = strings.TrimSpace(input.Status)
	input.ClientVersion = strings.TrimSpace(input.ClientVersion)
	input.OSType = strings.TrimSpace(input.OSType)

	if input.AgentID == "" || input.SecretKey == "" {
		return nil, fmt.Errorf("%w: agent_id and secret_key are required", ErrInvalidArgument)
	}
	if input.Status == "" {
		input.Status = "online"
	}

	agent, err := s.agents.Heartbeat(ctx, repository.HeartbeatAgentParams{
		AgentID:       input.AgentID,
		SecretKey:     input.SecretKey,
		Status:        input.Status,
		ClientVersion: input.ClientVersion,
		OSType:        input.OSType,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentAuthFailed
		}
		return nil, err
	}
	return agent, nil
}

func (s *AgentService) LoadConfig(ctx context.Context, input HeartbeatAgentInput) (*AgentConfig, error) {
	agent, err := s.Heartbeat(ctx, input)
	if err != nil {
		return nil, err
	}

	tunnels, err := s.tunnels.ListByAgent(ctx, agent.ID)
	if err != nil {
		return nil, err
	}

	tunnelIDs := make([]string, 0, len(tunnels))
	for _, tunnel := range tunnels {
		tunnelIDs = append(tunnelIDs, tunnel.ID)
	}

	routeMap, err := s.domainRoutes.ListByTunnelIDs(ctx, tunnelIDs)
	if err != nil {
		return nil, err
	}

	return &AgentConfig{
		Agent:        agent,
		Tunnels:      tunnels,
		DomainRoutes: routeMap,
	}, nil
}

func generateSecretKey(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate secret key: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
