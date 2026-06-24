package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestUsageBriefProductionBatchTitleUsesWorkSummaryWording(t *testing.T) {
	start := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		periodType string
		want       string
	}{
		{name: "daily", periodType: UsageBriefPeriodDaily, want: "2026-06-22 工作日报批次"},
		{name: "weekly", periodType: UsageBriefPeriodWeekly, want: "2026-06-22 至 2026-06-28 工作周报批次"},
		{name: "monthly", periodType: UsageBriefPeriodMonthly, want: "2026-06 工作月报批次"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := productionBatchTitle(tc.periodType, start, end); got != tc.want {
				t.Fatalf("productionBatchTitle() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUsageBriefWorkReportTitleUsesWorkSummaryWording(t *testing.T) {
	start := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		periodType string
		want       string
	}{
		{name: "daily", periodType: UsageBriefPeriodDaily, want: "2026-06-22 工作日报"},
		{name: "weekly", periodType: UsageBriefPeriodWeekly, want: "2026-06-22 至 2026-06-28 工作周报"},
		{name: "monthly", periodType: UsageBriefPeriodMonthly, want: "2026-06 工作月报"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := workReportTitle(tc.periodType, start, end); got != tc.want {
				t.Fatalf("workReportTitle() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUsageBriefWorkSummaryFinalInstructionRequiresFixedStructureAndUsageSentence(t *testing.T) {
	instruction := workSummaryFinalInstruction("工作日报", "请生成普通用户工作日报。")
	required := []string{
		"## 工作概览",
		"## 完成内容",
		"## 模块与设计进展",
		"## 贡献度判断",
		"## 风险与建议",
		"## 用量概括",
		"输入为空",
		"不要扩写空洞内容",
		"只能用一句话概括 token、成本、模型或请求量",
		"普通用户的工作成果与贡献",
	}
	for _, want := range required {
		if !strings.Contains(instruction, want) {
			t.Fatalf("instruction missing %q:\n%s", want, instruction)
		}
	}
}

func TestUsageBriefNoWorkReportContentIsShort(t *testing.T) {
	content := noWorkReportContent(UsageBriefPeriodDaily)
	required := []string{
		"## 工作概览",
		"当天未采集到任何使用记录",
		"## 用量概括",
		"请求数为 0",
	}
	for _, want := range required {
		if !strings.Contains(content, want) {
			t.Fatalf("no work report missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "## 完成内容") || strings.Contains(content, "## 风险与建议") {
		t.Fatalf("no work report should stay concise:\n%s", content)
	}
	if len([]rune(content)) > 120 {
		t.Fatalf("no work report too long: %d runes\n%s", len([]rune(content)), content)
	}
}

func TestUsageBriefDependencyWaitErrorDoesNotConsumeAIRetry(t *testing.T) {
	err := newUsageBriefDependencyWaitError("waiting for daily report")
	if !isUsageBriefDependencyWaitError(err) {
		t.Fatalf("expected dependency wait error")
	}
	if isRetryableUsageBriefError(err) {
		t.Fatalf("dependency wait should not be treated as AI retryable error")
	}
}

func TestUsageBriefRetryIsUnlimitedAndUsesShortDelay(t *testing.T) {
	svc := &UsageBriefService{}
	err := newUsageBriefAIError("temporary network error", true)
	job := UsageBriefJob{RetryCount: 999}
	if !svc.shouldRetryUsageBriefJob(err, job) {
		t.Fatalf("retryable AI error should keep retrying regardless of retry count")
	}
	before := time.Now()
	next := svc.nextRetryAt(job.RetryCount)
	if next.Before(before.Add(14*time.Second)) || next.After(before.Add(16*time.Second)) {
		t.Fatalf("next retry = %s, want about 15s after %s", next, before)
	}
}

func TestUsageBriefSettingsConcurrencyCapsAtThirtyTwo(t *testing.T) {
	ctx := context.Background()
	repo := &usageBriefSettingRepoStub{values: map[string]string{
		SettingKeyUsageBriefEnabled:       "true",
		SettingKeyUsageBriefConcurrency:   "64",
		SettingKeyUsageBriefOpenAIBaseURL: "https://api.openai.com",
	}}
	svc := NewUsageBriefService(nil, repo, nil)
	settings, err := svc.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings.Concurrency != UsageBriefMaxConcurrency {
		t.Fatalf("Concurrency = %d, want %d", settings.Concurrency, UsageBriefMaxConcurrency)
	}

	requested := 99
	if _, err := svc.UpdateSettings(ctx, UpdateUsageBriefSettingsRequest{Concurrency: &requested}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if repo.values[SettingKeyUsageBriefConcurrency] != "32" {
		t.Fatalf("saved concurrency = %q, want 32", repo.values[SettingKeyUsageBriefConcurrency])
	}
}

type usageBriefSettingRepoStub struct {
	values map[string]string
}

func (r *usageBriefSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *usageBriefSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (r *usageBriefSettingRepoStub) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *usageBriefSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r.values[key]
	}
	return out, nil
}

func (r *usageBriefSettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *usageBriefSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}

func (r *usageBriefSettingRepoStub) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestUsageBriefPromptWithPayloadKeepsUsageOutOfMainBody(t *testing.T) {
	prompt := promptWithPayload(workSummaryFinalInstruction("工作日报", "请生成普通用户工作日报。"), map[string]any{"user_id": 7})
	required := []string{
		"报告主体必须是普通用户工作总结",
		"不要把模型、token、成本、请求次数作为主体",
		"token、成本、模型等用量信息只允许在“用量概括”中用一句话总结",
	}
	for _, want := range required {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestUsageBriefPromptCompactionOnlyDropsStrictlyCoveredCodexRequests(t *testing.T) {
	records := []UsageBriefSourceRecord{
		{ID: 1, RequestPayload: codexUsageBriefPayload("thread-1", "thread-1:1", "turn-1", []string{"user:设计订单模块"})},
		{ID: 2, RequestPayload: codexUsageBriefPayload("thread-1", "thread-1:1", "turn-2", []string{"user:设计订单模块", "assistant:好的", "user:补充支付状态"})},
	}

	got := compactUsageBriefRecordsForPrompt(records)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %#v", len(got), got)
	}
	if got[0].ID != 2 {
		t.Fatalf("kept id = %d, want 2", got[0].ID)
	}
	if got[0].CoveredRequestCount != 2 || !got[0].PromptCompacted {
		t.Fatalf("expected coverage metadata, got %#v", got[0])
	}
	if len(got[0].CoveredUsageLogIDs) != 2 || got[0].CoveredUsageLogIDs[0] != 1 || got[0].CoveredUsageLogIDs[1] != 2 {
		t.Fatalf("covered ids = %#v, want [1 2]", got[0].CoveredUsageLogIDs)
	}
}

func TestUsageBriefPromptCompactionKeepsContextBeforeCodexCompaction(t *testing.T) {
	records := []UsageBriefSourceRecord{
		{ID: 1, RequestPayload: codexUsageBriefPayload("thread-1", "thread-1:1", "turn-1", []string{"user:设计订单模块", "assistant:好的", "user:补充支付状态"})},
		{ID: 2, InboundEndpoint: "/v1/responses/compact", RequestPayload: codexUsageBriefPayload("thread-1", "thread-1:1", "turn-compact", []string{"user:compact"})},
		{ID: 3, RequestPayload: codexUsageBriefPayload("thread-1", "thread-1:1", "turn-3", []string{"system:前文摘要", "user:继续实现退款"})},
	}

	got := compactUsageBriefRecordsForPrompt(records)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %#v", len(got), got)
	}
	for i, wantID := range []int64{1, 2, 3} {
		if got[i].ID != wantID {
			t.Fatalf("got[%d].ID = %d, want %d", i, got[i].ID, wantID)
		}
	}
}

func TestUsageBriefPromptCompactionDoesNotUseThreadIDAlone(t *testing.T) {
	records := []UsageBriefSourceRecord{
		{ID: 1, RequestPayload: codexUsageBriefPayload("thread-1", "thread-1:1", "turn-1", []string{"user:订单模块"})},
		{ID: 2, RequestPayload: codexUsageBriefPayload("thread-1", "thread-1:1", "turn-2", []string{"user:库存模块"})},
	}

	got := compactUsageBriefRecordsForPrompt(records)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
}

func TestUsageBriefPromptCompactionSeparatesConcurrentWindows(t *testing.T) {
	records := []UsageBriefSourceRecord{
		{ID: 1, RequestPayload: codexUsageBriefPayload("thread-1", "thread-1:1", "turn-1", []string{"user:订单模块"})},
		{ID: 2, RequestPayload: codexUsageBriefPayload("thread-1", "thread-1:2", "turn-2", []string{"user:订单模块", "assistant:好的", "user:支付模块"})},
	}

	got := compactUsageBriefRecordsForPrompt(records)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
}

func TestUsageBriefPromptCompactionSlimsCodexMetadata(t *testing.T) {
	payload := codexUsageBriefPayload("thread-1", "thread-1:1", "turn-1", []string{"user:订单模块"})
	got := compactUsageBriefRecordsForPrompt([]UsageBriefSourceRecord{{ID: 1, RequestPayload: payload}})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if strings.Contains(got[0].RequestPayload, "x-codex-turn-metadata") {
		t.Fatalf("slimmed payload still contains raw x-codex-turn-metadata: %s", got[0].RequestPayload)
	}
	if strings.Contains(got[0].RequestPayload, "reasoning.encrypted_content") {
		t.Fatalf("slimmed payload still contains encrypted reasoning include: %s", got[0].RequestPayload)
	}
	if !strings.Contains(got[0].RequestPayload, "codex_turn_metadata") {
		t.Fatalf("slimmed payload missing parsed codex_turn_metadata: %s", got[0].RequestPayload)
	}
}

func TestUsageBriefCompressedPromptCompactionUsesFinalMessagesAndKeepsBoundaries(t *testing.T) {
	base := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	records := []UsageBriefSourceRecord{
		{
			ID:                        3,
			CreatedAt:                 base.Add(3 * time.Minute),
			CompressedRequestPayload:  `{"messages":[{"role":"user","content":"你好"},{"role":"assistant","content":"A"},{"role":"assistant","content":"B"},{"role":"user","content":"继续"},{"role":"assistant","content":"C"},{"role":"assistant","content":"D"}]}`,
			CompressedResponsePayload: `{"messages":[]}`,
		},
		{
			ID:                        2,
			CreatedAt:                 base.Add(2 * time.Minute),
			CompressedRequestPayload:  `{"messages":[{"role":"user","content":"你好"},{"role":"assistant","content":"A"},{"role":"assistant","content":"B"},{"role":"user","content":"继续"}]}`,
			CompressedResponsePayload: `{"messages":[{"role":"assistant","content":"CD"}]}`,
		},
		{
			ID:                        1,
			CreatedAt:                 base.Add(time.Minute),
			CompressedRequestPayload:  `{"messages":[{"role":"user","content":"你好"}]}`,
			CompressedResponsePayload: `{"messages":[{"role":"assistant","content":"A"}]}`,
		},
	}
	svc := NewUsageBriefService(&usageBriefResumeRepo{}, nil, nil)
	got, err := svc.compressedUsageBriefRecordsForPrompt(context.Background(), records)
	if err != nil {
		t.Fatalf("compressedUsageBriefRecordsForPrompt: %v", err)
	}
	if len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("kept records = %#v, want only id 3", got)
	}
	if got[0].CoveredRequestCount != 3 || !got[0].PromptCompacted {
		t.Fatalf("coverage metadata = %#v, want compacted 3 records", got[0])
	}
	if strings.TrimSpace(got[0].RequestPayload) != "" || strings.TrimSpace(got[0].CompressedRequestPayload) != "" {
		t.Fatalf("full/compressed payload fields should not be used in prompt record: %#v", got[0])
	}
	if len(got[0].Messages) != 6 {
		t.Fatalf("messages len = %d, want original 6 boundaries: %#v", len(got[0].Messages), got[0].Messages)
	}
	if got[0].Messages[1].Content != "A" || got[0].Messages[2].Content != "B" || got[0].Messages[4].Content != "C" || got[0].Messages[5].Content != "D" {
		t.Fatalf("assistant boundaries were merged unexpectedly: %#v", got[0].Messages)
	}
}

func TestUsageBriefCompressedPromptCompactionMatchesTempDedupeAlgorithm(t *testing.T) {
	base := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	records := []UsageBriefSourceRecord{
		{
			ID:                        5,
			CreatedAt:                 base.Add(5 * time.Minute),
			CompressedRequestPayload:  `{"messages":[{"role":"user","content":"设计账号模块"},{"role":"assistant","content":"已完成登录"},{"role":"user","content":"继续做权限"}]}`,
			CompressedResponsePayload: `{"messages":[{"role":"assistant","content":"已完成角色权限和菜单控制"}]}`,
		},
		{
			ID:                        4,
			CreatedAt:                 base.Add(4 * time.Minute),
			CompressedRequestPayload:  `{"messages":[{"role":"user","content":"设计账号模块"},{"role":"assistant","content":"已完成登录"},{"role":"user","content":"继续做权限"}]}`,
			CompressedResponsePayload: `{"messages":[{"role":"assistant","content":"已完成角色权限"}]}`,
		},
		{
			ID:                        3,
			CreatedAt:                 base.Add(3 * time.Minute),
			CompressedRequestPayload:  `{"messages":[{"role":"user","content":"你好"},{"role":"assistant","content":"A"},{"role":"assistant","content":"B"},{"role":"user","content":"继续"},{"role":"assistant","content":"C"},{"role":"assistant","content":"D"}]}`,
			CompressedResponsePayload: `{"messages":[]}`,
		},
		{
			ID:                        2,
			CreatedAt:                 base.Add(2 * time.Minute),
			CompressedRequestPayload:  `{"messages":[{"role":"user","content":"你好"},{"role":"assistant","content":"A"},{"role":"assistant","content":"B"},{"role":"user","content":"继续"}]}`,
			CompressedResponsePayload: `{"messages":[{"role":"assistant","content":"CD"}]}`,
		},
		{
			ID:                        1,
			CreatedAt:                 base.Add(time.Minute),
			CompressedRequestPayload:  `{"messages":[{"role":"user","content":"你好"}]}`,
			CompressedResponsePayload: `{"messages":[{"role":"assistant","content":"A"}]}`,
		},
	}
	svc := NewUsageBriefService(&usageBriefResumeRepo{}, nil, nil)
	got, err := svc.compressedUsageBriefRecordsForPrompt(context.Background(), records)
	if err != nil {
		t.Fatalf("compressedUsageBriefRecordsForPrompt: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("kept records len = %d, want 2: %#v", len(got), got)
	}
	if got[0].ID != 5 || got[1].ID != 3 {
		t.Fatalf("kept record ids = [%d,%d], want [5,3]", got[0].ID, got[1].ID)
	}
	if len(got[1].Messages) != 6 {
		t.Fatalf("record 3 messages len = %d, want original 6 boundaries: %#v", len(got[1].Messages), got[1].Messages)
	}
	if got[1].Messages[1].Content != "A" || got[1].Messages[2].Content != "B" || got[1].Messages[4].Content != "C" || got[1].Messages[5].Content != "D" {
		t.Fatalf("record 3 assistant boundaries were merged unexpectedly: %#v", got[1].Messages)
	}
}

func TestUsageBriefCompressedPromptCompactionGeneratesMissingCompressedPayloads(t *testing.T) {
	repo := &usageBriefResumeRepo{}
	svc := NewUsageBriefService(repo, nil, nil)
	records := []UsageBriefSourceRecord{
		{
			ID:              8,
			CreatedAt:       time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
			RequestPayload:  `{"input":[{"role":"user","content":[{"type":"input_text","text":"设计订单模块"}]}]}`,
			ResponsePayload: `{"output":[{"role":"assistant","content":[{"type":"output_text","text":"订单模块已完成"}]}]}`,
		},
	}
	got, err := svc.compressedUsageBriefRecordsForPrompt(context.Background(), records)
	if err != nil {
		t.Fatalf("compressedUsageBriefRecordsForPrompt: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	updated, ok := repo.compressedUpdates[8]
	if !ok {
		t.Fatalf("missing compressed payload update")
	}
	if !strings.Contains(updated[0], "设计订单模块") || !strings.Contains(updated[1], "订单模块已完成") {
		t.Fatalf("unexpected compressed updates: %#v", updated)
	}
	if strings.Contains(got[0].MessagesJSON, "input_text") || strings.Contains(got[0].MessagesJSON, "output_text") {
		t.Fatalf("messages json should contain compressed messages, not full protocol json: %s", got[0].MessagesJSON)
	}
}

func codexUsageBriefPayload(threadID, windowID, turnID string, messages []string) string {
	input := make([]map[string]any, 0, len(messages))
	for _, item := range messages {
		role, text, ok := strings.Cut(item, ":")
		if !ok {
			role = "user"
			text = item
		}
		input = append(input, map[string]any{
			"type": "message",
			"role": role,
			"content": []map[string]string{
				{"type": "input_text", "text": text},
			},
		})
	}
	turnMeta, _ := json.Marshal(map[string]any{
		"installation_id": "install-1",
		"session_id":      threadID,
		"thread_id":       threadID,
		"turn_id":         turnID,
		"window_id":       windowID,
		"request_kind":    "turn",
		"workspace_kind":  "project",
		"workspaces": map[string]any{
			"/repo": map[string]any{
				"latest_git_commit_hash": "abc123",
				"has_changes":            true,
			},
		},
	})
	payload, _ := json.Marshal(map[string]any{
		"model": "gpt-5.5-codex",
		"client_metadata": map[string]any{
			"session_id":              "[REDACTED]",
			"thread_id":               threadID,
			"turn_id":                 turnID,
			"x-codex-window-id":       windowID,
			"x-codex-turn-metadata":   string(turnMeta),
			"x-codex-installation-id": "install-1",
		},
		"include": []string{"reasoning.encrypted_content"},
		"input":   input,
	})
	return string(payload)
}
