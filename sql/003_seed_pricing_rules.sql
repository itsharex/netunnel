alter table if exists pricing_rules
    add column if not exists display_name varchar(128),
    add column if not exists description text;

insert into pricing_rules (
    name,
    display_name,
    description,
    billing_mode,
    price_per_gb,
    free_quota_bytes,
    subscription_price,
    included_traffic_bytes,
    subscription_period,
    traffic_reset_period,
    is_unlimited,
    status
)
select
    seed.name,
    seed.display_name,
    seed.description,
    seed.billing_mode,
    seed.price_per_gb::numeric,
    seed.included_traffic_bytes,
    seed.subscription_price::numeric,
    seed.included_traffic_bytes,
    seed.subscription_period,
    seed.traffic_reset_period,
    seed.is_unlimited,
    'active'
from (
    values
        ('default-traffic', '按量流量包', '适合低频使用场景，按实际流量结算，长期有效。', 'traffic', '1.0000', 0::bigint, '0.0000', 'none', 'none', false),
        ('monthly-10g', '包月 10G', '适合轻量业务，每月含 10G 流量，超出后按量计费。', 'subscription', '1.0000', 10737418240::bigint, '29.9000', 'month', 'month', false),
        ('monthly-20g', '包月 20G', '适合稳定业务，每月含 20G 流量，超出后按量计费。', 'subscription', '1.0000', 21474836480::bigint, '49.9000', 'month', 'month', false),
        ('monthly-unlimited', '包月不限量', '适合持续在线业务，包月有效期内不限流量。', 'subscription', '0.0000', 0::bigint, '99.9000', 'month', 'month', true),
        ('yearly-10g', '包年 10G', '适合长期轻量业务，按年订阅，每月重置 10G 流量。', 'subscription', '1.0000', 10737418240::bigint, '299.0000', 'year', 'month', false),
        ('yearly-20g', '包年 20G', '适合长期稳定业务，按年订阅，每月重置 20G 流量。', 'subscription', '1.0000', 21474836480::bigint, '499.0000', 'year', 'month', false),
        ('yearly-unlimited', '包年不限量', '适合核心生产业务，按年订阅，全程不限流量。', 'subscription', '0.0000', 0::bigint, '999.0000', 'year', 'month', true)
) as seed(name, display_name, description, billing_mode, price_per_gb, included_traffic_bytes, subscription_price, subscription_period, traffic_reset_period, is_unlimited)
where not exists (
    select 1
    from pricing_rules pr
    where pr.name = seed.name
);
