package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNextUsageLogDetailRetentionRunUsesLocal0010(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	before := time.Date(2026, 6, 24, 0, 5, 0, 0, loc)
	require.Equal(t, time.Date(2026, 6, 24, 0, 10, 0, 0, loc), nextUsageLogDetailRetentionRun(before, loc))

	after := time.Date(2026, 6, 24, 0, 11, 0, 0, loc)
	require.Equal(t, time.Date(2026, 6, 25, 0, 10, 0, 0, loc), nextUsageLogDetailRetentionRun(after, loc))
}

func TestUsageLogDetailRetentionRunOnceGeneratesCompressedBeforeClearingFullPayloads(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	windowStart, _ := usageLogDetailRetentionYesterdayWindow(time.Now().In(loc), loc)
	yesterday := windowStart.Add(10 * time.Hour)
	requestPayload := `{"messages":[{"role":"user","content":"old request"}]}`
	responsePayload := `{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"old response"}]}]}`
	repo := &usageLogDetailRepoStub{
		cleanupDetails: []UsageLogDetail{
			{
				UsageLogID:          77,
				RequestPayloadJSON:  &requestPayload,
				ResponsePayloadJSON: &responsePayload,
				CreatedAt:           yesterday,
			},
		},
		detailByLogID: map[int64]*UsageLogDetail{
			77: {
				UsageLogID:          77,
				RequestPayloadJSON:  &requestPayload,
				ResponsePayloadJSON: &responsePayload,
				CreatedAt:           yesterday,
			},
		},
	}
	svc := &UsageLogDetailRetentionService{
		repo:   repo,
		loc:    loc,
		stopCh: make(chan struct{}),
	}

	svc.runOnce()

	require.Equal(t, 1, repo.clearFullPayloadCalls)
	detail, err := repo.GetByUsageLogID(context.Background(), 77)
	require.NoError(t, err)
	require.Nil(t, detail.RequestPayloadJSON)
	require.Nil(t, detail.ResponsePayloadJSON)
	require.NotNil(t, detail.CompressedRequestPayloadJSON)
	require.NotNil(t, detail.CompressedResponsePayloadJSON)
	require.JSONEq(t, `{"messages":[{"role":"user","content":"old request"}]}`, *detail.CompressedRequestPayloadJSON)
	require.JSONEq(t, `{"messages":[{"role":"assistant","content":"old response"}]}`, *detail.CompressedResponsePayloadJSON)
	require.NotNil(t, detail.FullPayloadsCleanedAt)
	status := svc.Snapshot()
	require.False(t, status.Running)
	require.Equal(t, UsageLogDetailRetentionPhaseIdle, status.Phase)
	require.EqualValues(t, 1, status.TotalPendingAtStart)
	require.EqualValues(t, 1, status.Cleaned)
	require.EqualValues(t, 0, status.RemainingPending)
	require.Equal(t, 100.0, status.ProgressPercent)
}

func TestUsageLogDetailRetentionRunOnceFallsBackAndClearsInvalidJSON(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	windowStart, _ := usageLogDetailRetentionYesterdayWindow(time.Now().In(loc), loc)
	yesterday := windowStart.Add(10 * time.Hour)
	invalidPayload := `not-json`
	repo := &usageLogDetailRepoStub{
		cleanupDetails: []UsageLogDetail{
			{
				UsageLogID:         88,
				RequestPayloadJSON: &invalidPayload,
				CreatedAt:          yesterday,
			},
		},
		detailByLogID: map[int64]*UsageLogDetail{
			88: {
				UsageLogID:         88,
				RequestPayloadJSON: &invalidPayload,
				CreatedAt:          yesterday,
			},
		},
	}
	svc := &UsageLogDetailRetentionService{
		repo:   repo,
		loc:    loc,
		stopCh: make(chan struct{}),
	}

	svc.runOnce()

	require.Equal(t, 1, repo.clearFullPayloadCalls)
	detail, err := repo.GetByUsageLogID(context.Background(), 88)
	require.NoError(t, err)
	require.Nil(t, detail.RequestPayloadJSON)
	require.NotNil(t, detail.CompressedRequestPayloadJSON)
	require.JSONEq(t, `{"messages":[]}`, *detail.CompressedRequestPayloadJSON)
	require.NotNil(t, detail.FullPayloadsCleanedAt)
	status := svc.Snapshot()
	require.EqualValues(t, 1, status.FallbackEmpty)
	require.EqualValues(t, 1, status.Cleaned)
}

func TestUsageLogDetailRetentionRunOnceOnlyCleansYesterdayWindow(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	windowStart, windowEnd := usageLogDetailRetentionYesterdayWindow(time.Now().In(loc), loc)
	olderPayload := `{"messages":[{"role":"user","content":"older"}]}`
	yesterdayPayload := `{"messages":[{"role":"user","content":"yesterday"}]}`
	todayPayload := `{"messages":[{"role":"user","content":"today"}]}`
	older := windowStart.Add(-time.Second)
	yesterday := windowStart.Add(12 * time.Hour)
	today := windowEnd
	repo := &usageLogDetailRepoStub{
		cleanupDetails: []UsageLogDetail{
			{UsageLogID: 1, RequestPayloadJSON: &olderPayload, CreatedAt: older},
			{UsageLogID: 2, RequestPayloadJSON: &yesterdayPayload, CreatedAt: yesterday},
			{UsageLogID: 3, RequestPayloadJSON: &todayPayload, CreatedAt: today},
		},
		detailByLogID: map[int64]*UsageLogDetail{
			1: {UsageLogID: 1, RequestPayloadJSON: &olderPayload, CreatedAt: older},
			2: {UsageLogID: 2, RequestPayloadJSON: &yesterdayPayload, CreatedAt: yesterday},
			3: {UsageLogID: 3, RequestPayloadJSON: &todayPayload, CreatedAt: today},
		},
	}
	svc := &UsageLogDetailRetentionService{
		repo:   repo,
		loc:    loc,
		stopCh: make(chan struct{}),
	}

	svc.runOnce()

	require.Equal(t, 1, repo.clearFullPayloadCalls)
	olderDetail, err := repo.GetByUsageLogID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, olderDetail.RequestPayloadJSON)
	yesterdayDetail, err := repo.GetByUsageLogID(context.Background(), 2)
	require.NoError(t, err)
	require.Nil(t, yesterdayDetail.RequestPayloadJSON)
	todayDetail, err := repo.GetByUsageLogID(context.Background(), 3)
	require.NoError(t, err)
	require.NotNil(t, todayDetail.RequestPayloadJSON)

	status := svc.Snapshot()
	require.NotNil(t, status.WindowStart)
	require.NotNil(t, status.WindowEnd)
	require.Equal(t, windowStart, *status.WindowStart)
	require.Equal(t, windowEnd, *status.WindowEnd)
}
