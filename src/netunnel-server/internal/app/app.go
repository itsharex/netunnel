package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"time"

	"netunnel/server/internal/config"
	repo "netunnel/server/internal/repository"
	"netunnel/server/internal/service"
	transporthttp "netunnel/server/internal/transport/http"
	transporttcp "netunnel/server/internal/transport/tcp"
)

// App wires together shared server dependencies.
type App struct {
	Config        config.Config
	DB            *sql.DB
	HTTPServer    *transporthttp.Server
	BridgeManager *transporttcp.BridgeManager
	TCPRuntime    *transporttcp.Runtime
	Billing       *service.BillingService
}

// Bootstrap initializes configuration, database connectivity, and migrations.
func Bootstrap(ctx context.Context, cfg config.Config) (*App, error) {
	db, err := repo.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	migrationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := repo.RunSQLMigrations(migrationCtx, db, cfg.MigrationsDir); err != nil {
		_ = db.Close()
		return nil, err
	}

	portRepo := repo.NewPortRepository(db)
	if err := portRepo.InitPorts(ctx, cfg.TCPPortRanges); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init server ports: %w", err)
	}
	log.Printf("server ports initialized: ranges=%v", cfg.TCPPortRanges)

	userRepo := repo.NewUserRepository(db)
	agentRepo := repo.NewAgentRepository(db)
	tunnelRepo := repo.NewTunnelRepository(db)
	domainRouteRepo := repo.NewDomainRouteRepository(db)
	runtimeRepo := repo.NewTunnelRuntimeRepository(db)
	usageRepo := repo.NewUsageRepository(db)
	billingRepo := repo.NewBillingRepository(db)
	paymentOrderRepo := repo.NewPaymentOrderRepository(db)

	bridgeManager := transporttcp.NewBridgeManager(cfg.BridgeListenAddr, runtimeRepo)
	billingSvc := service.NewBillingService(billingRepo, tunnelRepo, nil)
	paymentSvc := service.NewPaymentService(paymentOrderRepo, billingSvc, cfg.PublicAPIBaseURL)
	tcpRuntime := transporttcp.NewRuntime(ctx, bridgeManager, usageRepo, billingSvc)
	billingSvc = service.NewBillingService(billingRepo, tunnelRepo, tcpRuntime)
	paymentSvc = service.NewPaymentService(paymentOrderRepo, billingSvc, cfg.PublicAPIBaseURL)

	userSvc := service.NewUserService(userRepo)
	agentSvc := service.NewAgentService(agentRepo, tunnelRepo, domainRouteRepo)
	tunnelSvc := service.NewTunnelService(tunnelRepo, domainRouteRepo, tcpRuntime, portRepo, cfg.TCPPortRanges, cfg.PublicHost, cfg.HostDomainSuffix)
	usageSvc := service.NewUsageService(usageRepo)
	dashboardSvc := service.NewDashboardService(agentRepo, tunnelRepo, billingSvc, usageSvc)
	httpServer := transporthttp.NewServer(cfg.ListenAddr, cfg.HostDomainSuffix, agentSvc, userSvc, billingSvc, paymentSvc, tunnelSvc, usageSvc, dashboardSvc, bridgeManager, usageRepo)

	activeTunnels, err := tunnelRepo.ListActiveTCP(ctx)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	for _, tunnel := range activeTunnels {
		if err := tcpRuntime.Ensure(ctx, tunnel); err != nil {
			log.Printf("restore tcp runtime skipped: tunnel=%s remote_port=%v err=%v", tunnel.ID, tunnel.RemotePort, err)
			continue
		}
	}

	return &App{
		Config:        cfg,
		DB:            db,
		HTTPServer:    httpServer,
		BridgeManager: bridgeManager,
		TCPRuntime:    tcpRuntime,
		Billing:       billingSvc,
	}, nil
}

// Run starts the public HTTP API and blocks until shutdown.
func (a *App) Run(ctx context.Context) error {
	log.Printf("netunnel-server connected to postgres and applied migrations")
	log.Printf("database target: %s", maskDatabaseURL(a.Config.DatabaseURL))
	log.Printf("migrations dir: %s", a.Config.MigrationsDir)
	log.Printf("listen addr: %s", a.Config.ListenAddr)
	log.Printf("bridge listen addr: %s", a.Config.BridgeListenAddr)
	log.Printf("settlement interval: %s", a.Config.SettlementInterval)

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.HTTPServer.Start()
	}()
	go func() {
		if err := a.BridgeManager.Start(ctx); err != nil {
			log.Printf("bridge manager start skipped: %v", err)
		}
	}()
	go a.Billing.RunSettlementLoop(ctx, a.Config.SettlementInterval)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.HTTPServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown http server: %w", err)
		}
		return ctx.Err()
	}
}

// Close releases shared resources.
func (a *App) Close() error {
	if a.DB == nil {
		return nil
	}
	if err := a.DB.Close(); err != nil {
		return fmt.Errorf("close postgres: %w", err)
	}
	return nil
}

func maskDatabaseURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "invalid-database-url"
	}
	if parsed.User != nil {
		username := parsed.User.Username()
		parsed.User = url.UserPassword(username, "***")
	}
	return parsed.Redacted()
}
