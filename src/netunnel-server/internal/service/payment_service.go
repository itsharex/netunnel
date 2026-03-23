package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"netunnel/server/internal/domain"
	"netunnel/server/internal/repository"
)

const paymentOpenAPIBaseURL = "https://open.tx07.cn/api/v1/apps/app_mmzvo9v9e89cc5bbda9611551902/payment"

type PaymentService struct {
	orders           *repository.PaymentOrderRepository
	billing          *BillingService
	publicAPIBaseURL string
	httpClient       *http.Client
}

type CreatePaymentOrderInput struct {
	UserID           string `json:"user_id"`
	OrderType        string `json:"order_type"`
	PaymentProductID string `json:"payment_product_id"`
	PricingRuleID    string `json:"pricing_rule_id"`
	RechargeGB       int    `json:"recharge_gb"`
}

type PaymentOrderSnapshot struct {
	Order          *domain.PaymentOrder         `json:"order"`
	Session        *paymentSessionData          `json:"session,omitempty"`
	Applied        bool                         `json:"applied"`
	AppliedAt      *time.Time                   `json:"appliedAt,omitempty"`
	ApplyError     string                       `json:"applyError,omitempty"`
	Account        *domain.Account              `json:"account,omitempty"`
	Transaction    *domain.UserBusinessRecord   `json:"transaction,omitempty"`
	Subscription   *domain.UserSubscription     `json:"subscription,omitempty"`
	PricingRule    *domain.PricingRule          `json:"pricing_rule,omitempty"`
	BusinessNotify *paymentBusinessNotifyStatus `json:"businessNotify,omitempty"`
}

type PaymentNotifyInput struct {
	AppID          string             `json:"appId"`
	BizID          string             `json:"bizId"`
	SessionID      string             `json:"sessionId"`
	Status         string             `json:"status"`
	Amount         int                `json:"amount"`
	NotifyURL      string             `json:"notifyUrl"`
	PaidAt         *time.Time         `json:"paidAt"`
	PaymentProduct paymentProductInfo `json:"paymentProduct"`
}

type paymentCreateSessionRequest struct {
	PaymentProductID string `json:"paymentProductId"`
	BizID            string `json:"bizId"`
	NotifyURL        string `json:"notifyUrl"`
}

type paymentProductInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       int    `json:"price"`
}

type paymentBusinessNotifyStatus struct {
	Status     string     `json:"status"`
	Attempts   int        `json:"attempts"`
	NotifiedAt *time.Time `json:"notifiedAt"`
	Response   string     `json:"response"`
	Error      string     `json:"error"`
}

type paymentSessionData struct {
	SessionID      string                       `json:"sessionId"`
	AppID          string                       `json:"appId"`
	BizID          string                       `json:"bizId"`
	Status         string                       `json:"status"`
	Amount         int                          `json:"amount"`
	NotifyURL      string                       `json:"notifyUrl"`
	QRCodeURL      string                       `json:"qrCodeUrl"`
	CheckoutURL    string                       `json:"checkoutUrl"`
	PollURL        string                       `json:"pollUrl"`
	ExpiresAt      *time.Time                   `json:"expiresAt"`
	PaidAt         *time.Time                   `json:"paidAt"`
	PaymentProduct paymentProductInfo           `json:"paymentProduct"`
	BusinessNotify *paymentBusinessNotifyStatus `json:"businessNotify,omitempty"`
}

type paymentOpenAPIResponse struct {
	Code int                `json:"code"`
	Msg  string             `json:"msg"`
	Data paymentSessionData `json:"data"`
}

func NewPaymentService(orders *repository.PaymentOrderRepository, billing *BillingService, publicAPIBaseURL string) *PaymentService {
	return &PaymentService{
		orders:           orders,
		billing:          billing,
		publicAPIBaseURL: strings.TrimRight(strings.TrimSpace(publicAPIBaseURL), "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *PaymentService) CreatePaymentOrder(ctx context.Context, input CreatePaymentOrderInput) (*PaymentOrderSnapshot, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.OrderType = strings.TrimSpace(input.OrderType)
	input.PaymentProductID = strings.TrimSpace(input.PaymentProductID)
	input.PricingRuleID = strings.TrimSpace(input.PricingRuleID)

	if input.UserID == "" || input.OrderType == "" || input.PaymentProductID == "" {
		return nil, fmt.Errorf("%w: user_id, order_type and payment_product_id are required", ErrInvalidArgument)
	}
	if s.publicAPIBaseURL == "" {
		return nil, fmt.Errorf("public api base url is required")
	}

	switch input.OrderType {
	case "traffic_recharge":
		if input.RechargeGB <= 0 {
			return nil, fmt.Errorf("%w: recharge_gb is required", ErrInvalidArgument)
		}
	case "pricing_rule":
		if input.PricingRuleID == "" {
			return nil, fmt.Errorf("%w: pricing_rule_id is required", ErrInvalidArgument)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported order_type", ErrInvalidArgument)
	}

	bizID, err := generatePaymentBizID()
	if err != nil {
		return nil, err
	}
	notifyURL := s.publicAPIBaseURL + "/api/v1/payments/notify"

	_, err = s.orders.Create(ctx, repository.CreatePaymentOrderParams{
		BizID:            bizID,
		UserID:           input.UserID,
		OrderType:        input.OrderType,
		PaymentProductID: input.PaymentProductID,
		PricingRuleID:    input.PricingRuleID,
		RechargeGB:       input.RechargeGB,
		NotifyURL:        notifyURL,
	})
	if err != nil {
		return nil, fmt.Errorf("create payment order: %w", err)
	}

	session, rawSnapshot, err := s.createPlatformSession(ctx, paymentCreateSessionRequest{
		PaymentProductID: input.PaymentProductID,
		BizID:            bizID,
		NotifyURL:        notifyURL,
	})
	if err != nil {
		return nil, err
	}
	if err := s.updateOrderFromSession(ctx, bizID, session, rawSnapshot); err != nil {
		return nil, err
	}

	updatedOrder, err := s.orders.GetByBizID(ctx, bizID)
	if err != nil {
		return nil, err
	}
	return &PaymentOrderSnapshot{
		Order:          updatedOrder,
		Session:        session,
		BusinessNotify: session.BusinessNotify,
	}, nil
}

func (s *PaymentService) PollPaymentOrder(ctx context.Context, bizID string) (*PaymentOrderSnapshot, error) {
	bizID = strings.TrimSpace(bizID)
	if bizID == "" {
		return nil, fmt.Errorf("%w: biz_id is required", ErrInvalidArgument)
	}

	order, err := s.orders.GetByBizID(ctx, bizID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: payment order not found", ErrNotFound)
		}
		return nil, err
	}

	session, rawSnapshot, err := s.pollPlatformSession(ctx, bizID)
	if err != nil {
		return nil, err
	}
	if err := s.updateOrderFromSession(ctx, bizID, session, rawSnapshot); err != nil {
		return nil, err
	}

	snapshot := &PaymentOrderSnapshot{
		Session:        session,
		Applied:        order.ApplyStatus == "applied",
		ApplyError:     order.ApplyError,
		BusinessNotify: session.BusinessNotify,
	}

	if session.Status == "paid" {
		account, transaction, subscription, pricingRule, appliedAt, applyErr := s.applyPaidOrder(ctx, bizID)
		snapshot.Applied = applyErr == ""
		snapshot.ApplyError = applyErr
		snapshot.Account = account
		snapshot.Transaction = transaction
		snapshot.Subscription = subscription
		snapshot.PricingRule = pricingRule
		snapshot.AppliedAt = appliedAt
	}

	updatedOrder, err := s.orders.GetByBizID(ctx, bizID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: payment order not found", ErrNotFound)
		}
		return nil, err
	}
	snapshot.Order = updatedOrder
	snapshot.Applied = updatedOrder.ApplyStatus == "applied"
	snapshot.ApplyError = updatedOrder.ApplyError
	return snapshot, nil
}

func (s *PaymentService) HandleNotify(ctx context.Context, input PaymentNotifyInput) error {
	input.BizID = strings.TrimSpace(input.BizID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.Status = strings.TrimSpace(input.Status)
	input.NotifyURL = strings.TrimSpace(input.NotifyURL)
	input.PaymentProduct.ID = strings.TrimSpace(input.PaymentProduct.ID)

	if input.BizID == "" {
		return fmt.Errorf("%w: bizId is required", ErrInvalidArgument)
	}

	order, err := s.orders.GetByBizID(ctx, input.BizID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("%w: payment order not found", ErrNotFound)
		}
		return err
	}

	session, rawSnapshot, err := s.pollPlatformSession(ctx, input.BizID)
	if err != nil {
		return fmt.Errorf("verify payment notify via platform: %w", err)
	}
	if err := s.validatePlatformPaidSession(order, session, input); err != nil {
		return err
	}
	if err := s.orders.UpdateSession(ctx, repository.UpdatePaymentOrderSessionParams{
		BizID:                input.BizID,
		SessionID:            session.SessionID,
		PollURL:              session.PollURL,
		QRCodeURL:            session.QRCodeURL,
		CheckoutURL:          session.CheckoutURL,
		Amount:               session.Amount,
		PlatformStatus:       session.Status,
		BusinessNotifyStatus: businessNotifyStatusValue(session.BusinessNotify),
		BusinessNotifyError:  businessNotifyErrorValue(session.BusinessNotify),
		ExpiresAt:            session.ExpiresAt,
		PaidAt:               session.PaidAt,
		RawSnapshot:          rawSnapshot,
	}); err != nil {
		return err
	}

	_, _, _, _, _, applyErr := s.applyPaidOrder(ctx, input.BizID)
	if applyErr != "" {
		return fmt.Errorf(applyErr)
	}
	return nil
}

func (s *PaymentService) createPlatformSession(ctx context.Context, payload paymentCreateSessionRequest) (*paymentSessionData, string, error) {
	rawBody, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("marshal create payment session: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, paymentOpenAPIBaseURL+"/sessions", bytes.NewReader(rawBody))
	if err != nil {
		return nil, "", fmt.Errorf("create payment session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request payment session: %w", err)
	}
	defer resp.Body.Close()

	return parsePaymentOpenAPIResponse(resp)
}

func (s *PaymentService) pollPlatformSession(ctx context.Context, bizID string) (*paymentSessionData, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, paymentOpenAPIBaseURL+"/sessions/by-biz/"+bizID, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create poll payment session request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("poll payment session: %w", err)
	}
	defer resp.Body.Close()

	return parsePaymentOpenAPIResponse(resp)
}

func parsePaymentOpenAPIResponse(resp *http.Response) (*paymentSessionData, string, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read payment response: %w", err)
	}

	var payload paymentOpenAPIResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, string(body), fmt.Errorf("decode payment response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, string(body), fmt.Errorf(payload.Msg)
	}
	if payload.Code != 200 {
		return nil, string(body), fmt.Errorf(payload.Msg)
	}
	return &payload.Data, string(body), nil
}

func (s *PaymentService) validatePlatformPaidSession(order *domain.PaymentOrder, session *paymentSessionData, input PaymentNotifyInput) error {
	if order == nil || session == nil {
		return fmt.Errorf("payment order verification failed")
	}
	if strings.TrimSpace(session.BizID) != order.BizID {
		return fmt.Errorf("payment biz id mismatch")
	}
	if session.Status != "paid" {
		return fmt.Errorf("payment is not marked paid by platform")
	}
	if session.PaymentProduct.ID == "" || session.PaymentProduct.ID != order.PaymentProductID {
		return fmt.Errorf("payment product mismatch")
	}
	if session.Amount <= 0 {
		return fmt.Errorf("payment amount is invalid")
	}
	if input.SessionID != "" && session.SessionID != input.SessionID {
		return fmt.Errorf("payment session id mismatch")
	}
	if input.PaymentProduct.ID != "" && session.PaymentProduct.ID != input.PaymentProduct.ID {
		return fmt.Errorf("payment notify product mismatch")
	}
	if input.Amount > 0 && session.Amount != input.Amount {
		return fmt.Errorf("payment amount mismatch")
	}
	return nil
}

func (s *PaymentService) updateOrderFromSession(ctx context.Context, bizID string, session *paymentSessionData, rawSnapshot string) error {
	if session == nil {
		return nil
	}
	return s.orders.UpdateSession(ctx, repository.UpdatePaymentOrderSessionParams{
		BizID:                bizID,
		SessionID:            session.SessionID,
		PollURL:              session.PollURL,
		QRCodeURL:            session.QRCodeURL,
		CheckoutURL:          session.CheckoutURL,
		Amount:               session.Amount,
		PlatformStatus:       session.Status,
		BusinessNotifyStatus: businessNotifyStatusValue(session.BusinessNotify),
		BusinessNotifyError:  businessNotifyErrorValue(session.BusinessNotify),
		ExpiresAt:            session.ExpiresAt,
		PaidAt:               session.PaidAt,
		RawSnapshot:          rawSnapshot,
	})
}

func (s *PaymentService) applyPaidOrder(ctx context.Context, bizID string) (*domain.Account, *domain.UserBusinessRecord, *domain.UserSubscription, *domain.PricingRule, *time.Time, string) {
	shouldApply, err := s.orders.TryMarkApplying(ctx, bizID)
	if err != nil {
		return nil, nil, nil, nil, nil, err.Error()
	}
	if !shouldApply {
		order, getErr := s.orders.GetByBizID(ctx, bizID)
		if getErr != nil {
			return nil, nil, nil, nil, nil, getErr.Error()
		}
		if order.PaidAt != nil {
			return nil, nil, nil, nil, order.PaidAt, order.ApplyError
		}
		return nil, nil, nil, nil, nil, order.ApplyError
	}

	order, err := s.orders.GetByBizID(ctx, bizID)
	if err != nil {
		_ = s.orders.MarkApplyFailed(ctx, bizID, err.Error())
		return nil, nil, nil, nil, nil, err.Error()
	}

	switch order.OrderType {
	case "traffic_recharge":
		account, transaction, rechargeErr := s.billing.RechargeManual(ctx, ManualRechargeInput{
			UserID: order.UserID,
			Amount: fmt.Sprintf("%d", order.RechargeGB),
			Remark: fmt.Sprintf("payment recharge biz=%s product=%s", order.BizID, order.PaymentProductID),
		})
		if rechargeErr != nil {
			_ = s.orders.MarkApplyFailed(ctx, bizID, rechargeErr.Error())
			return nil, nil, nil, nil, nil, rechargeErr.Error()
		}
		if err := s.orders.MarkApplied(ctx, bizID); err != nil {
			return nil, nil, nil, nil, nil, err.Error()
		}
		return account, transaction, nil, nil, order.PaidAt, ""
	case "pricing_rule":
		result, activateErr := s.billing.ActivatePricingRuleAfterExternalPayment(ctx, ActivatePricingRuleInput{
			UserID:        order.UserID,
			PricingRuleID: order.PricingRuleID,
		})
		if activateErr != nil {
			_ = s.orders.MarkApplyFailed(ctx, bizID, activateErr.Error())
			return nil, nil, nil, nil, nil, activateErr.Error()
		}
		if err := s.orders.MarkApplied(ctx, bizID); err != nil {
			return nil, nil, nil, nil, nil, err.Error()
		}
		return result.Account, result.Transaction, result.Subscription, &result.PricingRule, order.PaidAt, ""
	default:
		err := fmt.Errorf("unsupported payment order type: %s", order.OrderType)
		_ = s.orders.MarkApplyFailed(ctx, bizID, err.Error())
		return nil, nil, nil, nil, nil, err.Error()
	}
}

func businessNotifyStatusValue(status *paymentBusinessNotifyStatus) string {
	if status == nil {
		return ""
	}
	return strings.TrimSpace(status.Status)
}

func businessNotifyErrorValue(status *paymentBusinessNotifyStatus) string {
	if status == nil {
		return ""
	}
	return strings.TrimSpace(status.Error)
}

func generatePaymentBizID() (string, error) {
	var token [12]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate payment biz id: %w", err)
	}
	return "pay_" + hex.EncodeToString(token[:]), nil
}
