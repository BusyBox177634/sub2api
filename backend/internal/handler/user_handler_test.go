package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type userHandlerRepoStub struct {
	user         *service.User
	getByIDCalls int
	updateCalls  int
}

func (s *userHandlerRepoStub) Create(context.Context, *service.User) error { return nil }
func (s *userHandlerRepoStub) GetByID(context.Context, int64) (*service.User, error) {
	s.getByIDCalls++
	if s.user != nil {
		return s.user, nil
	}
	return &service.User{}, nil
}
func (s *userHandlerRepoStub) GetByEmail(context.Context, string) (*service.User, error) {
	return &service.User{}, nil
}
func (s *userHandlerRepoStub) GetFirstAdmin(context.Context) (*service.User, error) {
	return &service.User{}, nil
}
func (s *userHandlerRepoStub) Update(context.Context, *service.User) error {
	s.updateCalls++
	return nil
}
func (s *userHandlerRepoStub) Delete(context.Context, int64) error { return nil }
func (s *userHandlerRepoStub) List(context.Context, pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *userHandlerRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *userHandlerRepoStub) UpdateBalance(context.Context, int64, float64) error { return nil }
func (s *userHandlerRepoStub) DeductBalance(context.Context, int64, float64) error { return nil }
func (s *userHandlerRepoStub) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (s *userHandlerRepoStub) ExistsByEmail(context.Context, string) (bool, error) { return false, nil }
func (s *userHandlerRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (s *userHandlerRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (s *userHandlerRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (s *userHandlerRepoStub) UpdateTotpSecret(context.Context, int64, *string) error { return nil }
func (s *userHandlerRepoStub) EnableTotp(context.Context, int64) error                { return nil }
func (s *userHandlerRepoStub) DisableTotp(context.Context, int64) error               { return nil }

func TestUserHandlerUpdateProfileRejectsUsernameChange(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &userHandlerRepoStub{
		user: &service.User{ID: 42, Username: "current-name"},
	}
	handler := NewUserHandler(service.NewUserService(repo, nil, nil))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	})
	router.PUT("/api/v1/user", handler.UpdateProfile)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/user", bytes.NewBufferString(`{"username":"new-name"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Zero(t, repo.getByIDCalls)
	require.Zero(t, repo.updateCalls)

	var resp response.Response
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, http.StatusForbidden, resp.Code)
	require.Equal(t, "USERNAME_CHANGE_RESTRICTED", resp.Reason)
	require.Equal(t, "username can only be changed by an administrator", resp.Message)
}
