ALTER TABLE user_business_records
ADD COLUMN IF NOT EXISTS billable_bytes BIGINT NOT NULL DEFAULT 0;

UPDATE user_business_records
SET billable_bytes = COALESCE(traffic_bytes, 0)
WHERE billable_bytes = 0
  AND COALESCE(traffic_bytes, 0) <> 0;

UPDATE user_business_records
SET traffic_bytes = COALESCE(((regexp_match(COALESCE(NULLIF(description, ''), NULLIF(remark, ''), ''), 'bytes=([0-9]+)'))[1])::bigint, traffic_bytes)
WHERE record_type IN ('traffic_settlement', 'subscription_traffic_settlement')
  AND COALESCE(NULLIF(description, ''), NULLIF(remark, ''), '') ~ '^(traffic settlement|subscription traffic) bytes=[0-9]+';
