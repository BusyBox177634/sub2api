package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryInsertSystemMetricsStoresDiskMountsJSON(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := &opsRepository{db: db}
	createdAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	input := &service.OpsInsertSystemMetricsInput{
		CreatedAt:     createdAt,
		WindowMinutes: 1,
		DiskMounts: []service.OpsDiskMountMetric{
			{MountPoint: "/", Device: "overlay", FSType: "overlay", TotalMB: 1000, UsedMB: 400, FreeMB: 600, UsagePercent: 40.0},
		},
	}

	mock.ExpectExec(`INSERT INTO ops_system_metrics`).
		WithArgs(
			createdAt,
			1,
			sql.NullString{},
			sql.NullInt64{},
			int64(0),
			int64(0),
			int64(0),
			int64(0),
			int64(0),
			int64(0),
			int64(0),
			int64(0),
			int64(0),
			sql.NullFloat64{},
			sql.NullFloat64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullFloat64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullFloat64{},
			sql.NullInt64{},
			sql.NullFloat64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullFloat64{},
			sqlmock.AnyArg(),
			sql.NullBool{},
			sql.NullBool{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullInt64{},
			sql.NullInt64{},
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, repo.InsertSystemMetrics(context.Background(), input))
	require.NoError(t, mock.ExpectationsWereMet())

	raw, err := marshalOpsDiskMounts(input.DiskMounts)
	require.NoError(t, err)
	var stored []service.OpsDiskMountMetric
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	require.Equal(t, input.DiskMounts, stored)
}

func TestOpsRepositoryGetLatestSystemMetricsLoadsDiskMounts(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := &opsRepository{db: db}
	createdAt := time.Date(2026, 6, 23, 12, 1, 0, 0, time.UTC)
	diskMounts := []service.OpsDiskMountMetric{
		{MountPoint: "/", Device: "overlay", FSType: "overlay", TotalMB: 2048, UsedMB: 1024, FreeMB: 1024, UsagePercent: 50.0},
		{MountPoint: "/data", Device: "/dev/vdb", FSType: "xfs", TotalMB: 4096, UsedMB: 1024, FreeMB: 3072, UsagePercent: 25.0},
	}
	raw, err := json.Marshal(diskMounts)
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT\s+id,\s+created_at,\s+window_minutes,`).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"created_at",
			"window_minutes",
			"cpu_usage_percent",
			"memory_used_mb",
			"memory_total_mb",
			"memory_usage_percent",
			"disk_mounts_json",
			"db_ok",
			"redis_ok",
			"redis_conn_total",
			"redis_conn_idle",
			"db_conn_active",
			"db_conn_idle",
			"db_conn_waiting",
			"goroutine_count",
			"concurrency_queue_depth",
			"account_switch_count",
		}).AddRow(
			int64(7),
			createdAt,
			1,
			nil,
			nil,
			nil,
			nil,
			raw,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		))

	snapshot, err := repo.GetLatestSystemMetrics(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, int64(7), snapshot.ID)
	require.Equal(t, diskMounts, snapshot.DiskMounts)
	require.NoError(t, mock.ExpectationsWereMet())
}
