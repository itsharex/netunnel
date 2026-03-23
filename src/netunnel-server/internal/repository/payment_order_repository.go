package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"netunnel/server/internal/domain"
)

type PaymentOrderRepository struct {
	db *sql.DB
}

type CreatePaymentOrderParams struct {
	BizID            string
	UserID           string
	OrderType        string
	PaymentProductID string
	PricingRuleID    string
	RechargeGB       int
	NotifyURL        string
}

type UpdatePaymentOrderSessionParams struct {
	BizID                string
	SessionID            string
	PollURL              string
	QRCodeURL            string
	CheckoutURL          string
	Amount               int
	PlatformStatus       string
	BusinessNotifyStatus string
	BusinessNotifyError  string
	ExpiresAt            *time.Time
	PaidAt               *time.Time
	RawSnapshot          string
}

func NewPaymentOrderRepository(db *sql.DB) *PaymentOrderRepository {
	return &PaymentOrderRepository{db: db}
}

func (r *PaymentOrderRepository) Create(ctx context.Context, params CreatePaymentOrderParams) (*domain.PaymentOrder, error) {
	const query = `
insert into payment_orders (
  biz_id, user_id, order_type, payment_product_id, pricing_rule_id, recharge_gb, notify_url
)
values ($1, $2, $3, $4, nullif($5, ''), nullif($6, 0), $7)
returning biz_id, user_id, order_type, payment_product_id, coalesce(pricing_rule_id, ''), coalesce(recharge_gb, 0),
          coalesce(session_id, ''), notify_url, coalesce(poll_url, ''), coalesce(qr_code_url, ''), coalesce(checkout_url, ''),
          amount, platform_status, apply_status, coalesce(business_notify_status, ''), coalesce(business_notify_error, ''),
          expires_at, paid_at, last_polled_at, coalesce(apply_error, ''), created_at, updated_at`

	return scanPaymentOrder(r.db.QueryRowContext(
		ctx,
		query,
		params.BizID,
		params.UserID,
		params.OrderType,
		params.PaymentProductID,
		params.PricingRuleID,
		params.RechargeGB,
		params.NotifyURL,
	))
}

func (r *PaymentOrderRepository) GetByBizID(ctx context.Context, bizID string) (*domain.PaymentOrder, error) {
	const query = `
select biz_id, user_id, order_type, payment_product_id, coalesce(pricing_rule_id, ''), coalesce(recharge_gb, 0),
       coalesce(session_id, ''), notify_url, coalesce(poll_url, ''), coalesce(qr_code_url, ''), coalesce(checkout_url, ''),
       amount, platform_status, apply_status, coalesce(business_notify_status, ''), coalesce(business_notify_error, ''),
       expires_at, paid_at, last_polled_at, coalesce(apply_error, ''), created_at, updated_at
from payment_orders
where biz_id = $1`

	return scanPaymentOrder(r.db.QueryRowContext(ctx, query, bizID))
}

func (r *PaymentOrderRepository) UpdateSession(ctx context.Context, params UpdatePaymentOrderSessionParams) error {
	const query = `
update payment_orders
set session_id = nullif($2, ''),
    poll_url = nullif($3, ''),
    qr_code_url = nullif($4, ''),
    checkout_url = nullif($5, ''),
    amount = $6,
    platform_status = $7,
    business_notify_status = nullif($8, ''),
    business_notify_error = nullif($9, ''),
    expires_at = $10,
    paid_at = $11,
    raw_snapshot = $12,
    last_polled_at = now(),
    updated_at = now()
where biz_id = $1`

	if _, err := r.db.ExecContext(
		ctx,
		query,
		params.BizID,
		params.SessionID,
		params.PollURL,
		params.QRCodeURL,
		params.CheckoutURL,
		params.Amount,
		params.PlatformStatus,
		params.BusinessNotifyStatus,
		params.BusinessNotifyError,
		params.ExpiresAt,
		params.PaidAt,
		params.RawSnapshot,
	); err != nil {
		return fmt.Errorf("update payment order session: %w", err)
	}
	return nil
}

func (r *PaymentOrderRepository) TryMarkApplying(ctx context.Context, bizID string) (bool, error) {
	const query = `
update payment_orders
set apply_status = 'processing', apply_error = null, updated_at = now()
where biz_id = $1 and apply_status in ('pending', 'failed')`

	result, err := r.db.ExecContext(ctx, query, bizID)
	if err != nil {
		return false, fmt.Errorf("mark payment order applying: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("payment order applying rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

func (r *PaymentOrderRepository) MarkApplied(ctx context.Context, bizID string) error {
	const query = `
update payment_orders
set apply_status = 'applied', apply_error = null, updated_at = now()
where biz_id = $1`

	if _, err := r.db.ExecContext(ctx, query, bizID); err != nil {
		return fmt.Errorf("mark payment order applied: %w", err)
	}
	return nil
}

func (r *PaymentOrderRepository) MarkApplyFailed(ctx context.Context, bizID, applyError string) error {
	const query = `
update payment_orders
set apply_status = 'failed', apply_error = $2, updated_at = now()
where biz_id = $1`

	if _, err := r.db.ExecContext(ctx, query, bizID, applyError); err != nil {
		return fmt.Errorf("mark payment order failed: %w", err)
	}
	return nil
}

func scanPaymentOrder(row *sql.Row) (*domain.PaymentOrder, error) {
	var item domain.PaymentOrder
	if err := row.Scan(
		&item.BizID,
		&item.UserID,
		&item.OrderType,
		&item.PaymentProductID,
		&item.PricingRuleID,
		&item.RechargeGB,
		&item.SessionID,
		&item.NotifyURL,
		&item.PollURL,
		&item.QRCodeURL,
		&item.CheckoutURL,
		&item.Amount,
		&item.PlatformStatus,
		&item.ApplyStatus,
		&item.BusinessNotifyStatus,
		&item.BusinessNotifyError,
		&item.ExpiresAt,
		&item.PaidAt,
		&item.LastPolledAt,
		&item.ApplyError,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}
