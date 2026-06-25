package service

import (
	"context"
	"time"
)

const (
	UsageBriefPeriodDaily   = "daily"
	UsageBriefPeriodWeekly  = "weekly"
	UsageBriefPeriodMonthly = "monthly"

	UsageBriefJobScopeProduction = "production"
	UsageBriefJobScopeTest       = "test"

	UsageBriefJobTypeDaily   = "daily"
	UsageBriefJobTypeWeekly  = "weekly"
	UsageBriefJobTypeMonthly = "monthly"
	UsageBriefJobTypeCustom  = "custom"

	UsageBriefStatusQueued    = "queued"
	UsageBriefStatusRunning   = "running"
	UsageBriefStatusSucceeded = "succeeded"
	UsageBriefStatusFailed    = "failed"
	UsageBriefStatusCanceled  = "canceled"
	UsageBriefStatusPaused    = "paused"
	UsageBriefStatusPartial   = "partial"

	UsageBriefDefaultModel                = "gpt-5.5"
	UsageBriefDefaultContextTokens        = 400000
	UsageBriefDefaultOutputReservedTokens = 128000
	UsageBriefDefaultConcurrency          = 4
	UsageBriefMaxConcurrency              = 32
)

const (
	UsageBriefTriggerManual = "manual"
	UsageBriefTriggerAuto   = "auto"

	UsageBriefChunkTypeSource = "source"
	UsageBriefChunkTypeMerge  = "merge"
)

const (
	UsageBriefReportGroupByPeriod = "period"
	UsageBriefReportGroupByUser   = "user"
)

const (
	SettingKeyUsageBriefEnabled              = "usage_brief_enabled"
	SettingKeyUsageBriefOpenAIBaseURL        = "usage_brief_openai_base_url"
	SettingKeyUsageBriefOpenAIAPIKey         = "usage_brief_openai_api_key"
	SettingKeyUsageBriefOpenAIModel          = "usage_brief_openai_model"
	SettingKeyUsageBriefContextTokens        = "usage_brief_context_tokens"
	SettingKeyUsageBriefOutputReservedTokens = "usage_brief_output_reserved_tokens"
	SettingKeyUsageBriefConcurrency          = "usage_brief_concurrency"
)

type UsageBriefSettings struct {
	Enabled              bool   `json:"enabled"`
	BaseURL              string `json:"base_url"`
	APIKey               string `json:"-"`
	APIKeyConfigured     bool   `json:"api_key_configured"`
	Model                string `json:"model"`
	ContextTokens        int    `json:"context_tokens"`
	OutputReservedTokens int    `json:"output_reserved_tokens"`
	Concurrency          int    `json:"concurrency"`
}

type UsageBriefPeriodView struct {
	Available   bool              `json:"available"`
	Reason      string            `json:"reason,omitempty"`
	Message     string            `json:"message,omitempty"`
	PeriodType  string            `json:"period_type"`
	PeriodStart time.Time         `json:"period_start"`
	PeriodEnd   time.Time         `json:"period_end"`
	Status      string            `json:"status,omitempty"`
	Report      *UsageBriefReport `json:"report,omitempty"`
	Job         *UsageBriefJob    `json:"job,omitempty"`
}

type UsageBriefTriggerProductionRequest struct {
	PeriodType  string
	PeriodDate  time.Time
	CreatedBy   int64
	TriggerKind string
}

type UsageBriefCreateTestJobRequest struct {
	UserID     int64
	GroupID    *int64
	RangeStart time.Time
	RangeEnd   time.Time
	CreatedBy  int64
}

type UpdateUsageBriefSettingsRequest struct {
	BaseURL              *string `json:"base_url"`
	APIKey               *string `json:"api_key"`
	ClearAPIKey          *bool   `json:"clear_api_key"`
	Model                *string `json:"model"`
	ContextTokens        *int    `json:"context_tokens"`
	OutputReservedTokens *int    `json:"output_reserved_tokens"`
	Concurrency          *int    `json:"concurrency"`
}

type UsageBriefReport struct {
	ID              int64      `json:"id"`
	UserID          int64      `json:"user_id"`
	UserEmail       string     `json:"user_email,omitempty"`
	Username        string     `json:"username,omitempty"`
	PeriodType      string     `json:"period_type"`
	PeriodStart     time.Time  `json:"period_start"`
	PeriodEnd       time.Time  `json:"period_end"`
	Status          string     `json:"status"`
	Title           string     `json:"title"`
	ContentMD       string     `json:"content_md"`
	SourceKind      string     `json:"source_kind"`
	IsAdminEdited   bool       `json:"is_admin_edited"`
	EditedBy        *int64     `json:"edited_by,omitempty"`
	GeneratedByJob  *int64     `json:"generated_by_job_id,omitempty"`
	InputUsageCount int        `json:"input_usage_count"`
	InputTokens     int        `json:"input_tokens"`
	OutputTokens    int        `json:"output_tokens"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	GeneratedAt     *time.Time `json:"generated_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type UsageBriefReportGroup struct {
	Key             string             `json:"key"`
	GroupBy         string             `json:"group_by"`
	Label           string             `json:"label"`
	PeriodType      string             `json:"period_type,omitempty"`
	PeriodStart     *time.Time         `json:"period_start,omitempty"`
	PeriodEnd       *time.Time         `json:"period_end,omitempty"`
	UserID          *int64             `json:"user_id,omitempty"`
	UserEmail       string             `json:"user_email,omitempty"`
	Username        string             `json:"username,omitempty"`
	ReportCount     int                `json:"report_count"`
	UserCount       int                `json:"user_count"`
	InputUsageCount int                `json:"input_usage_count"`
	InputTokens     int                `json:"input_tokens"`
	OutputTokens    int                `json:"output_tokens"`
	Reports         []UsageBriefReport `json:"reports"`
}

type UsageBriefBatch struct {
	ID                      int64      `json:"id"`
	BatchScope              string     `json:"batch_scope"`
	TriggerKind             string     `json:"trigger_kind"`
	Status                  string     `json:"status"`
	Title                   string     `json:"title"`
	Notes                   string     `json:"notes,omitempty"`
	PeriodType              *string    `json:"period_type,omitempty"`
	PeriodStart             *time.Time `json:"period_start,omitempty"`
	PeriodEnd               *time.Time `json:"period_end,omitempty"`
	RangeStart              *time.Time `json:"range_start,omitempty"`
	RangeEnd                *time.Time `json:"range_end,omitempty"`
	JobCount                int        `json:"job_count"`
	SucceededCount          int        `json:"succeeded_count"`
	FailedCount             int        `json:"failed_count"`
	CanceledCount           int        `json:"canceled_count"`
	RunningCount            int        `json:"running_count"`
	QueuedCount             int        `json:"queued_count"`
	RetryingCount           int        `json:"retrying_count"`
	TokenEstimatedTotal     int        `json:"token_estimated_total"`
	TokenEstimatedProcessed int        `json:"token_estimated_processed"`
	InputTokens             int        `json:"input_tokens"`
	OutputTokens            int        `json:"output_tokens"`
	CancelRequested         bool       `json:"cancel_requested"`
	PausedAt                *time.Time `json:"paused_at,omitempty"`
	DeletedAt               *time.Time `json:"deleted_at,omitempty"`
	CreatedBy               *int64     `json:"created_by,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type UsageBriefJob struct {
	ID                      int64      `json:"id"`
	BatchID                 *int64     `json:"batch_id,omitempty"`
	JobScope                string     `json:"job_scope"`
	JobType                 string     `json:"job_type"`
	Status                  string     `json:"status"`
	UserID                  *int64     `json:"user_id,omitempty"`
	UserEmail               string     `json:"user_email,omitempty"`
	Username                string     `json:"username,omitempty"`
	GroupID                 *int64     `json:"group_id,omitempty"`
	GroupName               string     `json:"group_name,omitempty"`
	PeriodType              *string    `json:"period_type,omitempty"`
	PeriodStart             *time.Time `json:"period_start,omitempty"`
	PeriodEnd               *time.Time `json:"period_end,omitempty"`
	RangeStart              *time.Time `json:"range_start,omitempty"`
	RangeEnd                *time.Time `json:"range_end,omitempty"`
	ProgressCurrent         int        `json:"progress_current"`
	ProgressTotal           int        `json:"progress_total"`
	CancelRequested         bool       `json:"cancel_requested"`
	RetryCount              int        `json:"retry_count"`
	NextRetryAt             *time.Time `json:"next_retry_at,omitempty"`
	Stage                   string     `json:"stage,omitempty"`
	ChunkCurrent            int        `json:"chunk_current"`
	ChunkTotal              int        `json:"chunk_total"`
	TokenEstimatedTotal     int        `json:"token_estimated_total"`
	TokenEstimatedProcessed int        `json:"token_estimated_processed"`
	InputTokens             int        `json:"input_tokens"`
	OutputTokens            int        `json:"output_tokens"`
	ReportID                *int64     `json:"report_id,omitempty"`
	ResultMD                string     `json:"result_md,omitempty"`
	ErrorMessage            string     `json:"error_message,omitempty"`
	LockedAt                *time.Time `json:"locked_at,omitempty"`
	StartedAt               *time.Time `json:"started_at,omitempty"`
	FinishedAt              *time.Time `json:"finished_at,omitempty"`
	CreatedBy               *int64     `json:"created_by,omitempty"`
	DeletedAt               *time.Time `json:"deleted_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type UsageBriefJobChunk struct {
	ID               int64      `json:"id"`
	JobID            int64      `json:"job_id"`
	ChunkIndex       int        `json:"chunk_index"`
	ChunkType        string     `json:"chunk_type"`
	Status           string     `json:"status"`
	TokenEstimated   int        `json:"token_estimated"`
	InputTokens      int        `json:"input_tokens"`
	OutputTokens     int        `json:"output_tokens"`
	RetryCount       int        `json:"retry_count"`
	ContentJSON      string     `json:"content_json,omitempty"`
	ConversationJSON string     `json:"conversation_json,omitempty"`
	SummaryMD        string     `json:"summary_md,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	LastErrorAt      *time.Time `json:"last_error_at,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type UsageBriefJobConversation struct {
	ID                  int64     `json:"id"`
	JobID               int64     `json:"job_id"`
	ConversationIndex   int       `json:"conversation_index"`
	UsageLogID          int64     `json:"usage_log_id"`
	CoveredUsageLogIDs  []int64   `json:"covered_usage_log_ids,omitempty"`
	CoveredRequestCount int       `json:"covered_request_count"`
	TokenEstimated      int       `json:"token_estimated"`
	ConversationJSON    string    `json:"conversation_json"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type UsageBriefReportFilter struct {
	UserID    *int64
	StartDate *time.Time
	EndDate   *time.Time

	PeriodType  string
	SourceKind  string
	Status      string
	Search      string
	SearchScope string
	Page        int
	PageSize    int
}

type UsageBriefReportGroupFilter struct {
	UsageBriefReportFilter
	GroupBy string
}

type UsageBriefJobFilter struct {
	Scope    string
	Status   string
	UserID   *int64
	BatchID  *int64
	Page     int
	PageSize int
}

type UsageBriefBatchFilter struct {
	Scope    string
	Status   string
	Page     int
	PageSize int
}

type UsageBriefJobChunkFilter struct {
	ChunkType string
	Page      int
	PageSize  int
}

type UsageBriefJobConversationFilter struct {
	Page     int
	PageSize int
}

type UsageBriefUsageRecordFilter struct {
	UserID          int64
	GroupID         *int64
	Start           time.Time
	End             time.Time
	AfterCreatedAt  *time.Time
	AfterID         int64
	BeforeCreatedAt *time.Time
	BeforeID        int64
	Limit           int
}

type UsageBriefUsageSummary struct {
	RequestCount      int
	InputTokens       int
	OutputTokens      int
	TotalCost         float64
	ActualCost        float64
	ModelCounts       map[string]int
	RequestTypeCounts map[string]int
}

type UsageBriefSourceRange struct {
	UserID  int64
	GroupID *int64
	Start   time.Time
	End     time.Time
}

type UsageBriefSourceRecord struct {
	ID                        int64     `json:"id"`
	CreatedAt                 time.Time `json:"created_at"`
	Model                     string    `json:"model"`
	RequestedModel            string    `json:"requested_model,omitempty"`
	UpstreamModel             string    `json:"upstream_model,omitempty"`
	GroupID                   *int64    `json:"group_id,omitempty"`
	RequestType               string    `json:"request_type"`
	InputTokens               int       `json:"input_tokens"`
	OutputTokens              int       `json:"output_tokens"`
	TotalCost                 float64   `json:"total_cost"`
	ActualCost                float64   `json:"actual_cost"`
	InboundEndpoint           string    `json:"inbound_endpoint,omitempty"`
	UpstreamEndpoint          string    `json:"upstream_endpoint,omitempty"`
	RequestPayload            string    `json:"request_payload_json,omitempty"`
	ResponsePayload           string    `json:"response_payload_json,omitempty"`
	CompressedRequestPayload  string    `json:"compressed_request_payload_json,omitempty"`
	CompressedResponsePayload string    `json:"compressed_response_payload_json,omitempty"`
}

type UsageBriefRepository interface {
	ListNormalUsers(ctx context.Context) ([]User, error)
	ListReports(ctx context.Context, filter UsageBriefReportFilter) ([]UsageBriefReport, int64, error)
	ListReportGroups(ctx context.Context, filter UsageBriefReportGroupFilter) ([]UsageBriefReportGroup, int64, error)
	GetReport(ctx context.Context, id int64) (*UsageBriefReport, error)
	GetReportByPeriod(ctx context.Context, userID int64, periodType string, periodStart, periodEnd time.Time) (*UsageBriefReport, error)
	UpsertGeneratedReport(ctx context.Context, report UsageBriefReport) (*UsageBriefReport, bool, error)
	UpdateReportByAdmin(ctx context.Context, id int64, title, contentMD string, adminID int64) (*UsageBriefReport, error)
	DeleteReport(ctx context.Context, id int64) error
	DeleteReportGroup(ctx context.Context, filter UsageBriefReportGroupFilter, groupKey string) (int64, error)

	ListBatches(ctx context.Context, filter UsageBriefBatchFilter) ([]UsageBriefBatch, int64, error)
	GetBatch(ctx context.Context, id int64) (*UsageBriefBatch, error)
	CreateBatch(ctx context.Context, batch UsageBriefBatch) (*UsageBriefBatch, error)
	UpdateBatchStats(ctx context.Context, id int64) error
	PauseBatch(ctx context.Context, id int64) (*UsageBriefBatch, error)
	ResumeBatch(ctx context.Context, id int64) (*UsageBriefBatch, error)
	CancelBatch(ctx context.Context, id int64) (*UsageBriefBatch, error)
	ResetBatch(ctx context.Context, id int64) (*UsageBriefBatch, error)
	RerunBatch(ctx context.Context, id int64) (*UsageBriefBatch, error)
	DeleteBatchJobChunks(ctx context.Context, batchID int64) error
	DeleteBatch(ctx context.Context, id int64) error

	ListJobs(ctx context.Context, filter UsageBriefJobFilter) ([]UsageBriefJob, int64, error)
	GetJob(ctx context.Context, id int64) (*UsageBriefJob, error)
	GetActiveJobByPeriod(ctx context.Context, userID int64, periodType string, periodStart, periodEnd time.Time) (*UsageBriefJob, error)
	CreateJob(ctx context.Context, job UsageBriefJob) (*UsageBriefJob, error)
	HasProductionJobOrReport(ctx context.Context, userID int64, periodType string, periodStart, periodEnd time.Time) (bool, error)
	ClaimQueuedJobs(ctx context.Context, limit int) ([]UsageBriefJob, error)
	RecoverRunningJobs(ctx context.Context) (int64, error)
	MarkJobRunning(ctx context.Context, id int64) error
	MarkJobPaused(ctx context.Context, id int64, stage string) error
	UpdateJobProgress(ctx context.Context, id int64, current, total int, stage string, chunkCurrent, chunkTotal, tokenProcessed, tokenTotal, inputTokens, outputTokens int) error
	CompleteJob(ctx context.Context, id int64, reportID *int64, resultMD, status string) error
	FailJob(ctx context.Context, id int64, errMsg string) error
	ScheduleJobRetry(ctx context.Context, id int64, errMsg string, nextRetryAt time.Time) error
	ScheduleJobDependencyWait(ctx context.Context, id int64, errMsg string, nextRetryAt time.Time) error
	MarkJobCanceled(ctx context.Context, id int64) error
	ClearJobCancelRequest(ctx context.Context, id int64) error
	CancelJob(ctx context.Context, id int64) (*UsageBriefJob, error)
	ResetJob(ctx context.Context, id int64) (*UsageBriefJob, error)
	RerunJob(ctx context.Context, id int64) (*UsageBriefJob, error)
	DeleteJob(ctx context.Context, id int64) error
	IsJobCancelRequested(ctx context.Context, id int64) (bool, error)

	DeleteJobChunks(ctx context.Context, jobID int64) error
	DeleteStaleJobChunks(ctx context.Context, jobID int64, chunkType string, maxChunkIndex int) error
	GetJobChunk(ctx context.Context, jobID int64, chunkIndex int, chunkType string) (*UsageBriefJobChunk, error)
	ListJobChunks(ctx context.Context, jobID int64, filter UsageBriefJobChunkFilter) ([]UsageBriefJobChunk, int64, error)
	ListJobChunkSummaries(ctx context.Context, jobID int64, filter UsageBriefJobChunkFilter) ([]UsageBriefJobChunk, int64, error)
	UpsertJobChunk(ctx context.Context, chunk UsageBriefJobChunk) (*UsageBriefJobChunk, error)
	DeleteJobConversations(ctx context.Context, jobID int64) error
	DeleteStaleJobConversations(ctx context.Context, jobID int64, maxConversationIndex int) error
	ListJobConversations(ctx context.Context, jobID int64, filter UsageBriefJobConversationFilter) ([]UsageBriefJobConversation, int64, error)
	UpsertJobConversation(ctx context.Context, conversation UsageBriefJobConversation) (*UsageBriefJobConversation, error)

	FetchUsageRecords(ctx context.Context, userID int64, groupID *int64, start, end time.Time, limit int) ([]UsageBriefSourceRecord, error)
	SummarizeUsageRecords(ctx context.Context, filter UsageBriefUsageRecordFilter) (UsageBriefUsageSummary, error)
	FetchUsageRecordsPage(ctx context.Context, filter UsageBriefUsageRecordFilter) ([]UsageBriefSourceRecord, error)
	UpdateUsageRecordCompressedPayloads(ctx context.Context, usageLogID int64, compressedRequestJSON *string, compressedResponseJSON *string) error
	ListDailyReports(ctx context.Context, userID int64, start, end time.Time) ([]UsageBriefReport, error)
	ListWeeklyReportsForMonth(ctx context.Context, userID int64, monthStart, monthEnd time.Time) ([]UsageBriefReport, error)
}
