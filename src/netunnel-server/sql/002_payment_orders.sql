CREATE TABLE IF NOT EXISTS payment_orders (
  biz_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  order_type TEXT NOT NULL,
  payment_product_id TEXT NOT NULL,
  pricing_rule_id TEXT,
  recharge_gb INTEGER,
  session_id TEXT,
  notify_url TEXT NOT NULL,
  poll_url TEXT,
  qr_code_url TEXT,
  checkout_url TEXT,
  amount INTEGER NOT NULL DEFAULT 0,
  platform_status TEXT NOT NULL DEFAULT 'pending',
  apply_status TEXT NOT NULL DEFAULT 'pending',
  business_notify_status TEXT,
  business_notify_error TEXT,
  expires_at TIMESTAMPTZ,
  paid_at TIMESTAMPTZ,
  last_polled_at TIMESTAMPTZ,
  apply_error TEXT,
  raw_snapshot TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS user_id TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS order_type TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS payment_product_id TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS pricing_rule_id TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS recharge_gb INTEGER;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS session_id TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS notify_url TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS poll_url TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS qr_code_url TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS checkout_url TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS amount INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS platform_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS apply_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS business_notify_status TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS business_notify_error TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS paid_at TIMESTAMPTZ;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS last_polled_at TIMESTAMPTZ;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS apply_error TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS raw_snapshot TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_payment_orders_user_id_created_at ON payment_orders (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_payment_orders_platform_status ON payment_orders (platform_status);
