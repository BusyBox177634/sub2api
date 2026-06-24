package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type usageBriefResumeRepo struct {
	UsageBriefRepository
	chunks              map[string]UsageBriefJobChunk
	conversations       map[int]UsageBriefJobConversation
	pages               [][]UsageBriefSourceRecord
	pageLimits          []int
	compressedUpdates   map[int64][2]string
	deletedJobChunks    int
	deletedStaleSources int
	progressUpdates     int
}

func usageBriefResumeChunkKey(jobID int64, chunkIndex int, chunkType string) string {
	return fmt.Sprintf("%d:%s:%d", jobID, chunkType, chunkIndex)
}

func (r *usageBriefResumeRepo) GetJobChunk(_ context.Context, jobID int64, chunkIndex int, chunkType string) (*UsageBriefJobChunk, error) {
	chunk, ok := r.chunks[usageBriefResumeChunkKey(jobID, chunkIndex, chunkType)]
	if !ok {
		return nil, nil
	}
	return &chunk, nil
}

func (r *usageBriefResumeRepo) UpsertJobChunk(_ context.Context, chunk UsageBriefJobChunk) (*UsageBriefJobChunk, error) {
	r.chunks[usageBriefResumeChunkKey(chunk.JobID, chunk.ChunkIndex, chunk.ChunkType)] = chunk
	return &chunk, nil
}

func (r *usageBriefResumeRepo) UpsertJobConversation(_ context.Context, conversation UsageBriefJobConversation) (*UsageBriefJobConversation, error) {
	if r.conversations == nil {
		r.conversations = map[int]UsageBriefJobConversation{}
	}
	r.conversations[conversation.ConversationIndex] = conversation
	return &conversation, nil
}

func (r *usageBriefResumeRepo) DeleteStaleJobConversations(_ context.Context, _ int64, maxConversationIndex int) error {
	for index := range r.conversations {
		if index > maxConversationIndex {
			delete(r.conversations, index)
		}
	}
	return nil
}

func (r *usageBriefResumeRepo) ListJobChunkSummaries(_ context.Context, jobID int64, filter UsageBriefJobChunkFilter) ([]UsageBriefJobChunk, int64, error) {
	chunkType := filter.ChunkType
	if chunkType == "" {
		chunkType = UsageBriefChunkTypeSource
	}
	chunks := make([]UsageBriefJobChunk, 0)
	for i := 1; ; i++ {
		chunk, ok := r.chunks[usageBriefResumeChunkKey(jobID, i, chunkType)]
		if !ok {
			break
		}
		chunk.ContentJSON = "{}"
		chunks = append(chunks, chunk)
	}
	return chunks, int64(len(chunks)), nil
}

func (r *usageBriefResumeRepo) FetchUsageRecordsPage(_ context.Context, filter UsageBriefUsageRecordFilter) ([]UsageBriefSourceRecord, error) {
	r.pageLimits = append(r.pageLimits, filter.Limit)
	if len(r.pages) == 0 {
		return nil, nil
	}
	page := r.pages[0]
	r.pages = r.pages[1:]
	return page, nil
}

func (r *usageBriefResumeRepo) DeleteJobChunks(_ context.Context, _ int64) error {
	r.deletedJobChunks++
	return nil
}

func (r *usageBriefResumeRepo) DeleteStaleJobChunks(_ context.Context, _ int64, chunkType string, _ int) error {
	if chunkType == UsageBriefChunkTypeSource {
		r.deletedStaleSources++
	}
	return nil
}

func (r *usageBriefResumeRepo) IsJobCancelRequested(_ context.Context, _ int64) (bool, error) {
	return false, nil
}

func (r *usageBriefResumeRepo) GetJob(_ context.Context, id int64) (*UsageBriefJob, error) {
	return &UsageBriefJob{ID: id}, nil
}

func (r *usageBriefResumeRepo) UpdateJobProgress(_ context.Context, _ int64, _, _ int, _ string, _, _, _, _, _, _ int) error {
	r.progressUpdates++
	return nil
}

func (r *usageBriefResumeRepo) UpdateUsageRecordCompressedPayloads(_ context.Context, usageLogID int64, compressedRequestJSON *string, compressedResponseJSON *string) error {
	if r.compressedUpdates == nil {
		r.compressedUpdates = map[int64][2]string{}
	}
	r.compressedUpdates[usageLogID] = [2]string{derefString(compressedRequestJSON), derefString(compressedResponseJSON)}
	return nil
}

func (r *usageBriefResumeRepo) MarkJobPaused(_ context.Context, id int64, stage string) error {
	return nil
}

func TestUsageBriefRecordChunksPreparedBeforeAICalls(t *testing.T) {
	job := UsageBriefJob{ID: 77}
	repo := &usageBriefResumeRepo{
		chunks: map[string]UsageBriefJobChunk{},
		pages: [][]UsageBriefSourceRecord{
			usageBriefTestRecords(1, 50),
			usageBriefTestRecords(51, 1),
		},
	}

	svc := NewUsageBriefService(repo, nil, nil)
	chunkTotal, totalEstimated, err := svc.prepareSourceRecordChunks(
		context.Background(),
		job,
		"日报",
		map[string]any{"user_id": int64(7)},
		UsageBriefSourceRange{UserID: 7, Start: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)},
		4000,
	)
	if err != nil {
		t.Fatalf("prepareSourceRecordChunks: %v", err)
	}
	if chunkTotal <= 0 {
		t.Fatalf("chunkTotal = %d, want > 0", chunkTotal)
	}
	if totalEstimated <= 0 {
		t.Fatalf("totalEstimated = %d, want > 0", totalEstimated)
	}
	if len(repo.pageLimits) != 2 || repo.pageLimits[0] != 50 || repo.pageLimits[1] != 50 {
		t.Fatalf("page limits = %#v, want two 50-record pages", repo.pageLimits)
	}
	for i := 1; i <= chunkTotal; i++ {
		chunk := repo.chunks[usageBriefResumeChunkKey(job.ID, i, UsageBriefChunkTypeSource)]
		if chunk.Status != UsageBriefStatusQueued {
			t.Fatalf("source chunk %d status = %q, want queued before AI", i, chunk.Status)
		}
		if strings.TrimSpace(chunk.SummaryMD) != "" {
			t.Fatalf("source chunk %d summary should be empty before AI", i)
		}
	}
	if len(repo.conversations) == 0 {
		t.Fatalf("expected independent conversation rows")
	}
	if len(repo.conversations) == 1 && chunkTotal == 1 {
		t.Fatalf("conversation rows should be independent from source chunk count; got one conversation and one chunk")
	}
	firstConversation := repo.conversations[1]
	if !strings.Contains(firstConversation.ConversationJSON, `"messages"`) {
		t.Fatalf("conversation json should include final request+response messages: %#v", firstConversation)
	}
	if strings.Contains(firstConversation.ConversationJSON, "compressed_request_payload_json") || strings.Contains(firstConversation.ConversationJSON, "compressed_response_payload_json") {
		t.Fatalf("conversation json should not expose compressed payload fields: %s", firstConversation.ConversationJSON)
	}
	firstChunk := repo.chunks[usageBriefResumeChunkKey(job.ID, 1, UsageBriefChunkTypeSource)]
	if strings.TrimSpace(firstChunk.ConversationJSON) != "" {
		t.Fatalf("source chunk should not store conversations anymore: %s", firstChunk.ConversationJSON)
	}
}

func TestUsageBriefGenerateRecordBriefContinuesAfterSourceChunks(t *testing.T) {
	job := UsageBriefJob{ID: 78}
	repo := &usageBriefResumeRepo{
		chunks: map[string]UsageBriefJobChunk{},
		pages: [][]UsageBriefSourceRecord{
			usageBriefTestRecords(1, 2),
		},
	}
	var callOrder []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callOrder = append(callOrder, "ai")
		_, _ = w.Write([]byte(`{"output_text":"## 摘要\n完成模块工作。","usage":{"input_tokens":11,"output_tokens":3}}`))
	}))
	defer server.Close()
	settingsRepo := &usageBriefSettingRepoStub{values: map[string]string{
		SettingKeyUsageBriefOpenAIBaseURL:        server.URL,
		SettingKeyUsageBriefOpenAIAPIKey:         "test-key",
		SettingKeyUsageBriefOpenAIModel:          "gpt-test",
		SettingKeyUsageBriefContextTokens:        "16000",
		SettingKeyUsageBriefOutputReservedTokens: "4096",
		SettingKeyUsageBriefConcurrency:          "4",
	}}
	svc := NewUsageBriefService(repo, settingsRepo, nil)
	content, inputTokens, outputTokens, status, err := svc.generateRecordBrief(
		context.Background(),
		job,
		"日报",
		map[string]any{"user_id": int64(7)},
		UsageBriefSourceRange{UserID: 7, Start: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)},
		workSummaryFinalInstruction("工作日报", "请生成普通用户工作日报。"),
	)
	if err != nil {
		t.Fatalf("generateRecordBrief err = %v", err)
	}
	if status != UsageBriefStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", status)
	}
	if strings.TrimSpace(content) == "" || inputTokens == 0 || outputTokens == 0 {
		t.Fatalf("content/tokens = %q %d/%d, want generated output and tokens", content, inputTokens, outputTokens)
	}
	if len(callOrder) != 2 {
		t.Fatalf("AI calls = %d, want source + merge", len(callOrder))
	}
	for key, chunk := range repo.chunks {
		if chunk.ChunkType == UsageBriefChunkTypeSource && (chunk.Status != UsageBriefStatusSucceeded || strings.TrimSpace(chunk.SummaryMD) == "") {
			t.Fatalf("chunk %s should be summarized: %#v", key, chunk)
		}
	}
}

func TestUsageBriefGenerateRecordBriefSkipsSourceChunkAfterRetries(t *testing.T) {
	oldDelay := usageBriefSourceChunkRetryDelay
	usageBriefSourceChunkRetryDelay = 0
	defer func() { usageBriefSourceChunkRetryDelay = oldDelay }()

	job := UsageBriefJob{ID: 79}
	chunkOneJSON := mustCompactJSON(map[string]any{"chunk_index": 1, "items": []any{map[string]any{"text": "第一个分片"}}})
	chunkTwoJSON := mustCompactJSON(map[string]any{"chunk_index": 2, "items": []any{map[string]any{"text": "第二个分片"}}})
	repo := &usageBriefResumeRepo{
		chunks: map[string]UsageBriefJobChunk{
			usageBriefResumeChunkKey(job.ID, 1, UsageBriefChunkTypeSource): {
				JobID:          job.ID,
				ChunkIndex:     1,
				ChunkType:      UsageBriefChunkTypeSource,
				Status:         UsageBriefStatusQueued,
				TokenEstimated: estimateTokenCount(chunkOneJSON),
				ContentJSON:    chunkOneJSON,
			},
			usageBriefResumeChunkKey(job.ID, 2, UsageBriefChunkTypeSource): {
				JobID:          job.ID,
				ChunkIndex:     2,
				ChunkType:      UsageBriefChunkTypeSource,
				Status:         UsageBriefStatusQueued,
				TokenEstimated: estimateTokenCount(chunkTwoJSON),
				ContentJSON:    chunkTwoJSON,
			},
		},
	}
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		if call <= usageBriefSourceChunkRetryLimit+1 {
			http.Error(w, "temporary upstream failure", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"output_text":"## 工作概览\n剩余分片已生成。","usage":{"input_tokens":7,"output_tokens":2}}`))
	}))
	defer server.Close()
	settingsRepo := &usageBriefSettingRepoStub{values: map[string]string{
		SettingKeyUsageBriefOpenAIBaseURL:        server.URL,
		SettingKeyUsageBriefOpenAIAPIKey:         "test-key",
		SettingKeyUsageBriefOpenAIModel:          "gpt-test",
		SettingKeyUsageBriefContextTokens:        "16000",
		SettingKeyUsageBriefOutputReservedTokens: "4096",
		SettingKeyUsageBriefConcurrency:          "4",
	}}
	svc := NewUsageBriefService(repo, settingsRepo, nil)
	settings, err := svc.requireUsageBriefAISettings(context.Background())
	if err != nil {
		t.Fatalf("requireUsageBriefAISettings err = %v", err)
	}
	content, _, _, status, err := svc.generatePreparedSourceBrief(
		context.Background(),
		settings,
		job,
		"日报",
		map[string]any{"user_id": int64(7)},
		workSummaryFinalInstruction("工作日报", "请生成普通用户工作日报。"),
		2,
		estimateTokenCount(chunkOneJSON)+estimateTokenCount(chunkTwoJSON),
	)
	if err != nil {
		t.Fatalf("generateRecordBrief err = %v", err)
	}
	if status != UsageBriefStatusPartial {
		t.Fatalf("status = %q, want partial", status)
	}
	if !strings.Contains(content, "部分成功") {
		t.Fatalf("content should include partial warning, got %q", content)
	}
	failed := repo.chunks[usageBriefResumeChunkKey(job.ID, 1, UsageBriefChunkTypeSource)]
	if failed.Status != UsageBriefStatusFailed {
		t.Fatalf("failed chunk status = %q, want failed", failed.Status)
	}
	if failed.RetryCount != usageBriefSourceChunkRetryLimit {
		t.Fatalf("retry_count = %d, want %d", failed.RetryCount, usageBriefSourceChunkRetryLimit)
	}
	if !strings.Contains(failed.ErrorMessage, "temporary upstream failure") {
		t.Fatalf("error_message = %q, want upstream failure detail", failed.ErrorMessage)
	}
	if failed.LastErrorAt == nil {
		t.Fatalf("last_error_at should be set")
	}
}

func usageBriefTestRecords(startID int64, count int) []UsageBriefSourceRecord {
	records := make([]UsageBriefSourceRecord, 0, count)
	base := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	for i := 0; i < count; i++ {
		id := startID + int64(i)
		records = append(records, UsageBriefSourceRecord{
			ID:                        id,
			CreatedAt:                 base.Add(time.Duration(id) * time.Minute),
			Model:                     "gpt-test",
			RequestType:               "chat",
			InputTokens:               10,
			OutputTokens:              5,
			CompressedRequestPayload:  fmt.Sprintf(`{"messages":[{"role":"user","content":"完成模块 %d"}]}`, id),
			CompressedResponsePayload: fmt.Sprintf(`{"messages":[{"role":"assistant","content":"完成模块 %d 的回复"}]}`, id),
		})
	}
	return records
}

func TestUsageBriefGenerateChunkedBriefReusesSucceededChunks(t *testing.T) {
	job := UsageBriefJob{ID: 99}
	settings := &UsageBriefSettings{
		BaseURL:              "http://example.invalid",
		APIKey:               "test-key",
		Model:                "gpt-test",
		ContextTokens:        16000,
		OutputReservedTokens: 4096,
	}
	meta := map[string]any{"user_id": int64(7)}
	items := []usageBriefPromptItem{
		{
			Payload: UsageBriefSourceRecord{
				ID:             1,
				CreatedAt:      time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
				Model:          "gpt-test",
				RequestType:    "chat",
				InputTokens:    12,
				OutputTokens:   34,
				RequestPayload: `{"messages":[{"role":"user","content":"设计订单模块"}]}`,
			},
			EstimatedTokens: 80,
		},
	}
	chunks := promptItemChunks(items, chunkInputTokenBudget(settings))
	sourcePayload := map[string]any{
		"label":       "日报",
		"item_kind":   "usage_records",
		"chunk_index": 1,
		"chunk_total": len(chunks),
		"meta":        meta,
		"items":       promptChunkPayloads(chunks[0].Items),
	}
	sourceSummary := "## 分片摘要\n完成订单模块设计。"
	sourceSummaryInputTokens := 101
	sourceSummaryOutputTokens := 21
	summaries := []usageBriefChunkSummary{
		{
			Title:           "日报分片 1/1",
			ContentMD:       sourceSummary,
			EstimatedTokens: estimateTokenCount(sourceSummary),
		},
	}
	finalPayload := map[string]any{
		"label":     "日报",
		"meta":      meta,
		"summaries": summaries,
	}
	finalSummary := "## 工作概览\n完成订单模块设计。\n\n## 用量概括\n本期 token 使用较少。"
	finalInputTokens := 55
	finalOutputTokens := 13
	repo := &usageBriefResumeRepo{chunks: map[string]UsageBriefJobChunk{
		usageBriefResumeChunkKey(job.ID, 1, UsageBriefChunkTypeSource): {
			JobID:          job.ID,
			ChunkIndex:     1,
			ChunkType:      UsageBriefChunkTypeSource,
			Status:         UsageBriefStatusSucceeded,
			TokenEstimated: chunks[0].EstimatedTokens,
			InputTokens:    sourceSummaryInputTokens,
			OutputTokens:   sourceSummaryOutputTokens,
			ContentJSON:    mustCompactJSON(sourcePayload),
			SummaryMD:      sourceSummary,
		},
		usageBriefResumeChunkKey(job.ID, 200001, UsageBriefChunkTypeMerge): {
			JobID:          job.ID,
			ChunkIndex:     200001,
			ChunkType:      UsageBriefChunkTypeMerge,
			Status:         UsageBriefStatusSucceeded,
			TokenEstimated: estimateTokenCount(mustCompactJSON(finalPayload)),
			InputTokens:    finalInputTokens,
			OutputTokens:   finalOutputTokens,
			ContentJSON:    mustCompactJSON(finalPayload),
			SummaryMD:      finalSummary,
		},
	}}
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	settings.BaseURL = server.URL

	svc := NewUsageBriefService(repo, nil, nil)
	svc.httpClient = server.Client()
	content, inputTokens, outputTokens, err := svc.generateChunkedBrief(
		context.Background(),
		settings,
		job,
		"日报",
		"usage_records",
		meta,
		items,
		workSummaryFinalInstruction("工作日报", "请生成普通用户工作日报。"),
	)
	if err != nil {
		t.Fatalf("generateChunkedBrief: %v", err)
	}
	if content != finalSummary {
		t.Fatalf("content = %q, want %q", content, finalSummary)
	}
	if inputTokens != sourceSummaryInputTokens+finalInputTokens {
		t.Fatalf("inputTokens = %d, want %d", inputTokens, sourceSummaryInputTokens+finalInputTokens)
	}
	if outputTokens != sourceSummaryOutputTokens+finalOutputTokens {
		t.Fatalf("outputTokens = %d, want %d", outputTokens, sourceSummaryOutputTokens+finalOutputTokens)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("AI endpoint calls = %d, want 0", got)
	}
	if repo.deletedJobChunks != 0 {
		t.Fatalf("resume generation should not delete all chunks, got %d calls", repo.deletedJobChunks)
	}
	if repo.deletedStaleSources != 1 {
		t.Fatalf("expected stale source cleanup once, got %d", repo.deletedStaleSources)
	}
	if repo.progressUpdates == 0 {
		t.Fatalf("expected progress updates")
	}
}

func TestUsageBriefChunkInputComparisonIgnoresJSONFormatting(t *testing.T) {
	a := `{"meta":{"user_id":7},"items":[{"id":1,"text":"hello"}]}`
	b := `{ "items" : [ { "text" : "hello", "id" : 1 } ], "meta" : { "user_id" : 7 } }`
	if !sameUsageBriefChunkInput(a, b) {
		t.Fatalf("expected semantically equal JSON payloads to match")
	}
	if sameUsageBriefChunkInput(a, `{"meta":{"user_id":8},"items":[{"id":1,"text":"hello"}]}`) {
		t.Fatalf("expected changed JSON payloads not to match")
	}
}

func TestUsageBriefReusableChunkRejectsChangedInput(t *testing.T) {
	repo := &usageBriefResumeRepo{chunks: map[string]UsageBriefJobChunk{
		usageBriefResumeChunkKey(12, 1, UsageBriefChunkTypeSource): {
			JobID:       12,
			ChunkIndex:  1,
			ChunkType:   UsageBriefChunkTypeSource,
			Status:      UsageBriefStatusSucceeded,
			ContentJSON: `{"items":[{"id":1,"text":"old"}]}`,
			SummaryMD:   "old summary",
		},
	}}
	svc := NewUsageBriefService(repo, nil, nil)
	chunk, ok, err := svc.reusableSucceededJobChunk(context.Background(), 12, 1, UsageBriefChunkTypeSource, `{"items":[{"id":1,"text":"new"}]}`)
	if err != nil {
		t.Fatalf("reusableSucceededJobChunk: %v", err)
	}
	if ok || chunk != nil {
		t.Fatalf("changed input should not reuse existing chunk: ok=%v chunk=%#v", ok, chunk)
	}
}
