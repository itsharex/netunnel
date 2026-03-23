package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"netunnel/server/internal/config"
	"netunnel/server/internal/domain"
	"netunnel/server/internal/repository"
)

type tcpRuntime interface {
	Ensure(ctx context.Context, tunnel domain.Tunnel) error
	Disable(tunnelID string) error
}

type TunnelService struct {
	tunnels          *repository.TunnelRepository
	domainRoutes     *repository.DomainRouteRepository
	ports            *repository.PortRepository
	runtime          tcpRuntime
	portRanges       []config.PortRange
	publicHost       string
	hostDomainSuffix string
}

func NewTunnelService(
	tunnels *repository.TunnelRepository,
	domainRoutes *repository.DomainRouteRepository,
	runtime tcpRuntime,
	ports *repository.PortRepository,
	portRanges []config.PortRange,
	publicHost string,
	hostDomainSuffix string,
) *TunnelService {
	return &TunnelService{
		tunnels:          tunnels,
		domainRoutes:     domainRoutes,
		ports:            ports,
		runtime:          runtime,
		portRanges:       portRanges,
		publicHost:       strings.TrimSpace(publicHost),
		hostDomainSuffix: strings.TrimSpace(hostDomainSuffix),
	}
}

type CreateTCPTunnelInput struct {
	UserID    string `json:"user_id"`
	AgentID   string `json:"agent_id"`
	Name      string `json:"name"`
	LocalHost string `json:"local_host"`
	LocalPort int    `json:"local_port"`
}

type CreateHTTPHostTunnelInput struct {
	UserID       string `json:"user_id"`
	AgentID      string `json:"agent_id"`
	Name         string `json:"name"`
	LocalHost    string `json:"local_host"`
	LocalPort    int    `json:"local_port"`
	DomainPrefix string `json:"domain_prefix"`
}

type UpdateTunnelInput struct {
	UserID    string `json:"user_id"`
	AgentID   string `json:"agent_id"`
	Name      string `json:"name"`
	LocalHost string `json:"local_host"`
	LocalPort int    `json:"local_port"`
	Domain    string `json:"domain"`
}

func (s *TunnelService) CreateTCP(ctx context.Context, input CreateTCPTunnelInput) (*domain.Tunnel, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.Name = strings.TrimSpace(input.Name)
	input.LocalHost = strings.TrimSpace(input.LocalHost)

	if input.UserID == "" || input.AgentID == "" || input.Name == "" || input.LocalHost == "" {
		return nil, fmt.Errorf("%w: user_id, agent_id, name, local_host are required", ErrInvalidArgument)
	}
	if input.LocalPort <= 0 || input.LocalPort > 65535 {
		return nil, fmt.Errorf("%w: local_port must be between 1 and 65535", ErrInvalidArgument)
	}

	allocatedPort, err := s.ports.AllocatePort(ctx, s.portRanges)
	if err != nil {
		return nil, fmt.Errorf("allocate port: %w", err)
	}

	tunnel, err := s.tunnels.CreateTCP(ctx, repository.CreateTunnelParams{
		UserID:     input.UserID,
		AgentID:    input.AgentID,
		Name:       input.Name,
		Type:       "tcp",
		LocalHost:  input.LocalHost,
		LocalPort:  input.LocalPort,
		RemotePort: &allocatedPort,
	})
	if err != nil {
		_ = s.ports.FreePort(ctx, allocatedPort)
		return nil, err
	}
	if err := s.ports.BindPortToTunnel(ctx, allocatedPort, tunnel.ID); err != nil {
		_ = s.tunnels.DeleteByIDAndUser(ctx, tunnel.ID, tunnel.UserID)
		_ = s.ports.FreePort(ctx, allocatedPort)
		return nil, fmt.Errorf("bind port to tunnel: %w", err)
	}

	if s.runtime != nil {
		if err := s.runtime.Ensure(ctx, *tunnel); err != nil {
			return nil, err
		}
	}
	return tunnel, nil
}

func (s *TunnelService) CreateHTTPHost(ctx context.Context, input CreateHTTPHostTunnelInput) (*domain.Tunnel, *domain.DomainRoute, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.Name = strings.TrimSpace(input.Name)
	input.LocalHost = strings.TrimSpace(input.LocalHost)
	input.DomainPrefix = strings.ToLower(strings.TrimSpace(input.DomainPrefix))

	if input.UserID == "" || input.AgentID == "" || input.Name == "" || input.LocalHost == "" {
		return nil, nil, fmt.Errorf("%w: user_id, agent_id, name, local_host are required", ErrInvalidArgument)
	}
	if input.LocalPort <= 0 || input.LocalPort > 65535 {
		return nil, nil, fmt.Errorf("%w: local_port must be between 1 and 65535", ErrInvalidArgument)
	}
	domain, err := s.buildManagedHostDomain(input.DomainPrefix)
	if err != nil {
		return nil, nil, err
	}

	tunnel, err := s.tunnels.CreateHTTPHost(ctx, repository.CreateTunnelParams{
		UserID:    input.UserID,
		AgentID:   input.AgentID,
		Name:      input.Name,
		Type:      "http_host",
		LocalHost: input.LocalHost,
		LocalPort: input.LocalPort,
	})
	if err != nil {
		return nil, nil, err
	}

	route, err := s.domainRoutes.Create(ctx, repository.CreateDomainRouteParams{
		TunnelID: tunnel.ID,
		Domain:   domain,
		Scheme:   "https",
	})
	if err != nil {
		return nil, nil, s.translateDomainRouteError(err)
	}

	return tunnel, route, nil
}

func (s *TunnelService) ListByUser(ctx context.Context, userID string) ([]domain.Tunnel, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}
	tunnels, err := s.tunnels.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range tunnels {
		s.populateTunnelAccessTarget(&tunnels[i])
	}
	return tunnels, nil
}

func (s *TunnelService) ListByAgent(ctx context.Context, agentID string) ([]domain.Tunnel, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("%w: agent_id is required", ErrInvalidArgument)
	}
	return s.tunnels.ListByAgent(ctx, agentID)
}

func (s *TunnelService) SetEnabled(ctx context.Context, tunnelID, userID string, enabled bool) (*domain.Tunnel, error) {
	tunnelID = strings.TrimSpace(tunnelID)
	userID = strings.TrimSpace(userID)

	if tunnelID == "" || userID == "" {
		return nil, fmt.Errorf("%w: tunnel_id and user_id are required", ErrInvalidArgument)
	}

	tunnel, err := s.tunnels.UpdateEnabled(ctx, tunnelID, userID, enabled)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if s.runtime == nil || tunnel.Type != "tcp" {
		return tunnel, nil
	}
	if enabled {
		if err := s.runtime.Ensure(ctx, *tunnel); err != nil {
			return nil, err
		}
		return tunnel, nil
	}
	if err := s.runtime.Disable(tunnel.ID); err != nil {
		return nil, err
	}
	return tunnel, nil
}

func (s *TunnelService) Update(ctx context.Context, tunnelID string, input UpdateTunnelInput) (*domain.Tunnel, *domain.DomainRoute, error) {
	tunnelID = strings.TrimSpace(tunnelID)
	input.UserID = strings.TrimSpace(input.UserID)
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.Name = strings.TrimSpace(input.Name)
	input.LocalHost = strings.TrimSpace(input.LocalHost)
	input.Domain = strings.ToLower(strings.TrimSpace(input.Domain))

	if tunnelID == "" || input.UserID == "" || input.AgentID == "" || input.Name == "" || input.LocalHost == "" {
		return nil, nil, fmt.Errorf("%w: tunnel_id, user_id, agent_id, name, local_host are required", ErrInvalidArgument)
	}
	if input.LocalPort <= 0 || input.LocalPort > 65535 {
		return nil, nil, fmt.Errorf("%w: local_port must be between 1 and 65535", ErrInvalidArgument)
	}

	current, err := s.tunnels.GetByIDAndUser(ctx, tunnelID, input.UserID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	updatedTunnel, err := s.tunnels.Update(ctx, repository.UpdateTunnelParams{
		TunnelID:  tunnelID,
		UserID:    input.UserID,
		AgentID:   input.AgentID,
		Name:      input.Name,
		LocalHost: input.LocalHost,
		LocalPort: input.LocalPort,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	var updatedRoute *domain.DomainRoute
	if current.Type == "http_host" {
		domain, err := s.buildManagedHostDomain(input.Domain)
		if err != nil {
			return nil, nil, err
		}

		routes, err := s.domainRoutes.ListByTunnel(ctx, tunnelID)
		if err != nil {
			return nil, nil, err
		}
		if len(routes) == 0 {
			return nil, nil, fmt.Errorf("%w: domain route not found", ErrNotFound)
		}

		updatedRoute, err = s.domainRoutes.UpdateByIDAndUser(ctx, repository.UpdateDomainRouteParams{
			RouteID: routes[0].ID,
			UserID:  input.UserID,
			Domain:  domain,
			Scheme:  "https",
		})
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, nil, ErrNotFound
			}
			return nil, nil, s.translateDomainRouteError(err)
		}
	}

	if s.runtime != nil && updatedTunnel.Type == "tcp" && updatedTunnel.Enabled {
		if err := s.runtime.Disable(updatedTunnel.ID); err != nil {
			return nil, nil, err
		}
		if err := s.runtime.Ensure(ctx, *updatedTunnel); err != nil {
			return nil, nil, err
		}
	}

	return updatedTunnel, updatedRoute, nil
}

func (s *TunnelService) Delete(ctx context.Context, tunnelID, userID string) error {
	tunnelID = strings.TrimSpace(tunnelID)
	userID = strings.TrimSpace(userID)

	if tunnelID == "" || userID == "" {
		return fmt.Errorf("%w: tunnel_id and user_id are required", ErrInvalidArgument)
	}

	tunnel, err := s.tunnels.GetByIDAndUser(ctx, tunnelID, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}

	if err := s.tunnels.DeleteByIDAndUser(ctx, tunnelID, userID); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}

	if tunnel.Type == "tcp" && tunnel.RemotePort != nil {
		_ = s.ports.FreePort(ctx, *tunnel.RemotePort)
	}

	if s.runtime != nil && tunnel.Type == "tcp" {
		if err := s.runtime.Disable(tunnel.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *TunnelService) ListDomainRoutes(ctx context.Context, tunnelID string) ([]domain.DomainRoute, error) {
	tunnelID = strings.TrimSpace(tunnelID)
	if tunnelID == "" {
		return nil, fmt.Errorf("%w: tunnel_id is required", ErrInvalidArgument)
	}
	routes, err := s.domainRoutes.ListByTunnel(ctx, tunnelID)
	if err != nil {
		return nil, err
	}
	for i := range routes {
		s.populateDomainRouteAccessURL(&routes[i])
	}
	return routes, nil
}

func (s *TunnelService) populateTunnelAccessTarget(tunnel *domain.Tunnel) {
	if tunnel == nil || tunnel.Type != "tcp" || tunnel.RemotePort == nil || s.publicHost == "" {
		return
	}
	tunnel.AccessTarget = fmt.Sprintf("%s:%d", s.publicHost, *tunnel.RemotePort)
}

func (s *TunnelService) populateDomainRouteAccessURL(route *domain.DomainRoute) {
	if route == nil || route.Domain == "" || route.Scheme == "" {
		return
	}
	route.AccessURL = fmt.Sprintf("%s://%s", route.Scheme, route.Domain)
}

func (s *TunnelService) buildManagedHostDomain(prefix string) (string, error) {
	suffix := strings.ToLower(strings.Trim(strings.TrimSpace(s.hostDomainSuffix), "."))
	if suffix == "" {
		return "", fmt.Errorf("%w: host domain suffix is not configured", ErrInvalidArgument)
	}

	prefix = strings.ToLower(strings.Trim(strings.TrimSpace(prefix), "."))
	if prefix == "" {
		prefix = fmt.Sprintf("a%d", time.Now().Unix())
	}

	return fmt.Sprintf("%s.%s", prefix, suffix), nil
}

func (s *TunnelService) DeleteDomainRoute(ctx context.Context, routeID, userID string) error {
	routeID = strings.TrimSpace(routeID)
	userID = strings.TrimSpace(userID)

	if routeID == "" || userID == "" {
		return fmt.Errorf("%w: route_id and user_id are required", ErrInvalidArgument)
	}

	if err := s.domainRoutes.DeleteByIDAndUser(ctx, routeID, userID); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *TunnelService) ResolveHTTPRoute(ctx context.Context, host, scheme string) (*repository.HTTPRouteTarget, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if host == "" {
		return nil, fmt.Errorf("%w: host is required", ErrInvalidArgument)
	}
	if scheme == "" {
		return nil, fmt.Errorf("%w: scheme is required", ErrInvalidArgument)
	}
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("%w: scheme must be http or https", ErrInvalidArgument)
	}

	target, err := s.domainRoutes.FindHTTPRouteByDomain(ctx, host, scheme)
	if err != nil {
		if err == sql.ErrNoRows {
			target, err = s.domainRoutes.FindHTTPRouteByDomain(ctx, host, "")
			if err == sql.ErrNoRows {
				return nil, ErrNotFound
			}
			if err != nil {
				return nil, err
			}
			return target, nil
		}
		return nil, err
	}
	return target, nil
}

func (s *TunnelService) translateDomainRouteError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	if pgErr.Code == "23505" && pgErr.ConstraintName == "domain_routes_domain_key" {
		return fmt.Errorf("%w: 该域名前缀已被占用，请更换后重试", ErrInvalidArgument)
	}

	return err
}
