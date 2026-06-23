package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWriteOpenAIFastPolicyBlockedResponseMarksBusinessLimited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	writeOpenAIFastPolicyBlockedResponse(c, &OpenAIFastBlockedError{Message: "custom fast policy block"})

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.True(t, HasOpsClientBusinessLimited(c))
	reason, ok := c.Get(OpsClientBusinessLimitedReasonKey)
	require.True(t, ok)
	require.Equal(t, OpsClientBusinessLimitedReasonLocalPolicyDenied, reason)
}

func TestOpsMetricsCollectorQueryErrorCountsExcludesCountTokens(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	collector := &OpsMetricsCollector{db: db}
	start := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectQuery(`(?s)FROM ops_error_logs\s+WHERE created_at >= \$1 AND created_at < \$2\s+AND is_count_tokens = FALSE`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"error_total",
			"business_limited",
			"error_sla",
			"upstream_excl",
			"upstream_429",
			"upstream_529",
		}).AddRow(int64(5), int64(2), int64(3), int64(1), int64(1), int64(1)))

	errorTotal, businessLimited, errorSLA, upstreamExcl429529, upstream429, upstream529, err := collector.queryErrorCounts(context.Background(), start, end)
	require.NoError(t, err)
	require.Equal(t, int64(5), errorTotal)
	require.Equal(t, int64(2), businessLimited)
	require.Equal(t, int64(3), errorSLA)
	require.Equal(t, int64(1), upstreamExcl429529)
	require.Equal(t, int64(1), upstream429)
	require.Equal(t, int64(1), upstream529)
	require.NoError(t, mock.ExpectationsWereMet())
	mock.ExpectClose()
	require.NoError(t, db.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollectVisibleDiskMountMetricsFiltersAndSortsMounts(t *testing.T) {
	dataDir := t.TempDir()
	hostDir := t.TempDir()
	runDir := t.TempDir()
	fileMount := dataDir + "/file"
	require.NoError(t, os.WriteFile(fileMount, []byte("not-dir"), 0o644))

	partitions := []opsDiskPartition{
		{device: "tmpfs", mountPoint: "/proc", fstype: "proc"},
		{device: "tmpfs", mountPoint: runDir, fstype: "tmpfs"},
		{device: "/run/host_mark/Users", mountPoint: dataDir, fstype: "fakeowner"},
		{device: "/run/host_mark/Users", mountPoint: hostDir, fstype: "fakeowner"},
		{device: "/run/desktop/mnt/host/Users", mountPoint: dataDir + "/desktop-host", fstype: "virtiofs"},
		{device: "/host_mnt/Users", mountPoint: dataDir + "/host-mnt", fstype: "virtiofs"},
		{device: "disk-file", mountPoint: fileMount, fstype: "ext4"},
		{device: "overlay", mountPoint: "/", fstype: "overlay"},
		{device: "disk-data-duplicate", mountPoint: dataDir + "/", fstype: "xfs"},
	}

	total := uint64(10 * bytesPerMB)
	used := uint64(7 * bytesPerMB)
	free := uint64(3 * bytesPerMB)
	mounts := collectVisibleDiskMountMetrics(partitions, dataDir, func(partition opsDiskPartition, mountPoint string) (*opsDiskUsage, error) {
		return &opsDiskUsage{
			path:        mountPoint,
			total:       total,
			used:        used,
			free:        free,
			usedPercent: 70.04,
		}, nil
	})

	require.Len(t, mounts, 2)
	require.Equal(t, "/", mounts[0].MountPoint)
	require.Equal(t, dataDir, mounts[1].MountPoint)
	require.Equal(t, "overlay", mounts[0].FSType)
	require.Equal(t, "container_root", mounts[0].Role)
	require.Equal(t, "data_dir", mounts[1].Role)
	require.Equal(t, "fakeowner", mounts[1].FSType)
	require.Equal(t, int64(10), mounts[0].TotalMB)
	require.Equal(t, int64(7), mounts[0].UsedMB)
	require.Equal(t, int64(3), mounts[0].FreeMB)
	require.Equal(t, 70.0, mounts[0].UsagePercent)
}

func TestShouldCollectDiskMountFiltersRuntimeAndPseudoMounts(t *testing.T) {
	require.False(t, shouldCollectDiskMount("/proc", "proc", "tmpfs", "mount"))
	require.False(t, shouldCollectDiskMount("/sys/fs/cgroup", "cgroup2", "cgroup", "mount"))
	require.False(t, shouldCollectDiskMount("/run/secrets", "ext4", "/dev/sda1", "mount"))
	require.False(t, shouldCollectDiskMount("/var/run/docker.sock", "ext4", "/dev/sda1", "mount"))
	require.False(t, shouldCollectDiskMount("/data", "tmpfs", "tmpfs", "mount"))
	require.False(t, shouldCollectDiskMount("/host/data", "fakeowner", "/run/host_mark/Users", "mount"))
	require.False(t, shouldCollectDiskMount("/host/data", "virtiofs", "/run/host_mark/Users", "mount"))
	require.False(t, shouldCollectDiskMount("/host/data", "virtiofs", "/run/desktop/mnt/host/Users", "mount"))
	require.False(t, shouldCollectDiskMount("/host/data", "virtiofs", "/host_mnt/Users", "mount"))
	require.True(t, shouldCollectDiskMount("/app/data", "fakeowner", "/run/host_mark/Users", "data_dir"))
	require.True(t, shouldCollectDiskMount("/app/data", "virtiofs", "/run/desktop/mnt/host/Users", "data_dir"))
	require.True(t, shouldCollectDiskMount("/", "overlay", "overlay", "container_root"))
	require.True(t, shouldCollectDiskMount("/data", "xfs", "/dev/vdb1", "mount"))
}

func TestDiskMountRoleMarksDataDirAndContainerRoot(t *testing.T) {
	require.Equal(t, "data_dir", diskMountRole("/app/data", "/app/data"))
	require.Equal(t, "data_dir", diskMountRole("/app/data/", "/app/data"))
	require.Equal(t, "container_root", diskMountRole("/", "/app/data"))
	require.Equal(t, "mount", diskMountRole("/data", "/app/data"))
}

func TestParseDFDiskUsageParsesBusyBoxOutput(t *testing.T) {
	output := `Filesystem           1-blocks       Used Available Capacity Mounted on
/run/host_mark/Users 994662584320 866581954560 128080629760  87% /app/data`

	usage, err := parseDFDiskUsage("/app/data", output)
	require.NoError(t, err)
	require.Equal(t, "/app/data", usage.path)
	require.Equal(t, uint64(994662584320), usage.total)
	require.Equal(t, uint64(866581954560), usage.used)
	require.Equal(t, uint64(128080629760), usage.free)
	require.InDelta(t, 87.1, usage.usedPercent, 0.1)
}

func TestShouldUseDFDiskUsageOnlyForDockerDesktopBridgeMounts(t *testing.T) {
	require.True(t, shouldUseDFDiskUsage(opsDiskPartition{fstype: "fakeowner", device: "/run/host_mark/Users"}))
	require.True(t, shouldUseDFDiskUsage(opsDiskPartition{fstype: "virtiofs", device: "/run/desktop/mnt/host/Users"}))
	require.True(t, shouldUseDFDiskUsage(opsDiskPartition{fstype: "virtiofs", device: "/host_mnt/Users"}))
	require.False(t, shouldUseDFDiskUsage(opsDiskPartition{fstype: "overlay", device: "overlay"}))
	require.False(t, shouldUseDFDiskUsage(opsDiskPartition{fstype: "ext4", device: "/dev/vda1"}))
	require.False(t, shouldUseDFDiskUsage(opsDiskPartition{fstype: "xfs", device: "/dev/vdb1"}))
}

func TestParseDFDiskUsageRejectsMalformedOutput(t *testing.T) {
	_, err := parseDFDiskUsage("/app/data", "Filesystem 1-blocks Used\nbroken")
	require.Error(t, err)
}

func TestCollectVisibleDiskMountMetricsUsesDFCorrectedUsageForDockerDesktopDataDir(t *testing.T) {
	dataDir := t.TempDir()
	partitions := []opsDiskPartition{
		{device: "/run/host_mark/Users", mountPoint: dataDir, fstype: "fakeowner"},
		{device: "overlay", mountPoint: "/", fstype: "overlay"},
	}
	statfsLikeTotal := uint64(242837545 * bytesPerMB)
	dfTotal := uint64(994662584320)
	dfUsed := uint64(866581954560)
	dfFree := uint64(128080629760)

	mounts := collectVisibleDiskMountMetrics(partitions, dataDir, func(partition opsDiskPartition, mountPoint string) (*opsDiskUsage, error) {
		if shouldUseDFDiskUsage(partition) {
			return &opsDiskUsage{
				path:        mountPoint,
				total:       dfTotal,
				used:        dfUsed,
				free:        dfFree,
				usedPercent: (float64(dfUsed) / float64(dfUsed+dfFree)) * 100,
			}, nil
		}
		return &opsDiskUsage{
			path:        mountPoint,
			total:       10 * bytesPerMB,
			used:        7 * bytesPerMB,
			free:        3 * bytesPerMB,
			usedPercent: 70,
		}, nil
	})

	require.Len(t, mounts, 2)
	require.Equal(t, "/", mounts[0].MountPoint)
	require.Equal(t, dataDir, mounts[1].MountPoint)
	require.Equal(t, "data_dir", mounts[1].Role)
	require.Equal(t, int64(dfTotal/bytesPerMB), mounts[1].TotalMB)
	require.NotEqual(t, int64(statfsLikeTotal/bytesPerMB), mounts[1].TotalMB)
	require.Equal(t, int64(dfUsed/bytesPerMB), mounts[1].UsedMB)
	require.Equal(t, int64(dfFree/bytesPerMB), mounts[1].FreeMB)
	require.InDelta(t, 87.1, mounts[1].UsagePercent, 0.1)
}
