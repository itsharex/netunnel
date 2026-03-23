package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"netunnel/server/internal/domain"
	"netunnel/server/internal/repository"
)

var ErrInsufficientBalance = fmt.Errorf("insufficient balance")

type BillingService struct {
	billing *repository.BillingRepository
	tunnels *repository.TunnelRepository
	runtime interface {
		Disable(tunnelID string) error
		Ensure(ctx context.Context, tunnel domain.Tunnel) error
	}
}

func NewBillingService(billing *repository.BillingRepository, tunnels *repository.TunnelRepository, runtime interface {
	Disable(tunnelID string) error
	Ensure(ctx context.Context, tunnel domain.Tunnel) error
}) *BillingService {
	return &BillingService{billing: billing, tunnels: tunnels, runtime: runtime}
}

type ManualRechargeInput struct {
	UserID string `json:"user_id"`
	Amount string `json:"amount"`
	Remark string `json:"remark"`
}

type ActivatePricingRuleInput struct {
	UserID        string `json:"user_id"`
	PricingRuleID string `json:"pricing_rule_id"`
}

func (s *BillingService) GetAccount(ctx context.Context, userID string) (*domain.Account, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}
	return s.billing.EnsureAccount(ctx, userID)
}

func (s *BillingService) GetBillingProfile(ctx context.Context, userID string) (*repository.BillingProfile, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}
	return s.billing.GetBillingProfile(ctx, userID)
}

func (s *BillingService) ListPricingRules(ctx context.Context) ([]domain.PricingRule, error) {
	return s.billing.ListPricingRules(ctx, "active")
}

func (s *BillingService) ActivatePricingRule(ctx context.Context, input ActivatePricingRuleInput) (*repository.ActivatePricingRuleResult, error) {
	return s.activatePricingRule(ctx, input, true)
}

func (s *BillingService) ActivatePricingRuleAfterExternalPayment(ctx context.Context, input ActivatePricingRuleInput) (*repository.ActivatePricingRuleResult, error) {
	return s.activatePricingRule(ctx, input, false)
}

func (s *BillingService) activatePricingRule(ctx context.Context, input ActivatePricingRuleInput, chargeSubscription bool) (*repository.ActivatePricingRuleResult, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.PricingRuleID = strings.TrimSpace(input.PricingRuleID)
	if input.UserID == "" || input.PricingRuleID == "" {
		return nil, fmt.Errorf("%w: user_id and pricing_rule_id are required", ErrInvalidArgument)
	}
	var result *repository.ActivatePricingRuleResult
	var err error
	if chargeSubscription {
		result, err = s.billing.ActivatePricingRule(ctx, input.UserID, input.PricingRuleID)
	} else {
		result, err = s.billing.ActivatePricingRuleAfterExternalPayment(ctx, input.UserID, input.PricingRuleID)
	}
	if err != nil {
		if strings.Contains(err.Error(), ErrInsufficientBalance.Error()) {
			return nil, ErrInsufficientBalance
		}
		return nil, err
	}
	if err := s.restoreUserBillingDisabledTunnels(ctx, input.UserID); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *BillingService) RechargeManual(ctx context.Context, input ManualRechargeInput) (*domain.Account, *domain.UserBusinessRecord, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.Amount = strings.TrimSpace(input.Amount)
	input.Remark = strings.TrimSpace(input.Remark)
	if input.UserID == "" || input.Amount == "" {
		return nil, nil, fmt.Errorf("%w: user_id and amount are required", ErrInvalidArgument)
	}
	if input.Remark == "" {
		input.Remark = "manual recharge"
	}
	account, transaction, err := s.billing.CreateRecharge(ctx, repository.CreateRechargeParams{
		UserID: input.UserID,
		Amount: input.Amount,
		Remark: input.Remark,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := s.restoreUserBillingDisabledTunnels(ctx, input.UserID); err != nil {
		return nil, nil, err
	}
	return account, transaction, nil
}

func (s *BillingService) SettleUsage(ctx context.Context, userID string) (*repository.SettleUsageResult, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}
	log.Printf("billing settle requested: user=%s", userID)
	result, err := s.billing.SettleUsage(ctx, userID)
	if err != nil {
		if strings.Contains(err.Error(), ErrInsufficientBalance.Error()) {
			return nil, ErrInsufficientBalance
		}
		return nil, err
	}
	return result, nil
}

func (s *BillingService) ListTransactions(ctx context.Context, userID string, limit int) ([]domain.UserBusinessRecord, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 200", ErrInvalidArgument)
	}
	return s.billing.ListTransactions(ctx, userID, limit)
}

func (s *BillingService) AuthorizeTunnelOpen(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}
	profile, err := s.billing.GetBillingProfile(ctx, userID)
	if err != nil {
		return err
	}
	if profile.Subscription != nil || profile.PricingRule.IsUnlimited || profile.PricingRule.PricePerGB == "0" || profile.PricingRule.PricePerGB == "0.0000" {
		return nil
	}
	ok, err := s.billing.AccountHasPositiveBalance(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInsufficientBalance
	}
	return nil
}

func (s *BillingService) RunSettlementLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		log.Printf("billing auto settlement disabled")
		return
	}

	log.Printf("billing auto settlement enabled: interval=%s", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runSettlementOnce(ctx)
		}
	}
}

func (s *BillingService) runSettlementOnce(ctx context.Context) {
	users, err := s.billing.ListUsersWithUnbilledUsage(ctx, 100)
	if err != nil {
		log.Printf("billing settlement scan failed: %v", err)
		return
	}
	if len(users) == 0 {
		log.Printf("billing settlement scan: no users with unbilled usage")
		return
	}

	log.Printf("billing settlement scan: found %d user(s) with unbilled usage", len(users))

	for _, userID := range users {
		log.Printf("billing settlement start: user=%s", userID)
		result, err := s.SettleUsage(ctx, userID)
		if err != nil {
			if err == ErrInsufficientBalance || strings.Contains(err.Error(), ErrInsufficientBalance.Error()) {
				if disableErr := s.disableUserTunnels(ctx, userID); disableErr != nil {
					log.Printf("billing disable user tunnels failed: user=%s err=%v", userID, disableErr)
				} else {
					log.Printf("billing disabled user tunnels due to insufficient balance: user=%s", userID)
				}
				continue
			}
			log.Printf("billing settlement failed: user=%s err=%v", userID, err)
			continue
		}
		log.Printf(
			"billing settlement complete: user=%s charged_bytes=%d included_bytes=%d billable_bytes=%d charge_amount=%s balance=%s subscription=%t",
			userID,
			result.ChargedBytes,
			result.IncludedBytes,
			result.BillableBytes,
			result.ChargeAmount,
			result.Account.Balance,
			result.Subscription != nil,
		)
	}
}

func (s *BillingService) disableUserTunnels(ctx context.Context, userID string) error {
	if s.tunnels == nil {
		return nil
	}

	tunnels, err := s.tunnels.ListEnabledByUser(ctx, userID)
	if err != nil {
		return err
	}
	if len(tunnels) == 0 {
		return nil
	}

	if err := s.tunnels.DisableAllByUser(ctx, userID); err != nil {
		return err
	}

	if s.runtime != nil {
		for _, tunnel := range tunnels {
			if tunnel.Type == "tcp" {
				if err := s.runtime.Disable(tunnel.ID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *BillingService) restoreUserBillingDisabledTunnels(ctx context.Context, userID string) error {
	if s.tunnels == nil {
		return nil
	}

	tunnels, err := s.tunnels.ListDisabledByUserAndStatus(ctx, userID, "disabled_billing")
	if err != nil {
		return err
	}
	if len(tunnels) == 0 {
		return nil
	}

	if err := s.tunnels.RestoreBillingDisabledByUser(ctx, userID); err != nil {
		return err
	}

	if s.runtime != nil {
		for _, tunnel := range tunnels {
			if tunnel.Type == "tcp" {
				tunnel.Enabled = true
				tunnel.Status = "active"
				if err := s.runtime.Ensure(ctx, tunnel); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
