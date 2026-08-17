package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newTestOAuthAccount(id int64, extra map[string]any) *Account {
	clonedExtra := make(map[string]any, len(extra)+1)
	for key, value := range extra {
		clonedExtra[key] = value
	}
	if _, exists := clonedExtra[codexFingerprintSeedExtraKey]; !exists {
		clonedExtra[codexFingerprintSeedExtraKey] = fmt.Sprintf("00000000-0000-4000-8000-%012x", id)
	}
	return &Account{
		ID:       id,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    clonedExtra,
	}
}

type codexFingerprintSeedStoreStub struct {
	AccountRepository
	mu         sync.Mutex
	seed       string
	err        error
	calls      int
	candidates []string
}

func (s *codexFingerprintSeedStoreStub) EnsureCodexFingerprintSeed(_ context.Context, _ int64, candidate string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.candidates = append(s.candidates, candidate)
	if s.err != nil {
		return "", s.err
	}
	if s.seed == "" {
		s.seed = candidate
	}
	return s.seed, nil
}

// --- deriveStableUUIDv4 ---

func TestDeriveStableUUIDv4_Deterministic(t *testing.T) {
	a := deriveStableUUIDv4("test-seed-1")
	b := deriveStableUUIDv4("test-seed-1")
	assert.Equal(t, a, b, "同一种子应返回相同结果")
}

func TestDeriveStableUUIDv4_DifferentSeeds(t *testing.T) {
	a := deriveStableUUIDv4("seed-a")
	b := deriveStableUUIDv4("seed-b")
	assert.NotEqual(t, a, b, "不同种子应返回不同结果")
}

func TestDeriveStableUUIDv4_ValidFormat(t *testing.T) {
	result := deriveStableUUIDv4("test-seed")
	parsed, err := uuid.Parse(result)
	require.NoError(t, err, "应返回合法 UUID 格式")
	assert.Equal(t, uuid.Version(4), parsed.Version(), "应为 UUIDv4")
	assert.Equal(t, uuid.RFC4122, parsed.Variant(), "应为 RFC4122 变体")
}

// --- GetCodexFingerprintMode ---

func TestGetCodexFingerprintMode(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected codexFingerprintMode
	}{
		{"nil 账号", nil, codexFingerprintOff},
		{"非 OAuth 账号", &Account{Platform: PlatformOpenAI, Type: "api_key"}, codexFingerprintOff},
		// 收敛是显式 opt-in：缺省/空/非法一律 off（#5610）。存量账号普遍没有这个
		// extra 键，升级不得把它们静默切进收敛。
		{"无 extra 默认 off", newTestOAuthAccount(1, nil), codexFingerprintOff},
		{"空值默认 off", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: ""}), codexFingerprintOff},
		{"非法值默认 off", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "invalid"}), codexFingerprintOff},
		{"显式 off", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "off"}), codexFingerprintOff},
		{"device", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "device"}), codexFingerprintDevice},
		{"session", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "session"}), codexFingerprintSession},
		{"full", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "full"}), codexFingerprintFull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.account.GetCodexFingerprintMode())
		})
	}
}

// --- resolveConvergedInstallationID ---

func TestResolveConvergedInstallationID_UsesDeviceID(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{"openai_device_id": "real-device-id"})
	assert.Equal(t, "real-device-id", resolveConvergedInstallationID(account))
}

func TestResolveConvergedInstallationID_DerivesFromPersistentSeed(t *testing.T) {
	account := newTestOAuthAccount(42, nil)
	result := resolveConvergedInstallationID(account)
	_, err := uuid.Parse(result)
	require.NoError(t, err, "派生值应为合法 UUID")
	assert.Equal(t, result, resolveConvergedInstallationID(account), "确定性")
}

func TestResolveConvergedInstallationID_SameLocalIDDifferentSeeds(t *testing.T) {
	a := resolveConvergedInstallationID(newTestOAuthAccount(1, map[string]any{
		codexFingerprintSeedExtraKey: "11111111-1111-4111-8111-111111111111",
	}))
	b := resolveConvergedInstallationID(newTestOAuthAccount(1, map[string]any{
		codexFingerprintSeedExtraKey: "22222222-2222-4222-8222-222222222222",
	}))
	assert.NotEqual(t, a, b)
}

func TestResolveCodexFingerprintIDs_SameLocalIDDifferentSeedsNeverCollide(t *testing.T) {
	a := newTestOAuthAccount(1, map[string]any{
		codexFingerprintSeedExtraKey: "11111111-1111-4111-8111-111111111111",
	})
	b := newTestOAuthAccount(1, map[string]any{
		codexFingerprintSeedExtraKey: "22222222-2222-4222-8222-222222222222",
	})
	idsA := resolveCodexFingerprintIDs(a, "same-client-session", codexFingerprintFull)
	idsB := resolveCodexFingerprintIDs(b, "same-client-session", codexFingerprintFull)
	require.NotNil(t, idsA)
	require.NotNil(t, idsB)
	assert.NotEqual(t, idsA.installationID, idsB.installationID)
	assert.NotEqual(t, idsA.sessionID, idsB.sessionID)
	assert.NotEqual(t, idsA.threadID, idsB.threadID)
	assert.NotEqual(t, idsA.windowID, idsB.windowID)
}

func TestDiscardCodexFingerprintSeedForNewAccount(t *testing.T) {
	account := newTestOAuthAccount(43, map[string]any{
		codexFingerprintSeedExtraKey: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"ordinary":                   "kept",
	})
	DiscardCodexFingerprintSeedForNewAccount(account)
	assert.Empty(t, codexFingerprintSeed(account))
	assert.Equal(t, "kept", account.Extra["ordinary"])
}

// --- resolveConvergedThreadID ---

func TestResolveConvergedThreadID_PerClientSession(t *testing.T) {
	account := newTestOAuthAccount(1, nil)
	a := resolveConvergedThreadID(account, "session-aaa")
	b := resolveConvergedThreadID(account, "session-bbb")
	assert.NotEqual(t, a, b, "不同客户端 session 应得到不同 thread_id")
}

func TestResolveConvergedThreadID_Deterministic(t *testing.T) {
	account := newTestOAuthAccount(1, nil)
	a := resolveConvergedThreadID(account, "session-aaa")
	b := resolveConvergedThreadID(account, "session-aaa")
	assert.Equal(t, a, b, "同一客户端 session 应得到相同 thread_id")
}

func TestResolveConvergedThreadID_EmptySession(t *testing.T) {
	account := newTestOAuthAccount(1, nil)
	assert.Equal(t, "", resolveConvergedThreadID(account, ""))
}

// --- off 模式：resolveCodexFingerprintIDsFromRequest 返回 nil ---

func TestResolveCodexFingerprintIDsFromRequest_ExplicitOff(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "off"})
	ids := resolveCodexFingerprintIDsFromRequest(account, nil)
	assert.Nil(t, ids, "显式 off 模式应返回 nil")
}

// 未显式配置的存量账号不得被收敛（#5610）：默认返回 nil，出站身份保持
// v0.1.175 之前的客户端原值。
func TestResolveCodexFingerprintIDsFromRequest_DefaultIsOff(t *testing.T) {
	account := newTestOAuthAccount(1, nil)
	assert.Nil(t, resolveCodexFingerprintIDsFromRequest(account, nil), "无 extra 应视为 off")
}

// 管理员显式 opt-in 的账号行为不变。
func TestResolveCodexFingerprintIDsFromRequest_ExplicitOptInHonored(t *testing.T) {
	for _, mode := range []string{"device", "session", "full"} {
		t.Run(mode, func(t *testing.T) {
			account := newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: mode})
			headers := http.Header{}
			if mode == "session" {
				headers.Set("session-id", "client-session")
			}
			ids := resolveCodexFingerprintIDsFromRequest(account, headers)
			require.NotNil(t, ids, "显式配置必须生效")
			assert.Equal(t, codexFingerprintMode(mode), ids.mode)
			assert.NotEmpty(t, ids.installationID)
		})
	}
}

func TestResolveCodexFingerprintIDsForRequest_PersistsSeedOnce(t *testing.T) {
	account := &Account{
		ID:       71,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			codexFingerprintModeExtraKey: "session",
		},
	}
	store := &codexFingerprintSeedStoreStub{}
	svc := &OpenAIGatewayService{accountRepo: store}
	c := newFingerprintStageTestContext(t)
	c.Request.Header.Set("session-id", "client-session-71")
	c.Set("api_key", &APIKey{ID: 91})

	first := svc.resolveCodexFingerprintIDsForRequest(context.Background(), c, account, []byte(`{"prompt_cache_key":"client-session-71"}`))
	require.NotNil(t, first)
	require.Equal(t, codexFingerprintSession, first.mode)
	require.Len(t, store.candidates, 1)
	seed := store.seed
	require.NotEmpty(t, seed)
	assert.Empty(t, codexFingerprintSeed(account), "shared scheduler snapshots are not mutated on the request path")
	fingerprintAccount := accountWithCodexFingerprintSeed(account, seed)
	require.Equal(t, resolveConvergedSessionIDForClient(fingerprintAccount, "client-session-71", 91), first.sessionID)

	second := svc.resolveCodexFingerprintIDsForRequest(context.Background(), c, account, []byte(`{"prompt_cache_key":"client-session-71"}`))
	require.NotNil(t, second)
	assert.Equal(t, 1, store.calls, "a materialized seed must not be written again")
	assert.Equal(t, first.installationID, second.installationID)
	assert.Equal(t, first.sessionID, second.sessionID)
	assert.Equal(t, first.threadID, second.threadID)
	assert.NotEqual(t, first.turnID, second.turnID)

	// A fresh gateway/account snapshot has no in-memory seed cache, but the
	// repository's persisted value keeps the upstream identity stable.
	restartedAccount := &Account{
		ID:       account.ID,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{codexFingerprintModeExtraKey: "session"},
	}
	restartedSvc := &OpenAIGatewayService{accountRepo: store}
	restarted := restartedSvc.resolveCodexFingerprintIDsForRequest(context.Background(), c, restartedAccount, []byte(`{"prompt_cache_key":"client-session-71"}`))
	require.NotNil(t, restarted)
	assert.Equal(t, 2, store.calls)
	assert.Equal(t, first.installationID, restarted.installationID)
	assert.Equal(t, first.sessionID, restarted.sessionID)
	assert.Equal(t, first.threadID, restarted.threadID)
}

func TestResolveCodexFingerprintIDsForRequest_SeedFailureFailsOpen(t *testing.T) {
	account := &Account{
		ID:       72,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			codexFingerprintModeExtraKey: "full",
		},
	}
	store := &codexFingerprintSeedStoreStub{err: errors.New("database unavailable")}
	svc := &OpenAIGatewayService{accountRepo: store}

	ids := svc.resolveCodexFingerprintIDsForRequest(context.Background(), newFingerprintStageTestContext(t), account, []byte(`{"prompt_cache_key":"client-session-72"}`))
	assert.Nil(t, ids)
	assert.Empty(t, codexFingerprintSeed(account))
	assert.Equal(t, 1, store.calls)
}

func TestResolveCodexFingerprintIDsForRequest_ConcurrentFirstUseKeepsAccountSnapshotReadOnly(t *testing.T) {
	account := &Account{
		ID:       721,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			codexFingerprintModeExtraKey: "session",
		},
	}
	store := &codexFingerprintSeedStoreStub{}
	svc := &OpenAIGatewayService{accountRepo: store}

	const workers = 16
	contexts := make([]*gin.Context, workers)
	for i := range contexts {
		contexts[i] = newFingerprintStageTestContext(t)
		contexts[i].Request.Header.Set("session-id", "concurrent-client-session")
	}
	results := make(chan *codexFingerprintIDs, workers)
	var wg sync.WaitGroup
	for _, c := range contexts {
		wg.Add(1)
		go func(c *gin.Context) {
			defer wg.Done()
			results <- svc.resolveCodexFingerprintIDsForRequest(context.Background(), c, account, []byte(`{"prompt_cache_key":"concurrent-client-session"}`))
		}(c)
	}
	wg.Wait()
	close(results)

	var first *codexFingerprintIDs
	for ids := range results {
		require.NotNil(t, ids)
		if first == nil {
			first = ids
			continue
		}
		assert.Equal(t, first.installationID, ids.installationID)
		assert.Equal(t, first.sessionID, ids.sessionID)
		assert.Equal(t, first.threadID, ids.threadID)
	}
	assert.Empty(t, codexFingerprintSeed(account))
	require.NotEmpty(t, store.seed)
}

func TestResolveCodexFingerprintIDs_SessionWithoutClientSignalFallsBackToDevice(t *testing.T) {
	account := newTestOAuthAccount(73, map[string]any{codexFingerprintModeExtraKey: "session"})
	ids := resolveCodexFingerprintIDsForClient(account, "", 7, codexFingerprintSession)
	require.NotNil(t, ids)
	assert.Equal(t, codexFingerprintDevice, ids.mode)
	assert.NotEmpty(t, ids.installationID)
	assert.Empty(t, ids.sessionID)
	assert.Empty(t, ids.threadID)
}

func TestResolveCodexFingerprintIDs_SessionScopeIncludesAPIKey(t *testing.T) {
	account := newTestOAuthAccount(74, map[string]any{codexFingerprintModeExtraKey: "session"})
	a := resolveCodexFingerprintIDsForClient(account, "shared-client-session", 1001, codexFingerprintSession)
	b := resolveCodexFingerprintIDsForClient(account, "shared-client-session", 1002, codexFingerprintSession)
	require.NotNil(t, a)
	require.NotNil(t, b)
	assert.NotEqual(t, a.sessionID, b.sessionID)
	assert.NotEqual(t, a.threadID, b.threadID)
}

// --- applyCodexFingerprintHeaders: off 模式 ---

func TestApplyCodexFingerprintHeaders_OffMode(t *testing.T) {
	h := http.Header{}
	h.Set("x-codex-installation-id", "original-install-id")
	h.Set("x-codex-window-id", "original-window-id")

	applyCodexFingerprintHeaders(h, nil)

	assert.Equal(t, "original-install-id", h.Get("x-codex-installation-id"), "nil ids 不改写")
	assert.Equal(t, "original-window-id", h.Get("x-codex-window-id"), "nil ids 不改写")
}

// --- applyCodexFingerprintHeaders: device 模式 ---

func TestApplyCodexFingerprintHeaders_DeviceMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		"openai_device_id":           "converged-device",
	})
	turnMetadata := `{"installation_id":"user-install","session_id":"user-session","sandbox":"seccomp"}`
	h := http.Header{}
	h.Set("x-codex-installation-id", "user-install")
	h.Set("x-codex-window-id", "user-window:0")
	h.Set("x-codex-turn-metadata", turnMetadata)

	ids := resolveCodexFingerprintIDsFromRequest(account, nil)
	applyCodexFingerprintHeaders(h, ids)

	assert.Equal(t, "converged-device", h.Get("x-codex-installation-id"), "installation_id 应收敛")
	assert.Equal(t, "user-window:0", h.Get("x-codex-window-id"), "device 模式不改写 window_id")

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &meta))
	assert.Equal(t, "converged-device", meta["installation_id"])
	assert.Equal(t, "user-session", meta["session_id"], "device 模式不改写 session_id")
	assert.Equal(t, "seccomp", meta["sandbox"], "非指纹字段保留原样")
}

// --- applyCodexFingerprintHeaders: session 模式 ---

func TestApplyCodexFingerprintHeaders_SessionMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-session-aaa")

	turnMetadata := `{"installation_id":"user-install","session_id":"user-session","thread_id":"user-thread","turn_id":"user-turn","window_id":"user-thread:0","sandbox":"seccomp","thread_source":"user"}`
	h := http.Header{}
	h.Set("x-codex-installation-id", "user-install")
	h.Set("x-codex-window-id", "user-thread:0")
	h.Set("x-codex-turn-metadata", turnMetadata)
	h.Set("x-client-request-id", "user-thread")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	applyCodexFingerprintHeaders(h, ids)

	convergedInstall := resolveConvergedInstallationID(account)
	convergedSession := resolveConvergedSessionIDForClient(account, "client-session-aaa", 0)
	convergedThread := resolveConvergedThreadID(account, "client-session-aaa")

	assert.Equal(t, convergedInstall, h.Get("x-codex-installation-id"))
	assert.Equal(t, convergedSession, h.Get("session-id"))
	assert.Equal(t, convergedSession, h.Get("session_id"), "下划线形式也应被改写")
	assert.Equal(t, convergedSession, h.Get("conversation_id"), "conversation_id 也应与 session 一致")
	assert.Equal(t, convergedThread, h.Get("thread-id"))
	assert.Equal(t, convergedThread, h.Get("x-client-request-id"))
	assert.Equal(t, convergedThread+":0", h.Get("x-codex-window-id"))

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &meta))
	assert.Equal(t, convergedInstall, meta["installation_id"])
	assert.Equal(t, convergedSession, meta["session_id"])
	assert.Equal(t, convergedThread, meta["thread_id"])
	assert.NotEqual(t, "user-turn", meta["turn_id"], "turn_id 应被新生成的值替换")
	assert.Equal(t, "seccomp", meta["sandbox"], "sandbox 保留原样")
	assert.Equal(t, "user", meta["thread_source"], "thread_source 保留原样")
}

// --- session 模式：不同客户端得到不同 thread ---

func TestApplyCodexFingerprintHeaders_SessionMode_DifferentClients(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})

	makeTurnMeta := func() string {
		return `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`
	}

	clientA := http.Header{}
	clientA.Set("session-id", "client-A")
	idsA := resolveCodexFingerprintIDsFromRequest(account, clientA)
	hA := http.Header{}
	hA.Set("x-codex-turn-metadata", makeTurnMeta())
	applyCodexFingerprintHeaders(hA, idsA)

	clientB := http.Header{}
	clientB.Set("session-id", "client-B")
	idsB := resolveCodexFingerprintIDsFromRequest(account, clientB)
	hB := http.Header{}
	hB.Set("x-codex-turn-metadata", makeTurnMeta())
	applyCodexFingerprintHeaders(hB, idsB)

	assert.NotEqual(t, hA.Get("session-id"), hB.Get("session-id"), "不同客户端 session_id 应隔离")
	assert.NotEqual(t, hA.Get("thread-id"), hB.Get("thread-id"), "不同客户端 thread_id 应不同")
	assert.NotEqual(t, hA.Get("x-codex-window-id"), hB.Get("x-codex-window-id"), "不同客户端 window_id 应不同")
	assert.Equal(t, hA.Get("x-codex-installation-id"), hB.Get("x-codex-installation-id"))
}

// --- full 模式 ---

func TestApplyCodexFingerprintHeaders_FullMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "full",
	})
	convergedSession := resolveConvergedSessionID(account)

	clientA := http.Header{}
	clientA.Set("session-id", "client-A")
	idsA := resolveCodexFingerprintIDsFromRequest(account, clientA)
	hA := http.Header{}
	hA.Set("x-codex-turn-metadata", `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`)
	applyCodexFingerprintHeaders(hA, idsA)

	clientB := http.Header{}
	clientB.Set("session-id", "client-B")
	idsB := resolveCodexFingerprintIDsFromRequest(account, clientB)
	hB := http.Header{}
	hB.Set("x-codex-turn-metadata", `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`)
	applyCodexFingerprintHeaders(hB, idsB)

	assert.Equal(t, hA.Get("thread-id"), hB.Get("thread-id"), "full 模式 thread_id 应相同")
	assert.Equal(t, convergedSession, hA.Get("thread-id"), "full 模式 thread_id 应等于 session_id")
	assert.Equal(t, hA.Get("x-codex-window-id"), hB.Get("x-codex-window-id"), "full 模式 window_id 应相同")
}

// --- H1 修复验证：头和体的 turn_id 一致性 ---

func TestFingerprintIDs_HeaderAndBody_TurnID_Consistent(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-session-xyz")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	// 头改写
	h := http.Header{}
	h.Set("x-codex-turn-metadata", `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`)
	applyCodexFingerprintHeaders(h, ids)

	// 体改写（使用同一份 ids）
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "x",
			"session_id":              "x",
			"turn_id":                 "x",
			"x-codex-turn-metadata":   `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`,
		},
	}
	applyCodexFingerprintRequestBody(reqBody, ids)

	// 从头 turn-metadata JSON 提取 turn_id
	var headerMeta map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &headerMeta))
	headerTurnID, ok := headerMeta["turn_id"].(string)
	require.True(t, ok, "头 turn-metadata 应包含 string 类型的 turn_id")

	// 从体 client_metadata 提取 turn_id
	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok, "请求体应包含 client_metadata")
	bodyTurnID, ok := cm["turn_id"].(string)
	require.True(t, ok, "体 client_metadata 应包含 string 类型的 turn_id")

	// 从体内嵌 turn-metadata JSON 提取 turn_id
	embeddedRaw, ok := cm["x-codex-turn-metadata"].(string)
	require.True(t, ok, "体 client_metadata 应包含 x-codex-turn-metadata 字符串")
	var bodyMeta map[string]any
	require.NoError(t, json.Unmarshal([]byte(embeddedRaw), &bodyMeta))
	bodyEmbeddedTurnID, ok := bodyMeta["turn_id"].(string)
	require.True(t, ok, "体内嵌 turn-metadata 应包含 string 类型的 turn_id")

	assert.Equal(t, headerTurnID, bodyTurnID, "头和体的 turn_id 必须一致")
	assert.Equal(t, headerTurnID, bodyEmbeddedTurnID, "头和体内嵌 turn-metadata 的 turn_id 必须一致")
	assert.Equal(t, ids.turnID, headerTurnID, "所有 turn_id 都应来自同一份 ids")
	assert.Equal(t, ids.sessionID, reqBody["prompt_cache_key"], "body 缓存键必须与出站 session_id 一致")
}

// --- applyCodexFingerprintClientMetadata ---

func TestApplyCodexFingerprintClientMetadata_OffMode(t *testing.T) {
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "original",
		},
	}
	modified := applyCodexFingerprintClientMetadata(reqBody, nil)
	assert.False(t, modified, "nil ids 不改写")
}

func TestApplyCodexFingerprintClientMetadata_DeviceMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		"openai_device_id":           "converged-device",
	})
	ids := resolveCodexFingerprintIDsFromRequest(account, nil)
	require.NotNil(t, ids)

	embeddedMeta := `{"installation_id":"x","session_id":"user-session","sandbox":"seccomp"}`
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "original-install",
			"session_id":              "user-session",
			"x-codex-turn-metadata":   embeddedMeta,
		},
	}

	modified := applyCodexFingerprintClientMetadata(reqBody, ids)
	require.True(t, modified)

	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "converged-device", cm["x-codex-installation-id"])
	assert.Equal(t, "user-session", cm["session_id"], "device 模式不改 session_id")

	turnMetaStr, ok := cm["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(turnMetaStr), &meta))
	assert.Equal(t, "converged-device", meta["installation_id"])
	assert.Equal(t, "seccomp", meta["sandbox"], "非指纹字段保留原样")
}

func TestApplyCodexFingerprintClientMetadata_SessionMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-session-aaa")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	embeddedMeta := `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0","sandbox":"seccomp"}`
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "original-install",
			"session_id":              "original-session",
			"x-codex-turn-metadata":   embeddedMeta,
		},
	}

	modified := applyCodexFingerprintClientMetadata(reqBody, ids)
	require.True(t, modified)

	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok)
	convergedInstall := resolveConvergedInstallationID(account)
	convergedSession := resolveConvergedSessionIDForClient(account, "client-session-aaa", 0)
	convergedThread := resolveConvergedThreadID(account, "client-session-aaa")

	assert.Equal(t, convergedInstall, cm["x-codex-installation-id"])
	assert.Equal(t, convergedSession, cm["session_id"])
	assert.Equal(t, convergedThread, cm["thread_id"])
	assert.Equal(t, convergedThread+":0", cm["x-codex-window-id"])

	turnMetaStr, ok := cm["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(turnMetaStr), &meta))
	assert.Equal(t, convergedInstall, meta["installation_id"])
	assert.Equal(t, convergedSession, meta["session_id"])
	assert.Equal(t, "seccomp", meta["sandbox"], "非指纹字段保留原样")
}

func TestApplyCodexFingerprintClientMetadata_FullMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "full",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "any-client")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"session_id":            "x",
			"thread_id":             "x",
			"x-codex-turn-metadata": `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`,
		},
	}

	modified := applyCodexFingerprintClientMetadata(reqBody, ids)
	require.True(t, modified)

	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok)
	convergedSession := resolveConvergedSessionID(account)

	assert.Equal(t, convergedSession, cm["session_id"])
	assert.Equal(t, convergedSession, cm["thread_id"], "full 模式 thread_id 应等于 session_id")
}

// --- extractClientSessionID ---

func TestExtractClientSessionID(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		expected string
	}{
		{"连字符形式优先", func() http.Header {
			h := http.Header{}
			h.Set("session-id", "hyphen-form")
			h.Set("session_id", "underscore-form")
			return h
		}(), "hyphen-form"},
		{"回退到下划线形式", func() http.Header {
			h := http.Header{}
			h.Set("session_id", "underscore-form")
			return h
		}(), "underscore-form"},
		{"都没有", http.Header{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractClientSessionID(tt.headers))
		})
	}
}

// --- 透传路径：raw 字节版 client_metadata 改写 ---

// rawVsMapClientMetadata 用同一份 ids 分别跑 map 版与 raw 字节版，
// 返回两侧最终的 client_metadata 解码结果。
func rawVsMapClientMetadata(t *testing.T, body []byte, ids *codexFingerprintIDs) (map[string]any, map[string]any) {
	t.Helper()

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	applyCodexFingerprintClientMetadata(decoded, ids)
	mapCM, _ := decoded["client_metadata"].(map[string]any)

	rawBody, changed, err := applyCodexFingerprintClientMetadataRaw(body, ids)
	require.NoError(t, err)
	require.True(t, changed)
	var rawDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawBody, &rawDecoded))
	rawCM, _ := rawDecoded["client_metadata"].(map[string]any)
	return mapCM, rawCM
}

func TestApplyCodexFingerprintClientMetadataRaw_MatchesMapVariant(t *testing.T) {
	embedded := `{\"installation_id\":\"real-install\",\"session_id\":\"real-session\",\"sandbox\":\"seatbelt\"}`
	bodies := map[string]string{
		"no_client_metadata": `{"model":"gpt-5.6-sol","input":[],"stream":true}`,
		"object_with_extras": `{"model":"gpt-5.6-sol","client_metadata":{"session_id":"client-session","traceparent":"00-abc-def-01","x-codex-turn-metadata":"` + embedded + `"},"stream":true}`,
		"non_object_value":   `{"model":"gpt-5.6-sol","client_metadata":"bogus","stream":true}`,
	}
	for _, mode := range []codexFingerprintMode{codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull} {
		account := newTestOAuthAccount(4242, nil)
		ids := resolveCodexFingerprintIDs(account, "client-sess-raw", mode)
		require.NotNil(t, ids)
		for name, body := range bodies {
			t.Run(string(mode)+"/"+name, func(t *testing.T) {
				mapCM, rawCM := rawVsMapClientMetadata(t, []byte(body), ids)
				assert.Equal(t, mapCM, rawCM, "raw 字节版与 map 版的 client_metadata 结果必须逐点一致")
			})
		}
	}
}

func TestApplyCodexFingerprintClientMetadataRaw_PreservesUnrelatedFields(t *testing.T) {
	account := newTestOAuthAccount(4243, nil)
	ids := resolveCodexFingerprintIDs(account, "client-sess-preserve", codexFingerprintSession)
	require.NotNil(t, ids)

	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":"hi"}],"stream":true,"prompt_cache_key":"pck-1"}`)
	out, changed, err := applyCodexFingerprintClientMetadataRaw(body, ids)
	require.NoError(t, err)
	require.True(t, changed)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(out, &decoded))
	assert.Equal(t, "gpt-5.6-sol", decoded["model"])
	assert.Equal(t, "pck-1", decoded["prompt_cache_key"])
	assert.Equal(t, true, decoded["stream"])
	cm, _ := decoded["client_metadata"].(map[string]any)
	require.NotNil(t, cm)
	assert.Equal(t, ids.sessionID, cm["session_id"])
	assert.Equal(t, ids.turnID, cm["turn_id"])
}

func TestApplyCodexFingerprintClientMetadataRaw_Noop(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol"}`)
	out, changed, err := applyCodexFingerprintClientMetadataRaw(body, nil)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, body, out)

	out, changed, err = applyCodexFingerprintClientMetadataRaw(nil, &codexFingerprintIDs{mode: codexFingerprintSession, installationID: "x"})
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Nil(t, out)
}

func TestApplyCodexFingerprintRequestBody_AlignsPromptCacheKeyWithSession(t *testing.T) {
	account := newTestOAuthAccount(4245, map[string]any{codexFingerprintModeExtraKey: "session"})
	ids := resolveCodexFingerprintIDsForClient(account, "client-cache-session", 44, codexFingerprintSession)
	require.NotNil(t, ids)

	mapBody := map[string]any{
		"model":            "gpt-5.6-sol",
		"prompt_cache_key": "client-cache-session",
	}
	require.True(t, applyCodexFingerprintRequestBody(mapBody, ids))
	assert.Equal(t, ids.sessionID, mapBody["prompt_cache_key"])

	rawBody := []byte(`{"model":"gpt-5.6-sol","prompt_cache_key":"client-cache-session"}`)
	rawOut, changed, err := applyCodexFingerprintRequestBodyRaw(rawBody, ids)
	require.NoError(t, err)
	require.True(t, changed)
	assert.Equal(t, ids.sessionID, gjson.GetBytes(rawOut, "prompt_cache_key").String())
	assert.Equal(t, ids.sessionID, gjson.GetBytes(rawOut, "client_metadata.session_id").String())
}

func TestApplyCodexFingerprintRequestBody_DeviceKeepsPromptCacheKey(t *testing.T) {
	account := newTestOAuthAccount(4246, map[string]any{
		codexFingerprintModeExtraKey: "device",
		"openai_device_id":           "configured-device",
	})
	ids := resolveCodexFingerprintIDs(account, "", codexFingerprintDevice)
	require.NotNil(t, ids)
	body := []byte(`{"model":"gpt-5.6-sol","prompt_cache_key":"client-cache-session"}`)
	out, changed, err := applyCodexFingerprintRequestBodyRaw(body, ids)
	require.NoError(t, err)
	require.True(t, changed)
	assert.Equal(t, "client-cache-session", gjson.GetBytes(out, "prompt_cache_key").String())
}

func TestNextCodexFingerprintIDsForTurn_RetainsStableScope(t *testing.T) {
	account := newTestOAuthAccount(4247, map[string]any{codexFingerprintModeExtraKey: "session"})
	first := resolveCodexFingerprintIDs(account, "client-turn-session", codexFingerprintSession)
	require.NotNil(t, first)
	next := nextCodexFingerprintIDsForTurn(first)
	require.NotNil(t, next)
	assert.Equal(t, first.installationID, next.installationID)
	assert.Equal(t, first.sessionID, next.sessionID)
	assert.Equal(t, first.threadID, next.threadID)
	assert.Equal(t, first.windowID, next.windowID)
	assert.NotEqual(t, first.turnID, next.turnID)
}

// --- context 暂存与出站头应用（透传/非透传共用 seam）---

func newFingerprintStageTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c
}

func TestStageCodexFingerprintIDs_NilOverwritesPreviousAccount(t *testing.T) {
	c := newFingerprintStageTestContext(t)
	accountA := newTestOAuthAccount(1001, nil)
	idsA := resolveCodexFingerprintIDs(accountA, "sess-x", codexFingerprintSession)
	require.NotNil(t, idsA)
	stageCodexFingerprintIDs(c, idsA)

	// failover 切到 off 模式账号：无条件覆写为 nil，上一账号 IDs 不得残留
	stageCodexFingerprintIDs(c, nil)

	h := http.Header{}
	h.Set("session_id", "isolated-session")
	accountB := newTestOAuthAccount(1002, map[string]any{"codex_fingerprint_mode": "off"})
	applyStagedCodexFingerprintHeaders(c, accountB, h)
	assert.Equal(t, "isolated-session", h.Get("session_id"), "off 账号不得应用上一账号的收敛 ID")
	assert.Empty(t, h.Get("x-codex-installation-id"))
}

func TestApplyStagedCodexFingerprintHeaders_SkipsNonOAuthAccount(t *testing.T) {
	c := newFingerprintStageTestContext(t)
	oauthIDs := resolveCodexFingerprintIDs(newTestOAuthAccount(1003, nil), "sess-y", codexFingerprintSession)
	require.NotNil(t, oauthIDs)
	stageCodexFingerprintIDs(c, oauthIDs)

	h := http.Header{}
	apiKeyAccount := &Account{ID: 1004, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	applyStagedCodexFingerprintHeaders(c, apiKeyAccount, h)
	assert.Empty(t, h.Get("x-codex-installation-id"), "stale 收敛 ID 不得应用到非 OAuth 账号")
}

func TestBuildUpstreamRequestOpenAIPassthrough_AppliesStagedFingerprint(t *testing.T) {
	svc := &OpenAIGatewayService{}
	// 收敛是显式 opt-in（#5610）：显式开启后验证透传路径的出站头收敛。
	account := newTestOAuthAccount(2001, map[string]any{
		"openai_oauth_passthrough": true,
		"codex_fingerprint_mode":   "session",
	})

	c := newFingerprintStageTestContext(t)
	c.Request.Header.Set("session_id", "real-client-session")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color")
	c.Request.Header.Set("originator", "codex_cli_rs")
	c.Request.Header.Set("x-codex-turn-metadata", `{"installation_id":"real-install","session_id":"real-session","sandbox":"seatbelt"}`)

	// 复刻 forwardOpenAIPassthrough 的解析+暂存 seam（默认 session 模式）
	ids := resolveCodexFingerprintIDsFromRequest(account, c.Request.Header)
	require.NotNil(t, ids)
	stageCodexFingerprintIDs(c, ids)

	body := []byte(`{"model":"gpt-5.6-sol","input":[],"stream":true}`)
	req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "test-token")
	require.NoError(t, err)

	assert.Equal(t, ids.sessionID, req.Header.Get("session_id"), "session 模式下出站 session_id 应为客户端范围的稳定伪匿名值")
	assert.Equal(t, ids.installationID, req.Header.Get("x-codex-installation-id"))
	assert.Equal(t, ids.windowID, req.Header.Get("x-codex-window-id"))
	assert.Equal(t, ids.threadID, req.Header.Get("x-client-request-id"))
	turnMetadata := req.Header.Get("x-codex-turn-metadata")
	require.NotEmpty(t, turnMetadata)
	assert.Contains(t, turnMetadata, ids.sessionID, "turn-metadata JSON 中的 session_id 应被收敛")
	assert.Contains(t, turnMetadata, `"sandbox":"seatbelt"`, "turn-metadata 未指定字段应原样保留")
}

func TestBuildUpstreamRequestOpenAIPassthrough_OffModeKeepsIsolatedSession(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := newTestOAuthAccount(2002, map[string]any{
		"openai_oauth_passthrough": true,
		"codex_fingerprint_mode":   "off",
	})

	c := newFingerprintStageTestContext(t)
	c.Request.Header.Set("session_id", "real-client-session")
	c.Request.Header.Set("originator", "codex_cli_rs")

	ids := resolveCodexFingerprintIDsFromRequest(account, c.Request.Header)
	require.Nil(t, ids)
	stageCodexFingerprintIDs(c, ids)

	body := []byte(`{"model":"gpt-5.6-sol","input":[],"stream":true}`)
	req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "test-token")
	require.NoError(t, err)

	assert.NotEmpty(t, req.Header.Get("session_id"))
	assert.NotEqual(t, resolveConvergedSessionID(account), req.Header.Get("session_id"), "off 模式不得收敛 session_id")
	assert.Empty(t, req.Header.Get("x-codex-window-id"))
}

func TestBuildOpenAIWSHeaders_AppliesStagedFingerprint(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := newTestOAuthAccount(2003, map[string]any{codexFingerprintModeExtraKey: "session"})
	c := newFingerprintStageTestContext(t)
	c.Request.Header.Set("session-id", "client-ws-session")
	ids := resolveCodexFingerprintIDsFromRequest(account, c.Request.Header)
	require.NotNil(t, ids)
	stageCodexFingerprintIDs(c, ids)

	headers, _, err := svc.buildOpenAIWSHeaders(
		context.Background(),
		c,
		account,
		"token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true,
		"",
		`{"session_id":"client-ws-session"}`,
		"client-ws-session",
		"gpt-5.6-sol",
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, ids.installationID, headers.Get("x-codex-installation-id"))
	assert.Equal(t, ids.sessionID, headers.Get("session_id"))
	assert.Equal(t, ids.sessionID, headers.Get("session-id"))
	assert.Equal(t, ids.sessionID, headers.Get("conversation_id"))
	assert.Equal(t, ids.threadID, headers.Get("thread-id"))
	assert.Equal(t, ids.threadID, headers.Get("x-client-request-id"))
}

func TestApplyCodexFingerprintClientMetadataRaw_NonObjectBodyUntouched(t *testing.T) {
	account := newTestOAuthAccount(4244, nil)
	ids := resolveCodexFingerprintIDs(account, "client-sess-nonobj", codexFingerprintSession)
	require.NotNil(t, ids)

	for _, body := range []string{`[1,2,3]`, `"plain string"`, `not json at all`} {
		out, changed, err := applyCodexFingerprintClientMetadataRaw([]byte(body), ids)
		require.NoError(t, err)
		assert.False(t, changed, "非 JSON 对象 body 不应被改写: %s", body)
		assert.Equal(t, []byte(body), out)
	}
}
