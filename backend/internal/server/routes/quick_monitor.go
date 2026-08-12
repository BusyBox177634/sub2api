package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func RegisterQuickMonitorRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	settingService *service.SettingService,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	if h == nil || h.Admin == nil || settingService == nil {
		return
	}

	quick := v1.Group("/quick-monitor/:suffix")
	quick.Use(panelRateLimiter.PublicIP())
	quick.Use(quickMonitorSuffixGuard(settingService))
	{
		quick.GET("/status", func(c *gin.Context) {
			response.Success(c, gin.H{"enabled": true})
		})
		registerQuickMonitorDashboardRoutes(quick, h)
		registerQuickMonitorOpsRoutes(quick, h)
		registerQuickMonitorUserRoutes(quick, h)
		registerQuickMonitorAccountRoutes(quick, h)
		registerQuickMonitorGroupRoutes(quick, h)
		registerQuickMonitorSubscriptionRoutes(quick, h)
		registerQuickMonitorUsageRoutes(quick, h)
		registerQuickMonitorUsageBriefRoutes(quick, h)
	}
}

func quickMonitorSuffixGuard(settingService *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !settingService.IsQuickOpsMonitorSuffixAllowed(c.Request.Context(), c.Param("suffix")) {
			response.NotFound(c, "Quick monitor is not available")
			c.Abort()
			return
		}
		c.Set("quick_monitor_readonly", true)
		c.Next()
	}
}

func registerQuickMonitorDashboardRoutes(group *gin.RouterGroup, h *handler.Handlers) {
	dashboard := group.Group("/dashboard")
	{
		dashboard.GET("/snapshot-v2", h.Admin.Dashboard.GetSnapshotV2)
		dashboard.GET("/stats", h.Admin.Dashboard.GetStats)
		dashboard.GET("/realtime", h.Admin.Dashboard.GetRealtimeMetrics)
		dashboard.GET("/trend", h.Admin.Dashboard.GetUsageTrend)
		dashboard.GET("/models", h.Admin.Dashboard.GetModelStats)
		dashboard.GET("/groups", h.Admin.Dashboard.GetGroupStats)
		dashboard.GET("/api-keys-trend", h.Admin.Dashboard.GetAPIKeyUsageTrend)
		dashboard.GET("/users-trend", h.Admin.Dashboard.GetUserUsageTrend)
		dashboard.GET("/users-ranking", h.Admin.Dashboard.GetUserSpendingRanking)
		dashboard.POST("/users-usage", h.Admin.Dashboard.GetBatchUsersUsage)
		dashboard.POST("/api-keys-usage", h.Admin.Dashboard.GetBatchAPIKeysUsage)
		dashboard.GET("/user-breakdown", h.Admin.Dashboard.GetUserBreakdown)
	}
}

func registerQuickMonitorOpsRoutes(group *gin.RouterGroup, h *handler.Handlers) {
	ops := group.Group("/ops")
	{
		ops.GET("/concurrency", h.Admin.Ops.GetConcurrencyStats)
		ops.GET("/user-concurrency", h.Admin.Ops.GetUserConcurrencyStats)
		ops.GET("/account-availability", h.Admin.Ops.GetAccountAvailability)
		ops.GET("/realtime-traffic", h.Admin.Ops.GetRealtimeTrafficSummary)
		ops.GET("/alert-rules", h.Admin.Ops.ListAlertRules)
		ops.GET("/alert-events", h.Admin.Ops.ListAlertEvents)
		ops.GET("/alert-events/:id", h.Admin.Ops.GetAlertEvent)
		ops.GET("/email-notification/config", h.Admin.Ops.GetEmailNotificationConfig)
		ops.GET("/runtime/alert", h.Admin.Ops.GetAlertRuntimeSettings)
		ops.GET("/runtime/logging", h.Admin.Ops.GetRuntimeLogConfig)
		ops.GET("/advanced-settings", h.Admin.Ops.GetAdvancedSettings)
		ops.GET("/settings/metric-thresholds", h.Admin.Ops.GetMetricThresholds)
		ops.GET("/ws/qps", h.Admin.Ops.QPSWSHandler)
		ops.GET("/errors", h.Admin.Ops.GetErrorLogs)
		ops.GET("/errors/:id", h.Admin.Ops.GetErrorLogByID)
		ops.GET("/request-errors", h.Admin.Ops.ListRequestErrors)
		ops.GET("/request-errors/:id", h.Admin.Ops.GetRequestError)
		ops.GET("/request-errors/:id/upstream-errors", h.Admin.Ops.ListRequestErrorUpstreamErrors)
		ops.GET("/upstream-errors", h.Admin.Ops.ListUpstreamErrors)
		ops.GET("/upstream-errors/:id", h.Admin.Ops.GetUpstreamError)
		ops.GET("/system-logs", h.Admin.Ops.ListSystemLogs)
		ops.GET("/system-logs/health", h.Admin.Ops.GetSystemLogIngestionHealth)
		ops.GET("/dashboard/snapshot-v2", h.Admin.Ops.GetDashboardSnapshotV2)
		ops.GET("/dashboard/overview", h.Admin.Ops.GetDashboardOverview)
		ops.GET("/dashboard/throughput-trend", h.Admin.Ops.GetDashboardThroughputTrend)
		ops.GET("/dashboard/latency-histogram", h.Admin.Ops.GetDashboardLatencyHistogram)
		ops.GET("/dashboard/error-trend", h.Admin.Ops.GetDashboardErrorTrend)
		ops.GET("/dashboard/error-distribution", h.Admin.Ops.GetDashboardErrorDistribution)
		ops.GET("/dashboard/openai-token-stats", h.Admin.Ops.GetDashboardOpenAITokenStats)
	}
}

func registerQuickMonitorUserRoutes(group *gin.RouterGroup, h *handler.Handlers) {
	users := group.Group("/users")
	{
		users.GET("", h.Admin.User.List)
		users.GET("/:id", h.Admin.User.GetByID)
		users.GET("/:id/api-keys", h.Admin.User.GetUserAPIKeys)
		users.GET("/:id/usage", h.Admin.User.GetUserUsage)
		users.GET("/:id/balance-history", h.Admin.User.GetBalanceHistory)
		users.GET("/:id/rpm-status", h.Admin.User.GetUserRPMStatus)
		users.GET("/:id/platform-quotas", h.Admin.User.GetUserPlatformQuotas)
		users.GET("/:id/attributes", h.Admin.UserAttribute.GetUserAttributes)
		users.GET("/:id/subscriptions", h.Admin.Subscription.ListByUser)
	}
}

func registerQuickMonitorAccountRoutes(group *gin.RouterGroup, h *handler.Handlers) {
	accounts := group.Group("/accounts")
	{
		accounts.GET("", h.Admin.Account.List)
		accounts.GET("/:id", h.Admin.Account.GetByID)
	}
}

func registerQuickMonitorGroupRoutes(group *gin.RouterGroup, h *handler.Handlers) {
	groups := group.Group("/groups")
	{
		groups.GET("", h.Admin.Group.List)
		groups.GET("/all", h.Admin.Group.GetAll)
		groups.GET("/usage-summary", h.Admin.Group.GetUsageSummary)
		groups.GET("/capacity-summary", h.Admin.Group.GetCapacitySummary)
		groups.GET("/:id/models-list-candidates", h.Admin.Group.GetModelsListCandidates)
		groups.GET("/:id", h.Admin.Group.GetByID)
		groups.GET("/:id/stats", h.Admin.Group.GetStats)
		groups.GET("/:id/rate-multipliers", h.Admin.Group.GetGroupRateMultipliers)
		groups.GET("/:id/api-keys", h.Admin.Group.GetGroupAPIKeys)
		groups.GET("/:id/subscriptions", h.Admin.Subscription.ListByGroup)
	}
}

func registerQuickMonitorSubscriptionRoutes(group *gin.RouterGroup, h *handler.Handlers) {
	subscriptions := group.Group("/subscriptions")
	{
		subscriptions.GET("", h.Admin.Subscription.List)
		subscriptions.GET("/:id", h.Admin.Subscription.GetByID)
		subscriptions.GET("/:id/progress", h.Admin.Subscription.GetProgress)
	}
}

func registerQuickMonitorUsageRoutes(group *gin.RouterGroup, h *handler.Handlers) {
	usage := group.Group("/usage")
	{
		usage.GET("", h.Admin.Usage.List)
		usage.GET("/stats", h.Admin.Usage.Stats)
		usage.GET("/search-users", h.Admin.Usage.SearchUsers)
		usage.GET("/search-api-keys", h.Admin.Usage.SearchAPIKeys)
		usage.GET("/:id/detail", h.Admin.Usage.GetDetail)
	}
}

func registerQuickMonitorUsageBriefRoutes(group *gin.RouterGroup, h *handler.Handlers) {
	brief := group.Group("/usage-brief")
	{
		brief.GET("/report-groups", h.Admin.UsageBrief.ListReportGroups)
		brief.GET("/reports", h.Admin.UsageBrief.ListReports)
		brief.GET("/reports/:id", h.Admin.UsageBrief.GetReport)
	}
}
