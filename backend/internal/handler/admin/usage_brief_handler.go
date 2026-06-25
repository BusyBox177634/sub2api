package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type UsageBriefHandler struct {
	service *service.UsageBriefService
}

func NewUsageBriefHandler(service *service.UsageBriefService) *UsageBriefHandler {
	return &UsageBriefHandler{service: service}
}

func (h *UsageBriefHandler) GetSettings(c *gin.Context) {
	settings, err := h.service.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *UsageBriefHandler) UpdateSettings(c *gin.Context) {
	var req service.UpdateUsageBriefSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	settings, err := h.service.UpdateSettings(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *UsageBriefHandler) ListReports(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := service.UsageBriefReportFilter{
		PeriodType:  strings.TrimSpace(c.Query("period_type")),
		SourceKind:  strings.TrimSpace(c.Query("source_kind")),
		Status:      strings.TrimSpace(c.Query("status")),
		Search:      strings.TrimSpace(c.Query("search")),
		SearchScope: strings.TrimSpace(c.Query("search_scope")),
		Page:        page,
		PageSize:    pageSize,
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &userID
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

func (h *UsageBriefHandler) ListReportGroups(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter, ok := h.usageBriefReportGroupFilter(c, page, pageSize)
	if !ok {
		return
	}
	groups, total, err := h.service.ListReportGroups(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, groups, total, page, pageSize)
}

func (h *UsageBriefHandler) DeleteReportGroup(c *gin.Context) {
	groupKey := strings.TrimSpace(c.Query("group_key"))
	if groupKey == "" {
		response.BadRequest(c, "group_key is required")
		return
	}
	filter, ok := h.usageBriefReportGroupFilter(c, 1, 1)
	if !ok {
		return
	}
	deleted, err := h.service.DeleteReportGroup(c.Request.Context(), filter, groupKey)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted_count": deleted})
}

func (h *UsageBriefHandler) usageBriefReportGroupFilter(c *gin.Context, page, pageSize int) (service.UsageBriefReportGroupFilter, bool) {
	filter := service.UsageBriefReportGroupFilter{
		UsageBriefReportFilter: service.UsageBriefReportFilter{
			PeriodType:  strings.TrimSpace(c.Query("period_type")),
			SourceKind:  strings.TrimSpace(c.Query("source_kind")),
			Status:      strings.TrimSpace(c.Query("status")),
			Search:      strings.TrimSpace(c.Query("search")),
			SearchScope: strings.TrimSpace(c.Query("search_scope")),
			Page:        page,
			PageSize:    pageSize,
		},
		GroupBy: strings.TrimSpace(c.Query("group_by")),
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid user_id")
			return filter, false
		}
		filter.UserID = &userID
	}
	if !applyUsageBriefReportDateRange(c, &filter.UsageBriefReportFilter) {
		return filter, false
	}
	return filter, true
}

func (h *UsageBriefHandler) GetReport(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	report, err := h.service.GetReport(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, report)
}

type updateUsageBriefReportRequest struct {
	Title     string `json:"title"`
	ContentMD string `json:"content_md"`
}

func (h *UsageBriefHandler) UpdateReport(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	var req updateUsageBriefReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	report, err := h.service.UpdateReportByAdmin(c.Request.Context(), id, req.Title, req.ContentMD, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, report)
}

func (h *UsageBriefHandler) DeleteReport(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteReport(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

func (h *UsageBriefHandler) ListBatches(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := service.UsageBriefBatchFilter{
		Scope:    strings.TrimSpace(c.Query("scope")),
		Status:   strings.TrimSpace(c.Query("status")),
		Page:     page,
		PageSize: pageSize,
	}
	batches, total, err := h.service.ListBatches(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, batches, total, page, pageSize)
}

func (h *UsageBriefHandler) ListBatchJobs(c *gin.Context) {
	batchID, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	filter := service.UsageBriefJobFilter{
		Status:   strings.TrimSpace(c.Query("status")),
		Page:     page,
		PageSize: pageSize,
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &userID
	}
	jobs, total, err := h.service.GetBatchJobs(c.Request.Context(), batchID, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, jobs, total, page, pageSize)
}

func (h *UsageBriefHandler) ListJobs(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := service.UsageBriefJobFilter{
		Scope:    strings.TrimSpace(c.Query("scope")),
		Status:   strings.TrimSpace(c.Query("status")),
		Page:     page,
		PageSize: pageSize,
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &userID
	}
	if raw := strings.TrimSpace(c.Query("batch_id")); raw != "" {
		batchID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid batch_id")
			return
		}
		filter.BatchID = &batchID
	}
	jobs, total, err := h.service.ListJobs(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, jobs, total, page, pageSize)
}

type triggerUsageBriefProductionRequest struct {
	PeriodType string `json:"period_type"`
	PeriodDate string `json:"period_date"`
}

func (h *UsageBriefHandler) TriggerProduction(c *gin.Context) {
	var req triggerUsageBriefProductionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	periodDate, err := parseUsageBriefDateValue(req.PeriodDate)
	if err != nil {
		response.BadRequest(c, "period_date must be YYYY-MM-DD")
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	batch, jobs, err := h.service.TriggerProduction(c.Request.Context(), service.UsageBriefTriggerProductionRequest{
		PeriodType:  strings.TrimSpace(req.PeriodType),
		PeriodDate:  periodDate,
		CreatedBy:   subject.UserID,
		TriggerKind: service.UsageBriefTriggerManual,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"batch": batch, "items": jobs, "count": len(jobs)})
}

type createUsageBriefTestJobRequest struct {
	UserID     int64  `json:"user_id"`
	GroupID    *int64 `json:"group_id"`
	RangeStart string `json:"range_start"`
	RangeEnd   string `json:"range_end"`
}

func (h *UsageBriefHandler) CreateTestJob(c *gin.Context) {
	var req createUsageBriefTestJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	start, err := parseUsageBriefTimeValue(req.RangeStart)
	if err != nil {
		response.BadRequest(c, "range_start must be RFC3339 or YYYY-MM-DD")
		return
	}
	end, err := parseUsageBriefTimeValue(req.RangeEnd)
	if err != nil {
		response.BadRequest(c, "range_end must be RFC3339 or YYYY-MM-DD")
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	batch, job, err := h.service.CreateTestJob(c.Request.Context(), service.UsageBriefCreateTestJobRequest{
		UserID:     req.UserID,
		GroupID:    req.GroupID,
		RangeStart: start,
		RangeEnd:   end,
		CreatedBy:  subject.UserID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"batch": batch, "job": job})
}

func (h *UsageBriefHandler) PauseBatch(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	batch, err := h.service.PauseBatch(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, batch)
}

func (h *UsageBriefHandler) ResumeBatch(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	batch, err := h.service.ResumeBatch(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, batch)
}

func (h *UsageBriefHandler) CancelBatch(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	batch, err := h.service.CancelBatch(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, batch)
}

func (h *UsageBriefHandler) ResetBatch(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	batch, err := h.service.ResetBatch(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, batch)
}

func (h *UsageBriefHandler) RerunBatch(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	batch, err := h.service.RerunBatch(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, batch)
}

func (h *UsageBriefHandler) DeleteBatch(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteBatch(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

func (h *UsageBriefHandler) CancelJob(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	job, err := h.service.CancelJob(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, job)
}

func (h *UsageBriefHandler) ResetJob(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	job, err := h.service.ResetJob(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, job)
}

func (h *UsageBriefHandler) RerunJob(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	job, err := h.service.RerunJob(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, job)
}

func (h *UsageBriefHandler) ListJobChunks(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	if strings.TrimSpace(c.Query("page_size")) == "" && strings.TrimSpace(c.Query("limit")) == "" {
		pageSize = 50
	}
	filter := service.UsageBriefJobChunkFilter{
		ChunkType: strings.TrimSpace(c.Query("chunk_type")),
		Page:      page,
		PageSize:  pageSize,
	}
	chunks, total, err := h.service.ListJobChunks(c.Request.Context(), id, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, chunks, total, page, pageSize)
}

func (h *UsageBriefHandler) ListJobConversations(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	if strings.TrimSpace(c.Query("page_size")) == "" && strings.TrimSpace(c.Query("limit")) == "" {
		pageSize = 50
	}
	filter := service.UsageBriefJobConversationFilter{
		Page:     page,
		PageSize: pageSize,
	}
	conversations, total, err := h.service.ListJobConversations(c.Request.Context(), id, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, conversations, total, page, pageSize)
}

func (h *UsageBriefHandler) DeleteJob(c *gin.Context) {
	id, ok := parseUsageBriefIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteJob(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"id": id})
}

func parseUsageBriefIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid "+name)
		return 0, false
	}
	return id, true
}

func parseUsageBriefDateValue(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now(), nil
	}
	return time.Parse("2006-01-02", raw)
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

func parseUsageBriefTimeValue(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, strconv.ErrSyntax
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
