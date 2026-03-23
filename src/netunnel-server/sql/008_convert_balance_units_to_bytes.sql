ALTER TABLE accounts
ALTER COLUMN balance TYPE BIGINT
USING CASE
  WHEN balance IS NULL THEN NULL
  WHEN balance <> trunc(balance) THEN round(balance * 1073741824)::bigint
  WHEN abs(balance) >= 1048576 THEN round(balance)::bigint
  ELSE round(balance * 1073741824)::bigint
END;

ALTER TABLE user_business_records
ALTER COLUMN amount TYPE BIGINT
USING CASE
  WHEN amount IS NULL THEN NULL
  WHEN amount <> trunc(amount) THEN round(amount * 1073741824)::bigint
  WHEN abs(amount) >= 1048576 THEN round(amount)::bigint
  ELSE round(amount * 1073741824)::bigint
END;

ALTER TABLE user_business_records
ALTER COLUMN balance_before TYPE BIGINT
USING CASE
  WHEN balance_before IS NULL THEN NULL
  WHEN balance_before <> trunc(balance_before) THEN round(balance_before * 1073741824)::bigint
  WHEN abs(balance_before) >= 1048576 THEN round(balance_before)::bigint
  ELSE round(balance_before * 1073741824)::bigint
END;

ALTER TABLE user_business_records
ALTER COLUMN balance_after TYPE BIGINT
USING CASE
  WHEN balance_after IS NULL THEN NULL
  WHEN balance_after <> trunc(balance_after) THEN round(balance_after * 1073741824)::bigint
  WHEN abs(balance_after) >= 1048576 THEN round(balance_after)::bigint
  ELSE round(balance_after * 1073741824)::bigint
END;
