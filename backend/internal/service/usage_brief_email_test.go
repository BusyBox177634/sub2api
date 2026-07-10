//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
)

type usageBriefAutoEmailRepoStub struct {
	UsageBriefRepository
	enabled bool
	err     error
	calls   int
	userID  int64
}

func (r *usageBriefAutoEmailRepoStub) IsUserUsageBriefAutoEmailEnabled(_ context.Context, userID int64) (bool, error) {
	r.calls++
	r.userID = userID
	return r.enabled, r.err
}

func TestShouldAutoSendProductionJobEmailRequiresUserPreference(t *testing.T) {
	ctx := context.Background()
	userID := int64(42)
	job := UsageBriefJob{ID: 7, JobScope: UsageBriefJobScopeProduction, Status: UsageBriefStatusSucceeded, UserID: &userID}

	repo := &usageBriefAutoEmailRepoStub{enabled: false}
	svc := NewUsageBriefService(repo, nil, nil)
	if svc.shouldAutoSendProductionJobEmail(ctx, job) {
		t.Fatalf("auto send should be disabled by default")
	}
	if repo.calls != 1 || repo.userID != userID {
		t.Fatalf("preference lookup calls = %d userID = %d", repo.calls, repo.userID)
	}

	repo = &usageBriefAutoEmailRepoStub{enabled: true}
	svc = NewUsageBriefService(repo, nil, nil)
	if !svc.shouldAutoSendProductionJobEmail(ctx, job) {
		t.Fatalf("auto send should be enabled when user preference is true")
	}
}

func TestShouldAutoSendProductionJobEmailSkipsInvalidJobsAndPreferenceErrors(t *testing.T) {
	ctx := context.Background()
	userID := int64(42)
	repo := &usageBriefAutoEmailRepoStub{enabled: true}
	svc := NewUsageBriefService(repo, nil, nil)

	if svc.shouldAutoSendProductionJobEmail(ctx, UsageBriefJob{JobScope: UsageBriefJobScopeTest, Status: UsageBriefStatusSucceeded, UserID: &userID}) {
		t.Fatalf("test jobs must not auto send")
	}
	if repo.calls != 0 {
		t.Fatalf("preference should not be loaded for test jobs")
	}
	if svc.shouldAutoSendProductionJobEmail(ctx, UsageBriefJob{JobScope: UsageBriefJobScopeProduction, Status: UsageBriefStatusRunning, UserID: &userID}) {
		t.Fatalf("incomplete jobs must not auto send")
	}
	if svc.shouldAutoSendProductionJobEmail(ctx, UsageBriefJob{JobScope: UsageBriefJobScopeProduction, Status: UsageBriefStatusSucceeded}) {
		t.Fatalf("jobs without user must not auto send")
	}

	repo = &usageBriefAutoEmailRepoStub{enabled: true, err: errors.New("database unavailable")}
	svc = NewUsageBriefService(repo, nil, nil)
	if svc.shouldAutoSendProductionJobEmail(ctx, UsageBriefJob{ID: 8, JobScope: UsageBriefJobScopeProduction, Status: UsageBriefStatusPartial, UserID: &userID}) {
		t.Fatalf("preference load errors should skip auto send")
	}
}
