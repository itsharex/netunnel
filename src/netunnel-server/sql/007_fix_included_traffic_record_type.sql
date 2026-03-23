UPDATE user_business_records
SET record_type = 'subscription_traffic_settlement',
    reference_type = 'subscription_traffic_settlement'
WHERE record_type = 'traffic_settlement'
  AND COALESCE(billable_bytes, 0) = 0
  AND COALESCE(traffic_bytes, 0) > 0;
