ALTER TABLE user_business_records
DROP CONSTRAINT IF EXISTS chk_account_transactions_type;

ALTER TABLE user_business_records
DROP CONSTRAINT IF EXISTS chk_user_business_records_type;

ALTER TABLE user_business_records
ADD CONSTRAINT chk_user_business_records_type
CHECK (
  type IN (
    'recharge',
    'consume',
    'refund',
    'gift',
    'adjust',
    'traffic_recharge',
    'subscription_purchase',
    'subscription_renew',
    'traffic_settlement'
  )
);
