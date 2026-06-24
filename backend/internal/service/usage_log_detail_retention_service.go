package service

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	usageLogDetailRetentionBatchSize    = 100
	usageLogDetailRetentionTimeout      = 5 * time.Minute
	usageLogDetailRetentionBacklogDelay = 5 * time.Second
	usageLogDetailEmptyCompressedJSON   = `{"messages":[]}`
)

type UsageLogDetailRetentionService struct {
	repo UsageLogDetailRepository
	loc  *time.Location

	statusMu sync.RWMutex
	status   UsageLogDetailRetentionStatus

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
}

type UsageLogDetailRetentionPhase string

const (
	UsageLogDetailRetentionPhaseIdle       UsageLogDetailRetentionPhase = "idle"
	UsageLogDetailRetentionPhaseCounting   UsageLogDetailRetentionPhase = "counting"
	UsageLogDetailRetentionPhaseProcessing UsageLogDetailRetentionPhase = "processing"
	UsageLogDetailRetentionPhaseWaiting    UsageLogDetailRetentionPhase = "waiting"
	UsageLogDetailRetentionPhaseStopped    UsageLogDetailRetentionPhase = "stopped"
)

type UsageLogDetailRetentionStatus struct {
	Enabled bool                         `json:"enabled"`
	Running bool                         `json:"running"`
	Phase   UsageLogDetailRetentionPhase `json:"phase"`

	Cutoff      *time.Time `json:"cutoff,omitempty"`
	WindowStart *time.Time `json:"window_start,omitempty"`
	WindowEnd   *time.Time `json:"window_end,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	NextRunAt   *time.Time `json:"next_run_at,omitempty"`

	TotalPendingAtStart int64 `json:"total_pending_at_start"`
	RemainingPending    int64 `json:"remaining_pending"`
	Processed           int64 `json:"processed"`
	Cleaned             int64 `json:"cleaned"`
	CompressedRequest   int64 `json:"compressed_request"`
	CompressedResponse  int64 `json:"compressed_response"`
	FallbackEmpty       int64 `json:"fallback_empty"`
	Skipped             int64 `json:"skipped"`
	Failed              int64 `json:"failed"`

	LastUsageLogID int64   `json:"last_usage_log_id,omitempty"`
	LastError      *string `json:"last_error,omitempty"`

	ProgressPercent float64 `json:"progress_percent"`
}

type usageLogDetailRetentionRunStats struct {
	windowStart         time.Time
	windowEnd           time.Time
	startedAt           time.Time
	totalPendingAtStart int64
	remainingPending    int64
	processed           int64
	cleaned             int64
	compressedRequest   int64
	compressedResponse  int64
	fallbackEmpty       int64
	skipped             int64
	failed              int64
	lastUsageLogID      int64
	lastError           *string
}

func NewUsageLogDetailRetentionService(repo UsageLogDetailRepository, cfg *config.Config) *UsageLogDetailRetentionService {
	loc := time.Local
	if cfg != nil && strings.TrimSpace(cfg.Timezone) != "" {
		if parsed, err := time.LoadLocation(strings.TrimSpace(cfg.Timezone)); err == nil && parsed != nil {
			loc = parsed
		}
	}
	return &UsageLogDetailRetentionService{
		repo:   repo,
		loc:    loc,
		stopCh: make(chan struct{}),
		status: UsageLogDetailRetentionStatus{
			Enabled: true,
			Phase:   UsageLogDetailRetentionPhaseIdle,
		},
	}
}

func (s *UsageLogDetailRetentionService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.startOnce.Do(func() {
		logger.LegacyPrintf("service.usage_detail_retention", "[UsageDetailRetention] started tz=%s", s.loc.String())
		go s.runLoop()
	})
}

func (s *UsageLogDetailRetentionService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.updateStatus(func(st *UsageLogDetailRetentionStatus) {
			st.Enabled = false
			st.Running = false
			st.Phase = UsageLogDetailRetentionPhaseStopped
		})
		logger.LegacyPrintf("service.usage_detail_retention", "[UsageDetailRetention] stopped")
	})
}

func (s *UsageLogDetailRetentionService) runLoop() {
	s.updateNextRun(nextUsageLogDetailRetentionRun(time.Now().In(s.location()), s.location()))
	remaining := s.runOnce()
	for {
		var next time.Time
		if remaining > 0 {
			next = time.Now().In(s.location()).Add(usageLogDetailRetentionBacklogDelay)
		} else {
			next = nextUsageLogDetailRetentionRun(time.Now().In(s.location()), s.location())
		}
		s.updateNextRun(next)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-timer.C:
			remaining = s.runOnce()
		case <-s.stopCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
}

func (s *UsageLogDetailRetentionService) Snapshot() UsageLogDetailRetentionStatus {
	if s == nil {
		return UsageLogDetailRetentionStatus{Enabled: false, Phase: UsageLogDetailRetentionPhaseStopped}
	}
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return cloneUsageLogDetailRetentionStatus(s.status)
}

func (s *UsageLogDetailRetentionService) location() *time.Location {
	if s == nil || s.loc == nil {
		return time.Local
	}
	return s.loc
}

func nextUsageLogDetailRetentionRun(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	base := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 10, 0, 0, loc)
	if now.Before(base) {
		return base
	}
	return base.AddDate(0, 0, 1)
}

func (s *UsageLogDetailRetentionService) runOnce() (remaining int64) {
	if s == nil || s.repo == nil {
		return 0
	}
	loc := s.location()
	now := time.Now().In(loc)
	windowStart, windowEnd := usageLogDetailRetentionYesterdayWindow(now, loc)
	ctx, cancel := context.WithTimeout(context.Background(), usageLogDetailRetentionTimeout)
	defer cancel()

	stats := &usageLogDetailRetentionRunStats{
		windowStart: windowStart,
		windowEnd:   windowEnd,
		startedAt:   time.Now(),
	}
	s.beginRunStatus(stats)
	defer func() {
		stats.remainingPending = maxInt64(stats.totalPendingAtStart-stats.cleaned, 0)
		if count, err := s.repo.CountFullPayloadCleanupPending(context.Background(), windowStart, windowEnd); err == nil {
			stats.remainingPending = count
			remaining = count
		} else {
			msg := err.Error()
			stats.lastError = &msg
		}
		s.finishRunStatus(stats)
		logger.LegacyPrintf(
			"service.usage_detail_retention",
			"[UsageDetailRetention] run finished window_start=%s window_end=%s total=%d processed=%d cleaned=%d compressed_request=%d compressed_response=%d fallback_empty=%d skipped=%d failed=%d remaining=%d",
			windowStart.Format(time.RFC3339),
			windowEnd.Format(time.RFC3339),
			stats.totalPendingAtStart,
			stats.processed,
			stats.cleaned,
			stats.compressedRequest,
			stats.compressedResponse,
			stats.fallbackEmpty,
			stats.skipped,
			stats.failed,
			stats.remainingPending,
		)
	}()

	total, err := s.repo.CountFullPayloadCleanupPending(ctx, windowStart, windowEnd)
	if err != nil {
		stats.failed++
		msg := err.Error()
		stats.lastError = &msg
		logger.LegacyPrintf("service.usage_detail_retention", "[UsageDetailRetention] count failed err=%v", err)
		return 0
	}
	stats.totalPendingAtStart = total
	stats.remainingPending = total
	s.updateRunStatus(stats, UsageLogDetailRetentionPhaseProcessing)
	if total == 0 {
		return 0
	}

	for {
		details, err := s.repo.ListForFullPayloadCleanup(ctx, windowStart, windowEnd, usageLogDetailRetentionBatchSize)
		if err != nil {
			stats.failed++
			msg := err.Error()
			stats.lastError = &msg
			logger.LegacyPrintf("service.usage_detail_retention", "[UsageDetailRetention] list failed err=%v", err)
			return stats.remainingPending
		}
		if len(details) == 0 {
			break
		}
		for i := range details {
			detail := details[i]
			stats.processed++
			stats.lastUsageLogID = detail.UsageLogID
			changedRequest, changedResponse, fallbackEmpty := ensureUsageLogDetailCompressedPayloadsForCleanup(&detail)
			if changedRequest {
				stats.compressedRequest++
			}
			if changedResponse {
				stats.compressedResponse++
			}
			if fallbackEmpty {
				stats.fallbackEmpty++
			}
			if !usageLogDetailCompressedPayloadsReadyForCleanup(&detail) {
				stats.skipped++
				logger.LegacyPrintf("service.usage_detail_retention", "[UsageDetailRetention] skip clear because compressed payload is missing usage_log_id=%d", detail.UsageLogID)
				s.updateRunStatus(stats, UsageLogDetailRetentionPhaseProcessing)
				continue
			}
			if err := s.repo.ClearFullPayloads(ctx, detail.UsageLogID, detail.CompressedRequestPayloadJSON, detail.CompressedResponsePayloadJSON, time.Now()); err != nil {
				stats.failed++
				msg := err.Error()
				stats.lastError = &msg
				logger.LegacyPrintf("service.usage_detail_retention", "[UsageDetailRetention] clear failed usage_log_id=%d err=%v", detail.UsageLogID, err)
				return stats.remainingPending
			}
			stats.cleaned++
			stats.remainingPending = maxInt64(stats.totalPendingAtStart-stats.cleaned, 0)
			s.updateRunStatus(stats, UsageLogDetailRetentionPhaseProcessing)
		}
		if len(details) < usageLogDetailRetentionBatchSize {
			break
		}
	}
	return stats.remainingPending
}

func usageLogDetailRetentionYesterdayWindow(now time.Time, loc *time.Location) (time.Time, time.Time) {
	if loc == nil {
		loc = time.Local
	}
	end := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 0, 0, 0, loc)
	return end.AddDate(0, 0, -1), end
}

func ensureUsageLogDetailCompressedPayloadsForCleanup(detail *UsageLogDetail) (changedRequest bool, changedResponse bool, fallbackEmpty bool) {
	if detail == nil {
		return false, false, false
	}
	if strings.TrimSpace(derefString(detail.CompressedRequestPayloadJSON)) == "" && strings.TrimSpace(derefString(detail.RequestPayloadJSON)) != "" {
		compressed := CompressUsageLogPayloadJSON(detail.RequestPayloadJSON, UsageLogPayloadKindRequest)
		if compressed == nil {
			empty := usageLogDetailEmptyCompressedJSON
			compressed = &empty
			fallbackEmpty = true
		}
		detail.CompressedRequestPayloadJSON = compressed
		changedRequest = true
	}
	if strings.TrimSpace(derefString(detail.CompressedResponsePayloadJSON)) == "" && strings.TrimSpace(derefString(detail.ResponsePayloadJSON)) != "" {
		compressed := CompressUsageLogPayloadJSON(detail.ResponsePayloadJSON, UsageLogPayloadKindResponse)
		if compressed == nil {
			empty := usageLogDetailEmptyCompressedJSON
			compressed = &empty
			fallbackEmpty = true
		}
		detail.CompressedResponsePayloadJSON = compressed
		changedResponse = true
	}
	return changedRequest, changedResponse, fallbackEmpty
}

func usageLogDetailCompressedPayloadsReadyForCleanup(detail *UsageLogDetail) bool {
	if detail == nil {
		return false
	}
	if strings.TrimSpace(derefString(detail.RequestPayloadJSON)) != "" && strings.TrimSpace(derefString(detail.CompressedRequestPayloadJSON)) == "" {
		return false
	}
	if strings.TrimSpace(derefString(detail.ResponsePayloadJSON)) != "" && strings.TrimSpace(derefString(detail.CompressedResponsePayloadJSON)) == "" {
		return false
	}
	return true
}

func (s *UsageLogDetailRetentionService) beginRunStatus(stats *usageLogDetailRetentionRunStats) {
	s.updateStatus(func(st *UsageLogDetailRetentionStatus) {
		st.Enabled = true
		st.Running = true
		st.Phase = UsageLogDetailRetentionPhaseCounting
		st.Cutoff = cloneTimeValue(stats.windowEnd)
		st.WindowStart = cloneTimeValue(stats.windowStart)
		st.WindowEnd = cloneTimeValue(stats.windowEnd)
		st.StartedAt = cloneTimeValue(stats.startedAt)
		st.FinishedAt = nil
		st.TotalPendingAtStart = 0
		st.RemainingPending = 0
		st.Processed = 0
		st.Cleaned = 0
		st.CompressedRequest = 0
		st.CompressedResponse = 0
		st.FallbackEmpty = 0
		st.Skipped = 0
		st.Failed = 0
		st.LastUsageLogID = 0
		st.LastError = nil
		st.ProgressPercent = 0
	})
	logger.LegacyPrintf(
		"service.usage_detail_retention",
		"[UsageDetailRetention] run started window_start=%s window_end=%s",
		stats.windowStart.Format(time.RFC3339),
		stats.windowEnd.Format(time.RFC3339),
	)
}

func (s *UsageLogDetailRetentionService) updateRunStatus(stats *usageLogDetailRetentionRunStats, phase UsageLogDetailRetentionPhase) {
	s.updateStatus(func(st *UsageLogDetailRetentionStatus) {
		st.Running = true
		st.Phase = phase
		applyUsageLogDetailRetentionStats(st, stats)
	})
}

func (s *UsageLogDetailRetentionService) finishRunStatus(stats *usageLogDetailRetentionRunStats) {
	finishedAt := time.Now()
	s.updateStatus(func(st *UsageLogDetailRetentionStatus) {
		st.Running = false
		if stats.remainingPending > 0 {
			st.Phase = UsageLogDetailRetentionPhaseWaiting
		} else {
			st.Phase = UsageLogDetailRetentionPhaseIdle
		}
		st.FinishedAt = &finishedAt
		applyUsageLogDetailRetentionStats(st, stats)
	})
}

func (s *UsageLogDetailRetentionService) updateNextRun(next time.Time) {
	s.updateStatus(func(st *UsageLogDetailRetentionStatus) {
		st.NextRunAt = &next
	})
}

func (s *UsageLogDetailRetentionService) updateStatus(fn func(*UsageLogDetailRetentionStatus)) {
	if s == nil || fn == nil {
		return
	}
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	fn(&s.status)
}

func applyUsageLogDetailRetentionStats(st *UsageLogDetailRetentionStatus, stats *usageLogDetailRetentionRunStats) {
	if st == nil || stats == nil {
		return
	}
	st.Cutoff = cloneTimeValue(stats.windowEnd)
	st.WindowStart = cloneTimeValue(stats.windowStart)
	st.WindowEnd = cloneTimeValue(stats.windowEnd)
	st.StartedAt = cloneTimeValue(stats.startedAt)
	st.TotalPendingAtStart = stats.totalPendingAtStart
	st.RemainingPending = stats.remainingPending
	st.Processed = stats.processed
	st.Cleaned = stats.cleaned
	st.CompressedRequest = stats.compressedRequest
	st.CompressedResponse = stats.compressedResponse
	st.FallbackEmpty = stats.fallbackEmpty
	st.Skipped = stats.skipped
	st.Failed = stats.failed
	st.LastUsageLogID = stats.lastUsageLogID
	st.LastError = cloneStringPtr(stats.lastError)
	st.ProgressPercent = usageLogDetailRetentionProgressPercent(stats.totalPendingAtStart, stats.remainingPending, stats.cleaned)
}

func usageLogDetailRetentionProgressPercent(total, remaining, cleaned int64) float64 {
	if total <= 0 {
		return 100
	}
	done := cleaned
	if remaining >= 0 {
		done = total - remaining
	}
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	return math.Round((float64(done)/float64(total))*1000) / 10
}

func cloneUsageLogDetailRetentionStatus(st UsageLogDetailRetentionStatus) UsageLogDetailRetentionStatus {
	st.Cutoff = cloneTimePtr(st.Cutoff)
	st.WindowStart = cloneTimePtr(st.WindowStart)
	st.WindowEnd = cloneTimePtr(st.WindowEnd)
	st.StartedAt = cloneTimePtr(st.StartedAt)
	st.FinishedAt = cloneTimePtr(st.FinishedAt)
	st.NextRunAt = cloneTimePtr(st.NextRunAt)
	st.LastError = cloneStringPtr(st.LastError)
	return st
}

func cloneTimeValue(t time.Time) *time.Time {
	copied := t
	return &copied
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
