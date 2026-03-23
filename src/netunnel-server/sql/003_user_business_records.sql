DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_name = 'account_transactions'
  ) AND NOT EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_name = 'user_business_records'
  ) THEN
    ALTER TABLE account_transactions RENAME TO user_business_records;
  END IF;
END $$;

ALTER TABLE user_business_records ADD COLUMN IF NOT EXISTS record_type TEXT;
ALTER TABLE user_business_records ADD COLUMN IF NOT EXISTS related_resource_type TEXT;
ALTER TABLE user_business_records ADD COLUMN IF NOT EXISTS related_resource_id TEXT;
ALTER TABLE user_business_records ADD COLUMN IF NOT EXISTS traffic_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE user_business_records ADD COLUMN IF NOT EXISTS package_expires_at TIMESTAMPTZ;
ALTER TABLE user_business_records ADD COLUMN IF NOT EXISTS payment_order_biz_id TEXT;
ALTER TABLE user_business_records ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

UPDATE user_business_records
SET record_type = CASE
    WHEN reference_type = 'manual_recharge' THEN 'traffic_recharge'
    WHEN reference_type = 'subscription_activation' THEN 'subscription_purchase'
    WHEN reference_type = 'traffic_settlement' THEN 'traffic_settlement'
    ELSE COALESCE(NULLIF(type, ''), 'unknown')
  END
WHERE record_type IS NULL OR record_type = '';

UPDATE user_business_records
SET related_resource_type = COALESCE(NULLIF(reference_type, ''), '')
WHERE related_resource_type IS NULL;

UPDATE user_business_records
SET related_resource_id = reference_id::text
WHERE related_resource_id IS NULL AND reference_id IS NOT NULL;

UPDATE user_business_records
SET description = COALESCE(NULLIF(remark, ''), '')
WHERE description = '';

CREATE INDEX IF NOT EXISTS idx_user_business_records_user_record_type_created_at
  ON user_business_records (user_id, record_type, created_at DESC);
