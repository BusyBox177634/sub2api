package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
)

const (
	usageLogDetailMaxStoredRequestBytes  = 64 * 1024
	usageLogDetailMaxStoredResponseBytes = 64 * 1024
	usageLogDetailUpsertRetryInterval    = 25 * time.Millisecond
	usageLogDetailUpsertRetryTimeout     = 750 * time.Millisecond
)

type UsageLogDetailUnavailableReason string

const (
	UsageLogDetailUnavailableReasonDisabled    UsageLogDetailUnavailableReason = "disabled"
	UsageLogDetailUnavailableReasonHistorical  UsageLogDetailUnavailableReason = "historical"
	UsageLogDetailUnavailableReasonNotCaptured UsageLogDetailUnavailableReason = "not_captured"
)

type UsageLogMessage struct {
	Role   string `json:"role"`
	Source string `json:"source"`
	Text   string `json:"text"`
}

type UsageLogDetail struct {
	UsageLogID int64

	RequestPayloadJSON *string
	ResponsePayloadJSON *string

	RequestPayloadBytes  *int
	ResponsePayloadBytes *int

	RequestTruncated  bool
	ResponseTruncated bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

type UsageLogDetailView struct {
	Available bool `json:"available"`

	Reason *UsageLogDetailUnavailableReason `json:"reason,omitempty"`

	RequestMessages  []UsageLogMessage `json:"request_messages"`
	ResponseMessages []UsageLogMessage `json:"response_messages"`

	RequestPayloadJSON  *string `json:"request_payload_json,omitempty"`
	ResponsePayloadJSON *string `json:"response_payload_json,omitempty"`

	RequestTruncated  bool `json:"request_truncated"`
	ResponseTruncated bool `json:"response_truncated"`
}

type UsageLogDetailResponseFormat string

const (
	UsageLogDetailResponseFormatJSON UsageLogDetailResponseFormat = "json"
	UsageLogDetailResponseFormatSSE  UsageLogDetailResponseFormat = "sse"
)

type UsageLogDetailCapture struct {
	RequestBody []byte

	ResponseBody            []byte
	ResponseBodyBytes       int
	ResponseCaptureTruncated bool
	ResponseFormat          UsageLogDetailResponseFormat
}

type UsageLogDetailRepository interface {
	UpsertByRequestAndAPIKey(ctx context.Context, requestID string, apiKeyID int64, detail *UsageLogDetail) error
	GetByUsageLogID(ctx context.Context, usageLogID int64) (*UsageLogDetail, error)
}

var ErrUsageLogDetailUsageTargetNotReady = errors.New("usage log detail target not ready")

func BuildUsageLogDetailFromCapture(capture *UsageLogDetailCapture) *UsageLogDetail {
	if capture == nil {
		return nil
	}

	requestPayloadJSON, requestTruncated, requestPayloadBytes := prepareUsageLogDetailPayload(capture.RequestBody, usageLogDetailMaxStoredRequestBytes)
	responsePayloadJSON, responseTruncated, responsePayloadBytes := prepareUsageLogDetailResponsePayload(capture)

	return &UsageLogDetail{
		RequestPayloadJSON:  requestPayloadJSON,
		ResponsePayloadJSON: responsePayloadJSON,
		RequestPayloadBytes: requestPayloadBytes,
		ResponsePayloadBytes: responsePayloadBytes,
		RequestTruncated:    requestTruncated,
		ResponseTruncated:   responseTruncated,
	}
}

func BuildUsageLogDetailView(
	settingsEnabled bool,
	log *UsageLog,
	detail *UsageLogDetail,
) *UsageLogDetailView {
	view := &UsageLogDetailView{
		Available:         false,
		RequestMessages:   []UsageLogMessage{},
		ResponseMessages:  []UsageLogMessage{},
		RequestTruncated:  false,
		ResponseTruncated: false,
	}

	if detail == nil {
		reason := UsageLogDetailUnavailableReasonHistorical
		if !settingsEnabled {
			reason = UsageLogDetailUnavailableReasonDisabled
		}
		view.Reason = &reason
		return view
	}

	view.RequestPayloadJSON = cloneStringPtr(detail.RequestPayloadJSON)
	view.ResponsePayloadJSON = cloneStringPtr(detail.ResponsePayloadJSON)
	view.RequestTruncated = detail.RequestTruncated
	view.ResponseTruncated = detail.ResponseTruncated

	requestPayload := strings.TrimSpace(derefString(detail.RequestPayloadJSON))
	responsePayload := strings.TrimSpace(derefString(detail.ResponsePayloadJSON))
	if requestPayload == "" && responsePayload == "" {
		reason := UsageLogDetailUnavailableReasonNotCaptured
		view.Reason = &reason
		return view
	}

	view.Available = true
	view.RequestMessages = parseUsageLogRequestMessages(log, requestPayload)
	view.ResponseMessages = parseUsageLogResponseMessages(log, responsePayload)
	return view
}

func prepareUsageLogDetailPayload(raw []byte, maxBytes int) (payloadJSON *string, truncated bool, payloadBytes *int) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	sanitized, sanitizedTruncated, bytesLen := sanitizeAndTrimRequestBody(raw, maxBytes)
	if sanitized != "" {
		out := sanitized
		payloadJSON = &out
	}
	n := bytesLen
	payloadBytes = &n
	return payloadJSON, sanitizedTruncated, payloadBytes
}

func prepareUsageLogDetailResponsePayload(capture *UsageLogDetailCapture) (payloadJSON *string, truncated bool, payloadBytes *int) {
	if capture == nil {
		return nil, false, nil
	}

	var raw []byte
	switch capture.ResponseFormat {
	case UsageLogDetailResponseFormatSSE:
		raw = normalizeUsageLogSSEPayload(capture.ResponseBody)
	default:
		raw = capture.ResponseBody
	}

	payloadJSON, sanitizedTruncated, payloadBytes := prepareUsageLogDetailPayload(raw, usageLogDetailMaxStoredResponseBytes)

	if capture.ResponseBodyBytes > 0 {
		n := capture.ResponseBodyBytes
		payloadBytes = &n
	}
	return payloadJSON, capture.ResponseCaptureTruncated || sanitizedTruncated, payloadBytes
}

func normalizeUsageLogSSEPayload(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}

	assistantText, terminalResponse := collectUsageLogSSEAssistantContent(raw)
	if payload := buildUsageLogResponsePayloadWithFallback(terminalResponse, assistantText); len(payload) > 0 {
		return payload
	}
	return nil
}

func collectUsageLogSSEAssistantContent(raw []byte) (assistantText string, terminalResponse []byte) {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	var builder strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" || !gjson.Valid(payload) {
			continue
		}

		payloadBytes := []byte(payload)
		eventType := strings.TrimSpace(gjson.GetBytes(payloadBytes, "type").String())
		switch eventType {
		case "response.output_text.delta":
			builder.WriteString(gjson.GetBytes(payloadBytes, "delta").String())
		case "content_block_delta":
			builder.WriteString(gjson.GetBytes(payloadBytes, "delta.text").String())
		}

		if content := gjson.GetBytes(payloadBytes, "choices.0.delta.content").String(); content != "" {
			builder.WriteString(content)
		}

		if geminiText := strings.TrimSpace(extractGeminiCandidateText(gjson.GetBytes(payloadBytes, "response"))); geminiText != "" {
			builder.WriteString(geminiText)
		} else if geminiText := strings.TrimSpace(extractGeminiCandidateTextBytes(payloadBytes)); geminiText != "" {
			builder.WriteString(geminiText)
		}

		if isUsageLogTerminalResponseEvent(eventType) {
			responseRaw := strings.TrimSpace(gjson.GetBytes(payloadBytes, "response").Raw)
			if responseRaw != "" && json.Valid([]byte(responseRaw)) {
				return strings.TrimSpace(builder.String()), []byte(responseRaw)
			}
		}
	}

	return strings.TrimSpace(builder.String()), nil
}

func isUsageLogTerminalResponseEvent(eventType string) bool {
	switch eventType {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}

func mustMarshalUsageLogSnapshot(snapshotType string, assistantText string) []byte {
	type contentItem struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type messageItem struct {
		Role    string        `json:"role"`
		Content []contentItem `json:"content"`
	}
	payload := struct {
		Type     string        `json:"type"`
		Messages []messageItem `json:"messages"`
	}{
		Type: snapshotType,
		Messages: []messageItem{
			{
				Role: "assistant",
				Content: []contentItem{
					{
						Type: "text",
						Text: assistantText,
					},
				},
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return encoded
}

func buildUsageLogResponsePayloadWithFallback(payload []byte, assistantText string) []byte {
	if len(payload) > 0 && json.Valid(payload) {
		copied := make([]byte, len(payload))
		copy(copied, payload)
		if len(parseUsageLogResponseMessages(nil, string(copied))) > 0 {
			return copied
		}
	}
	if strings.TrimSpace(assistantText) == "" {
		return nil
	}
	return mustMarshalUsageLogSnapshot("usage_detail_stream_snapshot", assistantText)
}

func extractGeminiCandidateTextBytes(raw []byte) string {
	return extractGeminiCandidateText(gjson.ParseBytes(raw))
}

func extractGeminiCandidateText(result gjson.Result) string {
	if !result.Exists() {
		return ""
	}
	if text := extractGJSONText(result.Get("candidates")); text != "" {
		return text
	}
	return extractGJSONText(result.Get("response.candidates"))
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func writeUsageLogAndDetailBestEffort(
	ctx context.Context,
	usageRepo UsageLogRepository,
	detailRepo UsageLogDetailRepository,
	usageLog *UsageLog,
	detail *UsageLogDetail,
	logKey string,
) {
	if usageRepo == nil || usageLog == nil {
		return
	}

	usageCtx, cancel := detachedBillingContext(ctx)
	defer cancel()

	createUsageLog := func() bool {
		if writer, ok := usageRepo.(usageLogBestEffortWriter); ok {
			if err := writer.CreateBestEffort(usageCtx, usageLog); err != nil {
				logger.LegacyPrintf(logKey, "Create usage log failed: %v", err)
				if IsUsageLogCreateDropped(err) {
					return false
				}
				if _, syncErr := usageRepo.Create(usageCtx, usageLog); syncErr != nil {
					logger.LegacyPrintf(logKey, "Create usage log sync fallback failed: %v", syncErr)
					return false
				}
			}
			return true
		}

		if _, err := usageRepo.Create(usageCtx, usageLog); err != nil {
			logger.LegacyPrintf(logKey, "Create usage log failed: %v", err)
			return false
		}
		return true
	}

	if !createUsageLog() {
		return
	}

	if detailRepo == nil || detail == nil || strings.TrimSpace(usageLog.RequestID) == "" {
		return
	}
	if err := upsertUsageLogDetailWithRetry(usageCtx, detailRepo, usageLog, detail); err != nil {
		logger.LegacyPrintf(logKey, "Create usage log detail failed: %v", err)
	}
}

func upsertUsageLogDetailWithRetry(
	ctx context.Context,
	repo UsageLogDetailRepository,
	usageLog *UsageLog,
	detail *UsageLogDetail,
) error {
	if repo == nil || usageLog == nil || detail == nil {
		return nil
	}

	deadline := time.Now().Add(usageLogDetailUpsertRetryTimeout)
	for {
		err := repo.UpsertByRequestAndAPIKey(ctx, usageLog.RequestID, usageLog.APIKeyID, detail)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrUsageLogDetailUsageTargetNotReady) {
			return err
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(usageLogDetailUpsertRetryInterval):
		}
	}
}
