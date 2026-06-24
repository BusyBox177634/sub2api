package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type UsageBriefHandler struct {
	service *service.UsageBriefService
}

func NewUsageBriefHandler(service *service.UsageBriefService) *UsageBriefHandler {
	return &UsageBriefHandler{service: service}
}

func (h *UsageBriefHandler) ListReports(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	filter := service.UsageBriefReportFilter{
		UserID:     &subject.UserID,
		PeriodType: strings.TrimSpace(c.Query("period_type")),
		SourceKind: service.UsageBriefJobScopeProduction,
		Status:     strings.TrimSpace(c.Query("status")),
		Page:       page,
		PageSize:   pageSize,
	}
	if !applyUsageBriefReportDateRange(c, &filter) {
		return
	}
	reports, total, err := h.service.ListReports(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, reports, total, page, pageSize)
}

func (h *UsageBriefHandler) GetReport(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid report id")
		return
	}
	report, err := h.service.GetUserReport(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, report)
}

func (h *UsageBriefHandler) GetPeriod(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	periodType := strings.TrimSpace(c.Query("period_type"))
	if periodType == "" {
		periodType = service.UsageBriefPeriodDaily
	}
	date, ok := parseUsageBriefDateQuery(c, "date")
	if !ok {
		return
	}
	view, err := h.service.GetUserPeriodView(c.Request.Context(), subject.UserID, periodType, date)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

func parseUsageBriefDateQuery(c *gin.Context, key string) (time.Time, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return time.Now(), true
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t, true
	}
	response.BadRequest(c, key+" must be YYYY-MM-DD")
	return time.Time{}, false
}

func applyUsageBriefReportDateRange(c *gin.Context, filter *service.UsageBriefReportFilter) bool {
	start, ok := parseOptionalUsageBriefDateQuery(c, "start_date")
	if !ok {
		return false
	}
	end, ok := parseOptionalUsageBriefDateQuery(c, "end_date")
	if !ok {
		return false
	}
	if start != nil {
		filter.StartDate = start
	}
	if end != nil {
		filter.EndDate = end
	}
	if filter.StartDate != nil && filter.EndDate != nil && filter.StartDate.After(*filter.EndDate) {
		response.BadRequest(c, "start_date must be before or equal to end_date")
		return false
	}
	return true
}

func parseOptionalUsageBriefDateQuery(c *gin.Context, key string) (*time.Time, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		response.BadRequest(c, key+" must be YYYY-MM-DD")
		return nil, false
	}
	return &t, true
}
