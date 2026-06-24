package service

import (
	"context"
	"time"
)

type usageLogDetailRepoStub struct {
	upsertCalls           int
	updateCompressedCalls int
	clearFullPayloadCalls int
	lastRequestID         string
	lastAPIKeyID          int64
	lastDetail            *UsageLogDetail
	upsertErr             error
	updateCompressedErr   error
	clearFullPayloadErr   error
	detailByLogID         map[int64]*UsageLogDetail
	getByUsageErr         error
	cleanupDetails        []UsageLogDetail
	listCleanupErr        error
	now                   time.Time
}

func (s *usageLogDetailRepoStub) UpsertByRequestAndAPIKey(ctx context.Context, requestID string, apiKeyID int64, detail *UsageLogDetail) error {
	s.upsertCalls++
	s.lastRequestID = requestID
	s.lastAPIKeyID = apiKeyID
	if detail != nil {
		copied := *detail
		s.lastDetail = &copied
	} else {
		s.lastDetail = nil
	}
	return s.upsertErr
}

func (s *usageLogDetailRepoStub) GetByUsageLogID(ctx context.Context, usageLogID int64) (*UsageLogDetail, error) {
	if s.getByUsageErr != nil {
		return nil, s.getByUsageErr
	}
	if s.detailByLogID == nil {
		return nil, nil
	}
	detail, ok := s.detailByLogID[usageLogID]
	if !ok || detail == nil {
		return nil, nil
	}
	copied := *detail
	return &copied, nil
}

func (s *usageLogDetailRepoStub) UpdateCompressedPayloads(ctx context.Context, usageLogID int64, compressedRequestJSON *string, compressedResponseJSON *string) error {
	s.updateCompressedCalls++
	if s.detailByLogID != nil {
		if detail := s.detailByLogID[usageLogID]; detail != nil {
			detail.CompressedRequestPayloadJSON = cloneStringPtr(compressedRequestJSON)
			detail.CompressedResponsePayloadJSON = cloneStringPtr(compressedResponseJSON)
		}
	}
	return s.updateCompressedErr
}

func (s *usageLogDetailRepoStub) CountFullPayloadCleanupPending(ctx context.Context, start, end time.Time) (int64, error) {
	if s.listCleanupErr != nil {
		return 0, s.listCleanupErr
	}
	var count int64
	if s.detailByLogID != nil {
		for _, detail := range s.detailByLogID {
			if detail == nil {
				continue
			}
			if usageLogDetailRepoStubInWindow(*detail, start, end) && (detail.RequestPayloadJSON != nil || detail.ResponsePayloadJSON != nil) {
				count++
			}
		}
		return count, nil
	}
	for _, detail := range s.cleanupDetails {
		if usageLogDetailRepoStubInWindow(detail, start, end) && (detail.RequestPayloadJSON != nil || detail.ResponsePayloadJSON != nil) {
			count++
		}
	}
	return count, nil
}

func (s *usageLogDetailRepoStub) ListForFullPayloadCleanup(ctx context.Context, start, end time.Time, limit int) ([]UsageLogDetail, error) {
	if s.listCleanupErr != nil {
		return nil, s.listCleanupErr
	}
	if len(s.cleanupDetails) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = len(s.cleanupDetails)
	}
	out := make([]UsageLogDetail, 0, limit)
	remaining := make([]UsageLogDetail, 0, len(s.cleanupDetails))
	for _, detail := range s.cleanupDetails {
		if len(out) < limit && usageLogDetailRepoStubInWindow(detail, start, end) && (detail.RequestPayloadJSON != nil || detail.ResponsePayloadJSON != nil) {
			out = append(out, detail)
			continue
		}
		remaining = append(remaining, detail)
	}
	s.cleanupDetails = remaining
	return out, nil
}

func (s *usageLogDetailRepoStub) ClearFullPayloads(ctx context.Context, usageLogID int64, compressedRequestJSON *string, compressedResponseJSON *string, cleanedAt time.Time) error {
	s.clearFullPayloadCalls++
	if s.detailByLogID != nil {
		if detail := s.detailByLogID[usageLogID]; detail != nil {
			detail.RequestPayloadJSON = nil
			detail.ResponsePayloadJSON = nil
			detail.CompressedRequestPayloadJSON = cloneStringPtr(compressedRequestJSON)
			detail.CompressedResponsePayloadJSON = cloneStringPtr(compressedResponseJSON)
			detail.FullPayloadsCleanedAt = &cleanedAt
		}
	}
	return s.clearFullPayloadErr
}

func usageLogDetailRepoStubInWindow(detail UsageLogDetail, start, end time.Time) bool {
	if detail.CreatedAt.IsZero() {
		return true
	}
	return !detail.CreatedAt.Before(start) && detail.CreatedAt.Before(end)
}
