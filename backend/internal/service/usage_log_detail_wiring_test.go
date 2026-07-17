package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type usageLogDetailWiringRepoStub struct{}

func (*usageLogDetailWiringRepoStub) UpsertByRequestAndAPIKey(context.Context, string, int64, *UsageLogDetail) error {
	return nil
}

func (*usageLogDetailWiringRepoStub) GetByUsageLogID(context.Context, int64) (*UsageLogDetail, error) {
	return nil, nil
}

func (*usageLogDetailWiringRepoStub) UpdateCompressedPayloads(context.Context, int64, *string, *string) error {
	return nil
}

func (*usageLogDetailWiringRepoStub) CountFullPayloadCleanupPending(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

func (*usageLogDetailWiringRepoStub) ListForFullPayloadCleanup(context.Context, time.Time, time.Time, int) ([]UsageLogDetail, error) {
	return nil, nil
}

func (*usageLogDetailWiringRepoStub) ClearFullPayloads(context.Context, int64, *string, *string, time.Time) error {
	return nil
}

func TestProvideUsageLogDetailWiring_AttachesReaderAndWriters(t *testing.T) {
	repo := &usageLogDetailWiringRepoStub{}
	settingService := &SettingService{}
	gatewayService := &GatewayService{}
	openAIGatewayService := &OpenAIGatewayService{}
	usageService := &UsageService{}

	wiring := ProvideUsageLogDetailWiring(
		gatewayService,
		openAIGatewayService,
		usageService,
		repo,
		settingService,
	)

	require.NotNil(t, wiring)
	require.Equal(t, repo, gatewayService.usageDetailRepo)
	require.Equal(t, repo, openAIGatewayService.usageDetailRepo)
	require.Equal(t, repo, usageService.usageDetailRepo)
	require.Same(t, settingService, usageService.settingService)
}
