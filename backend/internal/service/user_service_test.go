//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// --- mock: UserRepository ---

type mockUserRepo struct {
	updateBalanceErr  error
	updateBalanceFn   func(ctx context.Context, id int64, amount float64) error
	getByIDFn         func(context.Context, int64) (*User, error)
	tryDailyCheckInFn func(context.Context, int64, float64, time.Time, time.Time) (bool, error)
}

func (m *mockUserRepo) Create(context.Context, *User) error { return nil }
func (m *mockUserRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return &User{}, nil
}
func (m *mockUserRepo) GetByEmail(context.Context, string) (*User, error) { return &User{}, nil }
func (m *mockUserRepo) GetFirstAdmin(context.Context) (*User, error)      { return &User{}, nil }
func (m *mockUserRepo) Update(context.Context, *User) error               { return nil }
func (m *mockUserRepo) Delete(context.Context, int64) error               { return nil }
func (m *mockUserRepo) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockUserRepo) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockUserRepo) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	if m.updateBalanceFn != nil {
		return m.updateBalanceFn(ctx, id, amount)
	}
	return m.updateBalanceErr
}
func (m *mockUserRepo) TryDailyCheckIn(ctx context.Context, id int64, amount float64, dayStart, checkedInAt time.Time) (bool, error) {
	if m.tryDailyCheckInFn != nil {
		return m.tryDailyCheckInFn(ctx, id, amount, dayStart, checkedInAt)
	}
	return false, nil
}
func (m *mockUserRepo) DeductBalance(context.Context, int64, float64) error { return nil }
func (m *mockUserRepo) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (m *mockUserRepo) ExistsByEmail(context.Context, string) (bool, error) { return false, nil }
func (m *mockUserRepo) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (m *mockUserRepo) AddGroupToAllowedGroups(context.Context, int64, int64) error { return nil }
func (m *mockUserRepo) UpdateTotpSecret(context.Context, int64, *string) error      { return nil }
func (m *mockUserRepo) EnableTotp(context.Context, int64) error                     { return nil }
func (m *mockUserRepo) DisableTotp(context.Context, int64) error                    { return nil }

// --- mock: APIKeyAuthCacheInvalidator ---

type mockAuthCacheInvalidator struct {
	invalidatedUserIDs []int64
	mu                 sync.Mutex
}

func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByKey(context.Context, string)    {}
func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByGroupID(context.Context, int64) {}
func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidatedUserIDs = append(m.invalidatedUserIDs, userID)
}

// --- mock: BillingCache ---

type mockBillingCache struct {
	invalidateErr       error
	invalidateCallCount atomic.Int64
	invalidatedUserIDs  []int64
	mu                  sync.Mutex
}

func (m *mockBillingCache) GetUserBalance(context.Context, int64) (float64, error)  { return 0, nil }
func (m *mockBillingCache) SetUserBalance(context.Context, int64, float64) error    { return nil }
func (m *mockBillingCache) DeductUserBalance(context.Context, int64, float64) error { return nil }
func (m *mockBillingCache) InvalidateUserBalance(_ context.Context, userID int64) error {
	m.invalidateCallCount.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidatedUserIDs = append(m.invalidatedUserIDs, userID)
	return m.invalidateErr
}
func (m *mockBillingCache) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return nil, nil
}
func (m *mockBillingCache) SetSubscriptionCache(context.Context, int64, int64, *SubscriptionCacheData) error {
	return nil
}
func (m *mockBillingCache) UpdateSubscriptionUsage(context.Context, int64, int64, float64) error {
	return nil
}
func (m *mockBillingCache) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	return nil
}
func (m *mockBillingCache) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	return nil, nil
}
func (m *mockBillingCache) SetAPIKeyRateLimit(context.Context, int64, *APIKeyRateLimitCacheData) error {
	return nil
}
func (m *mockBillingCache) UpdateAPIKeyRateLimitUsage(context.Context, int64, float64) error {
	return nil
}
func (m *mockBillingCache) InvalidateAPIKeyRateLimit(context.Context, int64) error {
	return nil
}

type mockSettingRepo struct {
	values map[string]string
}

func (m *mockSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := m.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}
func (m *mockSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := m.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (m *mockSettingRepo) Set(_ context.Context, key, value string) error {
	if m.values == nil {
		m.values = map[string]string{}
	}
	m.values[key] = value
	return nil
}
func (m *mockSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := m.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}
func (m *mockSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	if m.values == nil {
		m.values = map[string]string{}
	}
	for key, value := range settings {
		m.values[key] = value
	}
	return nil
}
func (m *mockSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	result := make(map[string]string, len(m.values))
	for key, value := range m.values {
		result[key] = value
	}
	return result, nil
}
func (m *mockSettingRepo) Delete(_ context.Context, key string) error {
	delete(m.values, key)
	return nil
}

type mockRedeemCodeRepo struct {
	created []*RedeemCode
}

func (m *mockRedeemCodeRepo) Create(_ context.Context, code *RedeemCode) error {
	if code != nil {
		cloned := *code
		m.created = append(m.created, &cloned)
	}
	return nil
}
func (m *mockRedeemCodeRepo) CreateBatch(context.Context, []RedeemCode) error        { return nil }
func (m *mockRedeemCodeRepo) GetByID(context.Context, int64) (*RedeemCode, error)    { return nil, nil }
func (m *mockRedeemCodeRepo) GetByCode(context.Context, string) (*RedeemCode, error) { return nil, nil }
func (m *mockRedeemCodeRepo) Update(context.Context, *RedeemCode) error              { return nil }
func (m *mockRedeemCodeRepo) Delete(context.Context, int64) error                    { return nil }
func (m *mockRedeemCodeRepo) Use(context.Context, int64, int64) error                { return nil }
func (m *mockRedeemCodeRepo) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockRedeemCodeRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockRedeemCodeRepo) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	return nil, nil
}
func (m *mockRedeemCodeRepo) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockRedeemCodeRepo) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	return 0, nil
}

func newTestSettingService(values map[string]string) *SettingService {
	return NewSettingService(&mockSettingRepo{values: values}, &config.Config{})
}

// --- 测试 ---

func TestUpdateBalance_Success(t *testing.T) {
	repo := &mockUserRepo{}
	cache := &mockBillingCache{}
	svc := NewUserService(repo, nil, cache, nil, nil)

	err := svc.UpdateBalance(context.Background(), 42, 100.0)
	require.NoError(t, err)

	// 等待异步 goroutine 完成
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "应异步调用 InvalidateUserBalance")

	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Equal(t, []int64{42}, cache.invalidatedUserIDs, "应对 userID=42 失效缓存")
}

func TestUpdateBalance_NilBillingCache_NoPanic(t *testing.T) {
	repo := &mockUserRepo{}
	svc := NewUserService(repo, nil, nil, nil, nil) // billingCache = nil

	err := svc.UpdateBalance(context.Background(), 1, 50.0)
	require.NoError(t, err, "billingCache 为 nil 时不应 panic")
}

func TestUpdateBalance_CacheFailure_DoesNotAffectReturn(t *testing.T) {
	repo := &mockUserRepo{}
	cache := &mockBillingCache{invalidateErr: errors.New("redis connection refused")}
	svc := NewUserService(repo, nil, cache, nil, nil)

	err := svc.UpdateBalance(context.Background(), 99, 200.0)
	require.NoError(t, err, "缓存失效失败不应影响主流程返回值")

	// 等待异步 goroutine 完成（即使失败也应调用）
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "即使失败也应调用 InvalidateUserBalance")
}

func TestUpdateBalance_RepoError_ReturnsError(t *testing.T) {
	repo := &mockUserRepo{updateBalanceErr: errors.New("database error")}
	cache := &mockBillingCache{}
	svc := NewUserService(repo, nil, cache, nil, nil)

	err := svc.UpdateBalance(context.Background(), 1, 100.0)
	require.Error(t, err, "repo 失败时应返回错误")
	require.Contains(t, err.Error(), "update balance")

	// repo 失败时不应触发缓存失效
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, int64(0), cache.invalidateCallCount.Load(),
		"repo 失败时不应调用 InvalidateUserBalance")
}

func TestUpdateBalance_WithAuthCacheInvalidator(t *testing.T) {
	repo := &mockUserRepo{}
	auth := &mockAuthCacheInvalidator{}
	cache := &mockBillingCache{}
	svc := NewUserService(repo, auth, cache, nil, nil)

	err := svc.UpdateBalance(context.Background(), 77, 300.0)
	require.NoError(t, err)

	// 验证 auth cache 同步失效
	auth.mu.Lock()
	require.Equal(t, []int64{77}, auth.invalidatedUserIDs)
	auth.mu.Unlock()

	// 验证 billing cache 异步失效
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestNewUserService_FieldsAssignment(t *testing.T) {
	repo := &mockUserRepo{}
	auth := &mockAuthCacheInvalidator{}
	cache := &mockBillingCache{}
	settingService := newTestSettingService(nil)
	redeemRepo := &mockRedeemCodeRepo{}

	svc := NewUserService(repo, auth, cache, settingService, redeemRepo)
	require.NotNil(t, svc)
	require.Equal(t, repo, svc.userRepo)
	require.Equal(t, auth, svc.authCacheInvalidator)
	require.Equal(t, cache, svc.billingCache)
	require.Equal(t, settingService, svc.settingService)
	require.Equal(t, redeemRepo, svc.redeemRepo)
}

func TestGetDailyCheckInStatus(t *testing.T) {
	now := time.Now()
	repo := &mockUserRepo{
		getByIDFn: func(context.Context, int64) (*User, error) {
			return &User{LastCheckInAt: &now}, nil
		},
	}
	settingService := newTestSettingService(map[string]string{
		SettingKeyDailyCheckInEnabled:   "true",
		SettingKeyDailyCheckInMinReward: "1.5",
		SettingKeyDailyCheckInMaxReward: "1.5",
	})

	svc := NewUserService(repo, nil, nil, settingService, nil)
	status, err := svc.GetDailyCheckInStatus(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, status.Enabled)
	require.True(t, status.CheckedInToday)
	require.NotNil(t, status.LastCheckInAt)
}

func TestDailyCheckIn_Disabled(t *testing.T) {
	svc := NewUserService(&mockUserRepo{}, nil, nil, newTestSettingService(map[string]string{
		SettingKeyDailyCheckInEnabled:   "false",
		SettingKeyDailyCheckInMinReward: "1.5",
		SettingKeyDailyCheckInMaxReward: "1.5",
	}), nil)

	_, err := svc.DailyCheckIn(context.Background(), 1)
	require.ErrorIs(t, err, ErrDailyCheckInDisabled)
}

func TestDailyCheckIn_AlreadyCompleted(t *testing.T) {
	settingService := newTestSettingService(map[string]string{
		SettingKeyDailyCheckInEnabled:   "true",
		SettingKeyDailyCheckInMinReward: "1.5",
		SettingKeyDailyCheckInMaxReward: "1.5",
	})
	repo := &mockUserRepo{
		tryDailyCheckInFn: func(context.Context, int64, float64, time.Time, time.Time) (bool, error) {
			return false, nil
		},
	}
	svc := NewUserService(repo, nil, nil, settingService, nil)

	_, err := svc.DailyCheckIn(context.Background(), 1)
	require.ErrorIs(t, err, ErrDailyCheckInAlreadyCompleted)
}

func TestDailyCheckIn_SuccessCreatesHistoryAndInvalidatesCache(t *testing.T) {
	settingService := newTestSettingService(map[string]string{
		SettingKeyDailyCheckInEnabled:   "true",
		SettingKeyDailyCheckInMinReward: "2.5",
		SettingKeyDailyCheckInMaxReward: "2.5",
	})
	redeemRepo := &mockRedeemCodeRepo{}
	auth := &mockAuthCacheInvalidator{}
	cache := &mockBillingCache{}
	repo := &mockUserRepo{
		tryDailyCheckInFn: func(context.Context, int64, float64, time.Time, time.Time) (bool, error) {
			return true, nil
		},
		getByIDFn: func(context.Context, int64) (*User, error) {
			return &User{ID: 7, Balance: 12.5}, nil
		},
	}
	svc := NewUserService(repo, auth, cache, settingService, redeemRepo)

	result, err := svc.DailyCheckIn(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, 2.5, result.RewardAmount)
	require.Equal(t, 12.5, result.NewBalance)
	require.Len(t, redeemRepo.created, 1)
	require.Equal(t, RedeemTypeDailyCheckIn, redeemRepo.created[0].Type)

	auth.mu.Lock()
	require.Equal(t, []int64{7}, auth.invalidatedUserIDs)
	auth.mu.Unlock()
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
}
