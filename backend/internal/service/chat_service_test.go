//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type chatAPIKeyRepoStub struct {
	authRepoStub
	listByUserID func(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error)
}

func (s *chatAPIKeyRepoStub) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	if s.listByUserID == nil {
		panic("unexpected ListByUserID call")
	}
	return s.listByUserID(ctx, userID, params, filters)
}

func TestChatServiceListAvailableAPIKeys_AllowsNilUserForEligibleKeys(t *testing.T) {
	repo := &chatAPIKeyRepoStub{
		listByUserID: func(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{
				{
					ID:     1,
					UserID: userID,
					Status: StatusAPIKeyActive,
					Group:  &Group{ID: 11, Platform: PlatformOpenAI},
				},
				{
					ID:     2,
					UserID: userID,
					Status: StatusAPIKeyActive,
					Group:  &Group{ID: 12, Platform: PlatformAnthropic},
				},
			}, &pagination.PaginationResult{}, nil
		},
	}

	svc := NewChatService(nil, &APIKeyService{apiKeyRepo: repo}, nil, nil)
	keys, err := svc.ListAvailableAPIKeys(context.Background(), 99)

	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.Equal(t, int64(1), keys[0].ID)
	require.Equal(t, int64(2), keys[1].ID)
}

func TestChatServiceListAvailableAPIKeys_StillFiltersIneligibleKeys(t *testing.T) {
	expiredAt := time.Now().Add(-time.Minute)
	repo := &chatAPIKeyRepoStub{
		listByUserID: func(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
			return []APIKey{
				{
					ID:     1,
					UserID: userID,
					Status: StatusAPIKeyActive,
					Group:  &Group{ID: 11, Platform: PlatformOpenAI},
				},
				{
					ID:     2,
					UserID: userID,
					Status: StatusAPIKeyDisabled,
					Group:  &Group{ID: 12, Platform: PlatformOpenAI},
				},
				{
					ID:        3,
					UserID:    userID,
					Status:    StatusAPIKeyActive,
					Group:     &Group{ID: 13, Platform: PlatformOpenAI},
					ExpiresAt: &expiredAt,
				},
				{
					ID:        4,
					UserID:    userID,
					Status:    StatusAPIKeyActive,
					Group:     &Group{ID: 14, Platform: PlatformOpenAI},
					Quota:     1,
					QuotaUsed: 1,
				},
				{
					ID:     5,
					UserID: userID,
					Status: StatusAPIKeyActive,
					Group:  &Group{ID: 15, Platform: PlatformGemini},
				},
			}, &pagination.PaginationResult{}, nil
		},
	}

	svc := NewChatService(nil, &APIKeyService{apiKeyRepo: repo}, nil, nil)
	keys, err := svc.ListAvailableAPIKeys(context.Background(), 99)

	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, int64(1), keys[0].ID)
}
