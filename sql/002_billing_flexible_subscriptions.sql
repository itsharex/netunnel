alter table if exists traffic_usages
    add column if not exists billed_bytes bigint not null default 0;

alter table if exists pricing_rules
    add column if not exists subscription_price numeric(18, 4) not null default 0,
    add column if not exists included_traffic_bytes bigint not null default 0,
    add column if not exists subscription_period varchar(32) not null default 'none',
    add column if not exists traffic_reset_period varchar(32) not null default 'none',
    add column if not exists is_unlimited boolean not null default false;

update pricing_rules
set included_traffic_bytes = free_quota_bytes
where included_traffic_bytes = 0
  and free_quota_bytes > 0;

alter table if exists pricing_rules
    drop constraint if exists chk_pricing_rules_mode;

do $$
begin
    if not exists (
        select 1
        from pg_constraint
        where conname = 'chk_pricing_rules_mode'
    ) then
        alter table pricing_rules
            add constraint chk_pricing_rules_mode
            check (billing_mode in ('traffic', 'subscription'));
    end if;
end $$;

do $$
begin
    if not exists (
        select 1
        from pg_constraint
        where conname = 'chk_pricing_rules_subscription_period'
    ) then
        alter table pricing_rules
            add constraint chk_pricing_rules_subscription_period
            check (subscription_period in ('none', 'month', 'year'));
    end if;
end $$;

do $$
begin
    if not exists (
        select 1
        from pg_constraint
        where conname = 'chk_pricing_rules_traffic_reset_period'
    ) then
        alter table pricing_rules
            add constraint chk_pricing_rules_traffic_reset_period
            check (traffic_reset_period in ('none', 'month', 'year'));
    end if;
end $$;

create table if not exists user_subscriptions (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users(id) on delete cascade,
    pricing_rule_id uuid not null references pricing_rules(id) on delete restrict,
    status varchar(32) not null default 'active',
    started_at timestamptz not null default now(),
    current_period_start timestamptz not null default now(),
    current_period_end timestamptz,
    current_period_used_bytes bigint not null default 0,
    expires_at timestamptz,
    cancelled_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

do $$
begin
    if not exists (
        select 1
        from pg_constraint
        where conname = 'chk_user_subscriptions_status'
    ) then
        alter table user_subscriptions
            add constraint chk_user_subscriptions_status
            check (status in ('active', 'expired', 'cancelled'));
    end if;
end $$;

create index if not exists idx_user_subscriptions_user_id on user_subscriptions(user_id);
create index if not exists idx_user_subscriptions_rule_id on user_subscriptions(pricing_rule_id);
create index if not exists idx_user_subscriptions_status on user_subscriptions(status);
create unique index if not exists uq_user_subscriptions_active_user
    on user_subscriptions(user_id)
    where status = 'active' and cancelled_at is null;
