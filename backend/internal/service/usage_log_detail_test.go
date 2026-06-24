package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildUsageLogDetailFromCapture_RedactsRequestAndNormalizesSSE(t *testing.T) {
	requestBody := []byte(`{"model":"gpt-5","api_key":"secret-key","messages":[{"role":"user","content":"hello"}]}`)
	responseBody := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\" world\"}\n\ndata: [DONE]\n\n")

	detail := BuildUsageLogDetailFromCapture(&UsageLogDetailCapture{
		RequestBody:              requestBody,
		ResponseBody:             responseBody,
		ResponseBodyBytes:        len(responseBody),
		ResponseCaptureTruncated: false,
		ResponseFormat:           UsageLogDetailResponseFormatSSE,
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

func TestBuildUsageLogDetailFromCapture_DoesNotTruncateLargeJSONPayloads(t *testing.T) {
	longRequest := strings.Repeat("request-content-", 5000)
	longResponse := strings.Repeat("response-content-", 5000)
	requestBody := []byte(`{"model":"gpt-5","api_key":"secret-key","messages":[{"role":"user","content":"` + longRequest + `"}]}`)
	responseBody := []byte(`{"choices":[{"message":{"role":"assistant","content":"` + longResponse + `"}}]}`)

	detail := BuildUsageLogDetailFromCapture(&UsageLogDetailCapture{
		RequestBody:       requestBody,
		ResponseBody:      responseBody,
		ResponseBodyBytes: len(responseBody),
		ResponseFormat:    UsageLogDetailResponseFormatJSON,
	})

	require.NotNil(t, detail)
	require.False(t, detail.RequestTruncated)
	require.False(t, detail.ResponseTruncated)
	require.NotNil(t, detail.RequestPayloadJSON)
	require.NotNil(t, detail.ResponsePayloadJSON)
	require.Contains(t, *detail.RequestPayloadJSON, longRequest)
	require.Contains(t, *detail.ResponsePayloadJSON, longResponse)
	require.NotContains(t, *detail.RequestPayloadJSON, "secret-key")
	require.Contains(t, *detail.RequestPayloadJSON, `[REDACTED]`)
	require.NotNil(t, detail.CompressedRequestPayloadJSON)
	require.Contains(t, *detail.CompressedRequestPayloadJSON, longRequest)
	require.NotNil(t, detail.RequestPayloadBytes)
	require.Equal(t, len(requestBody), *detail.RequestPayloadBytes)
	require.NotNil(t, detail.ResponsePayloadBytes)
	require.Equal(t, len(responseBody), *detail.ResponsePayloadBytes)
}

func TestCompressUsageLogPayloadJSON_OpenAIResponsesRequestKeepsOnlyUserAssistantText(t *testing.T) {
	payload := `{
		"client_metadata":{"session_id":"secret"},
		"input":[
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"dev"}]},
			{"type":"reasoning","summary":[]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_image","image_url":"data:image/png;base64,abc"}]},
			{"type":"function_call","name":"tool"},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]},
			{"type":"function_call_output","output":"tool output"}
		]
	}`

	got := CompressUsageLogPayloadJSON(&payload, UsageLogPayloadKindRequest)
	require.NotNil(t, got)
	require.JSONEq(t, `{"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"answer"}]}`, *got)
	require.NotContains(t, *got, "client_metadata")
	require.NotContains(t, *got, "function_call")
	require.NotContains(t, *got, "base64")
}

func TestCompressUsageLogPayloadJSON_OpenAIResponsesInputWithoutMessageType(t *testing.T) {
	payload := `{
		"input": [
			{"content":[{"text":"你好","type":"input_text"}],"role":"user"},
			{"content":[{"text":"你好！我在这，想让我帮你做什么？","type":"output_text"}],"role":"assistant"},
			{"content":[{"text":"你是谁","type":"input_text"}],"role":"user"}
		],
		"model":"gpt-5.4",
		"stream":true
	}`

	got := CompressUsageLogPayloadJSON(&payload, UsageLogPayloadKindRequest)
	require.NotNil(t, got)
	require.JSONEq(t, `{
		"messages": [
			{"role":"user","content":"你好"},
			{"role":"assistant","content":"你好！我在这，想让我帮你做什么？"},
			{"role":"user","content":"你是谁"}
		]
	}`, *got)
}

func TestCompressUsageLogPayloadJSON_OpenAIResponsesToolOnlyResponseIsEmpty(t *testing.T) {
	payload := `{"output":[{"type":"function_call","name":"exec_command","arguments":"{}"}]}`

	got := CompressUsageLogPayloadJSON(&payload, UsageLogPayloadKindResponse)
	require.NotNil(t, got)
	require.JSONEq(t, `{"messages":[]}`, *got)
}

func TestCompressUsageLogPayloadJSON_ChatCompletionsResponse(t *testing.T) {
	payload := `{"choices":[{"message":{"role":"assistant","content":"chat answer"}}],"usage":{"total_tokens":3}}`

	got := CompressUsageLogPayloadJSON(&payload, UsageLogPayloadKindResponse)
	require.NotNil(t, got)
	require.JSONEq(t, `{"messages":[{"role":"assistant","content":"chat answer"}]}`, *got)
	require.NotContains(t, *got, "usage")
}

func TestCompressUsageLogPayloadJSON_AnthropicDropsToolsAndPseudoContext(t *testing.T) {
	payload := `{
		"messages":[
			{"role":"user","content":[{"type":"text","text":"<system-reminder>auto</system-reminder>"}]},
			{"role":"user","content":[{"type":"text","text":"real question"},{"type":"tool_result","content":"tool result"}]},
			{"role":"assistant","content":[{"type":"tool_use","name":"Read"},{"type":"text","text":"real answer"}]},
			{"role":"user","content":[{"type":"image","source":{"type":"base64","data":"abc"}}]}
		]
	}`

	got := CompressUsageLogPayloadJSON(&payload, UsageLogPayloadKindRequest)
	require.NotNil(t, got)
	require.JSONEq(t, `{"messages":[{"role":"user","content":"real question"},{"role":"assistant","content":"real answer"}]}`, *got)
	require.NotContains(t, *got, "tool_result")
	require.NotContains(t, *got, "base64")
	require.NotContains(t, *got, "system-reminder")
}

func TestBuildUsageLogDetailView_ParsesRequestAndResponseMessages(t *testing.T) {
	requestPayload := `{"system":"You are a helpful assistant","messages":[{"role":"user","content":"Hi"}]}`
	responsePayload := `{"choices":[{"message":{"role":"assistant","content":"Hello back"}}]}`
	compressedRequest := `{"messages":[{"role":"user","content":"Hi"}]}`
	compressedResponse := `{"messages":[{"role":"assistant","content":"Hello back"}]}`

	view := BuildUsageLogDetailView(true, &UsageLog{ID: 1}, &UsageLogDetail{
		UsageLogID:                    1,
		RequestPayloadJSON:            &requestPayload,
		ResponsePayloadJSON:           &responsePayload,
		CompressedRequestPayloadJSON:  &compressedRequest,
		CompressedResponsePayloadJSON: &compressedResponse,
	})

	require.True(t, view.Available)
	require.Len(t, view.RequestMessages, 1)
	require.Equal(t, "user", view.RequestMessages[0].Role)
	require.Len(t, view.ResponseMessages, 1)
	require.Equal(t, "assistant", view.ResponseMessages[0].Role)
	require.Equal(t, "Hello back", view.ResponseMessages[0].Text)
}

func TestBuildUsageLogDetailView_UsesCompressedPayloadAfterFullPayloadCleaned(t *testing.T) {
	cleanedAt := time.Now()
	compressedRequest := `{"messages":[{"role":"user","content":"compressed request"}]}`
	view := BuildUsageLogDetailView(true, &UsageLog{ID: 1}, &UsageLogDetail{
		UsageLogID:                   1,
		CompressedRequestPayloadJSON: &compressedRequest,
		FullPayloadsCleanedAt:        &cleanedAt,
	})

	require.True(t, view.Available)
	require.Nil(t, view.RequestPayloadJSON)
	require.Equal(t, &cleanedAt, view.FullPayloadsCleanedAt)
	require.Len(t, view.RequestMessages, 1)
	require.Equal(t, "compressed request", view.RequestMessages[0].Text)
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
