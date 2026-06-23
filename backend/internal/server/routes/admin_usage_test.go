package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminUsageRouteRepoStub struct {
	service.UsageLogRepository
}

func (s *adminUsageRouteRepoStub) GetByID(ctx context.Context, id int64) (*service.UsageLog, error) {
	return &service.UsageLog{
		ID:        id,
		UserID:    7,
		APIKeyID:  8,
		RequestID: "req-admin-route-detail",
	}, nil
}

func TestRegisterUsageRoutesRegistersAdminUsageDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	usageSvc := service.NewUsageService(&adminUsageRouteRepoStub{}, nil, nil, nil)
	router := gin.New()
	registerUsageRoutes(router.Group("/admin"), &handler.Handlers{
		Admin: &handler.AdminHandlers{
			Usage: adminhandler.NewUsageHandler(usageSvc, nil, nil, nil),
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/usage/14/detail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"available":false`)
}
