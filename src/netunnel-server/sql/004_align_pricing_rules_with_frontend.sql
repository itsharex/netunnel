UPDATE pricing_rules
SET status = 'inactive',
    updated_at = NOW()
WHERE name IN ('monthly-10g', 'monthly-20g', 'yearly-10g', 'yearly-20g');

DELETE FROM pricing_rules pr
WHERE pr.name IN ('monthly-10g', 'monthly-20g', 'yearly-10g', 'yearly-20g')
  AND pr.status <> 'active'
  AND NOT EXISTS (
    SELECT 1
    FROM user_subscriptions us
    WHERE us.pricing_rule_id::text = pr.id::text
  )
  AND NOT EXISTS (
    SELECT 1
    FROM user_pricing_rules upr
    WHERE upr.pricing_rule_id::text = pr.id::text
  )
  AND NOT EXISTS (
    SELECT 1
    FROM payment_orders po
    WHERE po.pricing_rule_id = pr.id::text
  );

UPDATE pricing_rules
SET display_name = '按量流量',
    description = '无到期时间，优先使用包年包月套餐。',
    billing_mode = 'traffic',
    price_per_gb = 1.0000,
    subscription_price = 0,
    included_traffic_bytes = 0,
    subscription_period = 'none',
    traffic_reset_period = 'none',
    is_unlimited = false,
    status = 'active',
    updated_at = NOW()
WHERE name = 'default-traffic';

UPDATE pricing_rules
SET display_name = '不限量包月',
    description = '不限量包月套餐，固定 5 元。未到期续费，将会延长到期时间。',
    billing_mode = 'subscription',
    price_per_gb = 0,
    subscription_price = 5.0000,
    included_traffic_bytes = 0,
    subscription_period = 'month',
    traffic_reset_period = 'month',
    is_unlimited = true,
    status = 'active',
    updated_at = NOW()
WHERE name = 'monthly-unlimited';

UPDATE pricing_rules
SET display_name = '不限量包年',
    description = '不限量包年套餐，固定 40 元。未到期续费，将会延长到期时间。',
    billing_mode = 'subscription',
    price_per_gb = 0,
    subscription_price = 40.0000,
    included_traffic_bytes = 0,
    subscription_period = 'year',
    traffic_reset_period = 'month',
    is_unlimited = true,
    status = 'active',
    updated_at = NOW()
WHERE name = 'yearly-unlimited';

INSERT INTO pricing_rules (
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
SELECT
  'default-traffic',
  '按量流量',
  '无到期时间，优先使用包年包月套餐。',
  'traffic',
  1.0000,
  0,
  0,
  0,
  'none',
  'none',
  false,
  'active'
WHERE NOT EXISTS (
  SELECT 1 FROM pricing_rules WHERE name = 'default-traffic'
);

INSERT INTO pricing_rules (
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
SELECT
  'monthly-unlimited',
  '不限量包月',
  '不限量包月套餐，固定 5 元。未到期续费，将会延长到期时间。',
  'subscription',
  0,
  0,
  5.0000,
  0,
  'month',
  'month',
  true,
  'active'
WHERE NOT EXISTS (
  SELECT 1 FROM pricing_rules WHERE name = 'monthly-unlimited'
);

INSERT INTO pricing_rules (
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
SELECT
  'yearly-unlimited',
  '不限量包年',
  '不限量包年套餐，固定 40 元。未到期续费，将会延长到期时间。',
  'subscription',
  0,
  0,
  40.0000,
  0,
  'year',
  'month',
  true,
  'active'
WHERE NOT EXISTS (
  SELECT 1 FROM pricing_rules WHERE name = 'yearly-unlimited'
);
