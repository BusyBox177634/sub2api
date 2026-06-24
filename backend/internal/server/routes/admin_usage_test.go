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

func TestRegisterUsageBriefRoutesRegistersQueueAndReportGroupRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	registerUsageBriefRoutes(router.Group("/admin"), &handler.Handlers{
		Admin: &handler.AdminHandlers{
			UsageBrief: adminhandler.NewUsageBriefHandler(nil),
		},
	})

	foundReportGroups := false
	foundDeleteReportGroups := false
	foundDeleteJob := false
	foundRerunBatch := false
	foundRerunJob := false
	foundJobChunks := false
	foundJobConversations := false
	for _, route := range router.Routes() {
		if route.Method == http.MethodDelete && route.Path == "/admin/usage-brief/jobs/:id" {
			foundDeleteJob = true
		}
		if route.Method == http.MethodGet && route.Path == "/admin/usage-brief/report-groups" {
			foundReportGroups = true
		}
		if route.Method == http.MethodDelete && route.Path == "/admin/usage-brief/report-groups" {
			foundDeleteReportGroups = true
		}
		if route.Method == http.MethodPost && route.Path == "/admin/usage-brief/batches/:id/rerun" {
			foundRerunBatch = true
		}
		if route.Method == http.MethodPost && route.Path == "/admin/usage-brief/jobs/:id/rerun" {
			foundRerunJob = true
		}
		if route.Method == http.MethodGet && route.Path == "/admin/usage-brief/jobs/:id/chunks" {
			foundJobChunks = true
		}
		if route.Method == http.MethodGet && route.Path == "/admin/usage-brief/jobs/:id/conversations" {
			foundJobConversations = true
		}
	}
	require.True(t, foundDeleteJob, "DELETE /admin/usage-brief/jobs/:id route is not registered")
	require.True(t, foundReportGroups, "GET /admin/usage-brief/report-groups route is not registered")
	require.True(t, foundDeleteReportGroups, "DELETE /admin/usage-brief/report-groups route is not registered")
	require.True(t, foundRerunBatch, "POST /admin/usage-brief/batches/:id/rerun route is not registered")
	require.True(t, foundRerunJob, "POST /admin/usage-brief/jobs/:id/rerun route is not registered")
	require.True(t, foundJobChunks, "GET /admin/usage-brief/jobs/:id/chunks route is not registered")
	require.True(t, foundJobConversations, "GET /admin/usage-brief/jobs/:id/conversations route is not registered")
}
