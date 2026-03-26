package domain

import "time"

// User represents the minimal user aggregate currently needed by the server.
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email,omitempty"`
	Nickname     string     `json:"nickname"`
	AvatarURL    string     `json:"avatar_url,omitempty"`
	PasswordHash string     `json:"-"`
	WechatOpenid string     `json:"wechat_openid,omitempty"`
	Status       string     `json:"status"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Agent represents a client installation managed by the service.
type Agent struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	Name            string     `json:"name"`
	MachineCode     string     `json:"machine_code"`
	SecretKey       string     `json:"secret_key,omitempty"`
	Status          string     `json:"status"`
	ClientVersion   string     `json:"client_version,omitempty"`
	OSType          string     `json:"os_type,omitempty"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Tunnel represents the externally exposed routing configuration.
type Tunnel struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	AgentID      string    `json:"agent_id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	Enabled      bool      `json:"enabled"`
	LocalHost    string    `json:"local_host"`
	LocalPort    int       `json:"local_port"`
	RemotePort   *int      `json:"remote_port,omitempty"`
	AccessTarget string    `json:"access_target,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type DomainRoute struct {
	ID        string    `json:"id"`
	TunnelID  string    `json:"tunnel_id"`
	Domain    string    `json:"domain"`
	Scheme    string    `json:"scheme"`
	AccessURL string    `json:"access_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TunnelConnectionStart struct {
	TunnelID   string
	AgentID    string
	Protocol   string
	SourceAddr string
	TargetAddr string
}

type TunnelConnectionFinish struct {
	ConnectionID string
	UserID       string
	AgentID      string
	TunnelID     string
	IngressBytes int64
	EgressBytes  int64
	Status       string
}

type TunnelConnectionProgress struct {
	ConnectionID string
	UserID       string
	AgentID      string
	TunnelID     string
	IngressBytes int64
	EgressBytes  int64
	Status       string
}

type TunnelConnection struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	TunnelID     string     `json:"tunnel_id"`
	AgentID      *string    `json:"agent_id,omitempty"`
	Protocol     string     `json:"protocol"`
	SourceAddr   string     `json:"source_addr,omitempty"`
	TargetAddr   string     `json:"target_addr,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	IngressBytes int64      `json:"ingress_bytes"`
	EgressBytes  int64      `json:"egress_bytes"`
	TotalBytes   int64      `json:"total_bytes"`
	Status       string     `json:"status"`
}

type TrafficUsage struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	AgentID      *string   `json:"agent_id,omitempty"`
	TunnelID     *string   `json:"tunnel_id,omitempty"`
	BucketTime   time.Time `json:"bucket_time"`
	IngressBytes int64     `json:"ingress_bytes"`
	EgressBytes  int64     `json:"egress_bytes"`
	TotalBytes   int64     `json:"total_bytes"`
	BilledBytes  int64     `json:"billed_bytes"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Account struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Balance   string    `json:"balance"`
	Currency  string    `json:"currency"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserBusinessRecord struct {
	ID                  string     `json:"id"`
	UserID              string     `json:"user_id"`
	AccountID           string     `json:"account_id"`
	RecordType          string     `json:"record_type"`
	ChangeAmount        string     `json:"change_amount"`
	TrafficBefore       string     `json:"traffic_balance_before"`
	TrafficAfter        string     `json:"traffic_balance_after"`
	RelatedResourceType string     `json:"related_resource_type,omitempty"`
	RelatedResourceID   *string    `json:"related_resource_id,omitempty"`
	TrafficBytes        int64      `json:"traffic_bytes,omitempty"`
	BillableBytes       int64      `json:"billable_bytes,omitempty"`
	PackageExpiresAt    *time.Time `json:"package_expires_at,omitempty"`
	PaymentOrderBizID   string     `json:"payment_order_biz_id,omitempty"`
	Description         string     `json:"description,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

type PaymentOrder struct {
	BizID                string     `json:"biz_id"`
	UserID               string     `json:"user_id"`
	OrderType            string     `json:"order_type"`
	PaymentProductID     string     `json:"payment_product_id"`
	PricingRuleID        string     `json:"pricing_rule_id,omitempty"`
	RechargeGB           int        `json:"recharge_gb,omitempty"`
	SessionID            string     `json:"session_id,omitempty"`
	NotifyURL            string     `json:"notify_url"`
	PollURL              string     `json:"poll_url,omitempty"`
	QRCodeURL            string     `json:"qr_code_url,omitempty"`
	CheckoutURL          string     `json:"checkout_url,omitempty"`
	Amount               int        `json:"amount"`
	PlatformStatus       string     `json:"platform_status"`
	ApplyStatus          string     `json:"apply_status"`
	BusinessNotifyStatus string     `json:"business_notify_status,omitempty"`
	BusinessNotifyError  string     `json:"business_notify_error,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	PaidAt               *time.Time `json:"paid_at,omitempty"`
	LastPolledAt         *time.Time `json:"last_polled_at,omitempty"`
	ApplyError           string     `json:"apply_error,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type PricingRule struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	DisplayName          string    `json:"display_name"`
	Description          string    `json:"description"`
	BillingMode          string    `json:"billing_mode"`
	PricePerGB           string    `json:"price_per_gb"`
	FreeQuotaBytes       int64     `json:"free_quota_bytes"`
	SubscriptionPrice    string    `json:"subscription_price"`
	IncludedTrafficBytes int64     `json:"included_traffic_bytes"`
	SubscriptionPeriod   string    `json:"subscription_period"`
	TrafficResetPeriod   string    `json:"traffic_reset_period"`
	IsUnlimited          bool      `json:"is_unlimited"`
	Status               string    `json:"status"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type UserSubscription struct {
	ID                     string     `json:"id"`
	UserID                 string     `json:"user_id"`
	PricingRuleID          string     `json:"pricing_rule_id"`
	Status                 string     `json:"status"`
	StartedAt              time.Time  `json:"started_at"`
	CurrentPeriodStart     time.Time  `json:"current_period_start"`
	CurrentPeriodEnd       *time.Time `json:"current_period_end,omitempty"`
	CurrentPeriodUsedBytes int64      `json:"current_period_used_bytes"`
	ExpiresAt              *time.Time `json:"expires_at,omitempty"`
	CancelledAt            *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type DashboardSummary struct {
	UserID                  string               `json:"user_id"`
	Account                 Account              `json:"account"`
	TotalUsers              int                  `json:"total_users"`
	OnlineUsers             int                  `json:"online_users"`
	TotalAgents             int                  `json:"total_agents"`
	OnlineAgents            int                  `json:"online_agents"`
	TotalTunnels            int                  `json:"total_tunnels"`
	EnabledTunnels          int                  `json:"enabled_tunnels"`
	DisabledBillingTunnels  int                  `json:"disabled_billing_tunnels"`
	RecentTrafficBytes24h   int64                `json:"recent_traffic_bytes_24h"`
	UnbilledTrafficBytes24h int64                `json:"unbilled_traffic_bytes_24h"`
	RecentBusinessRecords   []UserBusinessRecord `json:"recent_business_records"`
	RecentUsages            []TrafficUsage       `json:"recent_usages"`
}
