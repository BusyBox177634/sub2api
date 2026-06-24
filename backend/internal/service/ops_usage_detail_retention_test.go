package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type usageDetailRetentionStatusProviderStub struct {
	status UsageLogDetailRetentionStatus
}

func (s usageDetailRetentionStatusProviderStub) Snapshot() UsageLogDetailRetentionStatus {
	return s.status
}

func TestOpsDashboardOverviewAttachesUsageDetailRetentionStatus(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	end := time.Now()
	svc := NewOpsService(&opsRepoMock{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	svc.SetUsageLogDetailRetentionStatusProvider(usageDetailRetentionStatusProviderStub{
		status: UsageLogDetailRetentionStatus{
			Enabled:             true,
			Running:             true,
			Phase:               UsageLogDetailRetentionPhaseProcessing,
			TotalPendingAtStart: 100,
			RemainingPending:    25,
			Cleaned:             75,
			ProgressPercent:     75,
		},
	})

	overview, err := svc.GetDashboardOverview(context.Background(), &OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
	})

	require.NoError(t, err)
	require.NotNil(t, overview.UsageDetailRetention)
	require.True(t, overview.UsageDetailRetention.Running)
	require.Equal(t, UsageLogDetailRetentionPhaseProcessing, overview.UsageDetailRetention.Phase)
	require.EqualValues(t, 25, overview.UsageDetailRetention.RemainingPending)
	require.Equal(t, 75.0, overview.UsageDetailRetention.ProgressPercent)
}
