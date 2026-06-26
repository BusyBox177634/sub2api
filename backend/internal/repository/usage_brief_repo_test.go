package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func TestUsageBriefReportWhere_DateRangeUsesOverlap(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	where, args := usageBriefReportWhere(service.UsageBriefReportFilter{
		StartDate: &start,
		EndDate:   &end,
	})

	if !strings.Contains(where, "r.period_end >= $1::date") {
		t.Fatalf("expected start filter to match reports ending on/after range start, got %q", where)
	}
	if !strings.Contains(where, "r.period_start <= $2::date") {
		t.Fatalf("expected end filter to match reports starting on/before range end, got %q", where)
	}
	if len(args) != 2 || args[0] != start || args[1] != end {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestUsageBriefReportWhere_UserSearchScopeExcludesTitle(t *testing.T) {
	where, args := usageBriefReportWhere(service.UsageBriefReportFilter{
		Search:      "工作日报",
		SearchScope: "user",
	})

	if strings.Contains(where, "r.title") {
		t.Fatalf("expected user search scope to exclude report title, got %q", where)
	}
	if !strings.Contains(where, "COALESCE(u.email, '') ILIKE $1") || !strings.Contains(where, "COALESCE(u.username, '') ILIKE $1") {
		t.Fatalf("expected user search scope to match username/email, got %q", where)
	}
	if len(args) != 1 || args[0] != "%工作日报%" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestUsageBriefReportWhere_DefaultSearchIncludesTitle(t *testing.T) {
	where, args := usageBriefReportWhere(service.UsageBriefReportFilter{Search: "工作日报"})

	if !strings.Contains(where, "r.title ILIKE $1") {
		t.Fatalf("expected default search to include report title, got %q", where)
	}
	if len(args) != 1 || args[0] != "%工作日报%" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestUsageBriefRepositoryCreateBatch_EmptyNotesWritesEmptyString(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	adminID := int64(9)

	mock.ExpectQuery("INSERT INTO usage_brief_batches").
		WithArgs(
			service.UsageBriefJobScopeTest,
			service.UsageBriefTriggerManual,
			service.UsageBriefStatusQueued,
			"测试简报",
			"",
			nil,
			nil,
			nil,
			now,
			now.Add(time.Hour),
			adminID,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))

	mock.ExpectQuery("SELECT[\\s\\S]*FROM usage_brief_batches b[\\s\\S]*WHERE b\\.id = \\$1").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows(usageBriefBatchColumns()).
			AddRow(
				int64(42),
				service.UsageBriefJobScopeTest,
				service.UsageBriefTriggerManual,
				service.UsageBriefStatusQueued,
				"测试简报",
				"",
				nil,
				nil,
				nil,
				now,
				now.Add(time.Hour),
				1,
				0,
				0,
				0,
				0,
				1,
				0,
				0,
				0,
				0,
				0,
				false,
				nil,
				nil,
				adminID,
				now,
				now,
			))

	created, err := repo.CreateBatch(context.Background(), service.UsageBriefBatch{
		BatchScope:  service.UsageBriefJobScopeTest,
		TriggerKind: service.UsageBriefTriggerManual,
		Status:      service.UsageBriefStatusQueued,
		Title:       "测试简报",
		Notes:       "",
		RangeStart:  &now,
		RangeEnd:    ptrUsageBriefTestTime(now.Add(time.Hour)),
		CreatedBy:   &adminID,
	})
	if err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if created == nil || created.ID != 42 {
		t.Fatalf("unexpected created batch: %#v", created)
	}
	if created.Notes != "" {
		t.Fatalf("expected empty notes, got %q", created.Notes)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositorySummarizeUsageRecordsDoesNotLoadRequestPayload(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	start := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	mock.ExpectQuery("SELECT[\\s\\S]*FROM usage_logs ul\\s+WHERE ul\\.user_id = \\$1 AND ul\\.created_at >= \\$2 AND ul\\.created_at < \\$3[\\s\\S]*GROUP BY model_name, request_type").
		WithArgs(int64(7), start, end).
		WillReturnRows(sqlmock.NewRows([]string{"model_name", "request_type", "count", "input_tokens", "output_tokens", "total_cost", "actual_cost"}).
			AddRow("gpt-test", int16(service.RequestTypeStream), int64(2), int64(100), int64(50), float64(0.02), float64(0.01)))

	summary, err := repo.SummarizeUsageRecords(context.Background(), service.UsageBriefUsageRecordFilter{
		UserID: 7,
		Start:  start,
		End:    end,
	})
	if err != nil {
		t.Fatalf("SummarizeUsageRecords: %v", err)
	}
	if summary.RequestCount != 2 || summary.InputTokens != 100 || summary.OutputTokens != 50 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.ModelCounts["gpt-test"] != 2 || summary.RequestTypeCounts["stream"] != 2 {
		t.Fatalf("unexpected summary counts: %#v", summary)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryFetchUsageRecordsPageUsesCursorAndLimit(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	start := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	after := start.Add(time.Hour)

	mock.ExpectQuery("SELECT[\\s\\S]*LEFT JOIN usage_log_details d[\\s\\S]*AND \\(ul\\.created_at, ul\\.id\\) > \\(\\$4, \\$5\\)[\\s\\S]*ORDER BY ul\\.created_at ASC, ul\\.id ASC\\s+LIMIT \\$6").
		WithArgs(int64(7), start, end, after, int64(99), 50).
		WillReturnRows(sqlmock.NewRows(usageBriefSourceRecordColumns()).
			AddRow(int64(100), after.Add(time.Minute), "gpt-test", "gpt-test", "", nil, int16(service.RequestTypeStream), 10, 5, float64(0.01), float64(0.01), "/v1/responses", "/v1/responses", `{"input":[]}`, `{"output":[]}`, `{"messages":[]}`, `{"messages":[]}`))

	records, err := repo.FetchUsageRecordsPage(context.Background(), service.UsageBriefUsageRecordFilter{
		UserID:         7,
		Start:          start,
		End:            end,
		AfterCreatedAt: &after,
		AfterID:        99,
		Limit:          50,
	})
	if err != nil {
		t.Fatalf("FetchUsageRecordsPage: %v", err)
	}
	if len(records) != 1 || records[0].ID != 100 || records[0].RequestPayload == "" {
		t.Fatalf("unexpected records: %#v", records)
	}
	if records[0].CompressedRequestPayload == "" || records[0].CompressedResponsePayload == "" {
		t.Fatalf("compressed payloads missing: %#v", records[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryFetchUsageRecordsPageSupportsNewestFirstCursor(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	start := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	before := start.Add(12 * time.Hour)

	mock.ExpectQuery("SELECT[\\s\\S]*LEFT JOIN usage_log_details d[\\s\\S]*AND \\(ul\\.created_at, ul\\.id\\) < \\(\\$4, \\$5\\)[\\s\\S]*ORDER BY ul\\.created_at DESC, ul\\.id DESC\\s+LIMIT \\$6").
		WithArgs(int64(7), start, end, before, int64(200), 50).
		WillReturnRows(sqlmock.NewRows(usageBriefSourceRecordColumns()).
			AddRow(int64(199), before.Add(-time.Minute), "gpt-test", "gpt-test", "", nil, int16(service.RequestTypeStream), 10, 5, float64(0.01), float64(0.01), "/v1/responses", "/v1/responses", `{"input":[]}`, `{"output":[]}`, `{"messages":[]}`, `{"messages":[]}`))

	records, err := repo.FetchUsageRecordsPage(context.Background(), service.UsageBriefUsageRecordFilter{
		UserID:          7,
		Start:           start,
		End:             end,
		BeforeCreatedAt: &before,
		BeforeID:        200,
		Limit:           50,
	})
	if err != nil {
		t.Fatalf("FetchUsageRecordsPage: %v", err)
	}
	if len(records) != 1 || records[0].ID != 199 {
		t.Fatalf("unexpected records: %#v", records)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryHasProductionJobOrReportTreatsAnyNonDeletedProductionJobAsExisting(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	start := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	mock.ExpectQuery("SELECT EXISTS[\\s\\S]*FROM usage_brief_jobs[\\s\\S]*job_scope = \\$5[\\s\\S]*deleted_at IS NULL[\\s\\S]*").
		WithArgs(
			int64(7),
			service.UsageBriefPeriodDaily,
			dateOnly(start),
			dateOnly(end),
			service.UsageBriefJobScopeProduction,
		).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	exists, err := repo.HasProductionJobOrReport(context.Background(), 7, service.UsageBriefPeriodDaily, start, end)
	if err != nil {
		t.Fatalf("HasProductionJobOrReport: %v", err)
	}
	if !exists {
		t.Fatalf("exists = false, want true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryListJobConversationsPaginatesIndependentRows(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\)[\\s\\S]*FROM usage_brief_job_conversations[\\s\\S]*WHERE job_id = \\$1").
		WithArgs(int64(77)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))
	mock.ExpectQuery("SELECT[\\s\\S]*conversation_json::text[\\s\\S]*FROM usage_brief_job_conversations[\\s\\S]*ORDER BY conversation_index ASC[\\s\\S]*LIMIT \\$2 OFFSET \\$3").
		WithArgs(int64(77), 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "job_id", "conversation_index", "usage_log_id",
			"covered_usage_log_ids", "covered_request_count", "token_estimated",
			"conversation_json", "created_at", "updated_at",
		}).AddRow(int64(1), int64(77), 1, int64(101), pq.Array([]int64{101, 100}), 2, 88, `{"messages":[{"role":"user","content":"hello"}]}`, now, now))

	conversations, total, err := repo.ListJobConversations(context.Background(), 77, service.UsageBriefJobConversationFilter{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListJobConversations: %v", err)
	}
	if total != 3 || len(conversations) != 1 {
		t.Fatalf("total/len = %d/%d, want 3/1", total, len(conversations))
	}
	if conversations[0].ConversationIndex != 1 || conversations[0].CoveredRequestCount != 2 || len(conversations[0].CoveredUsageLogIDs) != 2 {
		t.Fatalf("unexpected conversation: %#v", conversations[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryResetBatchPreservesChunks(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	mock.ExpectExec("UPDATE usage_brief_jobs[\\s\\S]*cancel_requested = FALSE").
		WithArgs(
			int64(42),
			service.UsageBriefStatusQueued,
			service.UsageBriefStatusFailed,
			service.UsageBriefStatusCanceled,
			service.UsageBriefStatusRunning,
			service.UsageBriefEmailStatusPending,
		).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("UPDATE usage_brief_batches").
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE usage_brief_batches b").
		WithArgs(
			int64(42),
			service.UsageBriefStatusSucceeded,
			service.UsageBriefStatusFailed,
			service.UsageBriefStatusCanceled,
			service.UsageBriefStatusRunning,
			service.UsageBriefStatusQueued,
			service.UsageBriefStatusPaused,
			service.UsageBriefStatusPartial,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT[\\s\\S]*FROM usage_brief_batches b[\\s\\S]*WHERE b\\.id = \\$1").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows(usageBriefBatchColumns()).
			AddRow(
				int64(42),
				service.UsageBriefJobScopeProduction,
				service.UsageBriefTriggerManual,
				service.UsageBriefStatusQueued,
				"工作日报批次",
				"",
				service.UsageBriefPeriodDaily,
				now,
				now.Add(24*time.Hour),
				nil,
				nil,
				3,
				0,
				0,
				0,
				0,
				3,
				0,
				0,
				0,
				0,
				0,
				false,
				nil,
				nil,
				nil,
				now,
				now,
			))

	batch, err := repo.ResetBatch(context.Background(), 42)
	if err != nil {
		t.Fatalf("ResetBatch: %v", err)
	}
	if batch == nil || batch.ID != 42 {
		t.Fatalf("unexpected batch: %#v", batch)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryRerunBatchClearsChunksAndRequeuesInTransaction(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM usage_brief_job_chunks[\\s\\S]*batch_id = \\$1").
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectExec("DELETE FROM usage_brief_job_conversations[\\s\\S]*batch_id = \\$1").
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectExec("UPDATE usage_brief_jobs[\\s\\S]*cancel_requested = FALSE[\\s\\S]*WHERE batch_id = \\$1").
		WithArgs(int64(42), service.UsageBriefStatusQueued, service.UsageBriefEmailStatusPending).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("UPDATE usage_brief_batches[\\s\\S]*cancel_requested = FALSE").
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE usage_brief_batches b").
		WithArgs(
			int64(42),
			service.UsageBriefStatusSucceeded,
			service.UsageBriefStatusFailed,
			service.UsageBriefStatusCanceled,
			service.UsageBriefStatusRunning,
			service.UsageBriefStatusQueued,
			service.UsageBriefStatusPaused,
			service.UsageBriefStatusPartial,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT[\\s\\S]*FROM usage_brief_batches b[\\s\\S]*WHERE b\\.id = \\$1").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows(usageBriefBatchColumns()).
			AddRow(
				int64(42),
				service.UsageBriefJobScopeProduction,
				service.UsageBriefTriggerManual,
				service.UsageBriefStatusQueued,
				"工作日报批次",
				"",
				service.UsageBriefPeriodDaily,
				now,
				now,
				nil,
				nil,
				3,
				0,
				0,
				0,
				0,
				3,
				0,
				0,
				0,
				0,
				0,
				false,
				nil,
				nil,
				nil,
				now,
				now,
			))

	batch, err := repo.RerunBatch(context.Background(), 42)
	if err != nil {
		t.Fatalf("RerunBatch: %v", err)
	}
	if batch == nil || batch.ID != 42 || batch.Status != service.UsageBriefStatusQueued {
		t.Fatalf("unexpected batch: %#v", batch)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryRerunJobClearsChunksAndRequeuesInTransaction(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	batchID := int64(42)
	userID := int64(7)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM usage_brief_job_chunks[\\s\\S]*WHERE job_id = \\$1").
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM usage_brief_job_conversations[\\s\\S]*WHERE job_id = \\$1").
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE usage_brief_jobs[\\s\\S]*cancel_requested = FALSE[\\s\\S]*WHERE id = \\$1").
		WithArgs(
			int64(99),
			service.UsageBriefStatusQueued,
			service.UsageBriefStatusFailed,
			service.UsageBriefStatusCanceled,
			service.UsageBriefStatusRunning,
			service.UsageBriefStatusSucceeded,
			service.UsageBriefEmailStatusPending,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT[\\s\\S]*FROM usage_brief_jobs j[\\s\\S]*WHERE j\\.id = \\$1").
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows(usageBriefJobColumns()).
			AddRow(
				int64(99),
				batchID,
				service.UsageBriefJobScopeProduction,
				service.UsageBriefJobTypeDaily,
				service.UsageBriefStatusQueued,
				userID,
				"2@qq.com",
				"user2",
				nil,
				"",
				service.UsageBriefPeriodDaily,
				now,
				now,
				nil,
				nil,
				0,
				0,
				false,
				0,
				nil,
				"queued",
				0,
				0,
				0,
				0,
				0,
				0,
				nil,
				"",
				"",
				service.UsageBriefEmailStatusPending,
				nil,
				"",
				0,
				nil,
				nil,
				nil,
				nil,
				nil,
				now,
				now,
			))

	job, err := repo.RerunJob(context.Background(), 99)
	if err != nil {
		t.Fatalf("RerunJob: %v", err)
	}
	if job == nil || job.ID != 99 || job.Status != service.UsageBriefStatusQueued || job.CancelRequested {
		t.Fatalf("unexpected job: %#v", job)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryDeleteBatchSoftDeletesJobsAndClearsChunks(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE usage_brief_batches[\\s\\S]*deleted_at = COALESCE").
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE usage_brief_jobs[\\s\\S]*deleted_at = COALESCE[\\s\\S]*WHERE batch_id = \\$1").
		WithArgs(int64(42), service.UsageBriefStatusQueued, service.UsageBriefStatusRunning, service.UsageBriefStatusCanceled).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("DELETE FROM usage_brief_job_chunks[\\s\\S]*WHERE batch_id = \\$1").
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 9))
	mock.ExpectExec("DELETE FROM usage_brief_job_conversations[\\s\\S]*WHERE batch_id = \\$1").
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 9))
	mock.ExpectCommit()

	if err := repo.DeleteBatch(context.Background(), 42); err != nil {
		t.Fatalf("DeleteBatch: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryDeleteJobSoftDeletesAndClearsChunks(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE usage_brief_jobs[\\s\\S]*deleted_at = COALESCE[\\s\\S]*WHERE id = \\$1").
		WithArgs(int64(99), service.UsageBriefStatusQueued, service.UsageBriefStatusRunning, service.UsageBriefStatusCanceled).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM usage_brief_job_chunks WHERE job_id = \\$1").
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("DELETE FROM usage_brief_job_conversations WHERE job_id = \\$1").
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	if err := repo.DeleteJob(context.Background(), 99); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryListReportGroups_PeriodGroupsMultipleUsers(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	periodStart := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart
	createdAt := time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)
	groupKey := "daily|2026-06-22|2026-06-22"

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM \\(SELECT").
		WithArgs(service.UsageBriefJobScopeProduction).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("WITH filtered_reports[\\s\\S]*selected_groups").
		WithArgs(service.UsageBriefJobScopeProduction, 50, 0).
		WillReturnRows(sqlmock.NewRows(usageBriefReportGroupColumns()).
			AddRow(groupKey, int64(11), int64(4), "2@qq.com", "user2", service.UsageBriefPeriodDaily, periodStart, periodEnd, service.UsageBriefStatusSucceeded, "2 日报", "", service.UsageBriefJobScopeProduction, false, nil, int64(21), 3, 1000, 200, nil, createdAt, createdAt, createdAt).
			AddRow(groupKey, int64(12), int64(8), "3@qq.com", "user3", service.UsageBriefPeriodDaily, periodStart, periodEnd, service.UsageBriefStatusSucceeded, "3 日报", "", service.UsageBriefJobScopeProduction, false, nil, int64(22), 5, 2000, 300, nil, createdAt, createdAt, createdAt))

	groups, total, err := repo.ListReportGroups(context.Background(), service.UsageBriefReportGroupFilter{
		GroupBy: service.UsageBriefReportGroupByPeriod,
		UsageBriefReportFilter: service.UsageBriefReportFilter{
			Page:     1,
			PageSize: 50,
		},
	})
	if err != nil {
		t.Fatalf("ListReportGroups: %v", err)
	}
	if total != 1 || len(groups) != 1 {
		t.Fatalf("expected one group, total=%d groups=%d", total, len(groups))
	}
	group := groups[0]
	if group.GroupBy != service.UsageBriefReportGroupByPeriod || group.Key != groupKey {
		t.Fatalf("unexpected group identity: %#v", group)
	}
	if group.ReportCount != 2 || group.UserCount != 2 || len(group.Reports) != 2 {
		t.Fatalf("unexpected group counts: %#v", group)
	}
	if group.InputUsageCount != 8 || group.InputTokens != 3000 || group.OutputTokens != 500 {
		t.Fatalf("unexpected token totals: %#v", group)
	}
	if group.Reports[0].ContentMD != "" {
		t.Fatalf("expected report summary without markdown content")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryListReportGroups_UserGroupsMultiplePeriods(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	day1 := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM \\(SELECT").
		WithArgs(service.UsageBriefJobScopeProduction).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("WITH filtered_reports[\\s\\S]*selected_groups").
		WithArgs(service.UsageBriefJobScopeProduction, 50, 0).
		WillReturnRows(sqlmock.NewRows(usageBriefReportGroupColumns()).
			AddRow("4", int64(12), int64(4), "2@qq.com", "user2", service.UsageBriefPeriodDaily, day2, day2, service.UsageBriefStatusSucceeded, "6-22 日报", "", service.UsageBriefJobScopeProduction, false, nil, int64(22), 5, 2000, 300, nil, createdAt, createdAt, createdAt).
			AddRow("4", int64(11), int64(4), "2@qq.com", "user2", service.UsageBriefPeriodDaily, day1, day1, service.UsageBriefStatusSucceeded, "6-21 日报", "", service.UsageBriefJobScopeProduction, false, nil, int64(21), 3, 1000, 200, nil, createdAt, createdAt, createdAt))

	groups, total, err := repo.ListReportGroups(context.Background(), service.UsageBriefReportGroupFilter{
		GroupBy: service.UsageBriefReportGroupByUser,
		UsageBriefReportFilter: service.UsageBriefReportFilter{
			Page:     1,
			PageSize: 50,
		},
	})
	if err != nil {
		t.Fatalf("ListReportGroups: %v", err)
	}
	if total != 1 || len(groups) != 1 {
		t.Fatalf("expected one group, total=%d groups=%d", total, len(groups))
	}
	group := groups[0]
	if group.GroupBy != service.UsageBriefReportGroupByUser || group.UserID == nil || *group.UserID != 4 {
		t.Fatalf("unexpected user group: %#v", group)
	}
	if group.ReportCount != 2 || group.UserCount != 1 || len(group.Reports) != 2 {
		t.Fatalf("unexpected user group counts: %#v", group)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryListReportGroups_UserGroupAppliesDateRange(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	start := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	end := start
	createdAt := time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery("WITH filtered_reports[\\s\\S]*SELECT COUNT\\(\\*\\)").
		WithArgs(start, end, service.UsageBriefPeriodDaily, service.UsageBriefJobScopeProduction).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("WITH filtered_reports[\\s\\S]*selected_groups").
		WithArgs(start, end, service.UsageBriefPeriodDaily, service.UsageBriefJobScopeProduction, 50, 0).
		WillReturnRows(sqlmock.NewRows(usageBriefReportGroupColumns()).
			AddRow("4", int64(12), int64(4), "2@qq.com", "user2", service.UsageBriefPeriodDaily, start, end, service.UsageBriefStatusSucceeded, "6-22 日报", "", service.UsageBriefJobScopeProduction, false, nil, int64(22), 5, 2000, 300, nil, createdAt, createdAt, createdAt))

	groups, total, err := repo.ListReportGroups(context.Background(), service.UsageBriefReportGroupFilter{
		GroupBy: service.UsageBriefReportGroupByUser,
		UsageBriefReportFilter: service.UsageBriefReportFilter{
			StartDate:  &start,
			EndDate:    &end,
			PeriodType: service.UsageBriefPeriodDaily,
			Page:       1,
			PageSize:   50,
		},
	})
	if err != nil {
		t.Fatalf("ListReportGroups: %v", err)
	}
	if total != 1 || len(groups) != 1 || len(groups[0].Reports) != 1 {
		t.Fatalf("unexpected filtered groups: total=%d groups=%#v", total, groups)
	}
	if got := groups[0].Reports[0].PeriodStart; !got.Equal(start) {
		t.Fatalf("expected only filtered period report, got %s", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryDeleteReportGroup_UserGroupAppliesDateRangeAndProductionScope(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	start := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	end := start

	mock.ExpectExec("DELETE FROM usage_brief_reports r[\\s\\S]*r\\.source_kind = \\$4[\\s\\S]*r\\.user_id::text = \\$5").
		WithArgs(start, end, service.UsageBriefPeriodDaily, service.UsageBriefJobScopeProduction, "4").
		WillReturnResult(sqlmock.NewResult(0, 2))

	deleted, err := repo.DeleteReportGroup(context.Background(), service.UsageBriefReportGroupFilter{
		GroupBy: service.UsageBriefReportGroupByUser,
		UsageBriefReportFilter: service.UsageBriefReportFilter{
			StartDate:  &start,
			EndDate:    &end,
			PeriodType: service.UsageBriefPeriodDaily,
		},
	}, "4")
	if err != nil {
		t.Fatalf("DeleteReportGroup: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestUsageBriefRepositoryDeleteReportGroup_PeriodGroupDeletesMatchingProductionReports(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("new sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &usageBriefRepository{db: db}
	groupKey := "daily|2026-06-22|2026-06-22"

	mock.ExpectExec("DELETE FROM usage_brief_reports r[\\s\\S]*r\\.source_kind = \\$1[\\s\\S]*concat\\(r\\.period_type, '\\|', r\\.period_start::date, '\\|', r\\.period_end::date\\) = \\$2").
		WithArgs(service.UsageBriefJobScopeProduction, groupKey).
		WillReturnResult(sqlmock.NewResult(0, 2))

	deleted, err := repo.DeleteReportGroup(context.Background(), service.UsageBriefReportGroupFilter{
		GroupBy: service.UsageBriefReportGroupByPeriod,
	}, groupKey)
	if err != nil {
		t.Fatalf("DeleteReportGroup: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func usageBriefBatchColumns() []string {
	return []string{
		"id", "batch_scope", "trigger_kind", "status", "title", "notes",
		"period_type", "period_start", "period_end", "range_start", "range_end",
		"job_count", "succeeded_count", "failed_count", "canceled_count",
		"running_count", "queued_count", "retrying_count",
		"token_estimated_total", "token_estimated_processed", "input_tokens", "output_tokens",
		"cancel_requested", "paused_at", "deleted_at", "created_by", "created_at", "updated_at",
	}
}

func usageBriefReportGroupColumns() []string {
	return []string{
		"group_key",
		"id", "user_id", "email", "username",
		"period_type", "period_start", "period_end", "status", "title", "content_md",
		"source_kind", "is_admin_edited", "edited_by", "generated_by_job_id",
		"input_usage_count", "input_tokens", "output_tokens", "error_message",
		"generated_at", "created_at", "updated_at",
	}
}

func usageBriefJobColumns() []string {
	return []string{
		"id", "batch_id", "job_scope", "job_type", "status", "user_id",
		"email", "username", "group_id", "group_name",
		"period_type", "period_start", "period_end", "range_start", "range_end",
		"progress_current", "progress_total", "cancel_requested", "retry_count",
		"next_retry_at", "stage", "chunk_current", "chunk_total",
		"token_estimated_total", "token_estimated_processed", "input_tokens", "output_tokens",
		"report_id", "result_md", "error_message",
		"email_status", "email_sent_at", "email_error_message", "email_attempt_count",
		"locked_at", "started_at", "finished_at",
		"created_by", "deleted_at", "created_at", "updated_at",
	}
}

func usageBriefSourceRecordColumns() []string {
	return []string{
		"id", "created_at", "model", "requested_model", "upstream_model",
		"group_id", "request_type", "input_tokens", "output_tokens",
		"total_cost", "actual_cost", "inbound_endpoint", "upstream_endpoint",
		"request_payload_json", "response_payload_json", "compressed_request_payload_json", "compressed_response_payload_json",
	}
}

func ptrUsageBriefTestTime(value time.Time) *time.Time {
	return &value
}
