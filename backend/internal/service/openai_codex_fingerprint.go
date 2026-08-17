package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// codexFingerprintIDsContextKey 是暂存在 gin context 的收敛 ID 集合键。
// 由 Forward（非透传）或 forwardOpenAIPassthrough（透传）解析后写入，请求
// 构造器读取用于出站头改写——请求体与出站头必须共享同一份 IDs，保证
// turn_id 等随机字段一致。
const codexFingerprintIDsContextKey = "codex_fingerprint_ids"

// stageCodexFingerprintIDs 将本 attempt 解析出的收敛 ID 暂存到 gin context。
// 必须无条件覆写（含 nil）：failover 从收敛账号切到 off 账号时，上一账号的
// IDs 不得残留并被误应用到新账号的出站头（typed-nil 由应用侧 nil 守卫吸收）。
func stageCodexFingerprintIDs(c *gin.Context, ids *codexFingerprintIDs) {
	if c != nil {
		c.Set(codexFingerprintIDsContextKey, ids)
	}
}

// applyStagedCodexFingerprintHeaders 读取 context 暂存的收敛 ID 并改写出站头。
// 非透传与透传两个请求构造器共用本函数，防止应用语义漂移。仅 OAuth 账号
// 生效（stale 键在账号类型混合 failover 下由该门挡住）。
func applyStagedCodexFingerprintHeaders(c *gin.Context, account *Account, h http.Header) {
	if c == nil || account == nil || account.Type != AccountTypeOAuth {
		return
	}
	value, ok := c.Get(codexFingerprintIDsContextKey)
	if !ok {
		return
	}
	if ids, ok := value.(*codexFingerprintIDs); ok {
		applyCodexFingerprintHeaders(h, ids)
	}
}

// codexFingerprintMode 控制 OAuth 账号出站请求的设备指纹收敛强度。
// 多人共享同一 OAuth 账号时，每个用户的 Codex 客户端会携带各自不同的
// installation_id / session_id / thread_id，上游据此判定设备数和会话数。
// 收敛模式将这些标识改写为由账号级随机 seed 派生的稳定伪匿名值。
type codexFingerprintMode string

const (
	// codexFingerprintOff 不做任何收敛，原样透传客户端标识。
	// 这是默认值：收敛是显式 opt-in 的（见 GetCodexFingerprintMode）。
	codexFingerprintOff codexFingerprintMode = "off"
	// codexFingerprintDevice 仅收敛 installation_id 为账号级恒定值。
	// 上游看到 1 台设备 + 多会话（每用户各自的 session）。
	codexFingerprintDevice codexFingerprintMode = "device"
	// codexFingerprintSession 收敛 installation_id，并把每个客户端 session/thread
	// 改写为账号内稳定的伪匿名值，保留客户端之间的缓存边界。
	// 上游看到 1 台设备 + N 个稳定会话/线程，而不是一个共享缓存域。
	codexFingerprintSession codexFingerprintMode = "session"
	// codexFingerprintFull 收敛所有标识：installation_id + session_id + thread_id。
	// 上游看到 1 台设备 + 1 会话 + 1 线程，最激进。
	codexFingerprintFull codexFingerprintMode = "full"
)

const (
	codexFingerprintModeExtraKey = "codex_fingerprint_mode"
	// CodexFingerprintSeedExtraKey is stored internally in account extra. It is
	// deliberately hidden from ordinary account responses and cannot be edited
	// through account update APIs.
	CodexFingerprintSeedExtraKey = "codex_fingerprint_seed"
	// codexFingerprintSeedExtraKey 是内部维护的账号级随机种子。它从不发送给
	// 上游，只用于派生稳定的 Codex 标识，避免把本地自增 account.ID 暴露为
	// 跨部署可碰撞的指纹来源。
	codexFingerprintSeedExtraKey = CodexFingerprintSeedExtraKey
)

// codexFingerprintSeedStore 是有状态账号仓储的一个窄能力。保持它独立于
// AccountRepository，可避免所有只读测试桩为这个运行期写入实现无关方法。
// 生产仓储必须以原子“取或创建”语义实现，才能处理多实例首请求竞争。
type codexFingerprintSeedStore interface {
	EnsureCodexFingerprintSeed(ctx context.Context, accountID int64, candidate string) (string, error)
}

// GetCodexFingerprintMode 从账号 extra JSON 读取指纹收敛模式。
//
// **收敛是显式 opt-in**：未设置、空值或非法值一律按 off 处理，只有管理员
// 明确配置 device / session / full 才收敛。
//
// 历史：v0.1.175（#5553）把缺省值当作 session，导致升级后存量 OAuth 账号
// （普遍没有这个 extra 键）的每个非透传请求都被静默改写 installation /
// session / thread / turn / window 五类标识；#5555、#5556、#5582 报告的额度
// 缩水都卡在该版本边界，并有"回退 v0.1.173 即恢复"与"新账号开收敛后降额"
// 的 A/B 实测。上游的配额判定策略不可观测，因此这里取兼容安全的一侧：
// 不显式 opt-in 就保持 v0.1.175 之前的客户端身份（#5610）。
func (a *Account) GetCodexFingerprintMode() codexFingerprintMode {
	if a == nil || !a.IsOpenAIOAuth() {
		return codexFingerprintOff
	}
	raw := strings.TrimSpace(a.GetExtraString(codexFingerprintModeExtraKey))
	switch codexFingerprintMode(raw) {
	case codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull:
		return codexFingerprintMode(raw)
	default:
		return codexFingerprintOff
	}
}

// deriveStableUUIDv4 从种子确定性派生一个 UUIDv4 格式的字符串。
// 同一种子永远返回同一值。
func deriveStableUUIDv4(seed string) string {
	h := sha256.Sum256([]byte(seed))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16])
}

// codexFingerprintSeed 返回账号 extra 中合法、规范化的随机 UUID 种子。
// 非法或缺失值一律视为未初始化，不能退回到 account.ID。
func codexFingerprintSeed(account *Account) string {
	if account == nil {
		return ""
	}
	raw := strings.TrimSpace(account.GetExtraString(codexFingerprintSeedExtraKey))
	if raw == "" {
		return ""
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.String()
}

// DiscardCodexFingerprintSeedForNewAccount prevents an import/copy presented as
// a new account record from silently inheriting another account's identity.
// It is intentionally called by the repository at the final create boundary,
// so every account creation/import path gets the same protection.
func DiscardCodexFingerprintSeedForNewAccount(account *Account) {
	if account != nil && account.Extra != nil {
		delete(account.Extra, codexFingerprintSeedExtraKey)
	}
}

// preserveCodexFingerprintSeedOnAccountUpdate keeps the internally generated
// seed stable across full-extra admin edits. Incoming values are ignored: a seed
// can only be created atomically by the gateway path.
func preserveCodexFingerprintSeedOnAccountUpdate(existing, next map[string]any) {
	if next == nil {
		return
	}
	delete(next, codexFingerprintSeedExtraKey)
	if seed := codexFingerprintSeed(&Account{Extra: existing}); seed != "" {
		next[codexFingerprintSeedExtraKey] = seed
	}
}

// accountWithCodexFingerprintSeed returns a request-local account copy. Gateway
// request paths must not mutate Account.Extra in place: scheduler snapshots can
// be shared by concurrent requests, and a map write here would race readers.
func accountWithCodexFingerprintSeed(account *Account, seed string) *Account {
	if account == nil || seed == "" || codexFingerprintSeed(account) == seed {
		return account
	}
	copyAccount := *account
	copyExtra := make(map[string]any, len(account.Extra)+1)
	for key, value := range account.Extra {
		copyExtra[key] = value
	}
	copyExtra[codexFingerprintSeedExtraKey] = seed
	copyAccount.Extra = copyExtra
	return &copyAccount
}

func deriveCodexFingerprintUUID(seed, purpose, scope string) string {
	if seed == "" {
		return ""
	}
	return deriveStableUUIDv4("sub2api:codex-fingerprint:v2:" + purpose + ":" + seed + ":" + scope)
}

// resolveConvergedInstallationID returns an account-stable installation ID.
// An explicitly configured openai_device_id remains the highest-priority value.
func resolveConvergedInstallationID(account *Account) string {
	if account == nil {
		return ""
	}
	if deviceID := account.GetOpenAIDeviceID(); deviceID != "" {
		return deviceID
	}
	return deriveCodexFingerprintUUID(codexFingerprintSeed(account), "installation", "")
}

// resolveConvergedSessionID returns the account-level session used exclusively
// by full convergence mode.
func resolveConvergedSessionID(account *Account) string {
	return deriveCodexFingerprintUUID(codexFingerprintSeed(account), "full-session", "")
}

func codexFingerprintClientScope(clientSessionID string, apiKeyID int64) string {
	clientSessionID = strings.TrimSpace(clientSessionID)
	if clientSessionID == "" {
		return ""
	}
	return fmt.Sprintf("api-key:%d:client:%s", apiKeyID, clientSessionID)
}

// resolveConvergedSessionIDForClient preserves client cache boundaries in
// session mode while hiding the raw client identity behind the account seed.
func resolveConvergedSessionIDForClient(account *Account, clientSessionID string, apiKeyID int64) string {
	scope := codexFingerprintClientScope(clientSessionID, apiKeyID)
	if scope == "" {
		return ""
	}
	return deriveCodexFingerprintUUID(codexFingerprintSeed(account), "session", scope)
}

// resolveConvergedThreadID 按客户端原始 session-id 确定性派生 thread_id。
// 保留旧签名供纯单元测试/调用点使用；生产路径还会传入 API Key 做租户隔离。
func resolveConvergedThreadID(account *Account, clientSessionID string) string {
	return resolveConvergedThreadIDForClient(account, clientSessionID, 0)
}

func resolveConvergedThreadIDForClient(account *Account, clientSessionID string, apiKeyID int64) string {
	scope := codexFingerprintClientScope(clientSessionID, apiKeyID)
	if scope == "" {
		return ""
	}
	return deriveCodexFingerprintUUID(codexFingerprintSeed(account), "thread", scope)
}

// codexFingerprintIDs 收敛后的完整 ID 集合。
// 由 resolveCodexFingerprintIDs 一次性生成，同一个实例在头改写和体改写之间共享，
// 确保所有载体中的 turn_id 等随机字段一致。
type codexFingerprintIDs struct {
	mode           codexFingerprintMode
	installationID string
	sessionID      string
	threadID       string
	turnID         string
	windowID       string
}

// resolveCodexFingerprintIDs 按收敛模式计算出站 ID 集合。
// clientSessionID 是客户端原始的 session-id 头值（连字符形式），用于 session 模式下
// 派生账号内隔离的伪匿名 session/thread；full 模式使用账号级 session/thread。
// 返回 nil 表示 off 模式，不需要改写。
// 注意：包含随机生成的 turn_id，调用方必须只调用一次并共享结果给头改写和体改写。
func resolveCodexFingerprintIDs(account *Account, clientSessionID string, mode codexFingerprintMode) *codexFingerprintIDs {
	return resolveCodexFingerprintIDsForClient(account, clientSessionID, 0, mode)
}

func resolveCodexFingerprintIDsForClient(account *Account, clientSessionID string, apiKeyID int64, mode codexFingerprintMode) *codexFingerprintIDs {
	if mode == codexFingerprintOff {
		return nil
	}

	ids := &codexFingerprintIDs{mode: mode}

	ids.installationID = resolveConvergedInstallationID(account)
	if ids.installationID == "" {
		return nil
	}

	switch mode {
	case codexFingerprintDevice:
		return ids

	case codexFingerprintSession:
		// Session mode deliberately does not collapse users without a stable
		// client session signal into one account-wide cache domain. Degrade to
		// device-only convergence instead of fabricating a shared session.
		if strings.TrimSpace(clientSessionID) == "" {
			ids.mode = codexFingerprintDevice
			return ids
		}
		ids.sessionID = resolveConvergedSessionIDForClient(account, clientSessionID, apiKeyID)
		ids.threadID = resolveConvergedThreadIDForClient(account, clientSessionID, apiKeyID)
		if ids.sessionID == "" || ids.threadID == "" {
			return nil
		}
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.windowID = ids.threadID + ":0"
		return ids

	case codexFingerprintFull:
		ids.sessionID = resolveConvergedSessionID(account)
		ids.threadID = ids.sessionID
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.windowID = ids.threadID + ":0"
		return ids
	}

	return nil
}

func nextCodexFingerprintIDsForTurn(base *codexFingerprintIDs) *codexFingerprintIDs {
	if base == nil {
		return nil
	}
	next := *base
	if next.mode != codexFingerprintDevice {
		next.turnID = uuid.Must(uuid.NewV7()).String()
	}
	return &next
}

// extractClientSessionID 从请求头中提取客户端原始的会话标识。
// 优先取 session-id（连字符形式，Codex CLI 标准），回退到 session_id（下划线形式）。
// 返回的值尚未被 isolateOpenAISessionID 改写，是客户端的真实标识。
func extractClientSessionID(h http.Header) string {
	if h == nil {
		return ""
	}
	if v := strings.TrimSpace(h.Get("session-id")); v != "" {
		return v
	}
	if v := strings.TrimSpace(h.Get("session_id")); v != "" {
		return v
	}
	return strings.TrimSpace(h.Get("conversation_id"))
}

func extractCodexFingerprintClientSessionID(h http.Header, body []byte) string {
	if sessionID := extractClientSessionID(h); sessionID != "" {
		return sessionID
	}
	if len(body) == 0 {
		return ""
	}
	if promptCacheKey := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String()); promptCacheKey != "" {
		return promptCacheKey
	}
	return strings.TrimSpace(gjson.GetBytes(body, "client_metadata.session_id").String())
}

func codexFingerprintModeNeedsSeed(account *Account, mode codexFingerprintMode) bool {
	if account == nil || mode == codexFingerprintOff {
		return false
	}
	return mode != codexFingerprintDevice || strings.TrimSpace(account.GetOpenAIDeviceID()) == ""
}

func ensureCodexFingerprintSeedForAccount(ctx context.Context, accountRepo AccountRepository, account *Account) (string, error) {
	if seed := codexFingerprintSeed(account); seed != "" {
		return seed, nil
	}
	if account == nil || account.ID <= 0 {
		return "", fmt.Errorf("codex fingerprint seed requires a persisted account")
	}
	store, ok := accountRepo.(codexFingerprintSeedStore)
	if !ok || store == nil {
		return "", fmt.Errorf("codex fingerprint seed store is unavailable")
	}
	candidate, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate codex fingerprint seed: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	seed, err := store.EnsureCodexFingerprintSeed(ctx, account.ID, candidate.String())
	if err != nil {
		return "", fmt.Errorf("persist codex fingerprint seed: %w", err)
	}
	parsed, err := uuid.Parse(strings.TrimSpace(seed))
	if err != nil {
		return "", fmt.Errorf("persisted codex fingerprint seed is invalid: %w", err)
	}
	seed = parsed.String()
	return seed, nil
}

func (s *OpenAIGatewayService) ensureCodexFingerprintSeed(ctx context.Context, account *Account) (string, error) {
	if s == nil {
		return "", fmt.Errorf("codex fingerprint seed service is nil")
	}
	if seed := codexFingerprintSeed(account); seed != "" {
		return seed, nil
	}
	if account != nil && account.ID > 0 {
		if cached, ok := s.codexFingerprintSeedCache.Load(account.ID); ok {
			if seed, ok := cached.(string); ok {
				if parsed, err := uuid.Parse(strings.TrimSpace(seed)); err == nil {
					return parsed.String(), nil
				}
			}
			s.codexFingerprintSeedCache.Delete(account.ID)
		}
	}
	seed, err := ensureCodexFingerprintSeedForAccount(ctx, s.accountRepo, account)
	if err == nil && account != nil && account.ID > 0 {
		s.codexFingerprintSeedCache.Store(account.ID, seed)
	}
	return seed, err
}

// resolveCodexFingerprintIDsForRequest is the only gateway-facing resolver.
// It materializes the durable seed before any derived identifier can be sent
// upstream; a persistence failure safely falls back to ordinary passthrough.
func (s *OpenAIGatewayService) resolveCodexFingerprintIDsForRequest(ctx context.Context, c *gin.Context, account *Account, body []byte) *codexFingerprintIDs {
	if account == nil {
		return nil
	}
	mode := account.GetCodexFingerprintMode()
	if mode == codexFingerprintOff {
		return nil
	}
	fingerprintAccount := account
	if codexFingerprintModeNeedsSeed(account, mode) {
		seed, err := s.ensureCodexFingerprintSeed(ctx, account)
		if err != nil {
			slog.Warn("codex_fingerprint_seed_unavailable", "account_id", account.ID, "error", err)
			return nil
		}
		fingerprintAccount = accountWithCodexFingerprintSeed(account, seed)
	}
	var clientHeaders http.Header
	if c != nil && c.Request != nil {
		clientHeaders = c.Request.Header
	}
	return resolveCodexFingerprintIDsForClient(
		fingerprintAccount,
		extractCodexFingerprintClientSessionID(clientHeaders, body),
		getAPIKeyIDFromContext(c),
		mode,
	)
}

// resolveCodexFingerprintIDsFromRequest 从客户端原始请求头中提取 session-id，
// 结合账号配置一次性解析收敛 ID 集合。调用方应将返回的 ids 同时传给
// applyCodexFingerprintHeaders 和 applyCodexFingerprintRequestBody。
func resolveCodexFingerprintIDsFromRequest(account *Account, clientHeaders http.Header) *codexFingerprintIDs {
	if account == nil {
		return nil
	}
	mode := account.GetCodexFingerprintMode()
	if mode == codexFingerprintOff {
		return nil
	}
	return resolveCodexFingerprintIDsForClient(account, extractClientSessionID(clientHeaders), 0, mode)
}

// applyCodexFingerprintHeaders 按预计算的收敛 ID 改写出站 HTTP 头中的设备指纹。
// 在 buildUpstreamRequest 的白名单透传之后、enforceCodexIdentityHeaders 之前调用。
func applyCodexFingerprintHeaders(h http.Header, ids *codexFingerprintIDs) {
	if h == nil || ids == nil {
		return
	}

	// 所有非 off 模式都收敛 installation_id
	h.Set("x-codex-installation-id", ids.installationID)

	if ids.mode == codexFingerprintDevice {
		rewriteCodexTurnMetadataFields(h, map[string]any{
			"installation_id": ids.installationID,
		})
		return
	}

	// session / full 模式：改写所有相关头
	h.Set("x-codex-window-id", ids.windowID)
	h.Set("x-client-request-id", ids.threadID)
	// 连字符形式和下划线形式都改写，保证一致
	h.Set("session-id", ids.sessionID)
	h.Set("session_id", ids.sessionID)
	h.Set("conversation_id", ids.sessionID)
	h.Set("thread-id", ids.threadID)

	rewriteCodexTurnMetadataFields(h, map[string]any{
		"installation_id":         ids.installationID,
		"session_id":              ids.sessionID,
		"thread_id":               ids.threadID,
		"turn_id":                 ids.turnID,
		"window_id":               ids.windowID,
		"turn_started_at_unix_ms": time.Now().UnixMilli(),
	})
}

// rewriteCodexTurnMetadataFields 解析 x-codex-turn-metadata 头中的 JSON，
// 替换指定字段后回写。保留未指定字段原样（如 sandbox、thread_source 等）。
func rewriteCodexTurnMetadataFields(h http.Header, fields map[string]any) {
	raw := strings.TrimSpace(h.Get("x-codex-turn-metadata"))
	if raw == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return
	}
	for k, v := range fields {
		metadata[k] = v
	}
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return
	}
	h.Set("x-codex-turn-metadata", string(rebuilt))
}

// applyCodexFingerprintClientMetadata 按预计算的收敛 ID 改写请求体中的 client_metadata。
// 使用与头改写相同的 ids 实例，确保 turn_id 等随机字段一致。
func applyCodexFingerprintClientMetadata(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil || ids == nil {
		return false
	}

	existing, _ := reqBody["client_metadata"].(map[string]any)
	if existing == nil {
		existing = make(map[string]any)
	}

	if !applyCodexFingerprintToClientMetadataMap(existing, ids) {
		return false
	}
	reqBody["client_metadata"] = existing
	return true
}

// applyCodexFingerprintRequestBody applies all body-level convergence rules.
// In session/full mode prompt_cache_key is deliberately made identical to the
// outgoing session-id so upstream prompt-cache routing sees one coherent key.
func applyCodexFingerprintRequestBody(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil || ids == nil {
		return false
	}
	modified := applyCodexFingerprintClientMetadata(reqBody, ids)
	if ids.mode == codexFingerprintDevice || ids.sessionID == "" {
		return modified
	}
	if existing, ok := reqBody["prompt_cache_key"].(string); !ok || strings.TrimSpace(existing) != ids.sessionID {
		reqBody["prompt_cache_key"] = ids.sessionID
		modified = true
	}
	return modified
}

// applyCodexFingerprintToClientMetadataMap 是 client_metadata 改写的共享核心，
// map 版（非透传，body 已解码）与 raw 字节版（透传热路径）都经由它，保证两条
// 路径的收敛语义永不漂移。
func applyCodexFingerprintToClientMetadataMap(existing map[string]any, ids *codexFingerprintIDs) bool {
	if existing == nil || ids == nil {
		return false
	}

	modified := false

	if ids.installationID != "" {
		existing["x-codex-installation-id"] = ids.installationID
		modified = true
	}

	if ids.mode == codexFingerprintDevice {
		rewriteClientMetadataEmbeddedTurnMetadata(existing, map[string]any{
			"installation_id": ids.installationID,
		})
		return modified
	}

	// session / full 模式
	existing["session_id"] = ids.sessionID
	existing["thread_id"] = ids.threadID
	existing["turn_id"] = ids.turnID
	existing["x-codex-window-id"] = ids.windowID

	rewriteClientMetadataEmbeddedTurnMetadata(existing, map[string]any{
		"installation_id":         ids.installationID,
		"session_id":              ids.sessionID,
		"thread_id":               ids.threadID,
		"turn_id":                 ids.turnID,
		"window_id":               ids.windowID,
		"turn_started_at_unix_ms": time.Now().UnixMilli(),
	})
	return true
}

// applyCodexFingerprintClientMetadataRaw 在原始 JSON 字节上改写 client_metadata，
// 供透传路径使用——透传是热路径，禁止对可能高达数十 MB 的 body 做全量
// Unmarshal（见 forwardOpenAIPassthrough 的轻量提取注释）。实现为：gjson 提取
// client_metadata 小对象单独解码，经共享核心改写后 sjson 一次性拼回，body
// 其余字节原样保留。语义与 applyCodexFingerprintClientMetadata 逐点一致
// （含"非对象值整体替换为收敛集合"的行为）。
func applyCodexFingerprintClientMetadataRaw(body []byte, ids *codexFingerprintIDs) ([]byte, bool, error) {
	if len(body) == 0 || ids == nil {
		return body, false, nil
	}
	// 非 JSON 对象的 body（数组/标量/畸形）没有 client_metadata 语义，
	// sjson 在这类根上写字段会改写整体结构，直接放行保持原样。
	if !gjson.ParseBytes(body).IsObject() {
		return body, false, nil
	}

	existing := map[string]any{}
	if cm := gjson.GetBytes(body, "client_metadata"); cm.IsObject() {
		if err := json.Unmarshal([]byte(cm.Raw), &existing); err != nil {
			return body, false, fmt.Errorf("decode client_metadata for fingerprint: %w", err)
		}
	}

	if !applyCodexFingerprintToClientMetadataMap(existing, ids) {
		return body, false, nil
	}

	raw, err := json.Marshal(existing)
	if err != nil {
		return body, false, fmt.Errorf("encode converged client_metadata: %w", err)
	}
	next, err := sjson.SetRawBytes(body, "client_metadata", raw)
	if err != nil {
		return body, false, fmt.Errorf("splice converged client_metadata: %w", err)
	}
	return next, true, nil
}

// applyCodexFingerprintRequestBodyRaw is the raw-body equivalent of
// applyCodexFingerprintRequestBody. It keeps the hot passthrough path narrow:
// only client_metadata and prompt_cache_key are spliced into the original JSON.
func applyCodexFingerprintRequestBodyRaw(body []byte, ids *codexFingerprintIDs) ([]byte, bool, error) {
	if len(body) == 0 || ids == nil || !gjson.ParseBytes(body).IsObject() {
		return body, false, nil
	}
	next, modified, err := applyCodexFingerprintClientMetadataRaw(body, ids)
	if err != nil {
		return body, false, err
	}
	if ids.mode == codexFingerprintDevice || ids.sessionID == "" {
		return next, modified, nil
	}
	if existing := strings.TrimSpace(gjson.GetBytes(next, "prompt_cache_key").String()); existing != ids.sessionID {
		updated, err := sjson.SetBytes(next, "prompt_cache_key", ids.sessionID)
		if err != nil {
			return body, false, fmt.Errorf("splice converged prompt_cache_key: %w", err)
		}
		return updated, true, nil
	}
	return next, modified, nil
}

// rewriteClientMetadataEmbeddedTurnMetadata 改写 client_metadata 中内嵌的
// x-codex-turn-metadata JSON 字符串里的指定字段。
func rewriteClientMetadataEmbeddedTurnMetadata(clientMetadata map[string]any, fields map[string]any) {
	raw, ok := clientMetadata["x-codex-turn-metadata"].(string)
	if !ok || raw == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return
	}
	for k, v := range fields {
		metadata[k] = v
	}
	if rebuilt, err := json.Marshal(metadata); err == nil {
		clientMetadata["x-codex-turn-metadata"] = string(rebuilt)
	}
}
