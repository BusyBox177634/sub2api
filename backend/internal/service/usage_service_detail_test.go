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
