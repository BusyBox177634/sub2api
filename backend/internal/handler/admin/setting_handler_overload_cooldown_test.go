package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type overloadCooldownSettingsRepoStub struct {
	values map[string]string
}

func (r *overloadCooldownSettingsRepoStub) Get(_ context.Context, key string) (*service.Setting, error) {
	value, exists := r.values[key]
	if !exists {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: value}, nil
}

func (r *overloadCooldownSettingsRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return r.values[key], nil
}

func (r *overloadCooldownSettingsRepoStub) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *overloadCooldownSettingsRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, exists := r.values[key]; exists {
			values[key] = value
		}
	}
	return values, nil
}

func (r *overloadCooldownSettingsRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *overloadCooldownSettingsRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	values := make(map[string]string, len(r.values))
	for key, value := range r.values {
		values[key] = value
	}
	return values, nil
}

func (r *overloadCooldownSettingsRepoStub) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func newOverloadCooldownSettingsHandler(t *testing.T, settings service.OverloadCooldownSettings) (*SettingHandler, *overloadCooldownSettingsRepoStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	encoded, err := json.Marshal(settings)
	require.NoError(t, err)
	repo := &overloadCooldownSettingsRepoStub{
		values: map[string]string{service.SettingKeyOverloadCooldownSettings: string(encoded)},
	}
	settingService := service.NewSettingService(repo, &config.Config{})
	return NewSettingHandler(settingService, nil, nil, nil, nil, nil, nil), repo
}

func updateOverloadCooldownSettingsForTest(t *testing.T, handler *SettingHandler, payload string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/overload-cooldown", bytes.NewBufferString(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler.UpdateOverloadCooldownSettings(ctx)
	return recorder
}

func TestOverloadCooldownSettingsHandler_ManagesOpenAIMessageList(t *testing.T) {
	initialMessages := []string{"Custom capacity exhausted"}
	handler, repo := newOverloadCooldownSettingsHandler(t, service.OverloadCooldownSettings{
		Enabled:                true,
		CooldownMinutes:        10,
		OpenAIOverloadMessages: initialMessages,
	})

	getRecorder := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRecorder)
	getCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/overload-cooldown", nil)
	handler.GetOverloadCooldownSettings(getCtx)
	require.Equal(t, http.StatusOK, getRecorder.Code)

	var getPayload struct {
		Data struct {
			OpenAIOverloadMessages        []string `json:"openai_overload_messages"`
			DefaultOpenAIOverloadMessages []string `json:"default_openai_overload_messages"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRecorder.Body.Bytes(), &getPayload))
	require.Equal(t, initialMessages, getPayload.Data.OpenAIOverloadMessages)
	require.Equal(t, service.DefaultOpenAIOverloadCooldownMessages(), getPayload.Data.DefaultOpenAIOverloadMessages)

	// Old clients send only the original two fields. Their update must retain a
	// previously configured application-message list.
	legacyRecorder := updateOverloadCooldownSettingsForTest(t, handler, `{"enabled":true,"cooldown_minutes":20}`)
	require.Equal(t, http.StatusOK, legacyRecorder.Code)

	var stored service.OverloadCooldownSettings
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyOverloadCooldownSettings]), &stored))
	require.Equal(t, initialMessages, stored.OpenAIOverloadMessages)
	require.Equal(t, 20, stored.CooldownMinutes)

	emptyRecorder := updateOverloadCooldownSettingsForTest(t, handler, `{"enabled":true,"cooldown_minutes":20,"openai_overload_messages":[]}`)
	require.Equal(t, http.StatusOK, emptyRecorder.Code)
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyOverloadCooldownSettings]), &stored))
	require.NotNil(t, stored.OpenAIOverloadMessages)
	require.Empty(t, stored.OpenAIOverloadMessages)
}
