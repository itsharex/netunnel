-- 网跃通 PostgreSQL 初始化草案
-- 版本: v0.1
-- 日期: 2026-03-20

create extension if not exists pgcrypto;

create table if not exists users (
    id uuid primary key default gen_random_uuid(),
    email varchar(255) unique,
    phone varchar(32) unique,
    password_hash varchar(255) not null,
    nickname varchar(128) not null,
    avatar_url text,
    status varchar(32) not null default 'active',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists access_tokens (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    token varchar(255) not null unique,
    expires_at timestamptz not null,
    created_at timestamptz not null default now(),
    revoked_at timestamptz
);
create index if not exists idx_access_tokens_user_id on access_tokens(user_id);

create table if not exists agents (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    name varchar(128) not null,
    machine_code varchar(255) not null,
    secret_key varchar(255) not null,
    status varchar(32) not null default 'offline',
    client_version varchar(64),
    os_type varchar(64),
    last_heartbeat_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (user_id, machine_code)
);
create index if not exists idx_agents_user_id on agents(user_id);
create index if not exists idx_agents_status on agents(status);

create table if not exists tunnels (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    agent_id uuid not null references agents(id) on delete cascade,
    name varchar(128) not null,
    type varchar(32) not null,
    status varchar(32) not null default 'created',
    enabled boolean not null default true,
    local_host varchar(255) not null,
    local_port integer not null,
    remote_port integer,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint chk_tunnels_type check (type in ('tcp', 'http_host')),
    constraint chk_tunnels_local_port check (local_port > 0 and local_port <= 65535),
    constraint chk_tunnels_remote_port check (remote_port is null or (remote_port > 0 and remote_port <= 65535))
);
create index if not exists idx_tunnels_user_id on tunnels(user_id);
create index if not exists idx_tunnels_agent_id on tunnels(agent_id);
create index if not exists idx_tunnels_type on tunnels(type);
create unique index if not exists uq_tunnels_remote_port on tunnels(remote_port) where remote_port is not null;

create table if not exists domain_routes (
    id uuid primary key default gen_random_uuid(),
    tunnel_id uuid not null references tunnels(id) on delete cascade,
    domain varchar(255) not null unique,
    scheme varchar(16) not null default 'http',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint chk_domain_routes_scheme check (scheme in ('http', 'https'))
);
create index if not exists idx_domain_routes_tunnel_id on domain_routes(tunnel_id);

create table if not exists accounts (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null unique references users(id) on delete cascade,
    balance numeric(18, 4) not null default 0,
    currency varchar(16) not null default 'CNY',
    status varchar(32) not null default 'active',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create table if not exists pricing_rules (
    id uuid primary key default gen_random_uuid(),
    name varchar(128) not null,
    billing_mode varchar(32) not null default 'traffic',
    price_per_gb numeric(18, 4) not null,
    free_quota_bytes bigint not null default 0,
    status varchar(32) not null default 'active',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint chk_pricing_rules_mode check (billing_mode in ('traffic'))
);

create table if not exists user_pricing_rules (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    pricing_rule_id uuid not null references pricing_rules(id) on delete restrict,
    effective_at timestamptz not null default now(),
    expired_at timestamptz,
    created_at timestamptz not null default now()
);
create index if not exists idx_user_pricing_rules_user_id on user_pricing_rules(user_id);

create table if not exists recharge_orders (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    order_no varchar(64) not null unique,
    amount numeric(18, 4) not null,
    status varchar(32) not null default 'pending',
    payment_channel varchar(32) not null,
    paid_at timestamptz,
    created_at timestamptz not null default now(),
    constraint chk_recharge_orders_amount check (amount > 0)
);
create index if not exists idx_recharge_orders_user_id on recharge_orders(user_id);

create table if not exists payment_records (
    id uuid primary key default gen_random_uuid(),
    recharge_order_id uuid not null references recharge_orders(id) on delete cascade,
    channel varchar(32) not null,
    channel_order_no varchar(128),
    callback_payload jsonb,
    status varchar(32) not null,
    created_at timestamptz not null default now()
);
create index if not exists idx_payment_records_order_id on payment_records(recharge_order_id);

create table if not exists account_transactions (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    account_id uuid not null references accounts(id) on delete cascade,
    type varchar(32) not null,
    amount numeric(18, 4) not null,
    balance_before numeric(18, 4) not null,
    balance_after numeric(18, 4) not null,
    reference_type varchar(32),
    reference_id uuid,
    remark text,
    created_at timestamptz not null default now(),
    constraint chk_account_transactions_type check (type in ('recharge', 'consume', 'refund', 'gift', 'adjust'))
);
create index if not exists idx_account_transactions_user_id on account_transactions(user_id);
create index if not exists idx_account_transactions_account_id on account_transactions(account_id);

create table if not exists traffic_usages (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    agent_id uuid references agents(id) on delete set null,
    tunnel_id uuid references tunnels(id) on delete set null,
    bucket_time timestamptz not null,
    ingress_bytes bigint not null default 0,
    egress_bytes bigint not null default 0,
    total_bytes bigint not null default 0,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (tunnel_id, bucket_time)
);
create index if not exists idx_traffic_usages_user_id on traffic_usages(user_id);
create index if not exists idx_traffic_usages_agent_id on traffic_usages(agent_id);
create index if not exists idx_traffic_usages_bucket_time on traffic_usages(bucket_time);

create table if not exists tunnel_connections (
    id uuid primary key default gen_random_uuid(),
    tunnel_id uuid not null references tunnels(id) on delete cascade,
    agent_id uuid references agents(id) on delete set null,
    protocol varchar(16) not null,
    source_addr varchar(255),
    target_addr varchar(255),
    started_at timestamptz not null default now(),
    ended_at timestamptz,
    ingress_bytes bigint not null default 0,
    egress_bytes bigint not null default 0,
    total_bytes bigint not null default 0,
    status varchar(32) not null default 'active'
);
create index if not exists idx_tunnel_connections_tunnel_id on tunnel_connections(tunnel_id);
create index if not exists idx_tunnel_connections_started_at on tunnel_connections(started_at);

create table if not exists audit_logs (
    id uuid primary key default gen_random_uuid(),
    user_id uuid references users(id) on delete set null,
    action varchar(64) not null,
    target_type varchar(64),
    target_id uuid,
    content jsonb,
    created_at timestamptz not null default now()
);
create index if not exists idx_audit_logs_user_id on audit_logs(user_id);
create index if not exists idx_audit_logs_created_at on audit_logs(created_at);
