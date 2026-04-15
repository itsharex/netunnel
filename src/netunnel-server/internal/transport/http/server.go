package http

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"netunnel/server/internal/domain"
	"netunnel/server/internal/service"
)

type bridgeAcquirer interface {
	OpenDataStream(ctx context.Context, agentID, tunnelID string) (net.Conn, error)
}

type tunnelUsageRecorder interface {
	StartTunnelConnection(ctx context.Context, params domain.TunnelConnectionStart) (string, error)
	FinishTunnelConnection(ctx context.Context, params domain.TunnelConnectionFinish) error
}

type Server struct {
	httpServer       *http.Server
	agentSvc         *service.AgentService
	userSvc          *service.UserService
	billingSvc       *service.BillingService
	paymentSvc       *service.PaymentService
	tunnelSvc        *service.TunnelService
	usageSvc         *service.UsageService
	dashboardSvc     *service.DashboardService
	bridge           bridgeAcquirer
	recorder         tunnelUsageRecorder
	hostDomainSuffix string

	mu                             sync.Mutex
	httpTunnelActive               map[string]int
	publicHTTPActive               int
	publicHTTPLimitRejected        uint64
	publicHTTPIdleTimeouts         uint64
	publicHTTPDataStreamFailures   uint64
	publicHTTPDataSessionSuccesses uint64
	publicHTTPDataSessionFailures  uint64
	summaryOnce                    sync.Once
}

const publicHTTPBridgeAcquireTimeout = 10 * time.Second
const publicHTTPHeaderTimeout = 30 * time.Second
const publicHTTPBodyIdleTimeout = 60 * time.Second
const maxPublicHTTPPerTunnel = 32
const publicHTTPSummaryInterval = 1 * time.Minute

func NewServer(listenAddr string, hostDomainSuffix string, agentSvc *service.AgentService, userSvc *service.UserService, billingSvc *service.BillingService, paymentSvc *service.PaymentService, tunnelSvc *service.TunnelService, usageSvc *service.UsageService, dashboardSvc *service.DashboardService, bridge bridgeAcquirer, recorder tunnelUsageRecorder) *Server {
	server := &Server{
		agentSvc:         agentSvc,
		userSvc:          userSvc,
		billingSvc:       billingSvc,
		paymentSvc:       paymentSvc,
		tunnelSvc:        tunnelSvc,
		usageSvc:         usageSvc,
		dashboardSvc:     dashboardSvc,
		bridge:           bridge,
		recorder:         recorder,
		hostDomainSuffix: strings.TrimSpace(hostDomainSuffix),
		httpTunnelActive: make(map[string]int),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", server.handleHealth)
	mux.HandleFunc("/api/v1/dev/bootstrap-user", server.handleBootstrapUser)
	mux.HandleFunc("/api/v1/users/profile", server.handleGetUserProfile)
	mux.HandleFunc("/api/v1/agents/register", server.handleRegisterAgent)
	mux.HandleFunc("/api/v1/agents/heartbeat", server.handleHeartbeat)
	mux.HandleFunc("/api/v1/agents/config", server.handleAgentConfig)
	mux.HandleFunc("/api/v1/tunnels/tcp", server.handleCreateTCPTunnel)
	mux.HandleFunc("/api/v1/tunnels/http-host", server.handleCreateHTTPHostTunnel)
	mux.HandleFunc("/api/v1/tunnels", server.handleListTunnels)
	mux.HandleFunc("/api/v1/billing/account", server.handleGetBillingAccount)
	mux.HandleFunc("/api/v1/billing/profile", server.handleGetBillingProfile)
	mux.HandleFunc("/api/v1/billing/plans", server.handleBillingPlans)
	mux.HandleFunc("/api/v1/billing/plans/activate", server.handleActivateBillingPlan)
	mux.HandleFunc("/api/v1/billing/recharge/manual", server.handleManualRecharge)
	mux.HandleFunc("/api/v1/billing/settle", server.handleSettleBilling)
	mux.HandleFunc("/api/v1/billing/transactions", server.handleListBillingTransactions)
	mux.HandleFunc("/api/v1/billing/business-records", server.handleListBillingTransactions)
	mux.HandleFunc("/api/v1/payments/orders", server.handleCreatePaymentOrder)
	mux.HandleFunc("/api/v1/payments/orders/by-biz/", server.handlePaymentOrderByBiz)
	mux.HandleFunc("/api/v1/payments/notify", server.handlePaymentNotify)
	mux.HandleFunc("/api/v1/dashboard/summary", server.handleDashboardSummary)
	mux.HandleFunc("/api/v1/platform/config", server.handlePlatformConfig)
	mux.HandleFunc("/api/v1/usage/connections", server.handleListUsageConnections)
	mux.HandleFunc("/api/v1/usage/traffic", server.handleListUsageTraffic)
	mux.HandleFunc("/api/v1/domain-routes", server.handleListDomainRoutes)
	mux.HandleFunc("/api/v1/tunnels/", server.handleTunnelAction)
	mux.HandleFunc("/api/v1/domain-routes/", server.handleDomainRouteAction)
	mux.HandleFunc("/", server.handlePublicHTTP)

	server.httpServer = &http.Server{
		Addr:              listenAddr,
		Handler:           loggingMiddleware(corsMiddleware(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server
}

func (s *Server) Start() error {
	s.summaryOnce.Do(func() {
		go s.logSummaries()
	})
	log.Printf("http api listening on %s", s.httpServer.Addr)
	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) logSummaries() {
	ticker := time.NewTicker(publicHTTPSummaryInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.logSummary()
	}
}

func (s *Server) logSummary() {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf(
		"public http summary: active=%d active_tunnels=%d data_session_successes=%d data_session_failures=%d limit_rejected=%d idle_timeouts=%d data_stream_failures=%d",
		s.publicHTTPActive,
		len(s.httpTunnelActive),
		s.publicHTTPDataSessionSuccesses,
		s.publicHTTPDataSessionFailures,
		s.publicHTTPLimitRejected,
		s.publicHTTPIdleTimeouts,
		s.publicHTTPDataStreamFailures,
	)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}

func (s *Server) handleBootstrapUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var rawInput map[string]any
	if err := json.NewDecoder(r.Body).Decode(&rawInput); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	accessToken := hex.EncodeToString(tokenBytes)

	existingUserID, hasExistingUser := rawInput["existing_user_id"].(string)
	_, hasNickname := rawInput["nickname"].(string)
	_, hasAvatarURL := rawInput["avatar_url"].(string)
	_, hasWechatOpenid := rawInput["wechat_openid"].(string)
	if hasExistingUser && existingUserID != "" && !hasNickname && !hasAvatarURL && !hasWechatOpenid {
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id":      existingUserID,
			"access_token": accessToken,
		})
		return
	}

	var input service.BootstrapUserInput
	input.Email, _ = rawInput["email"].(string)
	input.Nickname, _ = rawInput["nickname"].(string)
	input.AvatarURL, _ = rawInput["avatar_url"].(string)
	input.Password, _ = rawInput["password"].(string)
	input.WechatOpenid, _ = rawInput["wechat_openid"].(string)

	user, err := s.userSvc.Bootstrap(r.Context(), input)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"user":         user,
		"access_token": accessToken,
	})
}

func (s *Server) handleGetUserProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	user, err := s.userSvc.GetByID(r.Context(), strings.TrimSpace(r.URL.Query().Get("user_id")))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user": user,
	})
}

func (s *Server) handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input service.RegisterAgentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	agent, created, err := s.agentSvc.Register(r.Context(), input)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, map[string]any{
		"created": created,
		"agent":   agent,
	})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input service.HeartbeatAgentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	agent, err := s.agentSvc.Heartbeat(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrAgentAuthFailed):
			writeError(w, http.StatusUnauthorized, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"agent": agent,
	})
}

func (s *Server) handleAgentConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input service.HeartbeatAgentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	config, err := s.agentSvc.LoadConfig(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrAgentAuthFailed):
			writeError(w, http.StatusUnauthorized, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"config": config,
	})
}

func (s *Server) handleCreateTCPTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input service.CreateTCPTunnelInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	tunnel, err := s.tunnelSvc.CreateTCP(r.Context(), input)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"tunnel": tunnel,
	})
}

func (s *Server) handleCreateHTTPHostTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input service.CreateHTTPHostTunnelInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	tunnel, route, err := s.tunnelSvc.CreateHTTPHost(r.Context(), input)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"tunnel": tunnel,
		"route":  route,
	})
}

func (s *Server) handleListTunnels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := r.URL.Query().Get("user_id")
	tunnels, err := s.tunnelSvc.ListByUser(r.Context(), userID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tunnels": tunnels,
	})
}

func (s *Server) handleListDomainRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	tunnelID := r.URL.Query().Get("tunnel_id")
	routes, err := s.tunnelSvc.ListDomainRoutes(r.Context(), tunnelID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"routes": routes,
	})
}

func (s *Server) handleListUsageConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	tunnelID := strings.TrimSpace(r.URL.Query().Get("tunnel_id"))
	limit := 20
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		limit = parsed
	}

	items, err := s.usageSvc.ListConnections(r.Context(), userID, tunnelID, limit)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"connections": items,
	})
}

func (s *Server) handleListUsageTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	tunnelID := strings.TrimSpace(r.URL.Query().Get("tunnel_id"))
	hours := 24
	if value := strings.TrimSpace(r.URL.Query().Get("hours")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "hours must be an integer")
			return
		}
		hours = parsed
	}

	items, err := s.usageSvc.ListTraffic(r.Context(), userID, tunnelID, hours)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"usages": items,
	})
}

func (s *Server) handleGetBillingAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	account, err := s.billingSvc.GetAccount(r.Context(), strings.TrimSpace(r.URL.Query().Get("user_id")))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"account": account,
	})
}

func (s *Server) handleGetBillingProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	profile, err := s.billingSvc.GetBillingProfile(r.Context(), strings.TrimSpace(r.URL.Query().Get("user_id")))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"account":      profile.Account,
		"pricing_rule": profile.PricingRule,
		"subscription": profile.Subscription,
	})
}

func (s *Server) handleBillingPlans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	items, err := s.billingSvc.ListPricingRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"pricing_rules": items,
	})
}

func (s *Server) handleActivateBillingPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input service.ActivatePricingRuleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	result, err := s.billingSvc.ActivatePricingRule(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrInsufficientBalance):
			writeError(w, http.StatusPaymentRequired, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"account":      result.Account,
		"pricing_rule": result.PricingRule,
		"subscription": result.Subscription,
		"transaction":  result.Transaction,
	})
}

func (s *Server) handleManualRecharge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input service.ManualRechargeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	account, transaction, err := s.billingSvc.RechargeManual(r.Context(), input)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"account":     account,
		"transaction": transaction,
	})
}

func (s *Server) handleSettleBilling(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	result, err := s.billingSvc.SettleUsage(r.Context(), input.UserID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrInsufficientBalance):
			writeError(w, http.StatusPaymentRequired, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"account":        result.Account,
		"pricing_rule":   result.PricingRule,
		"transaction":    result.Transaction,
		"charged_bytes":  result.ChargedBytes,
		"included_bytes": result.IncludedBytes,
		"billable_bytes": result.BillableBytes,
		"charge_amount":  result.ChargeAmount,
	})
}

func (s *Server) handleListBillingTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	limit := 20
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		limit = parsed
	}

	items, err := s.billingSvc.ListTransactions(r.Context(), userID, limit)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"business_records": items,
	})
}

func (s *Server) handleCreatePaymentOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input service.CreatePaymentOrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	result, err := s.paymentSvc.CreatePaymentOrder(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handlePaymentOrderByBiz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	bizID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/payments/orders/by-biz/"), "/")
	result, err := s.paymentSvc.PollPaymentOrder(r.Context(), bizID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePaymentNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input service.PaymentNotifyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if err := s.paymentSvc.HandleNotify(r.Context(), input); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("success"))
}

func (s *Server) handleDashboardSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	summary, err := s.dashboardSvc.BuildSummary(r.Context(), userID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidArgument) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"summary": summary,
	})
}

func (s *Server) handlePlatformConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"host_domain_suffix": s.hostDomainSuffix,
	})
}

func (s *Server) handleTunnelAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tunnels/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 1 && r.Method == http.MethodPut {
		s.handleUpdateTunnel(w, r, parts[0])
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		s.handleDeleteTunnel(w, r, parts[0])
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "enable":
			s.handleSetTunnelEnabled(w, r, parts[0], true)
			return
		case "disable":
			s.handleSetTunnelEnabled(w, r, parts[0], false)
			return
		}
	}

	writeError(w, http.StatusNotFound, "route not found")
}

func (s *Server) handleUpdateTunnel(w http.ResponseWriter, r *http.Request, tunnelID string) {
	var input service.UpdateTunnelInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	tunnel, route, err := s.tunnelSvc.Update(r.Context(), tunnelID, input)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tunnel": tunnel,
		"route":  route,
	})
}

func (s *Server) handleSetTunnelEnabled(w http.ResponseWriter, r *http.Request, tunnelID string, enabled bool) {
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))

	tunnel, err := s.tunnelSvc.SetEnabled(r.Context(), tunnelID, userID, enabled)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tunnel": tunnel,
	})
}

func (s *Server) handleDeleteTunnel(w http.ResponseWriter, r *http.Request, tunnelID string) {
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))

	err := s.tunnelSvc.Delete(r.Context(), tunnelID, userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":   true,
		"tunnel_id": tunnelID,
	})
}

func (s *Server) handleDomainRouteAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/domain-routes/")
	routeID := strings.Trim(path, "/")
	if routeID == "" || r.Method != http.MethodDelete {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}

	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	err := s.tunnelSvc.DeleteDomainRoute(r.Context(), routeID, userID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":  true,
		"route_id": routeID,
	})
}

func (s *Server) handlePublicHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}

	host := requestHost(r.Host)
	scheme := requestScheme(r)
	target, err := s.tunnelSvc.ResolveHTTPRoute(r.Context(), host, scheme)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidArgument), errors.Is(err, service.ErrNotFound):
			http.NotFound(w, r)
		default:
			writeError(w, http.StatusBadGateway, err.Error())
		}
		return
	}

	if err := s.billingSvc.AuthorizeTunnelOpen(r.Context(), target.Tunnel.UserID); err != nil {
		if errors.Is(err, service.ErrInsufficientBalance) {
			writeError(w, http.StatusPaymentRequired, err.Error())
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if !s.tryAcquireHTTPTunnelSlot(target.Tunnel.ID) {
		s.mu.Lock()
		s.publicHTTPLimitRejected++
		s.mu.Unlock()
		log.Printf("public http rejected by active limit: host=%s tunnel=%s limit=%d", host, target.Tunnel.ID, maxPublicHTTPPerTunnel)
		writeError(w, http.StatusTooManyRequests, "too many active tunnel requests")
		return
	}
	defer s.releaseHTTPTunnelSlot(target.Tunnel.ID)

	acquireCtx, cancel := context.WithTimeout(r.Context(), publicHTTPBridgeAcquireTimeout)
	defer cancel()

	startedAt := time.Now()
	bridgeConn, err := s.bridge.OpenDataStream(acquireCtx, target.Tunnel.AgentID, target.Tunnel.ID)
	if err != nil {
		s.mu.Lock()
		s.publicHTTPDataSessionFailures++
		s.mu.Unlock()
	} else {
		s.mu.Lock()
		s.publicHTTPDataSessionSuccesses++
		s.mu.Unlock()
	}
	if err != nil {
		s.mu.Lock()
		s.publicHTTPDataStreamFailures++
		s.mu.Unlock()
		log.Printf("public http open data stream failed: host=%s tunnel=%s took=%s err=%v", host, target.Tunnel.ID, time.Since(startedAt), err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer bridgeConn.Close()
	log.Printf("public http opened data stream: host=%s tunnel=%s took=%s", host, target.Tunnel.ID, time.Since(startedAt))

	connectionID := ""
	if s.recorder != nil {
		startedID, err := s.recorder.StartTunnelConnection(r.Context(), domain.TunnelConnectionStart{
			TunnelID:   target.Tunnel.ID,
			AgentID:    target.Tunnel.AgentID,
			Protocol:   scheme,
			SourceAddr: r.RemoteAddr,
			TargetAddr: net.JoinHostPort(target.Tunnel.LocalHost, strconv.Itoa(target.Tunnel.LocalPort)),
		})
		if err != nil {
			log.Printf("start http tunnel connection failed: tunnel=%s err=%v", target.Tunnel.ID, err)
		} else {
			connectionID = startedID
		}
	}

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	outReq.URL = &url.URL{
		Scheme:   "http",
		Host:     fmt.Sprintf("%s:%d", target.Tunnel.LocalHost, target.Tunnel.LocalPort),
		Path:     r.URL.Path,
		RawPath:  r.URL.RawPath,
		RawQuery: r.URL.RawQuery,
	}
	outReq.Host = requestHost(r.Host)
	outReq.Header = r.Header.Clone()
	outReq.Header.Set("Connection", "close")
	_ = bridgeConn.SetWriteDeadline(time.Now().Add(publicHTTPHeaderTimeout))

	if err := outReq.Write(bridgeConn); err != nil {
		_ = bridgeConn.SetWriteDeadline(time.Time{})
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = bridgeConn.SetWriteDeadline(time.Time{})
	log.Printf("public http wrote request: host=%s tunnel=%s", host, target.Tunnel.ID)

	stopClosing := make(chan struct{})
	go func() {
		select {
		case <-r.Context().Done():
			_ = bridgeConn.Close()
		case <-stopClosing:
		}
	}()
	defer close(stopClosing)

	_ = bridgeConn.SetReadDeadline(time.Now().Add(publicHTTPHeaderTimeout))
	readStartedAt := time.Now()
	resp, err := http.ReadResponse(bufio.NewReader(bridgeConn), outReq)
	if err != nil {
		if isHTTPNetTimeout(err) {
			s.mu.Lock()
			s.publicHTTPIdleTimeouts++
			s.mu.Unlock()
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	_ = bridgeConn.SetReadDeadline(time.Time{})
	log.Printf("public http read response headers: host=%s tunnel=%s status=%s took=%s", host, target.Tunnel.ID, resp.Status, time.Since(readStartedAt))

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	copied, copyErr := io.Copy(w, &idleTimeoutReader{conn: bridgeConn, reader: resp.Body, timeout: publicHTTPBodyIdleTimeout})
	if copyErr != nil && !errors.Is(copyErr, context.Canceled) {
		if isHTTPNetTimeout(copyErr) {
			s.mu.Lock()
			s.publicHTTPIdleTimeouts++
			s.mu.Unlock()
		}
		log.Printf("public http copy body failed: host=%s tunnel=%s err=%v", host, target.Tunnel.ID, copyErr)
	}
	log.Printf("public http copied response body: host=%s tunnel=%s bytes=%d", host, target.Tunnel.ID, copied)

	if s.recorder != nil && connectionID != "" {
		requestBytes := r.ContentLength
		if requestBytes < 0 {
			requestBytes = 0
		}
		finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer finishCancel()
		if err := s.recorder.FinishTunnelConnection(finishCtx, domain.TunnelConnectionFinish{
			ConnectionID: connectionID,
			UserID:       target.Tunnel.UserID,
			AgentID:      target.Tunnel.AgentID,
			TunnelID:     target.Tunnel.ID,
			IngressBytes: requestBytes,
			EgressBytes:  copied,
			Status:       "closed",
		}); err != nil {
			log.Printf("finish http tunnel connection failed: tunnel=%s err=%v", target.Tunnel.ID, err)
		}
	}
}

func (s *Server) tryAcquireHTTPTunnelSlot(tunnelID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	active := s.httpTunnelActive[tunnelID]
	if active >= maxPublicHTTPPerTunnel {
		return false
	}
	s.httpTunnelActive[tunnelID] = active + 1
	s.publicHTTPActive++
	return true
}

func (s *Server) releaseHTTPTunnelSlot(tunnelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	active := s.httpTunnelActive[tunnelID] - 1
	if active <= 0 {
		delete(s.httpTunnelActive, tunnelID)
	} else {
		s.httpTunnelActive[tunnelID] = active
	}
	if s.publicHTTPActive > 0 {
		s.publicHTTPActive--
	}
}

type idleTimeoutReader struct {
	conn    net.Conn
	reader  io.Reader
	timeout time.Duration
}

func (r *idleTimeoutReader) Read(p []byte) (int, error) {
	if r.timeout > 0 {
		_ = r.conn.SetReadDeadline(time.Now().Add(r.timeout))
	}
	n, err := r.reader.Read(p)
	if err == io.EOF {
		_ = r.conn.SetReadDeadline(time.Time{})
	}
	return n, err
}

func isHTTPNetTimeout(err error) bool {
	if err == nil {
		return false
	}
	netErr, ok := err.(net.Error)
	return ok && netErr.Timeout()
}

func requestHost(hostport string) string {
	host := strings.TrimSpace(hostport)
	if idx := strings.Index(host, ":"); idx >= 0 {
		return strings.ToLower(host[:idx])
	}
	return strings.ToLower(host)
}

func requestScheme(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		if idx := strings.Index(forwarded, ","); idx >= 0 {
			forwarded = forwarded[:idx]
		}
		forwarded = strings.ToLower(strings.TrimSpace(forwarded))
		if forwarded == "http" || forwarded == "https" {
			return forwarded
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": message,
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("http %s %s %s", r.Method, r.URL.Path, time.Since(startedAt))
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
