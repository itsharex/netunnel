package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"netunnel/server/internal/domain"
	"netunnel/server/internal/repository"
)

type UserService struct {
	users *repository.UserRepository
}

func NewUserService(users *repository.UserRepository) *UserService {
	return &UserService{users: users}
}

type BootstrapUserInput struct {
	Email        string `json:"email"`
	Nickname     string `json:"nickname"`
	AvatarURL    string `json:"avatar_url"`
	Password     string `json:"password"`
	WechatOpenid string `json:"wechat_openid"`
}

func (s *UserService) Bootstrap(ctx context.Context, input BootstrapUserInput) (*domain.User, error) {
	input.WechatOpenid = strings.TrimSpace(input.WechatOpenid)
	input.AvatarURL = strings.TrimSpace(input.AvatarURL)
	if input.WechatOpenid != "" {
		existing, err := s.users.FindByWechatOpenid(ctx, input.WechatOpenid)
		if err != nil {
			return nil, fmt.Errorf("check wechat user: %w", err)
		}
		if existing != nil {
			input.Nickname = strings.TrimSpace(input.Nickname)
			if input.Nickname != "" || input.AvatarURL != "" {
				if err := s.users.UpdateWechatProfile(ctx, existing.ID, input.Nickname, input.AvatarURL, input.WechatOpenid); err != nil {
					log.Printf("[bootstrap] failed to update wechat profile for user %q: %v", existing.ID, err)
				} else {
					if input.Nickname != "" {
						existing.Nickname = input.Nickname
					}
					if input.AvatarURL != "" {
						existing.AvatarURL = input.AvatarURL
					}
					existing.WechatOpenid = input.WechatOpenid
				}
			}
			log.Printf("[bootstrap] FindByWechatOpenid(%q) found user=%q", input.WechatOpenid, existing.ID)
			return existing, nil
		}
	}

	input.Email = strings.TrimSpace(input.Email)
	input.Nickname = strings.TrimSpace(input.Nickname)
	input.Password = strings.TrimSpace(input.Password)

	if input.Email == "" {
		return nil, fmt.Errorf("%w: email is required", ErrInvalidArgument)
	}
	if input.Nickname == "" || input.Password == "" {
		return nil, fmt.Errorf("%w: nickname and password are required", ErrInvalidArgument)
	}

	existing, err := s.users.FindByEmail(ctx, input.Email)
	if err != nil {
		return nil, fmt.Errorf("check existing user: %w", err)
	}
	log.Printf("[bootstrap] FindByEmail(%q) found=%v", input.Email, existing != nil)
	if existing != nil {
		if input.WechatOpenid != "" || input.Nickname != "" || input.AvatarURL != "" {
			if err := s.users.UpdateWechatProfile(ctx, existing.ID, input.Nickname, input.AvatarURL, input.WechatOpenid); err != nil {
				log.Printf("[bootstrap] failed to update wechat profile for user %q: %v", existing.ID, err)
			} else {
				if input.WechatOpenid != "" {
					log.Printf("[bootstrap] updated wechat_openid for user %q", existing.ID)
					existing.WechatOpenid = input.WechatOpenid
				}
				if input.Nickname != "" {
					existing.Nickname = input.Nickname
				}
				if input.AvatarURL != "" {
					existing.AvatarURL = input.AvatarURL
				}
			}
		}
		return existing, nil
	}

	hash := sha256.Sum256([]byte(input.Password))
	passwordHash := hex.EncodeToString(hash[:])

	return s.users.Create(ctx, input.Email, input.Nickname, input.AvatarURL, passwordHash, input.WechatOpenid)
}

func (s *UserService) GetByID(ctx context.Context, userID string) (*domain.User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidArgument)
	}

	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}
