package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterQuickMonitorRoutesReadOnlySubset(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	RegisterQuickMonitorRoutes(router.Group("/api/v1"), &handler.Handlers{
		Admin: &handler.AdminHandlers{
			Dashboard:  adminhandler.NewDashboardHandler(nil, nil),
			UsageBrief: adminhandler.NewUsageBriefHandler(nil),
		},
	}, nil)
	require.Empty(t, router.Routes(), "nil setting service should not register quick monitor routes")

	router = gin.New()
	registerQuickMonitorDashboardRoutes(router.Group("/api/v1/quick-monitor/:suffix"), &handler.Handlers{
		Admin: &handler.AdminHandlers{
			Dashboard: adminhandler.NewDashboardHandler(nil, nil),
		},
	})
	registerQuickMonitorOpsRoutes(router.Group("/api/v1/quick-monitor/:suffix"), &handler.Handlers{
		Admin: &handler.AdminHandlers{
			Ops: adminhandler.NewOpsHandler(nil),
		},
	})
	registerQuickMonitorUsageBriefRoutes(router.Group("/api/v1/quick-monitor/:suffix"), &handler.Handlers{
		Admin: &handler.AdminHandlers{
			UsageBrief: adminhandler.NewUsageBriefHandler(nil),
		},
	})

	foundDashboardStats := false
	foundOpsErrors := false
	foundOpsRequests := false
	forbiddenWrites := map[string]bool{
		"POST /api/v1/quick-monitor/:suffix/dashboard/aggregation/backfill": false,
		"PUT /api/v1/quick-monitor/:suffix/usage-brief/settings":            false,
		"POST /api/v1/quick-monitor/:suffix/usage-brief/jobs/production":    false,
		"DELETE /api/v1/quick-monitor/:suffix/usage-brief/report-groups":    false,
	}
	for _, route := range router.Routes() {
		if route.Method == "GET" && route.Path == "/api/v1/quick-monitor/:suffix/dashboard/stats" {
			foundDashboardStats = true
		}
		if route.Method == "GET" && route.Path == "/api/v1/quick-monitor/:suffix/ops/errors" {
			foundOpsErrors = true
		}
		if route.Method == "GET" && route.Path == "/api/v1/quick-monitor/:suffix/ops/requests" {
			foundOpsRequests = true
		}
		key := route.Method + " " + route.Path
		if _, forbidden := forbiddenWrites[key]; forbidden {
			forbiddenWrites[key] = true
		}
	}

	require.True(t, foundDashboardStats)
	require.True(t, foundOpsErrors)
	require.False(t, foundOpsRequests, "quick monitor must not expose recent request details")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/quick-monitor/demo/ops/requests", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	require.Equal(t, http.StatusNotFound, resp.Code)
	for route, found := range forbiddenWrites {
		require.False(t, found, "quick monitor must not register write route %s", route)
	}
}
