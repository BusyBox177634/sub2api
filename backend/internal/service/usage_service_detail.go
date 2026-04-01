package service

import (
	"context"
	"fmt"
)

func (s *UsageService) GetDetailByUsageLog(ctx context.Context, log *UsageLog) (*UsageLogDetailView, error) {
	if log == nil {
		return nil, fmt.Errorf("usage log is nil")
	}

	var settingsEnabled bool
	if s != nil && s.settingService != nil {
		settingsEnabled = s.settingService.IsUsageMessageRetentionEnabled(ctx)
	}

	var detail *UsageLogDetail
	var err error
	if s != nil && s.usageDetailRepo != nil {
		detail, err = s.usageDetailRepo.GetByUsageLogID(ctx, log.ID)
		if err != nil {
			return nil, fmt.Errorf("get usage log detail: %w", err)
		}
	}

	return BuildUsageLogDetailView(settingsEnabled, log, detail), nil
}
