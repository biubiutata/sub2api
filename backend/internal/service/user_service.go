package service

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"log"
	"math"
	"math/big"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

var (
	ErrUserNotFound                 = infraerrors.NotFound("USER_NOT_FOUND", "user not found")
	ErrPasswordIncorrect            = infraerrors.BadRequest("PASSWORD_INCORRECT", "current password is incorrect")
	ErrInsufficientPerms            = infraerrors.Forbidden("INSUFFICIENT_PERMISSIONS", "insufficient permissions")
	ErrDailyCheckInDisabled         = infraerrors.Forbidden("DAILY_CHECK_IN_DISABLED", "daily check-in is disabled")
	ErrDailyCheckInAlreadyCompleted = infraerrors.Conflict("DAILY_CHECK_IN_ALREADY_COMPLETED", "daily check-in already completed today")
)

// UserListFilters contains all filter options for listing users
type UserListFilters struct {
	Status     string           // User status filter
	Role       string           // User role filter
	Search     string           // Search in email, username
	Attributes map[int64]string // Custom attribute filters: attributeID -> value
	// IncludeSubscriptions controls whether ListWithFilters should load active subscriptions.
	// For large datasets this can be expensive; admin list pages should enable it on demand.
	// nil means not specified (default: load subscriptions for backward compatibility).
	IncludeSubscriptions *bool
}

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetFirstAdmin(ctx context.Context) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error)

	UpdateBalance(ctx context.Context, id int64, amount float64) error
	TryDailyCheckIn(ctx context.Context, id int64, amount float64, dayStart, checkedInAt time.Time) (bool, error)
	DeductBalance(ctx context.Context, id int64, amount float64) error
	UpdateConcurrency(ctx context.Context, id int64, amount int) error
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error)
	// AddGroupToAllowedGroups 将指定分组增量添加到用户的 allowed_groups（幂等，冲突忽略）
	AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error

	// TOTP 双因素认证
	UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error
	EnableTotp(ctx context.Context, userID int64) error
	DisableTotp(ctx context.Context, userID int64) error
}

// UpdateProfileRequest 更新用户资料请求
type UpdateProfileRequest struct {
	Email       *string `json:"email"`
	Username    *string `json:"username"`
	Concurrency *int    `json:"concurrency"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type DailyCheckInStatus struct {
	Enabled        bool
	CheckedInToday bool
	LastCheckInAt  *time.Time
}

type DailyCheckInResult struct {
	RewardAmount float64
	NewBalance   float64
	CheckedInAt  time.Time
}

// UserService 用户服务
type UserService struct {
	userRepo             UserRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCache         BillingCache
	settingService       *SettingService
	redeemRepo           RedeemCodeRepository
}

// NewUserService 创建用户服务实例
func NewUserService(userRepo UserRepository, authCacheInvalidator APIKeyAuthCacheInvalidator, billingCache BillingCache, settingService *SettingService, redeemRepo RedeemCodeRepository) *UserService {
	return &UserService{
		userRepo:             userRepo,
		authCacheInvalidator: authCacheInvalidator,
		billingCache:         billingCache,
		settingService:       settingService,
		redeemRepo:           redeemRepo,
	}
}

// GetFirstAdmin 获取首个管理员用户（用于 Admin API Key 认证）
func (s *UserService) GetFirstAdmin(ctx context.Context) (*User, error) {
	admin, err := s.userRepo.GetFirstAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("get first admin: %w", err)
	}
	return admin, nil
}

// GetProfile 获取用户资料
func (s *UserService) GetProfile(ctx context.Context, userID int64) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

// UpdateProfile 更新用户资料
func (s *UserService) UpdateProfile(ctx context.Context, userID int64, req UpdateProfileRequest) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	oldConcurrency := user.Concurrency

	// 更新字段
	if req.Email != nil {
		// 检查新邮箱是否已被使用
		exists, err := s.userRepo.ExistsByEmail(ctx, *req.Email)
		if err != nil {
			return nil, fmt.Errorf("check email exists: %w", err)
		}
		if exists && *req.Email != user.Email {
			return nil, ErrEmailExists
		}
		user.Email = *req.Email
	}

	if req.Username != nil {
		user.Username = *req.Username
	}

	if req.Concurrency != nil {
		user.Concurrency = *req.Concurrency
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	if s.authCacheInvalidator != nil && user.Concurrency != oldConcurrency {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}

	return user, nil
}

// ChangePassword 修改密码
// Security: Increments TokenVersion to invalidate all existing JWT tokens
func (s *UserService) ChangePassword(ctx context.Context, userID int64, req ChangePasswordRequest) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	// 验证当前密码
	if !user.CheckPassword(req.CurrentPassword) {
		return ErrPasswordIncorrect
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		return fmt.Errorf("set password: %w", err)
	}

	// Increment TokenVersion to invalidate all existing tokens
	// This ensures that any tokens issued before the password change become invalid
	user.TokenVersion++

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	return nil
}

// GetByID 根据ID获取用户（管理员功能）
func (s *UserService) GetByID(ctx context.Context, id int64) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

func (s *UserService) GetDailyCheckInStatus(ctx context.Context, userID int64) (*DailyCheckInStatus, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	enabled := s.isDailyCheckInEnabled(ctx) && s.hasValidDailyCheckInRewardRange(ctx)

	return &DailyCheckInStatus{
		Enabled:        enabled,
		CheckedInToday: hasCheckedInToday(user.LastCheckInAt),
		LastCheckInAt:  user.LastCheckInAt,
	}, nil
}

func (s *UserService) DailyCheckIn(ctx context.Context, userID int64) (*DailyCheckInResult, error) {
	if !s.isDailyCheckInEnabled(ctx) {
		return nil, ErrDailyCheckInDisabled
	}
	rewardAmount, err := s.generateDailyCheckInReward(ctx)
	if err != nil {
		return nil, err
	}

	checkedInAt := timezone.Now()
	ok, err := s.userRepo.TryDailyCheckIn(ctx, userID, rewardAmount, timezone.Today(), checkedInAt)
	if err != nil {
		return nil, fmt.Errorf("daily check-in: %w", err)
	}
	if !ok {
		return nil, ErrDailyCheckInAlreadyCompleted
	}

	s.invalidateBalanceCaches(ctx, userID)

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user after daily check-in: %w", err)
	}

	if s.redeemRepo != nil {
		code, codeErr := GenerateRedeemCode()
		if codeErr != nil {
			log.Printf("generate daily check-in record code failed: user_id=%d err=%v", userID, codeErr)
		} else {
			usedAt := checkedInAt
			record := &RedeemCode{
				Code:   code,
				Type:   RedeemTypeDailyCheckIn,
				Value:  rewardAmount,
				Status: StatusUsed,
				UsedBy: &user.ID,
				UsedAt: &usedAt,
			}
			if createErr := s.redeemRepo.Create(ctx, record); createErr != nil {
				log.Printf("create daily check-in record failed: user_id=%d err=%v", userID, createErr)
			}
		}
	}

	return &DailyCheckInResult{
		RewardAmount: rewardAmount,
		NewBalance:   user.Balance,
		CheckedInAt:  checkedInAt,
	}, nil
}

// List 获取用户列表（管理员功能）
func (s *UserService) List(ctx context.Context, params pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	users, pagination, err := s.userRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list users: %w", err)
	}
	return users, pagination, nil
}

// UpdateBalance 更新用户余额（管理员功能）
func (s *UserService) UpdateBalance(ctx context.Context, userID int64, amount float64) error {
	if err := s.userRepo.UpdateBalance(ctx, userID, amount); err != nil {
		return fmt.Errorf("update balance: %w", err)
	}
	s.invalidateBalanceCaches(ctx, userID)
	return nil
}

func (s *UserService) invalidateBalanceCaches(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.billingCache != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.billingCache.InvalidateUserBalance(cacheCtx, userID); err != nil {
				log.Printf("invalidate user balance cache failed: user_id=%d err=%v", userID, err)
			}
		}()
	}
}

func (s *UserService) isDailyCheckInEnabled(ctx context.Context) bool {
	return s.settingService != nil && s.settingService.IsDailyCheckInEnabled(ctx)
}

func (s *UserService) getDailyCheckInRewardRange(ctx context.Context) (float64, float64) {
	if s.settingService == nil {
		return 0, 0
	}
	return s.settingService.GetDailyCheckInRewardRange(ctx)
}

func (s *UserService) hasValidDailyCheckInRewardRange(ctx context.Context) bool {
	minReward, maxReward := s.getDailyCheckInRewardRange(ctx)
	minCents := int64(math.Round(minReward * 100))
	maxCents := int64(math.Round(maxReward * 100))
	return minCents > 0 && maxCents >= minCents
}

func (s *UserService) generateDailyCheckInReward(ctx context.Context) (float64, error) {
	minReward, maxReward := s.getDailyCheckInRewardRange(ctx)
	minCents := int64(math.Round(minReward * 100))
	maxCents := int64(math.Round(maxReward * 100))
	if minCents <= 0 || maxCents < minCents {
		return 0, ErrDailyCheckInDisabled
	}
	if minCents == maxCents {
		return float64(minCents) / 100, nil
	}
	span := maxCents - minCents + 1
	randomOffset, err := cryptorand.Int(cryptorand.Reader, big.NewInt(span))
	if err != nil {
		return 0, fmt.Errorf("generate daily check-in reward: %w", err)
	}
	return float64(minCents+randomOffset.Int64()) / 100, nil
}

func hasCheckedInToday(lastCheckInAt *time.Time) bool {
	if lastCheckInAt == nil {
		return false
	}
	return !lastCheckInAt.Before(timezone.Today())
}

// UpdateConcurrency 更新用户并发数（管理员功能）
func (s *UserService) UpdateConcurrency(ctx context.Context, userID int64, concurrency int) error {
	if err := s.userRepo.UpdateConcurrency(ctx, userID, concurrency); err != nil {
		return fmt.Errorf("update concurrency: %w", err)
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	return nil
}

// UpdateStatus 更新用户状态（管理员功能）
func (s *UserService) UpdateStatus(ctx context.Context, userID int64, status string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	user.Status = status

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}

	return nil
}

// Delete 删除用户（管理员功能）
func (s *UserService) Delete(ctx context.Context, userID int64) error {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}
