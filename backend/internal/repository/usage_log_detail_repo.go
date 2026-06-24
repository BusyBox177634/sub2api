package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type usageLogDetailRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewUsageLogDetailRepository(client *dbent.Client, sqlDB *sql.DB) service.UsageLogDetailRepository {
	return &usageLogDetailRepository{
		client: client,
		sql:    sqlDB,
	}
}

func (r *usageLogDetailRepository) UpsertByRequestAndAPIKey(
	ctx context.Context,
	requestID string,
	apiKeyID int64,
	detail *service.UsageLogDetail,
) error {
	if r == nil || r.sql == nil {
		return errors.New("nil usage log detail repository")
	}
	if detail == nil || strings.TrimSpace(requestID) == "" || apiKeyID <= 0 {
		return nil
	}

	query := `
WITH target AS (
	SELECT id
	FROM usage_logs
	WHERE request_id = $1 AND api_key_id = $2
	LIMIT 1
)
INSERT INTO usage_log_details (
	usage_log_id,
	request_payload_json,
	response_payload_json,
	compressed_request_payload_json,
	compressed_response_payload_json,
	request_payload_bytes,
	response_payload_bytes,
	request_truncated,
	response_truncated,
	created_at,
	updated_at
)
SELECT
	target.id,
	$3,
	$4,
	$5,
	$6,
	$7,
	$8,
	$9,
	$10,
	COALESCE($11, NOW()),
	COALESCE($12, NOW())
FROM target
ON CONFLICT (usage_log_id) DO UPDATE SET
	request_payload_json = EXCLUDED.request_payload_json,
	response_payload_json = EXCLUDED.response_payload_json,
	compressed_request_payload_json = EXCLUDED.compressed_request_payload_json,
	compressed_response_payload_json = EXCLUDED.compressed_response_payload_json,
	request_payload_bytes = EXCLUDED.request_payload_bytes,
	response_payload_bytes = EXCLUDED.response_payload_bytes,
	request_truncated = EXCLUDED.request_truncated,
	response_truncated = EXCLUDED.response_truncated,
	full_payloads_cleaned_at = NULL,
	updated_at = EXCLUDED.updated_at
`

	result, err := r.sql.ExecContext(
		ctx,
		query,
		strings.TrimSpace(requestID),
		apiKeyID,
		nullString(detail.RequestPayloadJSON),
		nullString(detail.ResponsePayloadJSON),
		nullString(detail.CompressedRequestPayloadJSON),
		nullString(detail.CompressedResponsePayloadJSON),
		nullInt(detail.RequestPayloadBytes),
		nullInt(detail.ResponsePayloadBytes),
		detail.RequestTruncated,
		detail.ResponseTruncated,
		nullTimeArg(detail.CreatedAt),
		nullTimeArg(detail.UpdatedAt),
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return service.ErrUsageLogDetailUsageTargetNotReady
	}
	return nil
}

func (r *usageLogDetailRepository) GetByUsageLogID(ctx context.Context, usageLogID int64) (*service.UsageLogDetail, error) {
	if r == nil || r.sql == nil {
		return nil, errors.New("nil usage log detail repository")
	}

	query := `
SELECT
	usage_log_id,
	request_payload_json,
	response_payload_json,
	compressed_request_payload_json,
	compressed_response_payload_json,
	request_payload_bytes,
	response_payload_bytes,
	request_truncated,
	response_truncated,
	full_payloads_cleaned_at,
	created_at,
	updated_at
FROM usage_log_details
WHERE usage_log_id = $1
`
	rows, err := r.sql.QueryContext(ctx, query, usageLogID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	record, err := scanUsageLogDetail(rows)
	if err != nil {
		return nil, err
	}
	return record, rows.Err()
}

func (r *usageLogDetailRepository) UpdateCompressedPayloads(ctx context.Context, usageLogID int64, compressedRequestJSON *string, compressedResponseJSON *string) error {
	if r == nil || r.sql == nil {
		return errors.New("nil usage log detail repository")
	}
	if usageLogID <= 0 {
		return nil
	}
	_, err := r.sql.ExecContext(ctx, `
UPDATE usage_log_details
SET compressed_request_payload_json = $2,
    compressed_response_payload_json = $3,
    updated_at = NOW()
WHERE usage_log_id = $1
`, usageLogID, nullString(compressedRequestJSON), nullString(compressedResponseJSON))
	return err
}

func (r *usageLogDetailRepository) CountFullPayloadCleanupPending(ctx context.Context, start, end time.Time) (int64, error) {
	if r == nil || r.sql == nil {
		return 0, errors.New("nil usage log detail repository")
	}
	var count int64
	err := scanSingleRow(ctx, r.sql, `
SELECT COUNT(*)
FROM usage_log_details
WHERE created_at >= $1
  AND created_at < $2
  AND (request_payload_json IS NOT NULL OR response_payload_json IS NOT NULL)
`, []any{start, end}, &count)
	return count, err
}

func (r *usageLogDetailRepository) ListForFullPayloadCleanup(ctx context.Context, start, end time.Time, limit int) ([]service.UsageLogDetail, error) {
	if r == nil || r.sql == nil {
		return nil, errors.New("nil usage log detail repository")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.sql.QueryContext(ctx, `
SELECT
	usage_log_id,
	request_payload_json,
	response_payload_json,
	compressed_request_payload_json,
	compressed_response_payload_json,
	request_payload_bytes,
	response_payload_bytes,
	request_truncated,
	response_truncated,
	full_payloads_cleaned_at,
	created_at,
	updated_at
FROM usage_log_details
WHERE created_at >= $1
  AND created_at < $2
  AND (request_payload_json IS NOT NULL OR response_payload_json IS NOT NULL)
ORDER BY created_at ASC, usage_log_id ASC
LIMIT $3
`, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	details := make([]service.UsageLogDetail, 0, limit)
	for rows.Next() {
		detail, err := scanUsageLogDetail(rows)
		if err != nil {
			return nil, err
		}
		details = append(details, *detail)
	}
	return details, rows.Err()
}

func (r *usageLogDetailRepository) ClearFullPayloads(ctx context.Context, usageLogID int64, compressedRequestJSON *string, compressedResponseJSON *string, cleanedAt time.Time) error {
	if r == nil || r.sql == nil {
		return errors.New("nil usage log detail repository")
	}
	if usageLogID <= 0 {
		return nil
	}
	_, err := r.sql.ExecContext(ctx, `
UPDATE usage_log_details
SET request_payload_json = NULL,
    response_payload_json = NULL,
    compressed_request_payload_json = COALESCE($2, compressed_request_payload_json),
    compressed_response_payload_json = COALESCE($3, compressed_response_payload_json),
    full_payloads_cleaned_at = $4,
    updated_at = NOW()
WHERE usage_log_id = $1
`, usageLogID, nullString(compressedRequestJSON), nullString(compressedResponseJSON), cleanedAt)
	return err
}

type usageLogDetailScanner interface {
	Scan(dest ...any) error
}

func scanUsageLogDetail(scanner usageLogDetailScanner) (*service.UsageLogDetail, error) {
	var (
		record                 service.UsageLogDetail
		requestPayloadJSON     sql.NullString
		responsePayloadJSON    sql.NullString
		compressedRequestJSON  sql.NullString
		compressedResponseJSON sql.NullString
		requestPayloadBytes    sql.NullInt64
		responsePayloadBytes   sql.NullInt64
		fullPayloadsCleanedAt  sql.NullTime
		createdAt              time.Time
		updatedAt              time.Time
	)
	if err := scanner.Scan(
		&record.UsageLogID,
		&requestPayloadJSON,
		&responsePayloadJSON,
		&compressedRequestJSON,
		&compressedResponseJSON,
		&requestPayloadBytes,
		&responsePayloadBytes,
		&record.RequestTruncated,
		&record.ResponseTruncated,
		&fullPayloadsCleanedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	record.RequestPayloadJSON = nullStringPtr(requestPayloadJSON)
	record.ResponsePayloadJSON = nullStringPtr(responsePayloadJSON)
	record.CompressedRequestPayloadJSON = nullStringPtr(compressedRequestJSON)
	record.CompressedResponsePayloadJSON = nullStringPtr(compressedResponseJSON)
	record.RequestPayloadBytes = nullInt64ToIntPtr(requestPayloadBytes)
	record.ResponsePayloadBytes = nullInt64ToIntPtr(responsePayloadBytes)
	record.FullPayloadsCleanedAt = nullTimeToPtr(fullPayloadsCleanedAt)
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt
	return &record, nil
}

func nullTimeArg(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullInt64ToIntPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copied := value.String
	return &copied
}
