package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"netunnel/server/internal/domain"
)

const defaultPricingRuleName = "default-traffic"
const defaultPricingRulePricePerGB = "1.0000"

type BillingRepository struct {
	db *sql.DB
}

type BillingProfile struct {
	Account      domain.Account           `json:"account"`
	PricingRule  domain.PricingRule       `json:"pricing_rule"`
	Subscription *domain.UserSubscription `json:"subscription,omitempty"`
}

type ActivatePricingRuleResult struct {
	Account      *domain.Account            `json:"account,omitempty"`
	PricingRule  domain.PricingRule         `json:"pricing_rule"`
	Subscription *domain.UserSubscription   `json:"subscription,omitempty"`
	Transaction  *domain.UserBusinessRecord `json:"transaction,omitempty"`
}

type SettleUsageResult struct {
	Account       domain.Account             `json:"account"`
	Transaction   *domain.UserBusinessRecord `json:"transaction,omitempty"`
	PricingRule   domain.PricingRule         `json:"pricing_rule"`
	Subscription  *domain.UserSubscription   `json:"subscription,omitempty"`
	ChargedBytes  int64                      `json:"charged_bytes"`
	ChargeAmount  string                     `json:"charge_amount"`
	IncludedBytes int64                      `json:"included_bytes"`
	BillableBytes int64                      `json:"billable_bytes"`
}

type activeBillingContext struct {
	rule         *domain.PricingRule
	subscription *domain.UserSubscription
}

func NewBillingRepository(db *sql.DB) *BillingRepository {
	return &BillingRepository{db: db}
}

func (r *BillingRepository) EnsureAccount(ctx context.Context, userID string) (*domain.Account, error) {
	const insertQuery = `
insert into accounts (user_id, balance, currency, status)
values ($1, 0, 'CNY', 'active')
on conflict (user_id) do nothing`
	if _, err := r.db.ExecContext(ctx, insertQuery, userID); err != nil {
		return nil, fmt.Errorf("ensure account: %w", err)
	}
	return r.GetAccountByUser(ctx, userID)
}

func (r *BillingRepository) GetAccountByUser(ctx context.Context, userID string) (*domain.Account, error) {
	return r.getAccountByUserQuerier(ctx, r.db, userID)
}

func (r *BillingRepository) AccountHasPositiveBalance(ctx context.Context, userID string) (bool, error) {
	const query = `
select balance > 0
from accounts
where user_id = $1`

	var ok bool
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&ok); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return ok, nil
}

func (r *BillingRepository) GetBillingProfile(ctx context.Context, userID string) (*BillingProfile, error) {
	account, err := r.EnsureAccount(ctx, userID)
	if err != nil {
		return nil, err
	}
	billing, err := r.resolveActiveBilling(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &BillingProfile{
		Account:      *account,
		PricingRule:  *billing.rule,
		Subscription: billing.subscription,
	}, nil
}

func (r *BillingRepository) ListPricingRules(ctx context.Context, status string) ([]domain.PricingRule, error) {
	status = strings.TrimSpace(status)
	query := `
select id, name, display_name, description, billing_mode, price_per_gb::text, free_quota_bytes, subscription_price::text,
       included_traffic_bytes, subscription_period, traffic_reset_period, is_unlimited, status, created_at, updated_at
from pricing_rules`
	args := make([]any, 0, 1)
	if status != "" {
		query += `
where status = $1`
		args = append(args, status)
	}
	query += `
order by created_at asc`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list pricing rules: %w", err)
	}
	defer rows.Close()

	items := make([]domain.PricingRule, 0)
	for rows.Next() {
		item, err := scanPricingRule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

type CreateRechargeParams struct {
	UserID string
	Amount string
	Remark string
}

func (r *BillingRepository) CreateRecharge(ctx context.Context, params CreateRechargeParams) (*domain.Account, *domain.UserBusinessRecord, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("begin recharge tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = r.ensureAccountQuerier(ctx, tx, params.UserID); err != nil {
		return nil, nil, fmt.Errorf("ensure recharge account: %w", err)
	}

	before, err := r.lockAccount(ctx, tx, params.UserID)
	if err != nil {
		return nil, nil, err
	}
	rechargeBytes, err := r.balanceAmountToBytes(ctx, tx, params.Amount)
	if err != nil {
		return nil, nil, fmt.Errorf("convert recharge amount to bytes: %w", err)
	}
	after, err := r.updateAccountBalance(ctx, tx, params.UserID, rechargeBytes, true)
	if err != nil {
		return nil, nil, fmt.Errorf("update recharge account: %w", err)
	}

	item, err := r.insertAccountTransaction(ctx, tx, createTransactionParams{
		UserID:              params.UserID,
		AccountID:           after.ID,
		RecordType:          "traffic_recharge",
		ChangeAmount:        rechargeBytes,
		TrafficBefore:       before.Balance,
		TrafficAfter:        after.Balance,
		RelatedResourceType: "manual_recharge",
		Description:         params.Remark,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("insert recharge transaction: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit recharge tx: %w", err)
	}
	return after, item, nil
}

func (r *BillingRepository) ActivatePricingRule(ctx context.Context, userID, pricingRuleID string) (*ActivatePricingRuleResult, error) {
	return r.activatePricingRule(ctx, userID, pricingRuleID, true)
}

func (r *BillingRepository) ActivatePricingRuleAfterExternalPayment(ctx context.Context, userID, pricingRuleID string) (*ActivatePricingRuleResult, error) {
	return r.activatePricingRule(ctx, userID, pricingRuleID, false)
}

func (r *BillingRepository) activatePricingRule(ctx context.Context, userID, pricingRuleID string, chargeSubscription bool) (*ActivatePricingRuleResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin activate pricing rule tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	rule, err := r.getPricingRuleByID(ctx, tx, pricingRuleID)
	if err != nil {
		return nil, err
	}

	result := &ActivatePricingRuleResult{PricingRule: *rule}
	if rule.BillingMode == "subscription" {
		if rule.SubscriptionPeriod == "none" {
			return nil, fmt.Errorf("subscription pricing rule %s has invalid subscription period", rule.ID)
		}

		if err = r.ensureAccountQuerier(ctx, tx, userID); err != nil {
			return nil, err
		}
		account, err := r.lockAccount(ctx, tx, userID)
		if err != nil {
			return nil, err
		}
		result.Account = account

		existingSubscription, existingRule, err := r.getActiveSubscriptionForUpdate(ctx, tx, userID)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == sql.ErrNoRows {
			existingSubscription = nil
			existingRule = nil
		}

		if chargeSubscription && !isZeroAmount(rule.SubscriptionPrice) {
			subscriptionChargeBytes, err := r.balanceAmountToBytes(ctx, tx, rule.SubscriptionPrice)
			if err != nil {
				return nil, fmt.Errorf("convert subscription price to bytes: %w", err)
			}

			enough, err := r.accountHasEnoughBalance(ctx, tx, userID, subscriptionChargeBytes)
			if err != nil {
				return nil, err
			}
			if !enough {
				return nil, fmt.Errorf("insufficient balance")
			}

			after, err := r.updateAccountBalance(ctx, tx, userID, subscriptionChargeBytes, false)
			if err != nil {
				return nil, err
			}
			result.Account = after

			recordType := "subscription_purchase"
			if existingSubscription != nil && existingRule != nil && existingRule.ID == rule.ID {
				recordType = "subscription_renew"
			}
			txItem, err := r.insertAccountTransaction(ctx, tx, createTransactionParams{
				UserID:              userID,
				AccountID:           after.ID,
				RecordType:          recordType,
				ChangeAmount:        negateNumericString(subscriptionChargeBytes),
				TrafficBefore:       account.Balance,
				TrafficAfter:        after.Balance,
				RelatedResourceType: "pricing_rule",
				RelatedResourceID:   &rule.ID,
				Description:         fmt.Sprintf("activate pricing rule %s", rule.Name),
			})
			if err != nil {
				return nil, err
			}
			result.Transaction = txItem
		}

		if existingSubscription != nil && existingRule != nil && existingRule.ID == rule.ID {
			if err = r.extendUserSubscription(ctx, tx, existingSubscription, rule); err != nil {
				return nil, err
			}
			result.Subscription = existingSubscription
		} else {
			if existingSubscription != nil {
				if _, err = tx.ExecContext(ctx, `
update user_subscriptions
set status = 'cancelled', cancelled_at = now(), updated_at = now()
where user_id = $1 and status = 'active' and cancelled_at is null`, userID); err != nil {
					return nil, fmt.Errorf("cancel active subscriptions: %w", err)
				}
			}

			now := time.Now().UTC()
			subscription, err := r.insertUserSubscription(ctx, tx, userID, rule, now)
			if err != nil {
				return nil, err
			}
			result.Subscription = subscription
		}
		if !chargeSubscription || isZeroAmount(rule.SubscriptionPrice) {
			recordType := "subscription_purchase"
			if existingSubscription != nil && existingRule != nil && existingRule.ID == rule.ID {
				recordType = "subscription_renew"
			}
			accountAfter := result.Account
			if accountAfter == nil {
				accountAfter, err = r.lockAccount(ctx, tx, userID)
				if err != nil {
					return nil, err
				}
				result.Account = accountAfter
			}
			expiry := result.Subscription.ExpiresAt
			txItem, err := r.insertAccountTransaction(ctx, tx, createTransactionParams{
				UserID:              userID,
				AccountID:           accountAfter.ID,
				RecordType:          recordType,
				ChangeAmount:        "0",
				TrafficBefore:       accountAfter.Balance,
				TrafficAfter:        accountAfter.Balance,
				RelatedResourceType: "pricing_rule",
				RelatedResourceID:   &rule.ID,
				PackageExpiresAt:    expiry,
				Description:         fmt.Sprintf("activate pricing rule %s", rule.Name),
			})
			if err != nil {
				return nil, err
			}
			result.Transaction = txItem
		} else if result.Subscription != nil && result.Transaction != nil {
			if err = r.updateBusinessRecordPackageExpiry(ctx, tx, result.Transaction.ID, result.Subscription.ExpiresAt); err != nil {
				return nil, err
			}
			result.Transaction.PackageExpiresAt = result.Subscription.ExpiresAt
		}
	} else {
		if _, err = tx.ExecContext(ctx, `
update user_pricing_rules
set expired_at = now()
where user_id = $1 and (expired_at is null or expired_at > now())`, userID); err != nil {
			return nil, fmt.Errorf("expire user pricing rules: %w", err)
		}

		if _, err = tx.ExecContext(ctx, `
insert into user_pricing_rules (user_id, pricing_rule_id, effective_at)
values ($1, $2, now())`, userID, rule.ID); err != nil {
			return nil, fmt.Errorf("assign user pricing rule: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit activate pricing rule tx: %w", err)
	}
	return result, nil
}

func (r *BillingRepository) EnsureActivePricingRule(ctx context.Context, userID string) (*domain.PricingRule, error) {
	const assignedQuery = `
select pr.id, pr.name, pr.display_name, pr.description, pr.billing_mode, pr.price_per_gb::text, pr.free_quota_bytes, pr.subscription_price::text,
       pr.included_traffic_bytes, pr.subscription_period, pr.traffic_reset_period, pr.is_unlimited, pr.status,
       pr.created_at, pr.updated_at
from user_pricing_rules upr
join pricing_rules pr on pr.id = upr.pricing_rule_id
where upr.user_id = $1
  and pr.status = 'active'
  and (upr.expired_at is null or upr.expired_at > now())
order by upr.effective_at desc
limit 1`

	rule, err := r.scanPricingRule(ctx, assignedQuery, userID)
	if err == nil {
		return rule, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	const defaultQuery = `
select id, name, display_name, description, billing_mode, price_per_gb::text, free_quota_bytes, subscription_price::text,
       included_traffic_bytes, subscription_period, traffic_reset_period, is_unlimited, status, created_at, updated_at
from pricing_rules
where name = $1
order by created_at asc
limit 1`
	rule, err = r.scanPricingRule(ctx, defaultQuery, defaultPricingRuleName)
	if err == sql.ErrNoRows {
		const insertDefault = `
insert into pricing_rules (
	name, display_name, description, billing_mode, price_per_gb, free_quota_bytes, subscription_price,
	included_traffic_bytes, subscription_period, traffic_reset_period, is_unlimited, status
)
values ($1, '按量流量包', '适合低频使用场景，按实际流量结算，长期有效。', 'traffic', $2::numeric, 0, 0, 0, 'none', 'none', false, 'active')
returning id, name, display_name, description, billing_mode, price_per_gb::text, free_quota_bytes, subscription_price::text,
          included_traffic_bytes, subscription_period, traffic_reset_period, is_unlimited, status, created_at, updated_at`
		rule, err = r.scanPricingRule(ctx, insertDefault, defaultPricingRuleName, defaultPricingRulePricePerGB)
	}
	if err != nil {
		return nil, err
	}

	if _, err = r.db.ExecContext(ctx, `
insert into user_pricing_rules (user_id, pricing_rule_id, effective_at)
values ($1, $2, now())`, userID, rule.ID); err != nil {
		return nil, fmt.Errorf("assign pricing rule: %w", err)
	}
	return rule, nil
}

func (r *BillingRepository) SettleUsage(ctx context.Context, userID string) (*SettleUsageResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin settle tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = r.ensureAccountQuerier(ctx, tx, userID); err != nil {
		return nil, err
	}
	before, err := r.lockAccount(ctx, tx, userID)
	if err != nil {
		return nil, err
	}

	billing, err := r.resolveActiveBillingTx(ctx, tx, userID)
	if err != nil {
		return nil, err
	}

	chargedBytes, err := r.loadUnbilledUsageBytes(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	result := &SettleUsageResult{
		Account:      *before,
		PricingRule:  *billing.rule,
		Subscription: billing.subscription,
		ChargedBytes: chargedBytes,
		ChargeAmount: "0.0000",
	}
	if chargedBytes == 0 {
		if err = tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit empty settle tx: %w", err)
		}
		return result, nil
	}

	billableBytes := chargedBytes
	includedBytes := int64(0)
	if billing.subscription != nil {
		if err = r.advanceSubscriptionPeriodIfNeeded(ctx, tx, billing.subscription, billing.rule); err != nil {
			return nil, err
		}
		result.Subscription = billing.subscription

		if billing.rule.IsUnlimited {
			includedBytes = chargedBytes
			billableBytes = 0
		} else {
			remaining := billing.rule.IncludedTrafficBytes - billing.subscription.CurrentPeriodUsedBytes
			if remaining < 0 {
				remaining = 0
			}
			if remaining > chargedBytes {
				remaining = chargedBytes
			}
			includedBytes = remaining
			billableBytes = chargedBytes - remaining
		}
	}
	result.IncludedBytes = includedBytes
	result.BillableBytes = billableBytes

	if billableBytes > 0 {
		chargeAmount, err := r.calculateChargeAmount(ctx, tx, billableBytes, billing.rule.PricePerGB)
		if err != nil {
			return nil, err
		}
		result.ChargeAmount = chargeAmount
	}

	if billableBytes > 0 && !isZeroAmount(result.ChargeAmount) {
		chargeBytes, err := r.balanceAmountToBytes(ctx, tx, result.ChargeAmount)
		if err != nil {
			return nil, fmt.Errorf("convert settle charge to bytes: %w", err)
		}

		enough, err := r.accountHasEnoughBalance(ctx, tx, userID, chargeBytes)
		if err != nil {
			return nil, err
		}
		if !enough {
			return nil, fmt.Errorf("insufficient balance")
		}

		after, err := r.updateAccountBalance(ctx, tx, userID, chargeBytes, false)
		if err != nil {
			return nil, fmt.Errorf("update settle account: %w", err)
		}
		result.Account = *after

		remark := fmt.Sprintf(
			"traffic settlement bytes=%d billable_bytes=%d included_bytes=%d",
			chargedBytes,
			billableBytes,
			includedBytes,
		)
		item, err := r.insertAccountTransaction(ctx, tx, createTransactionParams{
			UserID:              userID,
			AccountID:           after.ID,
			RecordType:          "traffic_settlement",
			ChangeAmount:        negateNumericString(chargeBytes),
			TrafficBefore:       before.Balance,
			TrafficAfter:        after.Balance,
			RelatedResourceType: "traffic_usage",
			TrafficBytes:        chargedBytes,
			BillableBytes:       billableBytes,
			Description:         remark,
		})
		if err != nil {
			return nil, fmt.Errorf("insert settle transaction: %w", err)
		}
		result.Transaction = item
	}

	if billing.subscription != nil && includedBytes > 0 {
		if result.Transaction == nil {
			item, err := r.insertAccountTransaction(ctx, tx, createTransactionParams{
				UserID:              userID,
				AccountID:           before.ID,
				RecordType:          "subscription_traffic_settlement",
				ChangeAmount:        "0",
				TrafficBefore:       before.Balance,
				TrafficAfter:        before.Balance,
				RelatedResourceType: "traffic_usage",
				TrafficBytes:        chargedBytes,
				BillableBytes:       billableBytes,
				PackageExpiresAt:    billing.subscription.ExpiresAt,
				Description: fmt.Sprintf(
					"subscription traffic bytes=%d included_bytes=%d expires_at=%v",
					chargedBytes,
					includedBytes,
					billing.subscription.ExpiresAt,
				),
			})
			if err != nil {
				return nil, fmt.Errorf("insert subscription usage transaction: %w", err)
			}
			result.Transaction = item
		}

		if err = r.updateSubscriptionUsage(ctx, tx, billing.subscription.ID, billing.subscription.CurrentPeriodUsedBytes+includedBytes); err != nil {
			return nil, err
		}
		billing.subscription.CurrentPeriodUsedBytes += includedBytes
		result.Subscription = billing.subscription
	}

	if err = r.markUsageBilled(ctx, tx, userID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit settle tx: %w", err)
	}
	return result, nil
}

func (r *BillingRepository) ListTransactions(ctx context.Context, userID string, limit int) ([]domain.UserBusinessRecord, error) {
	const query = `
select id, user_id, account_id, record_type, amount::text, balance_before::text, balance_after::text,
       coalesce(related_resource_type, reference_type, ''), related_resource_id, coalesce(traffic_bytes, 0),
       coalesce(billable_bytes, 0),
       package_expires_at, coalesce(payment_order_biz_id, ''), coalesce(description, remark, ''), created_at
from user_business_records
where user_id = $1
order by created_at desc
limit $2`

	rows, err := r.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list account transactions: %w", err)
	}
	defer rows.Close()

	items := make([]domain.UserBusinessRecord, 0)
	for rows.Next() {
		var item domain.UserBusinessRecord
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.AccountID,
			&item.RecordType,
			&item.ChangeAmount,
			&item.TrafficBefore,
			&item.TrafficAfter,
			&item.RelatedResourceType,
			&item.RelatedResourceID,
			&item.TrafficBytes,
			&item.BillableBytes,
			&item.PackageExpiresAt,
			&item.PaymentOrderBizID,
			&item.Description,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *BillingRepository) ListUsersWithUnbilledUsage(ctx context.Context, limit int) ([]string, error) {
	const query = `
select distinct user_id
from traffic_usages
where total_bytes > billed_bytes
order by user_id asc
limit $1`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list users with unbilled usage: %w", err)
	}
	defer rows.Close()

	userIDs := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, rows.Err()
}

func (r *BillingRepository) getAccountByUserQuerier(ctx context.Context, querier queryRower, userID string) (*domain.Account, error) {
	const query = `
select id, user_id, balance::text, currency, status, created_at, updated_at
from accounts
where user_id = $1`

	var item domain.Account
	if err := querier.QueryRowContext(ctx, query, userID).Scan(
		&item.ID,
		&item.UserID,
		&item.Balance,
		&item.Currency,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *BillingRepository) resolveActiveBilling(ctx context.Context, userID string) (*activeBillingContext, error) {
	subscription, rule, err := r.getActiveSubscription(ctx, r.db, userID)
	if err == nil {
		return &activeBillingContext{rule: rule, subscription: subscription}, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	rule, err = r.EnsureActivePricingRule(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &activeBillingContext{rule: rule}, nil
}

func (r *BillingRepository) resolveActiveBillingTx(ctx context.Context, tx *sql.Tx, userID string) (*activeBillingContext, error) {
	subscription, rule, err := r.getActiveSubscriptionForUpdate(ctx, tx, userID)
	if err == nil {
		return &activeBillingContext{rule: rule, subscription: subscription}, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	rule, err = r.EnsureActivePricingRule(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &activeBillingContext{rule: rule}, nil
}

func (r *BillingRepository) getActiveSubscription(ctx context.Context, querier queryRower, userID string) (*domain.UserSubscription, *domain.PricingRule, error) {
	const query = `
select us.id, us.user_id, us.pricing_rule_id, us.status, us.started_at, us.current_period_start, us.current_period_end,
       us.current_period_used_bytes, us.expires_at, us.cancelled_at, us.created_at, us.updated_at,
       pr.id, pr.name, pr.display_name, pr.description, pr.billing_mode, pr.price_per_gb::text, pr.free_quota_bytes, pr.subscription_price::text,
       pr.included_traffic_bytes, pr.subscription_period, pr.traffic_reset_period, pr.is_unlimited, pr.status,
       pr.created_at, pr.updated_at
from user_subscriptions us
join pricing_rules pr on pr.id = us.pricing_rule_id
where us.user_id = $1
  and us.status = 'active'
  and us.cancelled_at is null
  and us.started_at <= now()
  and (us.expires_at is null or us.expires_at > now())
  and pr.status = 'active'
order by us.started_at desc
limit 1`
	return scanSubscriptionWithRule(querier.QueryRowContext(ctx, query, userID))
}

func (r *BillingRepository) getActiveSubscriptionForUpdate(ctx context.Context, tx *sql.Tx, userID string) (*domain.UserSubscription, *domain.PricingRule, error) {
	const query = `
select us.id, us.user_id, us.pricing_rule_id, us.status, us.started_at, us.current_period_start, us.current_period_end,
       us.current_period_used_bytes, us.expires_at, us.cancelled_at, us.created_at, us.updated_at,
       pr.id, pr.name, pr.display_name, pr.description, pr.billing_mode, pr.price_per_gb::text, pr.free_quota_bytes, pr.subscription_price::text,
       pr.included_traffic_bytes, pr.subscription_period, pr.traffic_reset_period, pr.is_unlimited, pr.status,
       pr.created_at, pr.updated_at
from user_subscriptions us
join pricing_rules pr on pr.id = us.pricing_rule_id
where us.user_id = $1
  and us.status = 'active'
  and us.cancelled_at is null
  and us.started_at <= now()
  and (us.expires_at is null or us.expires_at > now())
  and pr.status = 'active'
order by us.started_at desc
limit 1
for update of us`
	return scanSubscriptionWithRule(tx.QueryRowContext(ctx, query, userID))
}

func (r *BillingRepository) getPricingRuleByID(ctx context.Context, querier queryRower, pricingRuleID string) (*domain.PricingRule, error) {
	const query = `
select id, name, display_name, description, billing_mode, price_per_gb::text, free_quota_bytes, subscription_price::text,
       included_traffic_bytes, subscription_period, traffic_reset_period, is_unlimited, status, created_at, updated_at
from pricing_rules
where id = $1 and status = 'active'`
	rule, err := r.scanPricingRule(ctx, query, pricingRuleID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("pricing rule not found")
		}
		return nil, err
	}
	return rule, nil
}

func (r *BillingRepository) insertUserSubscription(ctx context.Context, tx *sql.Tx, userID string, rule *domain.PricingRule, now time.Time) (*domain.UserSubscription, error) {
	currentPeriodEnd := computePeriodEnd(now, rule.TrafficResetPeriod)
	expiresAt := computePeriodEnd(now, rule.SubscriptionPeriod)

	const query = `
insert into user_subscriptions (
	user_id, pricing_rule_id, status, started_at, current_period_start, current_period_end,
	current_period_used_bytes, expires_at
)
values ($1, $2, 'active', $3, $3, $4, 0, $5)
returning id, user_id, pricing_rule_id, status, started_at, current_period_start, current_period_end,
          current_period_used_bytes, expires_at, cancelled_at, created_at, updated_at`

	var item domain.UserSubscription
	if err := tx.QueryRowContext(ctx, query, userID, rule.ID, now, currentPeriodEnd, expiresAt).Scan(
		&item.ID,
		&item.UserID,
		&item.PricingRuleID,
		&item.Status,
		&item.StartedAt,
		&item.CurrentPeriodStart,
		&item.CurrentPeriodEnd,
		&item.CurrentPeriodUsedBytes,
		&item.ExpiresAt,
		&item.CancelledAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("insert user subscription: %w", err)
	}
	return &item, nil
}

func (r *BillingRepository) extendUserSubscription(ctx context.Context, tx *sql.Tx, subscription *domain.UserSubscription, rule *domain.PricingRule) error {
	base := time.Now().UTC()
	if subscription.ExpiresAt != nil && subscription.ExpiresAt.After(base) {
		base = subscription.ExpiresAt.UTC()
	}

	nextExpiry := computePeriodEnd(base, rule.SubscriptionPeriod)
	if nextExpiry == nil {
		return fmt.Errorf("subscription pricing rule %s has invalid subscription period", rule.ID)
	}

	_, err := tx.ExecContext(ctx, `
update user_subscriptions
set expires_at = $2, updated_at = now()
where id = $1`,
		subscription.ID,
		nextExpiry,
	)
	if err != nil {
		return fmt.Errorf("extend user subscription: %w", err)
	}

	subscription.ExpiresAt = nextExpiry
	return nil
}

func (r *BillingRepository) advanceSubscriptionPeriodIfNeeded(ctx context.Context, tx *sql.Tx, subscription *domain.UserSubscription, rule *domain.PricingRule) error {
	if subscription.CurrentPeriodEnd == nil || rule.TrafficResetPeriod == "none" {
		return nil
	}

	now := time.Now().UTC()
	updated := false
	for subscription.CurrentPeriodEnd != nil && !now.Before(*subscription.CurrentPeriodEnd) {
		subscription.CurrentPeriodStart = *subscription.CurrentPeriodEnd
		subscription.CurrentPeriodEnd = computePeriodEnd(subscription.CurrentPeriodStart, rule.TrafficResetPeriod)
		subscription.CurrentPeriodUsedBytes = 0
		updated = true
	}
	if !updated {
		return nil
	}
	return r.updateSubscriptionState(ctx, tx, subscription)
}

func (r *BillingRepository) updateSubscriptionUsage(ctx context.Context, tx *sql.Tx, subscriptionID string, usedBytes int64) error {
	_, err := tx.ExecContext(ctx, `
update user_subscriptions
set current_period_used_bytes = $2, updated_at = now()
where id = $1`, subscriptionID, usedBytes)
	if err != nil {
		return fmt.Errorf("update subscription usage: %w", err)
	}
	return nil
}

func (r *BillingRepository) updateSubscriptionState(ctx context.Context, tx *sql.Tx, subscription *domain.UserSubscription) error {
	_, err := tx.ExecContext(ctx, `
update user_subscriptions
set current_period_start = $2,
    current_period_end = $3,
    current_period_used_bytes = $4,
    updated_at = now()
where id = $1`,
		subscription.ID,
		subscription.CurrentPeriodStart,
		subscription.CurrentPeriodEnd,
		subscription.CurrentPeriodUsedBytes,
	)
	if err != nil {
		return fmt.Errorf("update subscription state: %w", err)
	}
	return nil
}

func (r *BillingRepository) calculateChargeAmount(ctx context.Context, querier queryRower, bytes int64, pricePerGB string) (string, error) {
	const query = `
select round(($1::numeric / 1073741824) * $2::numeric, 4)::text`
	var chargeAmount string
	if err := querier.QueryRowContext(ctx, query, bytes, pricePerGB).Scan(&chargeAmount); err != nil {
		return "", fmt.Errorf("calculate charge amount: %w", err)
	}
	return chargeAmount, nil
}

func (r *BillingRepository) loadUnbilledUsageBytes(ctx context.Context, querier queryRower, userID string) (int64, error) {
	const query = `
select coalesce(sum(total_bytes - billed_bytes), 0)
from traffic_usages
where user_id = $1 and total_bytes > billed_bytes`
	var chargedBytes int64
	if err := querier.QueryRowContext(ctx, query, userID).Scan(&chargedBytes); err != nil {
		return 0, fmt.Errorf("query unbilled usage: %w", err)
	}
	return chargedBytes, nil
}

func (r *BillingRepository) markUsageBilled(ctx context.Context, tx *sql.Tx, userID string) error {
	if _, err := tx.ExecContext(ctx, `
update traffic_usages
set billed_bytes = total_bytes, updated_at = now()
where user_id = $1 and total_bytes > billed_bytes`, userID); err != nil {
		return fmt.Errorf("update billed usage: %w", err)
	}
	return nil
}

func (r *BillingRepository) ensureAccountQuerier(ctx context.Context, execer sqlExecer, userID string) error {
	_, err := execer.ExecContext(ctx, `
insert into accounts (user_id, balance, currency, status)
values ($1, 0, 'CNY', 'active')
on conflict (user_id) do nothing`, userID)
	return err
}

func (r *BillingRepository) lockAccount(ctx context.Context, tx *sql.Tx, userID string) (*domain.Account, error) {
	const query = `
select id, user_id, balance::text, currency, status, created_at, updated_at
from accounts
where user_id = $1
for update`

	var item domain.Account
	if err := tx.QueryRowContext(ctx, query, userID).Scan(
		&item.ID,
		&item.UserID,
		&item.Balance,
		&item.Currency,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *BillingRepository) updateAccountBalance(ctx context.Context, tx *sql.Tx, userID, amountBytes string, increase bool) (*domain.Account, error) {
	operator := "-"
	if increase {
		operator = "+"
	}
	query := fmt.Sprintf(`
update accounts
set balance = balance %s $2::bigint, updated_at = now()
where user_id = $1
returning id, user_id, balance::text, currency, status, created_at, updated_at`, operator)

	var item domain.Account
	if err := tx.QueryRowContext(ctx, query, userID, amountBytes).Scan(
		&item.ID,
		&item.UserID,
		&item.Balance,
		&item.Currency,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *BillingRepository) accountHasEnoughBalance(ctx context.Context, querier queryRower, userID, amount string) (bool, error) {
	const query = `
select balance >= $2::bigint
from accounts
where user_id = $1`
	var enough bool
	if err := querier.QueryRowContext(ctx, query, userID, amount).Scan(&enough); err != nil {
		return false, err
	}
	return enough, nil
}

type createTransactionParams struct {
	UserID              string
	AccountID           string
	RecordType          string
	ChangeAmount        string
	TrafficBefore       string
	TrafficAfter        string
	RelatedResourceType string
	RelatedResourceID   *string
	TrafficBytes        int64
	BillableBytes       int64
	PackageExpiresAt    *time.Time
	PaymentOrderBizID   string
	Description         string
}

func (r *BillingRepository) insertAccountTransaction(ctx context.Context, tx *sql.Tx, params createTransactionParams) (*domain.UserBusinessRecord, error) {
	legacyType := toLegacyTransactionType(params.RecordType, params.ChangeAmount)

	const query = `
insert into user_business_records (
	user_id, account_id, type, amount, balance_before, balance_after, reference_type, reference_id, remark,
	record_type, related_resource_type, related_resource_id, traffic_bytes, billable_bytes, package_expires_at, payment_order_biz_id, description
)
values ($1, $2, $3, $4::numeric, $5::numeric, $6::numeric, $7, $8, $9, $10, $11, $12, $13, $14, $15, nullif($16, ''), $17)
returning id, user_id, account_id, record_type, amount::text, balance_before::text, balance_after::text,
          related_resource_type, related_resource_id, coalesce(traffic_bytes, 0), coalesce(billable_bytes, 0),
          package_expires_at, coalesce(payment_order_biz_id, ''), description, created_at`

	var item domain.UserBusinessRecord
	if err := tx.QueryRowContext(
		ctx,
		query,
		params.UserID,
		params.AccountID,
		legacyType,
		params.ChangeAmount,
		params.TrafficBefore,
		params.TrafficAfter,
		params.RecordType,
		params.RelatedResourceID,
		params.Description,
		params.RecordType,
		params.RelatedResourceType,
		params.RelatedResourceID,
		params.TrafficBytes,
		params.BillableBytes,
		params.PackageExpiresAt,
		params.PaymentOrderBizID,
		params.Description,
	).Scan(
		&item.ID,
		&item.UserID,
		&item.AccountID,
		&item.RecordType,
		&item.ChangeAmount,
		&item.TrafficBefore,
		&item.TrafficAfter,
		&item.RelatedResourceType,
		&item.RelatedResourceID,
		&item.TrafficBytes,
		&item.BillableBytes,
		&item.PackageExpiresAt,
		&item.PaymentOrderBizID,
		&item.Description,
		&item.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func toLegacyTransactionType(recordType, changeAmount string) string {
	switch recordType {
	case "traffic_recharge":
		return "recharge"
	case "subscription_purchase", "subscription_renew", "traffic_settlement", "subscription_traffic_settlement":
		return "consume"
	}

	if strings.HasPrefix(strings.TrimSpace(changeAmount), "-") {
		return "consume"
	}
	return "adjust"
}

func (r *BillingRepository) updateBusinessRecordPackageExpiry(ctx context.Context, tx *sql.Tx, recordID string, expiresAt *time.Time) error {
	_, err := tx.ExecContext(ctx, `
update user_business_records
set package_expires_at = $2
where id = $1`,
		recordID,
		expiresAt,
	)
	if err != nil {
		return fmt.Errorf("update business record package expiry: %w", err)
	}
	return nil
}

func (r *BillingRepository) balanceAmountToBytes(ctx context.Context, querier queryRower, amount string) (string, error) {
	const query = `
select round($1::numeric * 1073741824)::bigint::text`
	var balanceBytes string
	if err := querier.QueryRowContext(ctx, query, amount).Scan(&balanceBytes); err != nil {
		return "", err
	}
	return balanceBytes, nil
}

func (r *BillingRepository) scanPricingRule(ctx context.Context, query string, args ...any) (*domain.PricingRule, error) {
	return scanPricingRule(r.db.QueryRowContext(ctx, query, args...))
}

func scanPricingRule(row scanner) (*domain.PricingRule, error) {
	var item domain.PricingRule
	if err := row.Scan(
		&item.ID,
		&item.Name,
		&item.DisplayName,
		&item.Description,
		&item.BillingMode,
		&item.PricePerGB,
		&item.FreeQuotaBytes,
		&item.SubscriptionPrice,
		&item.IncludedTrafficBytes,
		&item.SubscriptionPeriod,
		&item.TrafficResetPeriod,
		&item.IsUnlimited,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if item.IncludedTrafficBytes == 0 && item.FreeQuotaBytes > 0 {
		item.IncludedTrafficBytes = item.FreeQuotaBytes
	}
	return &item, nil
}

func scanSubscriptionWithRule(row scanner) (*domain.UserSubscription, *domain.PricingRule, error) {
	var subscription domain.UserSubscription
	rule := &domain.PricingRule{}
	if err := row.Scan(
		&subscription.ID,
		&subscription.UserID,
		&subscription.PricingRuleID,
		&subscription.Status,
		&subscription.StartedAt,
		&subscription.CurrentPeriodStart,
		&subscription.CurrentPeriodEnd,
		&subscription.CurrentPeriodUsedBytes,
		&subscription.ExpiresAt,
		&subscription.CancelledAt,
		&subscription.CreatedAt,
		&subscription.UpdatedAt,
		&rule.ID,
		&rule.Name,
		&rule.DisplayName,
		&rule.Description,
		&rule.BillingMode,
		&rule.PricePerGB,
		&rule.FreeQuotaBytes,
		&rule.SubscriptionPrice,
		&rule.IncludedTrafficBytes,
		&rule.SubscriptionPeriod,
		&rule.TrafficResetPeriod,
		&rule.IsUnlimited,
		&rule.Status,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	); err != nil {
		return nil, nil, err
	}
	if rule.IncludedTrafficBytes == 0 && rule.FreeQuotaBytes > 0 {
		rule.IncludedTrafficBytes = rule.FreeQuotaBytes
	}
	return &subscription, rule, nil
}

func computePeriodEnd(start time.Time, period string) *time.Time {
	switch period {
	case "month":
		end := start.AddDate(0, 1, 0)
		return &end
	case "year":
		end := start.AddDate(1, 0, 0)
		return &end
	default:
		return nil
	}
}

func isZeroAmount(amount string) bool {
	normalized := strings.TrimSpace(amount)
	normalized = strings.TrimPrefix(normalized, "+")
	normalized = strings.TrimLeft(normalized, "0")
	normalized = strings.TrimPrefix(normalized, ".")
	return normalized == ""
}

func negateNumericString(amount string) string {
	value := strings.TrimSpace(amount)
	if value == "" || value == "0" || value == "0.0" || value == "0.00" || value == "0.0000" {
		return "0.0000"
	}
	if strings.HasPrefix(value, "-") {
		return value
	}
	return "-" + value
}

type scanner interface {
	Scan(dest ...any) error
}

type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}
