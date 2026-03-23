DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'wechat_openid') THEN
    ALTER TABLE users ADD COLUMN wechat_openid VARCHAR(128) UNIQUE;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_users_wechat_openid ON users(wechat_openid);
