//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageServiceGetDetailByUsageLog_ReturnsStoredDetailWhenRepoWired(t *testing.T) {
	requestPayload := `{"messages":[{"role":"user","content":"hello"}]}`
	responsePayload := `{"choices":[{"message":{"role":"assistant","content":"world"}}]}`

	svc := NewUsageService(nil, nil, nil, nil)
	svc.SetSettingService(&SettingService{})
	svc.SetUsageLogDetailRepo(&usageLogDetailRepoStub{
		detailByLogID: map[int64]*UsageLogDetail{
			99: {
				UsageLogID:          99,
				RequestPayloadJSON:  &requestPayload,
				ResponsePayloadJSON: &responsePayload,
			},
		},
	})

	detail, err := svc.GetDetailByUsageLog(context.Background(), &UsageLog{ID: 99})

	require.NoError(t, err)
	require.NotNil(t, detail)
	require.True(t, detail.Available)
	require.Len(t, detail.RequestMessages, 1)
	require.Equal(t, "user", detail.RequestMessages[0].Role)
	require.Equal(t, "hello", detail.RequestMessages[0].Text)
	require.Len(t, detail.ResponseMessages, 1)
	require.Equal(t, "assistant", detail.ResponseMessages[0].Role)
	require.Equal(t, "world", detail.ResponseMessages[0].Text)
}

func TestUsageServiceGetDetailByUsageLog_GeneratesMissingCompressedPayloads(t *testing.T) {
	requestPayload := `{"messages":[{"role":"system","content":"hidden"},{"role":"user","content":"hello"}]}`
	responsePayload := `{"choices":[{"message":{"role":"assistant","content":"world"}}]}`
	repo := &usageLogDetailRepoStub{
		detailByLogID: map[int64]*UsageLogDetail{
			100: {
				UsageLogID:          100,
				RequestPayloadJSON:  &requestPayload,
				ResponsePayloadJSON: &responsePayload,
			},
		},
	}
	svc := NewUsageService(nil, nil, nil, nil)
	svc.SetSettingService(&SettingService{})
	svc.SetUsageLogDetailRepo(repo)

	detail, err := svc.GetDetailByUsageLog(context.Background(), &UsageLog{ID: 100})

	require.NoError(t, err)
	require.NotNil(t, detail)
	require.Equal(t, 1, repo.updateCompressedCalls)
	require.NotNil(t, detail.CompressedRequestPayloadJSON)
	require.NotNil(t, detail.CompressedResponsePayloadJSON)
	require.JSONEq(t, `{"messages":[{"role":"user","content":"hello"}]}`, *detail.CompressedRequestPayloadJSON)
	require.JSONEq(t, `{"messages":[{"role":"assistant","content":"world"}]}`, *detail.CompressedResponsePayloadJSON)
	require.Len(t, detail.RequestMessages, 1)
	require.Equal(t, "hello", detail.RequestMessages[0].Text)
}
