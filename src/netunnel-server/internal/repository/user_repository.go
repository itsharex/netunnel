package repository

import (
	"context"
	"database/sql"
	"fmt"

	"netunnel/server/internal/domain"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, email, nickname, avatarURL, passwordHash, wechatOpenid string) (*domain.User, error) {
	const query = `
insert into users (email, nickname, avatar_url, password_hash, wechat_openid, status)
values ($1, $2, $3, $4, $5, 'active')
returning id, email, nickname, avatar_url, password_hash, wechat_openid, status, created_at, updated_at`

	var user domain.User
	var avatar sql.NullString
	var ns sql.NullString
	err := r.db.QueryRowContext(ctx, query, nullableString(email), nickname, nullableString(avatarURL), passwordHash, nullableString(wechatOpenid)).Scan(
		&user.ID,
		&user.Email,
		&user.Nickname,
		&avatar,
		&user.PasswordHash,
		&ns,
		&user.Status,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	if ns.Valid {
		user.WechatOpenid = ns.String
	}
	if avatar.Valid {
		user.AvatarURL = avatar.String
	}

	return &user, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func scanUser(row *sql.Row) (*domain.User, error) {
	var user domain.User
	var avatarURL sql.NullString
	var wechatOpenid sql.NullString
	err := row.Scan(
		&user.ID,
		&user.Email,
		&user.Nickname,
		&avatarURL,
		&user.PasswordHash,
		&wechatOpenid,
		&user.Status,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan user: %w", err)
	}
	if wechatOpenid.Valid {
		user.WechatOpenid = wechatOpenid.String
	}
	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	}
	return &user, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	const query = `
        select id, email, nickname, avatar_url, password_hash, wechat_openid, status, created_at, updated_at
        from users where email = $1`

	return scanUser(r.db.QueryRowContext(ctx, query, email))
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	const query = `
        select id, email, nickname, avatar_url, password_hash, wechat_openid, status, created_at, updated_at
        from users where id = $1`

	return scanUser(r.db.QueryRowContext(ctx, query, id))
}

func (r *UserRepository) FindByWechatOpenid(ctx context.Context, openid string) (*domain.User, error) {
	const query = `
        select id, email, nickname, avatar_url, password_hash, wechat_openid, status, created_at, updated_at
        from users where wechat_openid = $1`

	return scanUser(r.db.QueryRowContext(ctx, query, openid))
}

func (r *UserRepository) UpdateWechatOpenid(ctx context.Context, userID, openid string) error {
	const query = `UPDATE users SET wechat_openid = $1 WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, openid, userID)
	if err != nil {
		return fmt.Errorf("update wechat_openid: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdateWechatProfile(ctx context.Context, userID, nickname, avatarURL, openid string) error {
	const query = `
UPDATE users
SET nickname = CASE WHEN $1 <> '' THEN $1 ELSE nickname END,
    avatar_url = CASE WHEN $2 <> '' THEN $2 ELSE avatar_url END,
    wechat_openid = CASE WHEN $3 <> '' THEN $3 ELSE wechat_openid END,
    updated_at = now()
WHERE id = $4`

	_, err := r.db.ExecContext(ctx, query, nickname, avatarURL, openid, userID)
	if err != nil {
		return fmt.Errorf("update wechat profile: %w", err)
	}
	return nil
}
