package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type usageBriefRepository struct {
	db *sql.DB
}

type usageBriefSQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func NewUsageBriefRepository(db *sql.DB) service.UsageBriefRepository {
	return &usageBriefRepository{db: db}
}

func (r *usageBriefRepository) ListNormalUsers(ctx context.Context) ([]service.User, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, email, username, role, status, created_at, updated_at
FROM users
WHERE role = $1 AND status = $2 AND deleted_at IS NULL
ORDER BY id ASC
`, service.RoleUser, service.StatusActive)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []service.User
	for rows.Next() {
		var u service.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *usageBriefRepository) ListReports(ctx context.Context, filter service.UsageBriefReportFilter) ([]service.UsageBriefReport, int64, error) {
	where, args := usageBriefReportWhere(filter)
	countQuery := "SELECT COUNT(*) FROM usage_brief_reports r LEFT JOIN users u ON u.id = r.user_id " + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	args = append(args, pageSize, (page-1)*pageSize)
	query := `
SELECT
	r.id, r.user_id, COALESCE(u.email, ''), COALESCE(u.username, ''),
	r.period_type, r.period_start, r.period_end, r.status, r.title, r.content_md,
	r.source_kind, r.is_admin_edited, r.edited_by, r.generated_by_job_id,
	r.input_usage_count, r.input_tokens, r.output_tokens, r.error_message,
	r.generated_at, r.created_at, r.updated_at
FROM usage_brief_reports r
LEFT JOIN users u ON u.id = r.user_id
` + where + `
ORDER BY r.period_start DESC, r.id DESC
LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	reports, err := scanUsageBriefReports(rows)
	if err != nil {
		return nil, 0, err
	}
	return reports, total, rows.Err()
}

func (r *usageBriefRepository) ListReportGroups(ctx context.Context, filter service.UsageBriefReportGroupFilter) ([]service.UsageBriefReportGroup, int64, error) {
	filter.SourceKind = service.UsageBriefJobScopeProduction
	groupBy := normalizeUsageBriefReportGroupBy(filter.GroupBy)
	where, args := usageBriefReportWhere(filter.UsageBriefReportFilter)
	groupExpr, groupOrder := usageBriefReportGroupSQL(groupBy)
	filteredReportsCTE := `
WITH filtered_reports AS (
	SELECT
		` + groupExpr + ` AS group_key,
		r.id, r.user_id, COALESCE(u.email, '') AS user_email, COALESCE(u.username, '') AS username,
		r.period_type, r.period_start, r.period_end, r.status, r.title,
		r.source_kind, r.is_admin_edited, r.edited_by, r.generated_by_job_id,
		r.input_usage_count, r.input_tokens, r.output_tokens, r.error_message,
		r.generated_at, r.created_at, r.updated_at
	FROM usage_brief_reports r
	LEFT JOIN users u ON u.id = r.user_id
` + where + `
)
`

	countQuery := filteredReportsCTE + `SELECT COUNT(*) FROM (SELECT group_key FROM filtered_reports GROUP BY group_key) grouped`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []service.UsageBriefReportGroup{}, 0, nil
	}

	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	args = append(args, pageSize, (page-1)*pageSize)
	query := filteredReportsCTE + `
, selected_groups AS (
	SELECT group_key,
		MAX(period_start) AS sort_period_start,
		MAX(period_end) AS sort_period_end,
		MAX(id) AS sort_id,
		MAX(user_email) AS sort_user_email
	FROM filtered_reports
	GROUP BY group_key
	ORDER BY ` + groupOrder + `
	LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args)) + `
)
SELECT
	sg.group_key,
	fr.id, fr.user_id, fr.user_email, fr.username,
	fr.period_type, fr.period_start, fr.period_end, fr.status, fr.title, '' AS content_md,
	fr.source_kind, fr.is_admin_edited, fr.edited_by, fr.generated_by_job_id,
	fr.input_usage_count, fr.input_tokens, fr.output_tokens, fr.error_message,
	fr.generated_at, fr.created_at, fr.updated_at
FROM selected_groups sg
JOIN filtered_reports fr ON fr.group_key = sg.group_key
ORDER BY sg.sort_period_start DESC, sg.sort_period_end DESC, sg.sort_id DESC, fr.period_start DESC, fr.id DESC
`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	groups, err := scanUsageBriefReportGroups(rows, groupBy)
	if err != nil {
		return nil, 0, err
	}
	return groups, total, rows.Err()
}

func normalizeUsageBriefReportGroupBy(value string) string {
	switch strings.TrimSpace(value) {
	case service.UsageBriefReportGroupByUser:
		return service.UsageBriefReportGroupByUser
	default:
		return service.UsageBriefReportGroupByPeriod
	}
}

func usageBriefReportGroupSQL(groupBy string) (string, string) {
	if groupBy == service.UsageBriefReportGroupByUser {
		return "r.user_id::text", "sort_user_email ASC, sort_period_start DESC, sort_id DESC"
	}
	return "concat(r.period_type, '|', r.period_start::date, '|', r.period_end::date)", "sort_period_start DESC, sort_period_end DESC, sort_id DESC"
}

func usageBriefReportWhere(filter service.UsageBriefReportFilter) (string, []any) {
	conditions := make([]string, 0, 5)
	args := make([]any, 0, 5)
	if filter.UserID != nil && *filter.UserID > 0 {
		args = append(args, *filter.UserID)
		conditions = append(conditions, fmt.Sprintf("r.user_id = $%d", len(args)))
	}
	if filter.StartDate != nil {
		args = append(args, *filter.StartDate)
		conditions = append(conditions, fmt.Sprintf("r.period_end >= $%d::date", len(args)))
	}
	if filter.EndDate != nil {
		args = append(args, *filter.EndDate)
		conditions = append(conditions, fmt.Sprintf("r.period_start <= $%d::date", len(args)))
	}
	if filter.PeriodType != "" {
		args = append(args, filter.PeriodType)
		conditions = append(conditions, fmt.Sprintf("r.period_type = $%d", len(args)))
	}
	if filter.SourceKind != "" {
		args = append(args, filter.SourceKind)
		conditions = append(conditions, fmt.Sprintf("r.source_kind = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("r.status = $%d", len(args)))
	}
	if q := strings.TrimSpace(filter.Search); q != "" {
		args = append(args, "%"+q+"%")
		conditions = append(conditions, fmt.Sprintf("(r.title ILIKE $%d OR COALESCE(u.email, '') ILIKE $%d OR COALESCE(u.username, '') ILIKE $%d)", len(args), len(args), len(args)))
	}
	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func (r *usageBriefRepository) GetReport(ctx context.Context, id int64) (*service.UsageBriefReport, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
	r.id, r.user_id, COALESCE(u.email, ''), COALESCE(u.username, ''),
	r.period_type, r.period_start, r.period_end, r.status, r.title, r.content_md,
	r.source_kind, r.is_admin_edited, r.edited_by, r.generated_by_job_id,
	r.input_usage_count, r.input_tokens, r.output_tokens, r.error_message,
	r.generated_at, r.created_at, r.updated_at
FROM usage_brief_reports r
LEFT JOIN users u ON u.id = r.user_id
WHERE r.id = $1
`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	reports, err := scanUsageBriefReports(rows)
	if err != nil {
		return nil, err
	}
	if len(reports) == 0 {
		return nil, nil
	}
	return &reports[0], nil
}

func (r *usageBriefRepository) GetReportByPeriod(ctx context.Context, userID int64, periodType string, periodStart, periodEnd time.Time) (*service.UsageBriefReport, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
	r.id, r.user_id, COALESCE(u.email, ''), COALESCE(u.username, ''),
	r.period_type, r.period_start, r.period_end, r.status, r.title, r.content_md,
	r.source_kind, r.is_admin_edited, r.edited_by, r.generated_by_job_id,
	r.input_usage_count, r.input_tokens, r.output_tokens, r.error_message,
	r.generated_at, r.created_at, r.updated_at
FROM usage_brief_reports r
LEFT JOIN users u ON u.id = r.user_id
WHERE r.user_id = $1 AND r.period_type = $2 AND r.period_start = $3::date AND r.period_end = $4::date
`, userID, periodType, dateOnly(periodStart), dateOnly(periodEnd))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	reports, err := scanUsageBriefReports(rows)
	if err != nil {
		return nil, err
	}
	if len(reports) == 0 {
		return nil, nil
	}
	return &reports[0], nil
}

func (r *usageBriefRepository) UpsertGeneratedReport(ctx context.Context, report service.UsageBriefReport) (*service.UsageBriefReport, bool, error) {
	var existingEdited bool
	var existingID sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
SELECT id, is_admin_edited
FROM usage_brief_reports
WHERE user_id = $1 AND period_type = $2 AND period_start = $3::date AND period_end = $4::date
`, report.UserID, report.PeriodType, dateOnly(report.PeriodStart), dateOnly(report.PeriodEnd)).Scan(&existingID, &existingEdited)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	if existingID.Valid && existingEdited {
		existing, err := r.GetReport(ctx, existingID.Int64)
		return existing, false, err
	}

	rows, err := r.db.QueryContext(ctx, `
INSERT INTO usage_brief_reports (
	user_id, period_type, period_start, period_end, status, title, content_md,
	source_kind, is_admin_edited, generated_by_job_id, input_usage_count,
	input_tokens, output_tokens, error_message, generated_at, updated_at
) VALUES (
	$1, $2, $3::date, $4::date, $5, $6, $7,
	$8, FALSE, $9, $10,
	$11, $12, $13, $14, NOW()
)
ON CONFLICT (user_id, period_type, period_start, period_end) DO UPDATE SET
	status = EXCLUDED.status,
	title = EXCLUDED.title,
	content_md = EXCLUDED.content_md,
	source_kind = EXCLUDED.source_kind,
	generated_by_job_id = EXCLUDED.generated_by_job_id,
	input_usage_count = EXCLUDED.input_usage_count,
	input_tokens = EXCLUDED.input_tokens,
	output_tokens = EXCLUDED.output_tokens,
	error_message = EXCLUDED.error_message,
	generated_at = EXCLUDED.generated_at,
	updated_at = NOW()
WHERE usage_brief_reports.is_admin_edited = FALSE
RETURNING id
`, report.UserID, report.PeriodType, dateOnly(report.PeriodStart), dateOnly(report.PeriodEnd),
		report.Status, report.Title, report.ContentMD, report.SourceKind, nullInt64Ptr(report.GeneratedByJob),
		report.InputUsageCount, report.InputTokens, report.OutputTokens, nullStringValue(report.ErrorMessage),
		nullTimePtr(report.GeneratedAt))
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	var id int64
	if rows.Next() {
		if err := rows.Scan(&id); err != nil {
			return nil, false, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if id == 0 && existingID.Valid {
		id = existingID.Int64
	}
	if id == 0 {
		return nil, false, nil
	}
	saved, err := r.GetReport(ctx, id)
	return saved, true, err
}

func (r *usageBriefRepository) UpdateReportByAdmin(ctx context.Context, id int64, title, contentMD string, adminID int64) (*service.UsageBriefReport, error) {
	result, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_reports
SET title = $2, content_md = $3, is_admin_edited = TRUE, edited_by = $4, status = $5, updated_at = NOW()
WHERE id = $1
`, id, title, contentMD, adminID, service.UsageBriefStatusSucceeded)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, nil
	}
	return r.GetReport(ctx, id)
}

func (r *usageBriefRepository) DeleteReport(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM usage_brief_reports WHERE id = $1`, id)
	return err
}

func (r *usageBriefRepository) DeleteReportGroup(ctx context.Context, filter service.UsageBriefReportGroupFilter, groupKey string) (int64, error) {
	filter.SourceKind = service.UsageBriefJobScopeProduction
	groupBy := normalizeUsageBriefReportGroupBy(filter.GroupBy)
	where, args := usageBriefReportWhere(filter.UsageBriefReportFilter)
	groupExpr, _ := usageBriefReportGroupSQL(groupBy)
	args = append(args, strings.TrimSpace(groupKey))
	query := `
DELETE FROM usage_brief_reports r
WHERE r.id IN (
	SELECT r.id
	FROM usage_brief_reports r
	LEFT JOIN users u ON u.id = r.user_id
` + where + `
	  AND ` + groupExpr + ` = $` + fmt.Sprint(len(args)) + `
)
`
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *usageBriefRepository) ListBatches(ctx context.Context, filter service.UsageBriefBatchFilter) ([]service.UsageBriefBatch, int64, error) {
	where, args := usageBriefBatchWhere(filter)
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_brief_batches b "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	args = append(args, pageSize, (page-1)*pageSize)
	query := usageBriefBatchSelectSQL + where + `
ORDER BY b.created_at DESC, b.id DESC
LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	batches, err := scanUsageBriefBatches(rows)
	return batches, total, err
}

func usageBriefBatchWhere(filter service.UsageBriefBatchFilter) (string, []any) {
	conditions := []string{"b.deleted_at IS NULL"}
	args := make([]any, 0, 2)
	if filter.Scope != "" {
		args = append(args, filter.Scope)
		conditions = append(conditions, fmt.Sprintf("b.batch_scope = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("b.status = $%d", len(args)))
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

const usageBriefBatchSelectSQL = `
SELECT
	b.id, b.batch_scope, b.trigger_kind, b.status, b.title, b.notes,
	b.period_type, b.period_start, b.period_end, b.range_start, b.range_end,
	b.job_count, b.succeeded_count, b.failed_count, b.canceled_count,
	b.running_count, b.queued_count, b.retrying_count,
	b.token_estimated_total, b.token_estimated_processed, b.input_tokens, b.output_tokens,
	b.cancel_requested, b.paused_at, b.deleted_at, b.created_by, b.created_at, b.updated_at
FROM usage_brief_batches b
`

func (r *usageBriefRepository) GetBatch(ctx context.Context, id int64) (*service.UsageBriefBatch, error) {
	rows, err := r.db.QueryContext(ctx, usageBriefBatchSelectSQL+`WHERE b.id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	batches, err := scanUsageBriefBatches(rows)
	if err != nil {
		return nil, err
	}
	if len(batches) == 0 {
		return nil, nil
	}
	return &batches[0], nil
}

func (r *usageBriefRepository) CreateBatch(ctx context.Context, batch service.UsageBriefBatch) (*service.UsageBriefBatch, error) {
	notes := strings.TrimSpace(batch.Notes)
	rows, err := r.db.QueryContext(ctx, `
INSERT INTO usage_brief_batches (
	batch_scope, trigger_kind, status, title, notes, period_type, period_start,
	period_end, range_start, range_end, created_by, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7::date,
	$8::date, $9, $10, $11, NOW(), NOW()
)
RETURNING id
`, batch.BatchScope, coalesceString(batch.TriggerKind, service.UsageBriefTriggerManual),
		coalesceString(batch.Status, service.UsageBriefStatusQueued), batch.Title, notes,
		nullStringPtrValue(batch.PeriodType), nullDatePtr(batch.PeriodStart), nullDatePtr(batch.PeriodEnd),
		nullTimePtr(batch.RangeStart), nullTimePtr(batch.RangeEnd), nullInt64Ptr(batch.CreatedBy))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var id int64
	if rows.Next() {
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if id == 0 {
		return nil, nil
	}
	return r.GetBatch(ctx, id)
}

func (r *usageBriefRepository) UpdateBatchStats(ctx context.Context, id int64) error {
	return updateUsageBriefBatchStats(ctx, r.db, id)
}

func updateUsageBriefBatchStats(ctx context.Context, execer usageBriefSQLExecutor, id int64) error {
	_, err := execer.ExecContext(ctx, `
WITH stats AS (
	SELECT
		COUNT(*)::int AS job_count,
		COUNT(*) FILTER (WHERE status = $2)::int AS succeeded_count,
		COUNT(*) FILTER (WHERE status = $3)::int AS failed_count,
		COUNT(*) FILTER (WHERE status = $4)::int AS canceled_count,
		COUNT(*) FILTER (WHERE status = $5)::int AS running_count,
		COUNT(*) FILTER (WHERE status = $6 AND (next_retry_at IS NULL OR next_retry_at <= NOW()))::int AS queued_count,
		COUNT(*) FILTER (WHERE status = $6 AND next_retry_at > NOW())::int AS retrying_count,
		COALESCE(SUM(token_estimated_total), 0)::bigint AS token_estimated_total,
		COALESCE(SUM(token_estimated_processed), 0)::bigint AS token_estimated_processed,
		COALESCE(SUM(input_tokens), 0)::bigint AS input_tokens,
		COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens
	FROM usage_brief_jobs
	WHERE batch_id = $1 AND deleted_at IS NULL
),
state AS (
	SELECT b.id, b.cancel_requested, b.paused_at, stats.*
	FROM usage_brief_batches b
	CROSS JOIN stats
	WHERE b.id = $1 AND b.deleted_at IS NULL
)
UPDATE usage_brief_batches b
SET job_count = state.job_count,
	succeeded_count = state.succeeded_count,
	failed_count = state.failed_count,
	canceled_count = state.canceled_count,
	running_count = state.running_count,
	queued_count = state.queued_count,
	retrying_count = state.retrying_count,
	token_estimated_total = state.token_estimated_total,
	token_estimated_processed = state.token_estimated_processed,
	input_tokens = state.input_tokens,
	output_tokens = state.output_tokens,
	status = CASE
		WHEN state.paused_at IS NOT NULL THEN $7
		WHEN state.job_count = 0 THEN $2
		WHEN state.succeeded_count = state.job_count THEN $2
		WHEN state.canceled_count = state.job_count THEN $4
		WHEN state.failed_count > 0 AND (state.succeeded_count + state.failed_count + state.canceled_count) = state.job_count THEN
			CASE WHEN state.succeeded_count > 0 THEN $8 ELSE $3 END
		WHEN state.cancel_requested = TRUE AND (state.running_count + state.queued_count + state.retrying_count) = 0 THEN $4
		WHEN state.running_count > 0 THEN $5
		WHEN (state.queued_count + state.retrying_count) > 0 THEN $6
		ELSE $8
	END,
	updated_at = NOW()
FROM state
WHERE b.id = state.id
`, id, service.UsageBriefStatusSucceeded, service.UsageBriefStatusFailed, service.UsageBriefStatusCanceled,
		service.UsageBriefStatusRunning, service.UsageBriefStatusQueued, service.UsageBriefStatusPaused,
		service.UsageBriefStatusPartial)
	return err
}

func (r *usageBriefRepository) PauseBatch(ctx context.Context, id int64) (*service.UsageBriefBatch, error) {
	result, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_batches
SET paused_at = COALESCE(paused_at, NOW()), status = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL AND status NOT IN ($3, $4)
`, id, service.UsageBriefStatusPaused, service.UsageBriefStatusSucceeded, service.UsageBriefStatusCanceled)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return r.GetBatch(ctx, id)
	}
	return r.GetBatch(ctx, id)
}

func (r *usageBriefRepository) ResumeBatch(ctx context.Context, id int64) (*service.UsageBriefBatch, error) {
	result, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_batches
SET paused_at = NULL, status = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
`, id, service.UsageBriefStatusQueued)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected > 0 {
		if err := r.UpdateBatchStats(ctx, id); err != nil {
			return nil, err
		}
	}
	return r.GetBatch(ctx, id)
}

func (r *usageBriefRepository) CancelBatch(ctx context.Context, id int64) (*service.UsageBriefBatch, error) {
	if _, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_batches
SET cancel_requested = TRUE, paused_at = NULL, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
`, id); err != nil {
		return nil, err
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET cancel_requested = TRUE,
	status = CASE WHEN status = $2 THEN $4 ELSE status END,
	stage = CASE WHEN status = $2 THEN 'canceled' ELSE stage END,
	finished_at = CASE WHEN status = $2 THEN COALESCE(finished_at, NOW()) ELSE finished_at END,
	updated_at = NOW()
WHERE batch_id = $1 AND deleted_at IS NULL AND status IN ($2, $3)
`, id, service.UsageBriefStatusQueued, service.UsageBriefStatusRunning, service.UsageBriefStatusCanceled); err != nil {
		return nil, err
	}
	if err := r.UpdateBatchStats(ctx, id); err != nil {
		return nil, err
	}
	return r.GetBatch(ctx, id)
}

func (r *usageBriefRepository) ResetBatch(ctx context.Context, id int64) (*service.UsageBriefBatch, error) {
	if _, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2,
	cancel_requested = FALSE,
	progress_current = 0,
	chunk_current = 0,
	chunk_total = 0,
	token_estimated_total = 0,
	token_estimated_processed = 0,
	input_tokens = 0,
	output_tokens = 0,
	error_message = NULL,
	next_retry_at = NULL,
	stage = 'queued',
	locked_at = NULL,
	started_at = NULL,
	finished_at = NULL,
	retry_count = 0,
	updated_at = NOW()
WHERE batch_id = $1 AND deleted_at IS NULL AND status IN ($2, $3, $4, $5)
`, id, service.UsageBriefStatusQueued, service.UsageBriefStatusFailed, service.UsageBriefStatusCanceled, service.UsageBriefStatusRunning); err != nil {
		return nil, err
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_batches
SET cancel_requested = FALSE, paused_at = NULL, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
`, id); err != nil {
		return nil, err
	}
	if err := r.UpdateBatchStats(ctx, id); err != nil {
		return nil, err
	}
	return r.GetBatch(ctx, id)
}

func (r *usageBriefRepository) RerunBatch(ctx context.Context, id int64) (*service.UsageBriefBatch, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `
DELETE FROM usage_brief_job_chunks
WHERE job_id IN (
	SELECT id FROM usage_brief_jobs
	WHERE batch_id = $1 AND deleted_at IS NULL
)
`, id); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM usage_brief_job_conversations
WHERE job_id IN (
	SELECT id FROM usage_brief_jobs
	WHERE batch_id = $1 AND deleted_at IS NULL
)
`, id); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2,
	cancel_requested = FALSE,
	progress_current = 0,
	progress_total = 0,
	chunk_current = 0,
	chunk_total = 0,
	token_estimated_total = 0,
	token_estimated_processed = 0,
	input_tokens = 0,
	output_tokens = 0,
	error_message = NULL,
	next_retry_at = NULL,
	stage = 'queued',
	locked_at = NULL,
	started_at = NULL,
	finished_at = NULL,
	retry_count = 0,
	report_id = NULL,
	result_md = NULL,
	updated_at = NOW()
WHERE batch_id = $1 AND deleted_at IS NULL
`, id, service.UsageBriefStatusQueued); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE usage_brief_batches
SET cancel_requested = FALSE, paused_at = NULL, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
`, id); err != nil {
		return nil, err
	}
	if err := updateUsageBriefBatchStats(ctx, tx, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return r.GetBatch(ctx, id)
}

func (r *usageBriefRepository) DeleteBatchJobChunks(ctx context.Context, batchID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(ctx, `
DELETE FROM usage_brief_job_chunks
WHERE job_id IN (
	SELECT id FROM usage_brief_jobs
	WHERE batch_id = $1
)
`, batchID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM usage_brief_job_conversations
WHERE job_id IN (
	SELECT id FROM usage_brief_jobs
	WHERE batch_id = $1
)
`, batchID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *usageBriefRepository) DeleteBatch(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `
UPDATE usage_brief_batches
SET deleted_at = COALESCE(deleted_at, NOW()),
	cancel_requested = TRUE,
	paused_at = NULL,
	updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM usage_brief_batches WHERE id = $1)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return sql.ErrNoRows
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET deleted_at = COALESCE(deleted_at, NOW()),
	cancel_requested = CASE WHEN status IN ($2, $3) THEN TRUE ELSE cancel_requested END,
	status = CASE WHEN status = $2 THEN $4 ELSE status END,
	stage = CASE WHEN status = $2 THEN 'canceled' ELSE stage END,
	finished_at = CASE WHEN status = $2 THEN COALESCE(finished_at, NOW()) ELSE finished_at END,
	updated_at = NOW()
WHERE batch_id = $1 AND deleted_at IS NULL
`, id, service.UsageBriefStatusQueued, service.UsageBriefStatusRunning, service.UsageBriefStatusCanceled); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM usage_brief_job_chunks
WHERE job_id IN (
	SELECT id FROM usage_brief_jobs
	WHERE batch_id = $1
)
`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM usage_brief_job_conversations
WHERE job_id IN (
	SELECT id FROM usage_brief_jobs
	WHERE batch_id = $1
)
`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *usageBriefRepository) ListJobs(ctx context.Context, filter service.UsageBriefJobFilter) ([]service.UsageBriefJob, int64, error) {
	where, args := usageBriefJobWhere(filter)
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_brief_jobs j LEFT JOIN users u ON u.id = j.user_id LEFT JOIN groups g ON g.id = j.group_id "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	args = append(args, pageSize, (page-1)*pageSize)
	query := usageBriefJobSelectSQL + where + `
ORDER BY j.created_at DESC, j.id DESC
LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	jobs, err := scanUsageBriefJobs(rows)
	return jobs, total, err
}

func usageBriefJobWhere(filter service.UsageBriefJobFilter) (string, []any) {
	conditions := make([]string, 0, 3)
	args := make([]any, 0, 3)
	conditions = append(conditions, "j.deleted_at IS NULL")
	if filter.Scope != "" {
		args = append(args, filter.Scope)
		conditions = append(conditions, fmt.Sprintf("j.job_scope = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("j.status = $%d", len(args)))
	}
	if filter.UserID != nil && *filter.UserID > 0 {
		args = append(args, *filter.UserID)
		conditions = append(conditions, fmt.Sprintf("j.user_id = $%d", len(args)))
	}
	if filter.BatchID != nil && *filter.BatchID > 0 {
		args = append(args, *filter.BatchID)
		conditions = append(conditions, fmt.Sprintf("j.batch_id = $%d", len(args)))
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

const usageBriefJobSelectSQL = `
SELECT
	j.id, j.batch_id, j.job_scope, j.job_type, j.status, j.user_id,
	COALESCE(u.email, ''), COALESCE(u.username, ''),
	j.group_id, COALESCE(g.name, ''),
	j.period_type, j.period_start, j.period_end,
	j.range_start, j.range_end, j.progress_current, j.progress_total,
	j.cancel_requested, j.retry_count, j.next_retry_at, j.stage,
	j.chunk_current, j.chunk_total, j.token_estimated_total, j.token_estimated_processed,
	j.input_tokens, j.output_tokens, j.report_id, j.result_md,
	j.error_message, j.locked_at, j.started_at, j.finished_at,
	j.created_by, j.deleted_at, j.created_at, j.updated_at
FROM usage_brief_jobs j
LEFT JOIN users u ON u.id = j.user_id
LEFT JOIN groups g ON g.id = j.group_id
`

func (r *usageBriefRepository) GetJob(ctx context.Context, id int64) (*service.UsageBriefJob, error) {
	rows, err := r.db.QueryContext(ctx, usageBriefJobSelectSQL+`WHERE j.id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	jobs, err := scanUsageBriefJobs(rows)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	return &jobs[0], nil
}

func (r *usageBriefRepository) GetActiveJobByPeriod(ctx context.Context, userID int64, periodType string, periodStart, periodEnd time.Time) (*service.UsageBriefJob, error) {
	return r.findExistingProductionJob(ctx, userID, periodType, periodStart, periodEnd)
}

func (r *usageBriefRepository) CreateJob(ctx context.Context, job service.UsageBriefJob) (*service.UsageBriefJob, error) {
	rows, err := r.db.QueryContext(ctx, `
INSERT INTO usage_brief_jobs (
	batch_id, job_scope, job_type, status, user_id, group_id, period_type, period_start,
	period_end, range_start, range_end, progress_current, progress_total,
	created_by, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8::date,
	$9::date, $10, $11, $12, $13,
	$14, NOW(), NOW()
)
ON CONFLICT DO NOTHING
RETURNING id
`, nullInt64Ptr(job.BatchID), job.JobScope, job.JobType, coalesceString(job.Status, service.UsageBriefStatusQueued),
		nullInt64Ptr(job.UserID), nullInt64Ptr(job.GroupID), nullStringPtrValue(job.PeriodType),
		nullDatePtr(job.PeriodStart), nullDatePtr(job.PeriodEnd), nullTimePtr(job.RangeStart),
		nullTimePtr(job.RangeEnd), job.ProgressCurrent, job.ProgressTotal, nullInt64Ptr(job.CreatedBy))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var id int64
	if rows.Next() {
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if id == 0 && job.JobScope == service.UsageBriefJobScopeProduction && job.UserID != nil && job.PeriodType != nil && job.PeriodStart != nil && job.PeriodEnd != nil {
		existing, err := r.findExistingProductionJob(ctx, *job.UserID, *job.PeriodType, *job.PeriodStart, *job.PeriodEnd)
		if err != nil || existing != nil {
			return existing, err
		}
	}
	if id == 0 {
		return nil, nil
	}
	return r.GetJob(ctx, id)
}

func (r *usageBriefRepository) findExistingProductionJob(ctx context.Context, userID int64, periodType string, start, end time.Time) (*service.UsageBriefJob, error) {
	rows, err := r.db.QueryContext(ctx, usageBriefJobSelectSQL+`
WHERE j.job_scope = $1
  AND j.user_id = $2
  AND j.period_type = $3
  AND j.period_start = $4::date
  AND j.period_end = $5::date
  AND j.status IN ($6, $7)
  AND j.deleted_at IS NULL
ORDER BY j.id DESC
LIMIT 1
`, service.UsageBriefJobScopeProduction, userID, periodType, dateOnly(start), dateOnly(end), service.UsageBriefStatusQueued, service.UsageBriefStatusRunning)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	jobs, err := scanUsageBriefJobs(rows)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	return &jobs[0], nil
}

func (r *usageBriefRepository) HasProductionJobOrReport(ctx context.Context, userID int64, periodType string, periodStart, periodEnd time.Time) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1 FROM usage_brief_reports
	WHERE user_id = $1 AND period_type = $2 AND period_start = $3::date AND period_end = $4::date
)
OR EXISTS (
	SELECT 1 FROM usage_brief_jobs
	WHERE job_scope = $5 AND user_id = $1 AND period_type = $2 AND period_start = $3::date AND period_end = $4::date
	  AND deleted_at IS NULL
)
`, userID, periodType, dateOnly(periodStart), dateOnly(periodEnd), service.UsageBriefJobScopeProduction).Scan(&exists)
	return exists, err
}

func (r *usageBriefRepository) ClaimQueuedJobs(ctx context.Context, limit int) ([]service.UsageBriefJob, error) {
	if limit <= 0 {
		limit = 1
	}
	rows, err := r.db.QueryContext(ctx, `
WITH picked AS (
	SELECT id
	FROM usage_brief_jobs
	WHERE status = $1
	  AND cancel_requested = FALSE
	  AND deleted_at IS NULL
	  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
	  AND NOT EXISTS (
		SELECT 1 FROM usage_brief_batches b
		WHERE b.id = usage_brief_jobs.batch_id
		  AND (b.deleted_at IS NOT NULL OR b.cancel_requested = TRUE OR b.paused_at IS NOT NULL)
	  )
	ORDER BY created_at ASC, id ASC
	LIMIT $2
	FOR UPDATE SKIP LOCKED
),
updated AS (
	UPDATE usage_brief_jobs j
	SET status = $3, locked_at = NOW(), started_at = COALESCE(started_at, NOW()), updated_at = NOW(), stage = 'running'
	FROM picked
	WHERE j.id = picked.id
	RETURNING j.id
)
`+usageBriefJobSelectSQL+`
JOIN updated ON updated.id = j.id
ORDER BY j.created_at ASC, j.id ASC
`, service.UsageBriefStatusQueued, limit, service.UsageBriefStatusRunning)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanUsageBriefJobs(rows)
}

func (r *usageBriefRepository) RecoverRunningJobs(ctx context.Context) (int64, error) {
	rows, err := r.db.QueryContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2,
	cancel_requested = FALSE,
	next_retry_at = NULL,
	locked_at = NULL,
	started_at = NULL,
	finished_at = NULL,
	stage = 'queued',
	error_message = COALESCE(NULLIF(error_message, ''), 'server restarted while job was running'),
	updated_at = NOW()
WHERE status = $1
  AND deleted_at IS NULL
  AND cancel_requested = FALSE
RETURNING batch_id
`, service.UsageBriefStatusRunning, service.UsageBriefStatusQueued)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	batchSet := map[int64]bool{}
	var recovered int64
	for rows.Next() {
		recovered++
		var batchID sql.NullInt64
		if err := rows.Scan(&batchID); err != nil {
			return recovered, err
		}
		if batchID.Valid {
			batchSet[batchID.Int64] = true
		}
	}
	if err := rows.Err(); err != nil {
		return recovered, err
	}
	for batchID := range batchSet {
		if err := r.UpdateBatchStats(ctx, batchID); err != nil {
			return recovered, err
		}
	}
	return recovered, nil
}

func (r *usageBriefRepository) MarkJobRunning(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2, started_at = COALESCE(started_at, NOW()), locked_at = NOW(), updated_at = NOW(), stage = 'running'
WHERE id = $1
`, id, service.UsageBriefStatusRunning)
	return err
}

func (r *usageBriefRepository) MarkJobPaused(ctx context.Context, id int64, stage string) error {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		stage = "paused"
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2,
	stage = $3,
	locked_at = NULL,
	next_retry_at = NULL,
	updated_at = NOW()
WHERE id = $1
`, id, service.UsageBriefStatusPaused, stage)
	return err
}

func (r *usageBriefRepository) UpdateJobProgress(ctx context.Context, id int64, current, total int, stage string, chunkCurrent, chunkTotal, tokenProcessed, tokenTotal, inputTokens, outputTokens int) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET progress_current = $2,
	progress_total = $3,
	stage = $4,
	chunk_current = $5,
	chunk_total = $6,
	token_estimated_processed = $7,
	token_estimated_total = $8,
	input_tokens = $9,
	output_tokens = $10,
	updated_at = NOW()
WHERE id = $1
`, id, current, total, strings.TrimSpace(stage), chunkCurrent, chunkTotal, tokenProcessed, tokenTotal, inputTokens, outputTokens)
	return err
}

func (r *usageBriefRepository) CompleteJob(ctx context.Context, id int64, reportID *int64, resultMD, status string) error {
	status = coalesceString(status, service.UsageBriefStatusSucceeded)
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2, report_id = $3, result_md = $4, error_message = NULL,
	progress_current = GREATEST(progress_current, progress_total),
	cancel_requested = FALSE, next_retry_at = NULL, stage = 'completed', finished_at = NOW(), updated_at = NOW()
WHERE id = $1
`, id, status, nullInt64Ptr(reportID), nullStringValue(resultMD))
	return err
}

func (r *usageBriefRepository) FailJob(ctx context.Context, id int64, errMsg string) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2, error_message = $3, next_retry_at = NULL, stage = 'failed', finished_at = NOW(), updated_at = NOW()
WHERE id = $1
`, id, service.UsageBriefStatusFailed, strings.TrimSpace(errMsg))
	return err
}

func (r *usageBriefRepository) ScheduleJobRetry(ctx context.Context, id int64, errMsg string, nextRetryAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2,
	retry_count = retry_count + 1,
	next_retry_at = $3,
	error_message = $4,
	locked_at = NULL,
	started_at = NULL,
	finished_at = NULL,
	stage = 'waiting_retry',
	updated_at = NOW()
WHERE id = $1
`, id, service.UsageBriefStatusQueued, nextRetryAt, strings.TrimSpace(errMsg))
	return err
}

func (r *usageBriefRepository) ScheduleJobDependencyWait(ctx context.Context, id int64, errMsg string, nextRetryAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2,
	next_retry_at = $3,
	error_message = $4,
	locked_at = NULL,
	started_at = NULL,
	finished_at = NULL,
	stage = 'waiting_dependencies',
	updated_at = NOW()
WHERE id = $1
`, id, service.UsageBriefStatusQueued, nextRetryAt, strings.TrimSpace(errMsg))
	return err
}

func (r *usageBriefRepository) MarkJobCanceled(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2, cancel_requested = TRUE, stage = 'canceled', finished_at = NOW(), updated_at = NOW()
WHERE id = $1
`, id, service.UsageBriefStatusCanceled)
	return err
}

func (r *usageBriefRepository) ClearJobCancelRequest(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET cancel_requested = FALSE, updated_at = NOW()
WHERE id = $1
`, id)
	return err
}

func (r *usageBriefRepository) CancelJob(ctx context.Context, id int64) (*service.UsageBriefJob, error) {
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET cancel_requested = TRUE,
	status = CASE WHEN status = $2 THEN $3 ELSE status END,
	finished_at = CASE WHEN status = $2 THEN NOW() ELSE finished_at END,
	updated_at = NOW()
WHERE id = $1 AND status IN ($2, $4)
`, id, service.UsageBriefStatusQueued, service.UsageBriefStatusCanceled, service.UsageBriefStatusRunning)
	if err != nil {
		return nil, err
	}
	return r.GetJob(ctx, id)
}

func (r *usageBriefRepository) ResetJob(ctx context.Context, id int64) (*service.UsageBriefJob, error) {
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2,
	cancel_requested = FALSE,
	progress_current = 0,
	chunk_current = 0,
	token_estimated_processed = 0,
	error_message = NULL,
	next_retry_at = NULL,
	stage = 'queued',
	locked_at = NULL,
	started_at = NULL,
	finished_at = NULL,
	retry_count = 0,
	updated_at = NOW()
WHERE id = $1 AND status IN ($3, $4, $5)
`, id, service.UsageBriefStatusQueued, service.UsageBriefStatusFailed, service.UsageBriefStatusCanceled, service.UsageBriefStatusRunning)
	if err != nil {
		return nil, err
	}
	return r.GetJob(ctx, id)
}

func (r *usageBriefRepository) RerunJob(ctx context.Context, id int64) (*service.UsageBriefJob, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `
DELETE FROM usage_brief_job_chunks
WHERE job_id = $1
`, id); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM usage_brief_job_conversations
WHERE job_id = $1
`, id); err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET status = $2,
	cancel_requested = FALSE,
	progress_current = 0,
	progress_total = 0,
	chunk_current = 0,
	chunk_total = 0,
	token_estimated_total = 0,
	token_estimated_processed = 0,
	input_tokens = 0,
	output_tokens = 0,
	error_message = NULL,
	next_retry_at = NULL,
	stage = 'queued',
	locked_at = NULL,
	started_at = NULL,
	finished_at = NULL,
	retry_count = 0,
	report_id = NULL,
	result_md = NULL,
	updated_at = NOW()
WHERE id = $1 AND status IN ($3, $4, $5, $6)
`, id, service.UsageBriefStatusQueued, service.UsageBriefStatusFailed, service.UsageBriefStatusCanceled,
		service.UsageBriefStatusRunning, service.UsageBriefStatusSucceeded)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return r.GetJob(ctx, id)
}

func (r *usageBriefRepository) DeleteJob(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `
UPDATE usage_brief_jobs
SET deleted_at = COALESCE(deleted_at, NOW()),
	cancel_requested = CASE WHEN status IN ($2, $3) THEN TRUE ELSE cancel_requested END,
	status = CASE WHEN status = $2 THEN $4 ELSE status END,
	finished_at = CASE WHEN status = $2 THEN COALESCE(finished_at, NOW()) ELSE finished_at END,
	updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
`, id, service.UsageBriefStatusQueued, service.UsageBriefStatusRunning, service.UsageBriefStatusCanceled)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM usage_brief_jobs WHERE id = $1)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return sql.ErrNoRows
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_brief_job_chunks WHERE job_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_brief_job_conversations WHERE job_id = $1`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *usageBriefRepository) IsJobCancelRequested(ctx context.Context, id int64) (bool, error) {
	var cancelRequested bool
	err := r.db.QueryRowContext(ctx, `
SELECT
	j.cancel_requested
	OR j.deleted_at IS NOT NULL
	OR COALESCE(b.cancel_requested, FALSE)
	OR b.deleted_at IS NOT NULL
FROM usage_brief_jobs j
LEFT JOIN usage_brief_batches b ON b.id = j.batch_id
WHERE j.id = $1
`, id).Scan(&cancelRequested)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return cancelRequested, err
}

func (r *usageBriefRepository) DeleteJobChunks(ctx context.Context, jobID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_brief_job_chunks WHERE job_id = $1`, jobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_brief_job_conversations WHERE job_id = $1`, jobID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *usageBriefRepository) DeleteStaleJobChunks(ctx context.Context, jobID int64, chunkType string, maxChunkIndex int) error {
	_, err := r.db.ExecContext(ctx, `
DELETE FROM usage_brief_job_chunks
WHERE job_id = $1 AND chunk_type = $2 AND chunk_index > $3
`, jobID, chunkType, maxChunkIndex)
	return err
}

func (r *usageBriefRepository) GetJobChunk(ctx context.Context, jobID int64, chunkIndex int, chunkType string) (*service.UsageBriefJobChunk, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
	id, job_id, chunk_index, chunk_type, status, token_estimated,
	input_tokens, output_tokens, retry_count, content_json::text, conversation_json::text, summary_md, error_message,
	last_error_at, started_at, finished_at, created_at, updated_at
FROM usage_brief_job_chunks
WHERE job_id = $1 AND chunk_index = $2 AND chunk_type = $3
LIMIT 1
`, jobID, chunkIndex, chunkType)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	chunks, err := scanUsageBriefJobChunks(rows)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	return &chunks[0], nil
}

func (r *usageBriefRepository) ListJobChunks(ctx context.Context, jobID int64, filter service.UsageBriefJobChunkFilter) ([]service.UsageBriefJobChunk, int64, error) {
	chunkType := strings.TrimSpace(filter.ChunkType)
	if chunkType == "" {
		chunkType = service.UsageBriefChunkTypeSource
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM usage_brief_job_chunks
WHERE job_id = $1 AND chunk_type = $2
`, jobID, chunkType).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	rows, err := r.db.QueryContext(ctx, `
SELECT
	id, job_id, chunk_index, chunk_type, status, token_estimated,
	input_tokens, output_tokens, retry_count, content_json::text, conversation_json::text, summary_md, error_message,
	last_error_at, started_at, finished_at, created_at, updated_at
FROM usage_brief_job_chunks
WHERE job_id = $1 AND chunk_type = $2
ORDER BY chunk_index ASC
LIMIT $3 OFFSET $4
`, jobID, chunkType, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	chunks, err := scanUsageBriefJobChunks(rows)
	return chunks, total, err
}

func (r *usageBriefRepository) ListJobChunkSummaries(ctx context.Context, jobID int64, filter service.UsageBriefJobChunkFilter) ([]service.UsageBriefJobChunk, int64, error) {
	chunkType := strings.TrimSpace(filter.ChunkType)
	if chunkType == "" {
		chunkType = service.UsageBriefChunkTypeSource
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM usage_brief_job_chunks
WHERE job_id = $1 AND chunk_type = $2
`, jobID, chunkType).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	rows, err := r.db.QueryContext(ctx, `
SELECT
	id, job_id, chunk_index, chunk_type, status, token_estimated,
	input_tokens, output_tokens, retry_count, '{}'::jsonb::text, '{}'::jsonb::text, summary_md, error_message,
	last_error_at, started_at, finished_at, created_at, updated_at
FROM usage_brief_job_chunks
WHERE job_id = $1 AND chunk_type = $2
ORDER BY chunk_index ASC
LIMIT $3 OFFSET $4
`, jobID, chunkType, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	chunks, err := scanUsageBriefJobChunks(rows)
	return chunks, total, err
}

func (r *usageBriefRepository) UpsertJobChunk(ctx context.Context, chunk service.UsageBriefJobChunk) (*service.UsageBriefJobChunk, error) {
	rows, err := r.db.QueryContext(ctx, `
INSERT INTO usage_brief_job_chunks (
	job_id, chunk_index, chunk_type, status, token_estimated,
	input_tokens, output_tokens, retry_count, content_json, conversation_json, summary_md, error_message,
	last_error_at, started_at, finished_at, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5,
	$6, $7, $8, COALESCE(NULLIF($9, '')::jsonb, '{}'::jsonb), COALESCE(NULLIF($10, '')::jsonb, '{}'::jsonb), $11, $12,
	$13, $14, $15, NOW(), NOW()
)
ON CONFLICT (job_id, chunk_index, chunk_type) DO UPDATE SET
	status = EXCLUDED.status,
	token_estimated = EXCLUDED.token_estimated,
	input_tokens = EXCLUDED.input_tokens,
	output_tokens = EXCLUDED.output_tokens,
	retry_count = EXCLUDED.retry_count,
	content_json = EXCLUDED.content_json,
	conversation_json = EXCLUDED.conversation_json,
	summary_md = EXCLUDED.summary_md,
	error_message = EXCLUDED.error_message,
	last_error_at = EXCLUDED.last_error_at,
	started_at = EXCLUDED.started_at,
	finished_at = EXCLUDED.finished_at,
	updated_at = NOW()
RETURNING
	id, job_id, chunk_index, chunk_type, status, token_estimated,
	input_tokens, output_tokens, retry_count, content_json::text, conversation_json::text, summary_md, error_message,
	last_error_at, started_at, finished_at, created_at, updated_at
`, chunk.JobID, chunk.ChunkIndex, coalesceString(chunk.ChunkType, service.UsageBriefChunkTypeSource),
		coalesceString(chunk.Status, service.UsageBriefStatusQueued), chunk.TokenEstimated,
		chunk.InputTokens, chunk.OutputTokens, chunk.RetryCount, strings.TrimSpace(chunk.ContentJSON),
		strings.TrimSpace(chunk.ConversationJSON), chunk.SummaryMD, nullStringValue(chunk.ErrorMessage),
		nullTimePtr(chunk.LastErrorAt), nullTimePtr(chunk.StartedAt), nullTimePtr(chunk.FinishedAt))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	chunks, err := scanUsageBriefJobChunks(rows)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	return &chunks[0], nil
}

func (r *usageBriefRepository) DeleteJobConversations(ctx context.Context, jobID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM usage_brief_job_conversations WHERE job_id = $1`, jobID)
	return err
}

func (r *usageBriefRepository) DeleteStaleJobConversations(ctx context.Context, jobID int64, maxConversationIndex int) error {
	_, err := r.db.ExecContext(ctx, `
DELETE FROM usage_brief_job_conversations
WHERE job_id = $1 AND conversation_index > $2
`, jobID, maxConversationIndex)
	return err
}

func (r *usageBriefRepository) ListJobConversations(ctx context.Context, jobID int64, filter service.UsageBriefJobConversationFilter) ([]service.UsageBriefJobConversation, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM usage_brief_job_conversations
WHERE job_id = $1
`, jobID).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	rows, err := r.db.QueryContext(ctx, `
SELECT
	id, job_id, conversation_index, usage_log_id,
	covered_usage_log_ids, covered_request_count, token_estimated,
	conversation_json::text, created_at, updated_at
FROM usage_brief_job_conversations
WHERE job_id = $1
ORDER BY conversation_index ASC
LIMIT $2 OFFSET $3
`, jobID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	conversations, err := scanUsageBriefJobConversations(rows)
	return conversations, total, err
}

func (r *usageBriefRepository) UpsertJobConversation(ctx context.Context, conversation service.UsageBriefJobConversation) (*service.UsageBriefJobConversation, error) {
	rows, err := r.db.QueryContext(ctx, `
INSERT INTO usage_brief_job_conversations (
	job_id, conversation_index, usage_log_id, covered_usage_log_ids,
	covered_request_count, token_estimated, conversation_json,
	created_at, updated_at
) VALUES (
	$1, $2, $3, $4,
	$5, $6, COALESCE(NULLIF($7, '')::jsonb, '{}'::jsonb),
	NOW(), NOW()
)
ON CONFLICT (job_id, conversation_index) DO UPDATE SET
	usage_log_id = EXCLUDED.usage_log_id,
	covered_usage_log_ids = EXCLUDED.covered_usage_log_ids,
	covered_request_count = EXCLUDED.covered_request_count,
	token_estimated = EXCLUDED.token_estimated,
	conversation_json = EXCLUDED.conversation_json,
	updated_at = NOW()
RETURNING
	id, job_id, conversation_index, usage_log_id,
	covered_usage_log_ids, covered_request_count, token_estimated,
	conversation_json::text, created_at, updated_at
`, conversation.JobID, conversation.ConversationIndex, conversation.UsageLogID,
		pq.Array(conversation.CoveredUsageLogIDs), conversation.CoveredRequestCount,
		conversation.TokenEstimated, strings.TrimSpace(conversation.ConversationJSON))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	conversations, err := scanUsageBriefJobConversations(rows)
	if err != nil {
		return nil, err
	}
	if len(conversations) == 0 {
		return nil, nil
	}
	return &conversations[0], nil
}

func (r *usageBriefRepository) FetchUsageRecords(ctx context.Context, userID int64, groupID *int64, start, end time.Time, limit int) ([]service.UsageBriefSourceRecord, error) {
	args := []any{userID, start, end}
	where := "WHERE ul.user_id = $1 AND ul.created_at >= $2 AND ul.created_at < $3"
	if groupID != nil && *groupID > 0 {
		args = append(args, *groupID)
		where += fmt.Sprintf(" AND ul.group_id = $%d", len(args))
	}
	if limit <= 0 {
		limit = 20000
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `
SELECT
	ul.id, ul.created_at, ul.model, COALESCE(ul.requested_model, ''), COALESCE(ul.upstream_model, ''),
	ul.group_id, COALESCE(ul.request_type, 0),
	ul.input_tokens, ul.output_tokens,
	COALESCE(ul.total_cost, 0)::float8, COALESCE(ul.actual_cost, 0)::float8,
	COALESCE(ul.inbound_endpoint, ''), COALESCE(ul.upstream_endpoint, ''),
	COALESCE(d.request_payload_json, ''),
	COALESCE(d.response_payload_json, ''),
	COALESCE(d.compressed_request_payload_json, ''),
	COALESCE(d.compressed_response_payload_json, '')
FROM usage_logs ul
LEFT JOIN usage_log_details d ON d.usage_log_id = ul.id
`+where+`
ORDER BY ul.created_at ASC, ul.id ASC
LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanUsageBriefSourceRecords(rows)
}

func (r *usageBriefRepository) SummarizeUsageRecords(ctx context.Context, filter service.UsageBriefUsageRecordFilter) (service.UsageBriefUsageSummary, error) {
	where, args := usageBriefRecordWhere(filter)
	rows, err := r.db.QueryContext(ctx, `
SELECT
	COALESCE(NULLIF(ul.requested_model, ''), NULLIF(ul.model, ''), 'unknown') AS model_name,
	COALESCE(ul.request_type, 0) AS request_type,
	COUNT(*)::bigint,
	COALESCE(SUM(ul.input_tokens), 0)::bigint,
	COALESCE(SUM(ul.output_tokens), 0)::bigint,
	COALESCE(SUM(ul.total_cost), 0)::float8,
	COALESCE(SUM(ul.actual_cost), 0)::float8
FROM usage_logs ul
`+where+`
GROUP BY model_name, request_type
ORDER BY model_name ASC, request_type ASC
`, args...)
	if err != nil {
		return service.UsageBriefUsageSummary{}, err
	}
	defer func() { _ = rows.Close() }()
	summary := service.UsageBriefUsageSummary{
		ModelCounts:       map[string]int{},
		RequestTypeCounts: map[string]int{},
	}
	for rows.Next() {
		var (
			modelName    string
			requestType  int16
			count        int64
			inputTokens  int64
			outputTokens int64
			totalCost    float64
			actualCost   float64
		)
		if err := rows.Scan(&modelName, &requestType, &count, &inputTokens, &outputTokens, &totalCost, &actualCost); err != nil {
			return service.UsageBriefUsageSummary{}, err
		}
		countInt := int(count)
		summary.RequestCount += countInt
		summary.InputTokens += int(inputTokens)
		summary.OutputTokens += int(outputTokens)
		summary.TotalCost += totalCost
		summary.ActualCost += actualCost
		summary.ModelCounts[modelName] += countInt
		summary.RequestTypeCounts[service.RequestTypeFromInt16(requestType).String()] += countInt
	}
	return summary, rows.Err()
}

func (r *usageBriefRepository) FetchUsageRecordsPage(ctx context.Context, filter service.UsageBriefUsageRecordFilter) ([]service.UsageBriefSourceRecord, error) {
	where, args := usageBriefRecordWhere(filter)
	if filter.AfterCreatedAt != nil && !filter.AfterCreatedAt.IsZero() {
		args = append(args, *filter.AfterCreatedAt, filter.AfterID)
		where += fmt.Sprintf(" AND (ul.created_at, ul.id) > ($%d, $%d)", len(args)-1, len(args))
	}
	orderBy := "ORDER BY ul.created_at ASC, ul.id ASC"
	if filter.BeforeCreatedAt != nil && !filter.BeforeCreatedAt.IsZero() {
		args = append(args, *filter.BeforeCreatedAt, filter.BeforeID)
		where += fmt.Sprintf(" AND (ul.created_at, ul.id) < ($%d, $%d)", len(args)-1, len(args))
		orderBy = "ORDER BY ul.created_at DESC, ul.id DESC"
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `
SELECT
	ul.id, ul.created_at, ul.model, COALESCE(ul.requested_model, ''), COALESCE(ul.upstream_model, ''),
	ul.group_id, COALESCE(ul.request_type, 0),
	ul.input_tokens, ul.output_tokens,
	COALESCE(ul.total_cost, 0)::float8, COALESCE(ul.actual_cost, 0)::float8,
	COALESCE(ul.inbound_endpoint, ''), COALESCE(ul.upstream_endpoint, ''),
	COALESCE(d.request_payload_json, ''),
	COALESCE(d.response_payload_json, ''),
	COALESCE(d.compressed_request_payload_json, ''),
	COALESCE(d.compressed_response_payload_json, '')
FROM usage_logs ul
LEFT JOIN usage_log_details d ON d.usage_log_id = ul.id
`+where+`
`+orderBy+`
LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanUsageBriefSourceRecords(rows)
}

func (r *usageBriefRepository) UpdateUsageRecordCompressedPayloads(ctx context.Context, usageLogID int64, compressedRequestJSON *string, compressedResponseJSON *string) error {
	if usageLogID <= 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE usage_log_details
SET compressed_request_payload_json = COALESCE($2, compressed_request_payload_json),
	compressed_response_payload_json = COALESCE($3, compressed_response_payload_json),
	updated_at = NOW()
WHERE usage_log_id = $1
`, usageLogID, nullStringPtrValue(compressedRequestJSON), nullStringPtrValue(compressedResponseJSON))
	return err
}

func usageBriefRecordWhere(filter service.UsageBriefUsageRecordFilter) (string, []any) {
	args := []any{filter.UserID, filter.Start, filter.End}
	where := "WHERE ul.user_id = $1 AND ul.created_at >= $2 AND ul.created_at < $3"
	if filter.GroupID != nil && *filter.GroupID > 0 {
		args = append(args, *filter.GroupID)
		where += fmt.Sprintf(" AND ul.group_id = $%d", len(args))
	}
	return where, args
}

func (r *usageBriefRepository) ListDailyReports(ctx context.Context, userID int64, start, end time.Time) ([]service.UsageBriefReport, error) {
	return r.listReportsByPeriodRange(ctx, userID, service.UsageBriefPeriodDaily, start, end, false)
}

func (r *usageBriefRepository) ListWeeklyReportsForMonth(ctx context.Context, userID int64, monthStart, monthEnd time.Time) ([]service.UsageBriefReport, error) {
	return r.listReportsByPeriodRange(ctx, userID, service.UsageBriefPeriodWeekly, monthStart, monthEnd, true)
}

func (r *usageBriefRepository) listReportsByPeriodRange(ctx context.Context, userID int64, periodType string, start, end time.Time, overlap bool) ([]service.UsageBriefReport, error) {
	condition := "r.period_start >= $3::date AND r.period_end <= $4::date"
	if overlap {
		condition = "r.period_start <= $4::date AND r.period_end >= $3::date"
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT
	r.id, r.user_id, COALESCE(u.email, ''), COALESCE(u.username, ''),
	r.period_type, r.period_start, r.period_end, r.status, r.title, r.content_md,
	r.source_kind, r.is_admin_edited, r.edited_by, r.generated_by_job_id,
	r.input_usage_count, r.input_tokens, r.output_tokens, r.error_message,
	r.generated_at, r.created_at, r.updated_at
FROM usage_brief_reports r
LEFT JOIN users u ON u.id = r.user_id
WHERE r.user_id = $1 AND r.period_type = $2 AND r.status IN ($5, $6) AND `+condition+`
ORDER BY r.period_start ASC
`, userID, periodType, dateOnly(start), dateOnly(end), service.UsageBriefStatusSucceeded, service.UsageBriefStatusPartial)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanUsageBriefReports(rows)
}

func scanUsageBriefReports(rows *sql.Rows) ([]service.UsageBriefReport, error) {
	reports := make([]service.UsageBriefReport, 0)
	for rows.Next() {
		var (
			report      service.UsageBriefReport
			editedBy    sql.NullInt64
			generatedBy sql.NullInt64
			errMsg      sql.NullString
			generatedAt sql.NullTime
		)
		if err := rows.Scan(
			&report.ID, &report.UserID, &report.UserEmail, &report.Username,
			&report.PeriodType, &report.PeriodStart, &report.PeriodEnd, &report.Status,
			&report.Title, &report.ContentMD, &report.SourceKind, &report.IsAdminEdited,
			&editedBy, &generatedBy, &report.InputUsageCount, &report.InputTokens,
			&report.OutputTokens, &errMsg, &generatedAt, &report.CreatedAt, &report.UpdatedAt,
		); err != nil {
			return nil, err
		}
		report.EditedBy = nullInt64ToPtr(editedBy)
		report.GeneratedByJob = nullInt64ToPtr(generatedBy)
		if errMsg.Valid {
			report.ErrorMessage = errMsg.String
		}
		if generatedAt.Valid {
			t := generatedAt.Time
			report.GeneratedAt = &t
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func scanUsageBriefReportGroups(rows *sql.Rows, groupBy string) ([]service.UsageBriefReportGroup, error) {
	groups := make([]service.UsageBriefReportGroup, 0)
	groupIndex := make(map[string]int)
	for rows.Next() {
		var (
			key         string
			report      service.UsageBriefReport
			editedBy    sql.NullInt64
			generatedBy sql.NullInt64
			errMsg      sql.NullString
			generatedAt sql.NullTime
		)
		if err := rows.Scan(
			&key,
			&report.ID, &report.UserID, &report.UserEmail, &report.Username,
			&report.PeriodType, &report.PeriodStart, &report.PeriodEnd, &report.Status,
			&report.Title, &report.ContentMD, &report.SourceKind, &report.IsAdminEdited,
			&editedBy, &generatedBy, &report.InputUsageCount, &report.InputTokens,
			&report.OutputTokens, &errMsg, &generatedAt, &report.CreatedAt, &report.UpdatedAt,
		); err != nil {
			return nil, err
		}
		report.EditedBy = nullInt64ToPtr(editedBy)
		report.GeneratedByJob = nullInt64ToPtr(generatedBy)
		if errMsg.Valid {
			report.ErrorMessage = errMsg.String
		}
		if generatedAt.Valid {
			t := generatedAt.Time
			report.GeneratedAt = &t
		}

		idx, ok := groupIndex[key]
		if !ok {
			group := service.UsageBriefReportGroup{
				Key:     key,
				GroupBy: groupBy,
				Reports: make([]service.UsageBriefReport, 0),
			}
			if groupBy == service.UsageBriefReportGroupByUser {
				userID := report.UserID
				group.UserID = &userID
				group.UserEmail = report.UserEmail
				group.Username = report.Username
				group.Label = report.UserEmail
				if group.Label == "" {
					group.Label = fmt.Sprintf("用户 #%d", report.UserID)
				}
			} else {
				periodStart := report.PeriodStart
				periodEnd := report.PeriodEnd
				group.PeriodType = report.PeriodType
				group.PeriodStart = &periodStart
				group.PeriodEnd = &periodEnd
				group.Label = usageBriefPeriodGroupLabel(report.PeriodType, periodStart, periodEnd)
			}
			groups = append(groups, group)
			idx = len(groups) - 1
			groupIndex[key] = idx
		}
		group := &groups[idx]
		group.ReportCount++
		group.InputUsageCount += report.InputUsageCount
		group.InputTokens += report.InputTokens
		group.OutputTokens += report.OutputTokens
		group.Reports = append(group.Reports, report)
	}
	for i := range groups {
		if groups[i].GroupBy == service.UsageBriefReportGroupByPeriod {
			users := make(map[int64]struct{}, len(groups[i].Reports))
			for _, report := range groups[i].Reports {
				users[report.UserID] = struct{}{}
			}
			groups[i].UserCount = len(users)
		} else {
			groups[i].UserCount = 1
		}
	}
	return groups, rows.Err()
}

func usageBriefPeriodGroupLabel(periodType string, start, end time.Time) string {
	switch periodType {
	case service.UsageBriefPeriodWeekly:
		return fmt.Sprintf("周报 %s 至 %s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	case service.UsageBriefPeriodMonthly:
		return fmt.Sprintf("月报 %s 至 %s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	default:
		return fmt.Sprintf("日报 %s", start.Format("2006-01-02"))
	}
}

func scanUsageBriefBatches(rows *sql.Rows) ([]service.UsageBriefBatch, error) {
	batches := make([]service.UsageBriefBatch, 0)
	for rows.Next() {
		var (
			batch       service.UsageBriefBatch
			notes       sql.NullString
			periodType  sql.NullString
			periodStart sql.NullTime
			periodEnd   sql.NullTime
			rangeStart  sql.NullTime
			rangeEnd    sql.NullTime
			pausedAt    sql.NullTime
			deletedAt   sql.NullTime
			createdBy   sql.NullInt64
		)
		if err := rows.Scan(
			&batch.ID, &batch.BatchScope, &batch.TriggerKind, &batch.Status, &batch.Title, &notes,
			&periodType, &periodStart, &periodEnd, &rangeStart, &rangeEnd,
			&batch.JobCount, &batch.SucceededCount, &batch.FailedCount, &batch.CanceledCount,
			&batch.RunningCount, &batch.QueuedCount, &batch.RetryingCount,
			&batch.TokenEstimatedTotal, &batch.TokenEstimatedProcessed, &batch.InputTokens, &batch.OutputTokens,
			&batch.CancelRequested, &pausedAt, &deletedAt, &createdBy, &batch.CreatedAt, &batch.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if notes.Valid {
			batch.Notes = notes.String
		}
		if periodType.Valid {
			v := periodType.String
			batch.PeriodType = &v
		}
		batch.PeriodStart = nullTimeToPtr(periodStart)
		batch.PeriodEnd = nullTimeToPtr(periodEnd)
		batch.RangeStart = nullTimeToPtr(rangeStart)
		batch.RangeEnd = nullTimeToPtr(rangeEnd)
		batch.PausedAt = nullTimeToPtr(pausedAt)
		batch.DeletedAt = nullTimeToPtr(deletedAt)
		batch.CreatedBy = nullInt64ToPtr(createdBy)
		batches = append(batches, batch)
	}
	return batches, rows.Err()
}

func scanUsageBriefJobs(rows *sql.Rows) ([]service.UsageBriefJob, error) {
	jobs := make([]service.UsageBriefJob, 0)
	for rows.Next() {
		var (
			job         service.UsageBriefJob
			batchID     sql.NullInt64
			userID      sql.NullInt64
			groupID     sql.NullInt64
			periodType  sql.NullString
			periodStart sql.NullTime
			periodEnd   sql.NullTime
			rangeStart  sql.NullTime
			rangeEnd    sql.NullTime
			nextRetryAt sql.NullTime
			stage       sql.NullString
			reportID    sql.NullInt64
			resultMD    sql.NullString
			errMsg      sql.NullString
			lockedAt    sql.NullTime
			startedAt   sql.NullTime
			finishedAt  sql.NullTime
			createdBy   sql.NullInt64
			deletedAt   sql.NullTime
		)
		if err := rows.Scan(
			&job.ID, &batchID, &job.JobScope, &job.JobType, &job.Status, &userID,
			&job.UserEmail, &job.Username, &groupID, &job.GroupName,
			&periodType, &periodStart, &periodEnd, &rangeStart, &rangeEnd,
			&job.ProgressCurrent, &job.ProgressTotal, &job.CancelRequested, &job.RetryCount,
			&nextRetryAt, &stage, &job.ChunkCurrent, &job.ChunkTotal,
			&job.TokenEstimatedTotal, &job.TokenEstimatedProcessed, &job.InputTokens, &job.OutputTokens,
			&reportID, &resultMD, &errMsg, &lockedAt, &startedAt, &finishedAt,
			&createdBy, &deletedAt, &job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			return nil, err
		}
		job.BatchID = nullInt64ToPtr(batchID)
		job.UserID = nullInt64ToPtr(userID)
		job.GroupID = nullInt64ToPtr(groupID)
		if periodType.Valid {
			v := periodType.String
			job.PeriodType = &v
		}
		job.PeriodStart = nullTimeToPtr(periodStart)
		job.PeriodEnd = nullTimeToPtr(periodEnd)
		job.RangeStart = nullTimeToPtr(rangeStart)
		job.RangeEnd = nullTimeToPtr(rangeEnd)
		job.NextRetryAt = nullTimeToPtr(nextRetryAt)
		if stage.Valid {
			job.Stage = stage.String
		}
		job.ReportID = nullInt64ToPtr(reportID)
		if resultMD.Valid {
			job.ResultMD = resultMD.String
		}
		if errMsg.Valid {
			job.ErrorMessage = errMsg.String
		}
		job.LockedAt = nullTimeToPtr(lockedAt)
		job.StartedAt = nullTimeToPtr(startedAt)
		job.FinishedAt = nullTimeToPtr(finishedAt)
		job.CreatedBy = nullInt64ToPtr(createdBy)
		job.DeletedAt = nullTimeToPtr(deletedAt)
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func scanUsageBriefJobChunks(rows *sql.Rows) ([]service.UsageBriefJobChunk, error) {
	chunks := make([]service.UsageBriefJobChunk, 0)
	for rows.Next() {
		var (
			chunk      service.UsageBriefJobChunk
			summaryMD  sql.NullString
			errMsg     sql.NullString
			lastErrAt  sql.NullTime
			startedAt  sql.NullTime
			finishedAt sql.NullTime
		)
		if err := rows.Scan(
			&chunk.ID, &chunk.JobID, &chunk.ChunkIndex, &chunk.ChunkType, &chunk.Status,
			&chunk.TokenEstimated, &chunk.InputTokens, &chunk.OutputTokens, &chunk.RetryCount, &chunk.ContentJSON,
			&chunk.ConversationJSON, &summaryMD, &errMsg, &lastErrAt, &startedAt, &finishedAt, &chunk.CreatedAt, &chunk.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if summaryMD.Valid {
			chunk.SummaryMD = summaryMD.String
		}
		if errMsg.Valid {
			chunk.ErrorMessage = errMsg.String
		}
		chunk.LastErrorAt = nullTimeToPtr(lastErrAt)
		chunk.StartedAt = nullTimeToPtr(startedAt)
		chunk.FinishedAt = nullTimeToPtr(finishedAt)
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

func scanUsageBriefJobConversations(rows *sql.Rows) ([]service.UsageBriefJobConversation, error) {
	conversations := make([]service.UsageBriefJobConversation, 0)
	for rows.Next() {
		var conversation service.UsageBriefJobConversation
		if err := rows.Scan(
			&conversation.ID,
			&conversation.JobID,
			&conversation.ConversationIndex,
			&conversation.UsageLogID,
			pq.Array(&conversation.CoveredUsageLogIDs),
			&conversation.CoveredRequestCount,
			&conversation.TokenEstimated,
			&conversation.ConversationJSON,
			&conversation.CreatedAt,
			&conversation.UpdatedAt,
		); err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}
	return conversations, rows.Err()
}

func scanUsageBriefSourceRecords(rows *sql.Rows) ([]service.UsageBriefSourceRecord, error) {
	records := make([]service.UsageBriefSourceRecord, 0)
	for rows.Next() {
		var (
			rec        service.UsageBriefSourceRecord
			groupIDVal sql.NullInt64
			reqType    int16
		)
		if err := rows.Scan(
			&rec.ID, &rec.CreatedAt, &rec.Model, &rec.RequestedModel, &rec.UpstreamModel,
			&groupIDVal, &reqType, &rec.InputTokens, &rec.OutputTokens,
			&rec.TotalCost, &rec.ActualCost, &rec.InboundEndpoint, &rec.UpstreamEndpoint,
			&rec.RequestPayload, &rec.ResponsePayload, &rec.CompressedRequestPayload, &rec.CompressedResponsePayload,
		); err != nil {
			return nil, err
		}
		if groupIDVal.Valid {
			v := groupIDVal.Int64
			rec.GroupID = &v
		}
		rec.RequestType = service.RequestTypeFromInt16(reqType).String()
		records = append(records, rec)
	}
	return records, rows.Err()
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	return page, pageSize
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func coalesceString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func nullInt64ToPtr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

func nullTimeToPtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}

func nullInt64Ptr(value *int64) any {
	if value == nil || *value == 0 {
		return nil
	}
	return *value
}

func nullStringPtrValue(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

func nullStringValue(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullTimePtr(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return *value
}

func nullDatePtr(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return dateOnly(*value)
}
