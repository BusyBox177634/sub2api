package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// codexFingerprintMode 控制 OAuth 账号出站请求的设备指纹收敛强度。
// 多人共享同一 OAuth 账号时，每个用户的 Codex 客户端会携带各自不同的
// installation_id / session_id / thread_id，上游据此判定设备数和会话数。
// 收敛模式将这些标识改写为账号级恒定值，减少上游可见的设备/会话指纹。
type codexFingerprintMode string

const (
	// codexFingerprintOff 不做任何收敛，原样透传客户端标识（默认行为）。
	codexFingerprintOff codexFingerprintMode = "off"
	// codexFingerprintDevice 仅收敛 installation_id 为账号级恒定值。
	// 上游看到 1 台设备 + 多会话（每用户各自的 session）。
	codexFingerprintDevice codexFingerprintMode = "device"
	// codexFingerprintSession 收敛 installation_id + session_id，
	// thread_id 按客户端原始 session-id 确定性派生（每个真实 Codex 会话一个独立线程）。
	// 上游看到 1 台设备 + 1 会话 + N 线程，最接近正常用户 spawn 子代理的模式。
	codexFingerprintSession codexFingerprintMode = "session"
	// codexFingerprintFull 收敛所有标识：installation_id + session_id + thread_id。
	// 上游看到 1 台设备 + 1 会话 + 1 线程，最激进。
	codexFingerprintFull codexFingerprintMode = "full"
	// codexFingerprintCPA mirrors CLIProxyAPI's identity-confuse behavior. It
	// pseudonymizes client-provided identifiers per account instead of converging
	// every client to a shared account-level identity.
	codexFingerprintCPA codexFingerprintMode = "cpa"
)

const codexFingerprintModeExtraKey = "codex_fingerprint_mode"

const codexCPAIdentityStateContextKey = "codex_cpa_identity_state"

// GetCodexFingerprintMode 从账号 extra JSON 读取指纹收敛模式。
// 未设置时默认 session（设备+会话收敛），显式设为 "off" 才关闭。
func (a *Account) GetCodexFingerprintMode() codexFingerprintMode {
	if a == nil || !a.IsOpenAIOAuth() {
		return codexFingerprintOff
	}
	raw := strings.TrimSpace(a.GetExtraString(codexFingerprintModeExtraKey))
	switch codexFingerprintMode(raw) {
	case codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull, codexFingerprintCPA:
		return codexFingerprintMode(raw)
	default:
		return codexFingerprintSession
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

// resolveConvergedInstallationID 返回账号级恒定的 installation_id。
// 优先使用管理员配置的真实 device_id，无则从 accountID 确定性派生。
func resolveConvergedInstallationID(account *Account) string {
	if account == nil {
		return ""
	}
	if deviceID := account.GetOpenAIDeviceID(); deviceID != "" {
		return deviceID
	}
	return deriveStableUUIDv4(fmt.Sprintf("sub2api:codex-install-id:v1:%d", account.ID))
}

// resolveConvergedSessionID 返回账号级恒定的 session_id。
func resolveConvergedSessionID(account *Account) string {
	if account == nil {
		return ""
	}
	return deriveStableUUIDv4(fmt.Sprintf("sub2api:codex-session-id:v1:%d", account.ID))
}

// resolveConvergedThreadID 按客户端原始 session-id 确定性派生 thread_id。
// 每个真实 Codex 会话（不同客户端启动实例）获得一个独立线程，
// 模拟正常用户 spawn 子代理或开多窗口的模式。
func resolveConvergedThreadID(account *Account, clientSessionID string) string {
	if account == nil || clientSessionID == "" {
		return ""
	}
	return deriveStableUUIDv4(fmt.Sprintf("sub2api:codex-thread-id:v1:%d:%s", account.ID, clientSessionID))
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
// 的 thread_id 派生——每个真实 Codex 会话得到一个独立线程。
// 返回 nil 表示 off 模式，不需要改写。
// 注意：包含随机生成的 turn_id，调用方必须只调用一次并共享结果给头改写和体改写。
func resolveCodexFingerprintIDs(account *Account, clientSessionID string, mode codexFingerprintMode) *codexFingerprintIDs {
	if mode == codexFingerprintOff || mode == codexFingerprintCPA {
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
		ids.sessionID = resolveConvergedSessionID(account)
		ids.threadID = resolveConvergedThreadID(account, clientSessionID)
		if ids.threadID == "" {
			ids.threadID = ids.sessionID
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

// extractClientSessionID 从请求头中提取客户端原始的会话标识。
// 优先取 session-id（连字符形式，Codex CLI 标准），回退到 session_id（下划线形式）。
// 返回的值尚未被 isolateOpenAISessionID 改写，是客户端的真实标识。
func extractClientSessionID(h http.Header) string {
	if v := strings.TrimSpace(h.Get("session-id")); v != "" {
		return v
	}
	return strings.TrimSpace(h.Get("session_id"))
}

// resolveCodexFingerprintIDsFromRequest 从客户端原始请求头中提取 session-id，
// 结合账号配置一次性解析收敛 ID 集合。调用方应将返回的 ids 同时传给
// applyCodexFingerprintHeaders 和 applyCodexFingerprintClientMetadata。
func resolveCodexFingerprintIDsFromRequest(account *Account, clientHeaders http.Header) *codexFingerprintIDs {
	if account == nil {
		return nil
	}
	mode := account.GetCodexFingerprintMode()
	if mode == codexFingerprintOff {
		return nil
	}
	clientSessionID := ""
	if clientHeaders != nil {
		clientSessionID = extractClientSessionID(clientHeaders)
	}
	return resolveCodexFingerprintIDs(account, clientSessionID, mode)
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

	modified := false

	if ids.installationID != "" {
		existing["x-codex-installation-id"] = ids.installationID
		modified = true
	}

	if ids.mode == codexFingerprintDevice {
		rewriteClientMetadataEmbeddedTurnMetadata(existing, map[string]any{
			"installation_id": ids.installationID,
		})
		if modified {
			reqBody["client_metadata"] = existing
		}
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

	reqBody["client_metadata"] = existing
	return true
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

// codexCPAIdentityState is the request-scoped equivalent of CLIProxyAPI's
// codexIdentityConfuseState. The state is shared by body/header rewriting and
// response restoration so a client never needs to observe pseudonymous values.
type codexCPAIdentityState struct {
	enabled                bool
	authID                 string
	originalPromptCacheKey string
	promptCacheKey         string
	originalInstallationID string
	turnIDs                []codexCPAIdentityReplacement
}

type codexCPAIdentityReplacement struct {
	original string
	confused string
}

func prepareCodexCPAIdentityState(account *Account, originalBody []byte, clientHeaders http.Header) *codexCPAIdentityState {
	if account == nil || account.GetCodexFingerprintMode() != codexFingerprintCPA {
		return nil
	}

	state := &codexCPAIdentityState{
		enabled: true,
		authID:  strconv.FormatInt(account.ID, 10),
	}
	if len(originalBody) == 0 {
		return state
	}

	var original map[string]any
	if err := json.Unmarshal(originalBody, &original); err != nil {
		return state
	}
	state.originalPromptCacheKey = codexCPAStringField(original, "prompt_cache_key")
	if state.originalPromptCacheKey != "" {
		state.promptCacheKey = codexCPAConfuseUUID(state.authID, "prompt-cache", state.originalPromptCacheKey)
	}
	if clientMetadata, _ := original["client_metadata"].(map[string]any); clientMetadata != nil {
		state.originalInstallationID = codexCPAStringField(clientMetadata, "x-codex-installation-id")
	}
	return state
}

// applyCodexCPAIdentityPayloadFromOriginal derives a fresh request state from
// the client payload, then applies it to a possibly transformed upstream body.
// WebSocket ingress uses this for each turn so mappings cannot bleed across
// turns on a long-lived connection.
func applyCodexCPAIdentityPayloadFromOriginal(account *Account, originalBody []byte, upstreamBody []byte, clientHeaders http.Header) ([]byte, *codexCPAIdentityState, error) {
	state := prepareCodexCPAIdentityState(account, originalBody, clientHeaders)
	if state == nil {
		return upstreamBody, nil, nil
	}
	var requestBody map[string]any
	if err := json.Unmarshal(upstreamBody, &requestBody); err != nil {
		return upstreamBody, state, nil
	}
	if !applyCodexCPAIdentityBody(requestBody, state) {
		return upstreamBody, state, nil
	}
	updatedBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal CPA Codex identity body: %w", err)
	}
	return updatedBody, state, nil
}

// prepareCodexCPAUpstreamPayload is the common final body preparation used by
// all OpenAI OAuth Codex entry points. Native Responses normally install a
// state earlier from the untouched client body; compatibility entry points can
// safely fall back to their final Responses body here.
func prepareCodexCPAUpstreamPayload(c *gin.Context, account *Account, body []byte, promptCacheKey string) ([]byte, string, *codexCPAIdentityState, error) {
	if account == nil || account.GetCodexFingerprintMode() != codexFingerprintCPA {
		return body, promptCacheKey, nil, nil
	}

	state := codexCPAIdentityStateFromContext(c)
	if state == nil {
		var clientHeaders http.Header
		if c != nil && c.Request != nil {
			clientHeaders = c.Request.Header
		}
		state = prepareCodexCPAIdentityState(account, body, clientHeaders)
		setCodexCPAIdentityState(c, state)
	}
	if state == nil {
		return body, promptCacheKey, nil, nil
	}

	var requestBody map[string]any
	if err := json.Unmarshal(body, &requestBody); err != nil {
		return body, codexCPAPromptCacheKeyForUpstream(promptCacheKey, state), state, nil
	}
	if !applyCodexCPAIdentityBody(requestBody, state) {
		return body, codexCPAPromptCacheKeyForUpstream(promptCacheKey, state), state, nil
	}
	updatedBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, "", nil, fmt.Errorf("marshal CPA Codex identity body: %w", err)
	}
	return updatedBody, codexCPAPromptCacheKeyForUpstream(promptCacheKey, state), state, nil
}

func codexCPAIdentityStateFromContext(c *gin.Context) *codexCPAIdentityState {
	if c == nil {
		return nil
	}
	value, ok := c.Get(codexCPAIdentityStateContextKey)
	if !ok {
		return nil
	}
	state, _ := value.(*codexCPAIdentityState)
	return state
}

func setCodexCPAIdentityState(c *gin.Context, state *codexCPAIdentityState) {
	if c == nil {
		return
	}
	if state == nil {
		c.Set(codexCPAIdentityStateContextKey, (*codexCPAIdentityState)(nil))
		return
	}
	c.Set(codexCPAIdentityStateContextKey, state)
}

func codexCPAStringField(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func codexCPAConfuseUUID(authID string, kind string, value string) string {
	name := strings.Join([]string{
		"cli-proxy-api",
		"codex",
		"identity-confuse",
		strings.TrimSpace(kind),
		strings.TrimSpace(authID),
		strings.TrimSpace(value),
	}, ":")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}

func (state *codexCPAIdentityState) confuseTurnID(turnID string) string {
	turnID = strings.TrimSpace(turnID)
	if state == nil || !state.enabled || strings.TrimSpace(state.authID) == "" || turnID == "" {
		return turnID
	}
	for _, replacement := range state.turnIDs {
		if replacement.original == turnID || replacement.confused == turnID {
			return replacement.confused
		}
	}
	confusedTurnID := codexCPAConfuseUUID(state.authID, "turn", turnID)
	state.turnIDs = append(state.turnIDs, codexCPAIdentityReplacement{original: turnID, confused: confusedTurnID})
	return confusedTurnID
}

func codexCPAPromptCacheKeyForUpstream(promptCacheKey string, state *codexCPAIdentityState) string {
	if state != nil && state.promptCacheKey != "" {
		return state.promptCacheKey
	}
	return promptCacheKey
}

// applyCodexCPAIdentityBody mirrors CLIProxyAPI's body rewrite: prompt cache
// and installation values are taken from the original client payload, while
// turn metadata is rewritten from the final upstream body.
func applyCodexCPAIdentityBody(reqBody map[string]any, state *codexCPAIdentityState) bool {
	if reqBody == nil || state == nil || !state.enabled {
		return false
	}

	modified := false
	if promptCacheKey := state.promptCacheKey; promptCacheKey != "" {
		if codexCPAStringField(reqBody, "prompt_cache_key") != promptCacheKey {
			reqBody["prompt_cache_key"] = promptCacheKey
			modified = true
		}
	}

	clientMetadata, _ := reqBody["client_metadata"].(map[string]any)
	if state.originalInstallationID != "" {
		if clientMetadata == nil {
			clientMetadata = make(map[string]any)
			reqBody["client_metadata"] = clientMetadata
		}
		installationID := codexCPAConfuseUUID(state.authID, "installation", state.originalInstallationID)
		if codexCPAStringField(clientMetadata, "x-codex-installation-id") != installationID {
			clientMetadata["x-codex-installation-id"] = installationID
			modified = true
		}
	}
	if clientMetadata == nil {
		return modified
	}

	if rawTurnMetadata := codexCPAStringField(clientMetadata, "x-codex-turn-metadata"); rawTurnMetadata != "" {
		if updated := applyCodexCPATurnMetadataIdentity(rawTurnMetadata, state); updated != rawTurnMetadata {
			clientMetadata["x-codex-turn-metadata"] = updated
			modified = true
		}
	}
	if state.promptCacheKey != "" && codexCPAStringField(clientMetadata, "x-codex-window-id") != "" {
		windowID := state.promptCacheKey + ":0"
		if codexCPAStringField(clientMetadata, "x-codex-window-id") != windowID {
			clientMetadata["x-codex-window-id"] = windowID
			modified = true
		}
	}
	return modified
}

func applyCodexCPAIdentityHeaders(headers http.Header, state *codexCPAIdentityState) {
	if headers == nil || state == nil || !state.enabled {
		return
	}
	if rawTurnMetadata := codexCPAHeaderValue(headers, "x-codex-turn-metadata"); rawTurnMetadata != "" {
		if updated := applyCodexCPATurnMetadataIdentity(rawTurnMetadata, state); updated != rawTurnMetadata {
			headers.Set("X-Codex-Turn-Metadata", updated)
		}
	}
	promptCacheKey := state.promptCacheKey
	if promptCacheKey == "" {
		return
	}

	codexCPASetSessionHeaderCasePreserved(headers, "Session-Id", promptCacheKey)
	if codexCPAHeaderValue(headers, "conversation_id") != "" {
		codexCPASetHeaderCasePreserved(headers, "Conversation_id", promptCacheKey)
	}
	headers.Set("X-Client-Request-Id", promptCacheKey)
	headers.Set("Thread-Id", promptCacheKey)
	headers.Set("X-Codex-Window-Id", promptCacheKey+":0")
}

func applyCodexCPATurnMetadataIdentity(rawTurnMetadata string, state *codexCPAIdentityState) string {
	if state == nil || !state.enabled || strings.TrimSpace(rawTurnMetadata) == "" {
		return rawTurnMetadata
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(rawTurnMetadata), &metadata); err != nil {
		if state.promptCacheKey != "" && state.originalPromptCacheKey != "" {
			return strings.ReplaceAll(rawTurnMetadata, state.originalPromptCacheKey, state.promptCacheKey)
		}
		return rawTurnMetadata
	}

	modified := false
	if state.promptCacheKey != "" {
		if _, exists := metadata["prompt_cache_key"]; exists {
			metadata["prompt_cache_key"] = state.promptCacheKey
			modified = true
		}
	}
	if turnID := codexCPAStringField(metadata, "turn_id"); turnID != "" {
		confusedTurnID := state.confuseTurnID(turnID)
		if confusedTurnID != turnID {
			metadata["turn_id"] = confusedTurnID
			modified = true
		}
	}
	if state.promptCacheKey != "" {
		if _, exists := metadata["window_id"]; exists {
			metadata["window_id"] = state.promptCacheKey + ":0"
			modified = true
		}
	}
	if !modified {
		if state.promptCacheKey != "" && state.originalPromptCacheKey != "" {
			return strings.ReplaceAll(rawTurnMetadata, state.originalPromptCacheKey, state.promptCacheKey)
		}
		return rawTurnMetadata
	}
	updated, err := json.Marshal(metadata)
	if err != nil {
		return rawTurnMetadata
	}
	return string(updated)
}

func codexCPAHeaderValue(headers http.Header, key string) string {
	key = strings.TrimSpace(key)
	if headers == nil || key == "" {
		return ""
	}
	if value := strings.TrimSpace(headers.Get(key)); value != "" {
		return value
	}
	for existingKey, values := range headers {
		if !strings.EqualFold(existingKey, key) {
			continue
		}
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func codexCPASetHeaderCasePreserved(headers http.Header, key string, value string) {
	if headers == nil {
		return
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	for existingKey := range headers {
		if strings.EqualFold(existingKey, key) {
			delete(headers, existingKey)
		}
	}
	headers[key] = []string{value}
}

func codexCPASetSessionHeaderCasePreserved(headers http.Header, fallbackKey string, value string) {
	if headers == nil {
		return
	}
	fallbackKey = strings.TrimSpace(fallbackKey)
	value = strings.TrimSpace(value)
	if fallbackKey == "" || value == "" {
		return
	}

	selectedKey := ""
	if _, ok := headers[fallbackKey]; ok && strings.EqualFold(fallbackKey, "session_id") {
		selectedKey = fallbackKey
	} else {
		for existingKey := range headers {
			if strings.EqualFold(existingKey, "session_id") {
				selectedKey = existingKey
				break
			}
		}
	}
	if selectedKey == "" {
		selectedKey = fallbackKey
	}
	for existingKey := range headers {
		normalized := strings.ToLower(strings.TrimSpace(existingKey))
		if (normalized == "session_id" || normalized == "session-id") && existingKey != selectedKey {
			delete(headers, existingKey)
		}
	}
	headers[selectedKey] = []string{value}
}

func applyCodexCPAIdentityConfuseResponsePayload(payload []byte, state *codexCPAIdentityState) []byte {
	if state == nil || !state.enabled {
		return payload
	}
	payload = replaceCodexCPAIdentityResponsePayload(payload, state.originalPromptCacheKey, state.promptCacheKey)
	for _, turnID := range state.turnIDs {
		payload = replaceCodexCPAIdentityResponsePayload(payload, turnID.original, turnID.confused)
	}
	return payload
}

func applyCodexCPAIdentityExposeResponsePayload(payload []byte, state *codexCPAIdentityState) []byte {
	if state == nil || !state.enabled {
		return payload
	}
	payload = replaceCodexCPAIdentityResponsePayload(payload, state.promptCacheKey, state.originalPromptCacheKey)
	for _, turnID := range state.turnIDs {
		payload = replaceCodexCPAIdentityResponsePayload(payload, turnID.confused, turnID.original)
	}
	return payload
}

func normalizeCodexCPAIdentityResponsePayload(payload []byte, state *codexCPAIdentityState) []byte {
	return applyCodexCPAIdentityConfuseResponsePayload(payload, state)
}

func exposeCodexCPAIdentityResponsePayload(payload []byte, state *codexCPAIdentityState) []byte {
	return applyCodexCPAIdentityExposeResponsePayload(payload, state)
}

// exposeCodexCPAIdentityJSONValue restores client-facing identifiers on a
// structured compatibility response after its upstream-facing representation
// has been parsed and transformed.
func exposeCodexCPAIdentityJSONValue(value any, state *codexCPAIdentityState) {
	if value == nil || state == nil || !state.enabled {
		return
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	payload = exposeCodexCPAIdentityResponsePayload(payload, state)
	if err := json.Unmarshal(payload, value); err != nil {
		return
	}
}

func replaceCodexCPAIdentityResponsePayload(payload []byte, from string, to string) []byte {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if len(payload) == 0 || from == "" || to == "" || from == to || !bytes.Contains(payload, []byte(from)) {
		return payload
	}
	return bytes.ReplaceAll(payload, []byte(from), []byte(to))
}
