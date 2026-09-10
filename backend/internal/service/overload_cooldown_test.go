//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// errSettingRepo: a SettingRepository that always returns errors on read
// ---------------------------------------------------------------------------

type errSettingRepo struct {
	mockSettingRepo // embed the existing mock from backup_service_test.go
	readErr         error
}

func (r *errSettingRepo) GetValue(_ context.Context, _ string) (string, error) {
	return "", r.readErr
}

func (r *errSettingRepo) Get(_ context.Context, _ string) (*Setting, error) {
	return nil, r.readErr
}

// ---------------------------------------------------------------------------
// overloadAccountRepoStub: records SetOverloaded calls
// ---------------------------------------------------------------------------

type overloadAccountRepoStub struct {
	mockAccountRepoForGemini
	overloadCalls      int
	lastOverloadID     int64
	lastOverloadEnd    time.Time
	tempUnschedCalls   int
	lastTempUnschedEnd time.Time
	errorCalls         int
}

func (r *overloadAccountRepoStub) SetOverloaded(_ context.Context, id int64, until time.Time) error {
	r.overloadCalls++
	r.lastOverloadID = id
	r.lastOverloadEnd = until
	return nil
}

func (r *overloadAccountRepoStub) SetError(_ context.Context, _ int64, _ string) error {
	r.errorCalls++
	return nil
}

func (r *overloadAccountRepoStub) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, _ string) error {
	r.tempUnschedCalls++
	r.lastTempUnschedEnd = until
	return nil
}

// ===========================================================================
// SettingService: GetOverloadCooldownSettings
// ===========================================================================

func TestGetOverloadCooldownSettings_DefaultsWhenNotSet(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes)
}

func TestGetOverloadCooldownSettings_ReadsFromDB(t *testing.T) {
	repo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: false, CooldownMinutes: 30})
	repo.data[SettingKeyOverloadCooldownSettings] = string(data)
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, 30, settings.CooldownMinutes)
}

func TestGetOverloadCooldownSettings_ClampsMinValue(t *testing.T) {
	repo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: true, CooldownMinutes: 0})
	repo.data[SettingKeyOverloadCooldownSettings] = string(data)
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, settings.CooldownMinutes)
}

func TestGetOverloadCooldownSettings_ClampsMaxValue(t *testing.T) {
	repo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: true, CooldownMinutes: 999})
	repo.data[SettingKeyOverloadCooldownSettings] = string(data)
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 120, settings.CooldownMinutes)
}

func TestGetOverloadCooldownSettings_InvalidJSON_ReturnsDefaults(t *testing.T) {
	repo := newMockSettingRepo()
	repo.data[SettingKeyOverloadCooldownSettings] = "not-json"
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes)
}

func TestGetOverloadCooldownSettings_EmptyValue_ReturnsDefaults(t *testing.T) {
	repo := newMockSettingRepo()
	repo.data[SettingKeyOverloadCooldownSettings] = ""
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes)
}

// ===========================================================================
// SettingService: SetOverloadCooldownSettings
// ===========================================================================

func TestSetOverloadCooldownSettings_Success(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
		Enabled:         false,
		CooldownMinutes: 25,
	})
	require.NoError(t, err)

	// Verify round-trip
	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, 25, settings.CooldownMinutes)
}

func TestSetOverloadCooldownSettings_RejectsNil(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})
	err := svc.SetOverloadCooldownSettings(context.Background(), nil)
	require.Error(t, err)
}

func TestSetOverloadCooldownSettings_EnabledRejectsOutOfRange(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})

	for _, minutes := range []int{0, -1, 121, 999} {
		err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
			Enabled: true, CooldownMinutes: minutes,
		})
		require.Error(t, err, "should reject enabled=true + cooldown_minutes=%d", minutes)
		require.Contains(t, err.Error(), "cooldown_minutes must be between 1-120")
	}
}

func TestSetOverloadCooldownSettings_DisabledNormalizesOutOfRange(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	// enabled=false + cooldown_minutes=0 应该保存成功，值被归一化为10
	err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
		Enabled: false, CooldownMinutes: 0,
	})
	require.NoError(t, err, "disabled with invalid minutes should NOT be rejected")

	// 验证持久化后读回来的值
	settings, err := svc.GetOverloadCooldownSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes, "should be normalized to default")
}

func TestSetOverloadCooldownSettings_AcceptsBoundaries(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{})

	for _, minutes := range []int{1, 60, 120} {
		err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
			Enabled: true, CooldownMinutes: minutes,
		})
		require.NoError(t, err, "should accept cooldown_minutes=%d", minutes)
	}
}

// ===========================================================================
// RateLimitService: handle529 behaviour
// ===========================================================================

func TestHandle529_EnabledFromDB_PausesAccount(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: true, CooldownMinutes: 15})
	settingRepo.data[SettingKeyOverloadCooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.Equal(t, int64(42), accountRepo.lastOverloadID)
	require.WithinDuration(t, before.Add(15*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
}

func TestHandle529_DisabledFromDB_SkipsAccount(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: false, CooldownMinutes: 15})
	settingRepo.data[SettingKeyOverloadCooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	svc.handle529(context.Background(), account)

	require.Equal(t, 0, accountRepo.overloadCalls, "should NOT pause when disabled")
}

func TestHandle529_NilSettingService_FallsBackToConfig(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	cfg := &config.Config{}
	cfg.RateLimit.OverloadCooldownMinutes = 20
	svc := NewRateLimitService(accountRepo, nil, cfg, nil, nil)
	// NOT calling SetSettingService — remains nil

	account := &Account{ID: 77, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.WithinDuration(t, before.Add(20*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
}

func TestHandle529_NilSettingService_ZeroConfig_DefaultsTen(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)

	account := &Account{ID: 88, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.WithinDuration(t, before.Add(10*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
}

func TestHandle529_DBReadError_FallsBackToConfig(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	errRepo := &errSettingRepo{readErr: context.DeadlineExceeded}
	errRepo.data = make(map[string]string)

	cfg := &config.Config{}
	cfg.RateLimit.OverloadCooldownMinutes = 7
	settingSvc := NewSettingService(errRepo, cfg)
	svc := NewRateLimitService(accountRepo, nil, cfg, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 99, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.WithinDuration(t, before.Add(7*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
}

func TestHandleUpstreamError_OverloadStatusesPauseAccount(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "502_bad_gateway", statusCode: http.StatusBadGateway},
		{name: "503_service_unavailable", statusCode: http.StatusServiceUnavailable},
		{name: "529_legacy_overload", statusCode: 529},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accountRepo := &overloadAccountRepoStub{}
			svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
			account := &Account{ID: 101, Platform: PlatformAnthropic, Type: AccountTypeOAuth}

			before := time.Now()
			shouldDisable := svc.HandleUpstreamError(
				context.Background(), account, tt.statusCode, http.Header{}, []byte(`{"error":{"message":"temporary upstream overload"}}`),
			)

			require.False(t, shouldDisable, "overload cooldown should keep the account active")
			require.Equal(t, 1, accountRepo.overloadCalls)
			require.Equal(t, int64(101), accountRepo.lastOverloadID)
			require.WithinDuration(t, before.Add(10*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
		})
	}
}

func TestHandleSyntheticUpstreamError_502And503DoNotPauseAccount(t *testing.T) {
	for _, statusCode := range []int{http.StatusBadGateway, http.StatusServiceUnavailable} {
		accountRepo := &overloadAccountRepoStub{}
		svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
		account := &Account{ID: 102, Platform: PlatformAnthropic, Type: AccountTypeOAuth}

		shouldDisable := svc.HandleSyntheticUpstreamError(
			context.Background(), account, statusCode, http.Header{}, []byte(`{"error":{"message":"local parser failure"}}`),
		)

		require.False(t, shouldDisable)
		require.Zero(t, accountRepo.overloadCalls, "synthetic HTTP-like errors must not trigger account overload cooldown")
	}
}

func TestHandleConfirmedOverloadSignal_OverridesSyntheticGuard(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	account := &Account{ID: 104, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	shouldDisable := svc.HandleConfirmedOverloadSignal(
		withSyntheticUpstreamError(context.Background()),
		account,
		http.Header{},
		[]byte(`{"error":{"message":"Our servers are currently overloaded. Please try again later"}}`),
	)

	require.False(t, shouldDisable)
	require.Equal(t, 1, accountRepo.overloadCalls)
}

func TestHandleConfirmedOverloadSignal_DisabledSettingSkipsAccount(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: false, CooldownMinutes: 10})
	settingRepo.data[SettingKeyOverloadCooldownSettings] = string(data)
	settingSvc := NewSettingService(settingRepo, &config.Config{})
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(settingSvc)
	account := &Account{ID: 105, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	shouldDisable := svc.HandleConfirmedOverloadSignal(
		context.Background(),
		account,
		http.Header{},
		[]byte(`{"error":{"message":"Selected model is at capacity"}}`),
	)

	require.False(t, shouldDisable)
	require.Zero(t, accountRepo.overloadCalls)
}

func TestHandleUpstreamError_Custom502UsesCustomErrorPolicy(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{}
	svc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
	account := &Account{
		ID:       103,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(http.StatusBadGateway)},
		},
	}

	shouldDisable := svc.HandleUpstreamError(
		context.Background(), account, http.StatusBadGateway, http.Header{}, []byte(`{"error":{"message":"custom policy"}}`),
	)

	require.True(t, shouldDisable)
	require.Zero(t, accountRepo.overloadCalls, "selected custom error codes must bypass overload cooldown")
	require.Equal(t, 1, accountRepo.errorCalls)
}

func TestHandleGrokAccountUpstreamError_OnlyRealHTTPOverloadPausesAccount(t *testing.T) {
	t.Run("real HTTP response", func(t *testing.T) {
		accountRepo := &overloadAccountRepoStub{}
		rateLimitSvc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
		gatewaySvc := &OpenAIGatewayService{accountRepo: accountRepo, rateLimitService: rateLimitSvc}
		account := &Account{ID: 104, Platform: PlatformGrok, Type: AccountTypeOAuth}

		before := time.Now()
		gatewaySvc.handleGrokAccountUpstreamError(
			context.Background(), account, http.StatusBadGateway, http.Header{}, []byte(`{"error":{"message":"bad gateway"}}`),
		)

		require.Equal(t, 1, accountRepo.overloadCalls)
		require.WithinDuration(t, before.Add(10*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
	})

	t.Run("WS payload status mapping", func(t *testing.T) {
		accountRepo := &overloadAccountRepoStub{}
		rateLimitSvc := NewRateLimitService(accountRepo, nil, &config.Config{}, nil, nil)
		gatewaySvc := &OpenAIGatewayService{accountRepo: accountRepo, rateLimitService: rateLimitSvc}
		account := &Account{ID: 105, Platform: PlatformGrok, Type: AccountTypeOAuth}

		before := time.Now()
		gatewaySvc.handleGrokAccountUpstreamError(
			withSyntheticUpstreamError(context.Background()), account, http.StatusBadGateway, http.Header{}, []byte(`{"error":{"message":"bad gateway"}}`),
		)

		require.Zero(t, accountRepo.overloadCalls)
		require.Equal(t, 1, accountRepo.tempUnschedCalls)
		require.WithinDuration(t, before.Add(2*time.Minute), accountRepo.lastTempUnschedEnd, 2*time.Second)
	})
}

// ===========================================================================
// Model: defaults & JSON round-trip
// ===========================================================================

func TestDefaultOverloadCooldownSettings(t *testing.T) {
	d := DefaultOverloadCooldownSettings()
	require.True(t, d.Enabled)
	require.Equal(t, 10, d.CooldownMinutes)
}

func TestOverloadCooldownSettings_JSONRoundTrip(t *testing.T) {
	original := OverloadCooldownSettings{Enabled: false, CooldownMinutes: 42}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded OverloadCooldownSettings
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, original, decoded)

	// Verify JSON uses snake_case field names
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	_, hasEnabled := raw["enabled"]
	_, hasCooldown := raw["cooldown_minutes"]
	require.True(t, hasEnabled, "JSON must use 'enabled'")
	require.True(t, hasCooldown, "JSON must use 'cooldown_minutes'")
}
