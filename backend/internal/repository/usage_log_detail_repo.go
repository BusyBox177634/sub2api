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
	COALESCE($9, NOW()),
	COALESCE($10, NOW())
FROM target
ON CONFLICT (usage_log_id) DO UPDATE SET
	request_payload_json = EXCLUDED.request_payload_json,
	response_payload_json = EXCLUDED.response_payload_json,
	request_payload_bytes = EXCLUDED.request_payload_bytes,
	response_payload_bytes = EXCLUDED.response_payload_bytes,
	request_truncated = EXCLUDED.request_truncated,
	response_truncated = EXCLUDED.response_truncated,
	updated_at = EXCLUDED.updated_at
`

	result, err := r.sql.ExecContext(
		ctx,
		query,
		strings.TrimSpace(requestID),
		apiKeyID,
		nullString(detail.RequestPayloadJSON),
		nullString(detail.ResponsePayloadJSON),
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
	request_payload_bytes,
	response_payload_bytes,
	request_truncated,
	response_truncated,
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

	var (
		record               service.UsageLogDetail
		requestPayloadJSON   sql.NullString
		responsePayloadJSON  sql.NullString
		requestPayloadBytes  sql.NullInt64
		responsePayloadBytes sql.NullInt64
		createdAt            time.Time
		updatedAt            time.Time
	)

	if err := rows.Scan(
		&record.UsageLogID,
		&requestPayloadJSON,
		&responsePayloadJSON,
		&requestPayloadBytes,
		&responsePayloadBytes,
		&record.RequestTruncated,
		&record.ResponseTruncated,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	record.RequestPayloadJSON = nullStringPtr(requestPayloadJSON)
	record.ResponsePayloadJSON = nullStringPtr(responsePayloadJSON)
	record.RequestPayloadBytes = nullInt64ToIntPtr(requestPayloadBytes)
	record.ResponsePayloadBytes = nullInt64ToIntPtr(responsePayloadBytes)
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt

	return &record, rows.Err()
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
