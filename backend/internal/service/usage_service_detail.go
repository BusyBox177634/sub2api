package service

import (
	"context"
	"fmt"
	"strings"
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
		if detail != nil && ensureUsageLogDetailCompressedPayloads(detail) {
			if err := s.usageDetailRepo.UpdateCompressedPayloads(ctx, detail.UsageLogID, detail.CompressedRequestPayloadJSON, detail.CompressedResponsePayloadJSON); err != nil {
				return nil, fmt.Errorf("update usage log detail compressed payloads: %w", err)
			}
		}
	}

	return BuildUsageLogDetailView(settingsEnabled, log, detail), nil
}

func ensureUsageLogDetailCompressedPayloads(detail *UsageLogDetail) bool {
	if detail == nil {
		return false
	}
	changed := false
	if strings.TrimSpace(derefString(detail.CompressedRequestPayloadJSON)) == "" && strings.TrimSpace(derefString(detail.RequestPayloadJSON)) != "" {
		detail.CompressedRequestPayloadJSON = CompressUsageLogPayloadJSON(detail.RequestPayloadJSON, UsageLogPayloadKindRequest)
		changed = true
	}
	if strings.TrimSpace(derefString(detail.CompressedResponsePayloadJSON)) == "" && strings.TrimSpace(derefString(detail.ResponsePayloadJSON)) != "" {
		detail.CompressedResponsePayloadJSON = CompressUsageLogPayloadJSON(detail.ResponsePayloadJSON, UsageLogPayloadKindResponse)
		changed = true
	}
	return changed
}
