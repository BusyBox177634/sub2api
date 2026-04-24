//go:build unit

package service

import "context"

type usageLogDetailRepoStub struct {
	upsertCalls   int
	lastRequestID string
	lastAPIKeyID  int64
	lastDetail    *UsageLogDetail
	upsertErr     error
	detailByLogID map[int64]*UsageLogDetail
	getByUsageErr error
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
