package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildUsageLogDetailFromCapture_RedactsRequestAndNormalizesSSE(t *testing.T) {
	requestBody := []byte(`{"model":"gpt-5","api_key":"secret-key","messages":[{"role":"user","content":"hello"}]}`)
	responseBody := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\" world\"}\n\ndata: [DONE]\n\n")

	detail := BuildUsageLogDetailFromCapture(&UsageLogDetailCapture{
		RequestBody:             requestBody,
		ResponseBody:            responseBody,
		ResponseBodyBytes:       len(responseBody),
		ResponseCaptureTruncated: false,
		ResponseFormat:          UsageLogDetailResponseFormatSSE,
	})

	require.NotNil(t, detail)
	require.NotNil(t, detail.RequestPayloadJSON)
	require.Contains(t, *detail.RequestPayloadJSON, `[REDACTED]`)
	require.NotContains(t, *detail.RequestPayloadJSON, `secret-key`)
	require.NotNil(t, detail.ResponsePayloadJSON)
	require.Contains(t, *detail.ResponsePayloadJSON, `usage_detail_stream_snapshot`)
	require.Contains(t, *detail.ResponsePayloadJSON, `hello world`)
	require.NotNil(t, detail.ResponsePayloadBytes)
	require.Equal(t, len(responseBody), *detail.ResponsePayloadBytes)
}

func TestBuildUsageLogDetailView_ParsesRequestAndResponseMessages(t *testing.T) {
	requestPayload := `{"system":"You are a helpful assistant","messages":[{"role":"user","content":"Hi"}]}`
	responsePayload := `{"choices":[{"message":{"role":"assistant","content":"Hello back"}}]}`

	view := BuildUsageLogDetailView(true, &UsageLog{ID: 1}, &UsageLogDetail{
		UsageLogID:         1,
		RequestPayloadJSON: &requestPayload,
		ResponsePayloadJSON: &responsePayload,
	})

	require.True(t, view.Available)
	require.Len(t, view.RequestMessages, 2)
	require.Equal(t, "system", view.RequestMessages[0].Role)
	require.Equal(t, "user", view.RequestMessages[1].Role)
	require.Len(t, view.ResponseMessages, 1)
	require.Equal(t, "assistant", view.ResponseMessages[0].Role)
	require.Equal(t, "Hello back", view.ResponseMessages[0].Text)
}

func TestBuildUsageLogDetailView_ReturnsHistoricalWhenMissingDetail(t *testing.T) {
	view := BuildUsageLogDetailView(true, &UsageLog{ID: 2}, nil)
	require.False(t, view.Available)
	require.NotNil(t, view.Reason)
	require.Equal(t, UsageLogDetailUnavailableReasonHistorical, *view.Reason)
}

func TestParseUsageLogRequestMessages_SkipsAssistantHistoryInInput(t *testing.T) {
	raw := `{
		"input": [
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"old answer"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"new question"}]}
		]
	}`

	messages := parseUsageLogRequestMessages(nil, raw)
	require.Len(t, messages, 1)
	require.Equal(t, "user", messages[0].Role)
	require.Equal(t, "new question", messages[0].Text)
}

func TestBuildUsageLogResponsePayloadWithFallback_UsesDeltaTextWhenTerminalPayloadHasNoMessages(t *testing.T) {
	payload := []byte(`{"id":"resp_1","usage":{"input_tokens":1,"output_tokens":2}}`)
	normalized := buildUsageLogResponsePayloadWithFallback(payload, "assistant from delta")
	require.NotNil(t, normalized)
	require.Contains(t, string(normalized), "assistant from delta")
}
