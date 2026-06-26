package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	usageBriefSourceChunkRetryLimit = 15
)

var usageBriefSourceChunkRetryDelay = 15 * time.Second

type UsageBriefService struct {
	repo                     UsageBriefRepository
	settings                 SettingRepository
	encryptor                SecretEncryptor
	httpClient               *http.Client
	notificationEmailService *NotificationEmailService

	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
	wg     sync.WaitGroup

	workerMu      sync.Mutex
	activeWorkers int
}

func NewUsageBriefService(repo UsageBriefRepository, settings SettingRepository, encryptor SecretEncryptor) *UsageBriefService {
	ctx, cancel := context.WithCancel(context.Background())
	return &UsageBriefService{
		repo:      repo,
		settings:  settings,
		encryptor: encryptor,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *UsageBriefService) SetNotificationEmailService(notificationEmailService *NotificationEmailService) {
	if s == nil {
		return
	}
	s.notificationEmailService = notificationEmailService
}

func (s *UsageBriefService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.once.Do(func() {
		if recovered, err := s.repo.RecoverRunningJobs(s.ctx); err != nil {
			logger.LegacyPrintf("service.usage_brief", "recover running jobs failed: %v", err)
		} else if recovered > 0 {
			logger.LegacyPrintf("service.usage_brief", "recovered %d running usage brief jobs", recovered)
		}
		s.wg.Add(2)
		go s.schedulerLoop()
		go s.workerLoop()
	})
}

func (s *UsageBriefService) Stop() {
	if s == nil {
		return
	}
	s.cancel()
	s.wg.Wait()
}

func (s *UsageBriefService) schedulerLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	s.ensureDueProductionJobs(s.ctx, time.Now())
	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			s.ensureDueProductionJobs(s.ctx, now)
		}
	}
}

func (s *UsageBriefService) workerLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if !s.IsEnabled(s.ctx) {
				continue
			}
			settings, err := s.GetSettings(s.ctx)
			if err != nil {
				logger.LegacyPrintf("service.usage_brief", "settings load failed: %v", err)
				continue
			}
			concurrency := settings.Concurrency
			if concurrency <= 0 {
				concurrency = UsageBriefDefaultConcurrency
			}
			capacity := s.workerCapacity(concurrency)
			if capacity <= 0 {
				continue
			}
			jobs, err := s.repo.ClaimQueuedJobs(s.ctx, capacity)
			if err != nil {
				logger.LegacyPrintf("service.usage_brief", "claim queued jobs failed: %v", err)
				continue
			}
			for _, job := range jobs {
				job := job
				s.incActiveWorkers()
				s.wg.Add(1)
				go func() {
					defer s.wg.Done()
					defer s.decActiveWorkers()
					s.processJob(s.ctx, job)
				}()
			}
		}
	}
}

func (s *UsageBriefService) workerCapacity(concurrency int) int {
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	return concurrency - s.activeWorkers
}

func (s *UsageBriefService) incActiveWorkers() {
	s.workerMu.Lock()
	s.activeWorkers++
	s.workerMu.Unlock()
}

func (s *UsageBriefService) decActiveWorkers() {
	s.workerMu.Lock()
	if s.activeWorkers > 0 {
		s.activeWorkers--
	}
	s.workerMu.Unlock()
}

func (s *UsageBriefService) IsEnabled(ctx context.Context) bool {
	if s == nil || s.settings == nil {
		return false
	}
	raw, err := s.settings.GetValue(ctx, SettingKeyUsageBriefEnabled)
	return err == nil && strings.EqualFold(strings.TrimSpace(raw), "true")
}

func (s *UsageBriefService) GetSettings(ctx context.Context) (*UsageBriefSettings, error) {
	if s == nil || s.settings == nil {
		return nil, infraerrors.ServiceUnavailable("USAGE_BRIEF_UNAVAILABLE", "usage brief service unavailable")
	}
	keys := []string{
		SettingKeyUsageBriefEnabled,
		SettingKeyUsageBriefOpenAIBaseURL,
		SettingKeyUsageBriefOpenAIAPIKey,
		SettingKeyUsageBriefOpenAIModel,
		SettingKeyUsageBriefContextTokens,
		SettingKeyUsageBriefOutputReservedTokens,
		SettingKeyUsageBriefConcurrency,
	}
	values, err := s.settings.GetMultiple(ctx, keys)
	if err != nil {
		return nil, err
	}

	apiKeyEncrypted := strings.TrimSpace(values[SettingKeyUsageBriefOpenAIAPIKey])
	apiKey := apiKeyEncrypted
	if apiKeyEncrypted != "" && s.encryptor != nil {
		if decrypted, decErr := s.encryptor.Decrypt(apiKeyEncrypted); decErr == nil {
			apiKey = decrypted
		}
	}
	out := &UsageBriefSettings{
		Enabled:              strings.EqualFold(values[SettingKeyUsageBriefEnabled], "true"),
		BaseURL:              strings.TrimRight(strings.TrimSpace(values[SettingKeyUsageBriefOpenAIBaseURL]), "/"),
		APIKey:               apiKey,
		APIKeyConfigured:     apiKeyEncrypted != "",
		Model:                strings.TrimSpace(values[SettingKeyUsageBriefOpenAIModel]),
		ContextTokens:        parseIntDefault(values[SettingKeyUsageBriefContextTokens], UsageBriefDefaultContextTokens),
		OutputReservedTokens: parseIntDefault(values[SettingKeyUsageBriefOutputReservedTokens], UsageBriefDefaultOutputReservedTokens),
		Concurrency:          parseIntDefault(values[SettingKeyUsageBriefConcurrency], UsageBriefDefaultConcurrency),
	}
	if out.BaseURL == "" {
		out.BaseURL = "https://api.openai.com"
	}
	if out.Model == "" {
		out.Model = UsageBriefDefaultModel
	}
	if out.ContextTokens < 16000 {
		out.ContextTokens = 16000
	}
	if out.OutputReservedTokens < 4096 {
		out.OutputReservedTokens = 4096
	}
	if out.OutputReservedTokens >= out.ContextTokens {
		out.OutputReservedTokens = out.ContextTokens / 4
	}
	if out.Concurrency < 1 {
		out.Concurrency = UsageBriefDefaultConcurrency
	}
	if out.Concurrency > UsageBriefMaxConcurrency {
		out.Concurrency = UsageBriefMaxConcurrency
	}
	return out, nil
}

func (s *UsageBriefService) UpdateSettings(ctx context.Context, req UpdateUsageBriefSettingsRequest) (*UsageBriefSettings, error) {
	current, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	updates := map[string]string{}
	if req.BaseURL != nil {
		base := strings.TrimRight(strings.TrimSpace(*req.BaseURL), "/")
		if base != "" {
			if _, err := url.ParseRequestURI(base); err != nil {
				return nil, infraerrors.BadRequest("USAGE_BRIEF_BASE_URL_INVALID", "base url is invalid")
			}
		}
		updates[SettingKeyUsageBriefOpenAIBaseURL] = base
	}
	if req.Model != nil {
		model := strings.TrimSpace(*req.Model)
		if model == "" {
			model = UsageBriefDefaultModel
		}
		updates[SettingKeyUsageBriefOpenAIModel] = model
	}
	if req.ContextTokens != nil {
		v := *req.ContextTokens
		if v < 16000 {
			v = 16000
		}
		if v > 2000000 {
			v = 2000000
		}
		updates[SettingKeyUsageBriefContextTokens] = strconv.Itoa(v)
	}
	if req.OutputReservedTokens != nil {
		v := *req.OutputReservedTokens
		if v < 4096 {
			v = 4096
		}
		if v > 1000000 {
			v = 1000000
		}
		updates[SettingKeyUsageBriefOutputReservedTokens] = strconv.Itoa(v)
	}
	if req.Concurrency != nil {
		v := *req.Concurrency
		if v < 1 {
			v = 1
		}
		if v > UsageBriefMaxConcurrency {
			v = UsageBriefMaxConcurrency
		}
		updates[SettingKeyUsageBriefConcurrency] = strconv.Itoa(v)
	}
	if req.ClearAPIKey != nil && *req.ClearAPIKey {
		updates[SettingKeyUsageBriefOpenAIAPIKey] = ""
	} else if req.APIKey != nil {
		apiKey := strings.TrimSpace(*req.APIKey)
		if apiKey != "" {
			if s.encryptor != nil {
				encrypted, encErr := s.encryptor.Encrypt(apiKey)
				if encErr != nil {
					return nil, encErr
				}
				apiKey = encrypted
			}
			updates[SettingKeyUsageBriefOpenAIAPIKey] = apiKey
		}
	}
	if len(updates) == 0 {
		return current, nil
	}
	if err := s.settings.SetMultiple(ctx, updates); err != nil {
		return nil, err
	}
	return s.GetSettings(ctx)
}

func (s *UsageBriefService) ListReports(ctx context.Context, filter UsageBriefReportFilter) ([]UsageBriefReport, int64, error) {
	return s.repo.ListReports(ctx, filter)
}

func (s *UsageBriefService) ListReportGroups(ctx context.Context, filter UsageBriefReportGroupFilter) ([]UsageBriefReportGroup, int64, error) {
	return s.repo.ListReportGroups(ctx, filter)
}

func (s *UsageBriefService) GetReport(ctx context.Context, id int64) (*UsageBriefReport, error) {
	report, err := s.repo.GetReport(ctx, id)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, infraerrors.NotFound("USAGE_BRIEF_REPORT_NOT_FOUND", "usage brief report not found")
	}
	return report, nil
}

func (s *UsageBriefService) GetUserReport(ctx context.Context, userID, reportID int64) (*UsageBriefReport, error) {
	report, err := s.GetReport(ctx, reportID)
	if err != nil {
		return nil, err
	}
	if report.UserID != userID {
		return nil, infraerrors.Forbidden("USAGE_BRIEF_FORBIDDEN", "not allowed to view this usage brief")
	}
	return report, nil
}

func (s *UsageBriefService) GetUserPeriodView(ctx context.Context, userID int64, periodType string, date time.Time) (*UsageBriefPeriodView, error) {
	start, end, err := UsageBriefPeriodBounds(periodType, date)
	if err != nil {
		return nil, err
	}
	view := &UsageBriefPeriodView{
		Available:   false,
		PeriodType:  periodType,
		PeriodStart: start,
		PeriodEnd:   end,
	}
	if isCurrentOrFuturePeriod(periodType, start, end, time.Now()) {
		view.Reason = "current_period_unavailable"
		view.Message = currentPeriodMessage(periodType)
		return view, nil
	}
	report, err := s.repo.GetReportByPeriod(ctx, userID, periodType, start, end)
	if err != nil {
		return nil, err
	}
	if report != nil {
		view.Report = report
		view.Status = report.Status
		view.Available = report.Status == UsageBriefStatusSucceeded
		if !view.Available {
			view.Reason = report.Status
			if report.ErrorMessage != "" {
				view.Message = report.ErrorMessage
			}
		}
		return view, nil
	}
	job, err := s.repo.GetActiveJobByPeriod(ctx, userID, periodType, start, end)
	if err != nil {
		return nil, err
	}
	if job != nil {
		view.Job = job
		view.Status = job.Status
		if job.Status == UsageBriefStatusQueued || job.Status == UsageBriefStatusRunning {
			view.Reason = "generating"
			view.Message = "报告生成中"
			return view, nil
		}
	}
	view.Reason = "not_generated"
	view.Message = "报告尚未生成"
	return view, nil
}

func (s *UsageBriefService) UpdateReportByAdmin(ctx context.Context, id int64, title, contentMD string, adminID int64) (*UsageBriefReport, error) {
	title = strings.TrimSpace(title)
	contentMD = strings.TrimSpace(contentMD)
	if title == "" {
		return nil, infraerrors.BadRequest("USAGE_BRIEF_TITLE_REQUIRED", "title is required")
	}
	if contentMD == "" {
		return nil, infraerrors.BadRequest("USAGE_BRIEF_CONTENT_REQUIRED", "content is required")
	}
	report, err := s.repo.UpdateReportByAdmin(ctx, id, title, contentMD, adminID)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, infraerrors.NotFound("USAGE_BRIEF_REPORT_NOT_FOUND", "usage brief report not found")
	}
	return report, nil
}

func (s *UsageBriefService) DeleteReport(ctx context.Context, id int64) error {
	return s.repo.DeleteReport(ctx, id)
}

func (s *UsageBriefService) DeleteReportGroup(ctx context.Context, filter UsageBriefReportGroupFilter, groupKey string) (int64, error) {
	return s.repo.DeleteReportGroup(ctx, filter, groupKey)
}

func (s *UsageBriefService) ListBatches(ctx context.Context, filter UsageBriefBatchFilter) ([]UsageBriefBatch, int64, error) {
	return s.repo.ListBatches(ctx, filter)
}

func (s *UsageBriefService) GetBatchJobs(ctx context.Context, batchID int64, filter UsageBriefJobFilter) ([]UsageBriefJob, int64, error) {
	if batchID <= 0 {
		return nil, 0, infraerrors.BadRequest("USAGE_BRIEF_BATCH_REQUIRED", "batch id is required")
	}
	filter.BatchID = &batchID
	return s.repo.ListJobs(ctx, filter)
}

func (s *UsageBriefService) ListJobs(ctx context.Context, filter UsageBriefJobFilter) ([]UsageBriefJob, int64, error) {
	return s.repo.ListJobs(ctx, filter)
}

func (s *UsageBriefService) PauseBatch(ctx context.Context, id int64) (*UsageBriefBatch, error) {
	batch, err := s.repo.PauseBatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if batch == nil {
		return nil, infraerrors.NotFound("USAGE_BRIEF_BATCH_NOT_FOUND", "usage brief batch not found")
	}
	return batch, nil
}

func (s *UsageBriefService) ResumeBatch(ctx context.Context, id int64) (*UsageBriefBatch, error) {
	batch, err := s.repo.ResumeBatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if batch == nil {
		return nil, infraerrors.NotFound("USAGE_BRIEF_BATCH_NOT_FOUND", "usage brief batch not found")
	}
	return batch, nil
}

func (s *UsageBriefService) CancelBatch(ctx context.Context, id int64) (*UsageBriefBatch, error) {
	batch, err := s.repo.CancelBatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if batch == nil {
		return nil, infraerrors.NotFound("USAGE_BRIEF_BATCH_NOT_FOUND", "usage brief batch not found")
	}
	return batch, nil
}

func (s *UsageBriefService) ResetBatch(ctx context.Context, id int64) (*UsageBriefBatch, error) {
	batch, err := s.repo.ResetBatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if batch == nil {
		return nil, infraerrors.NotFound("USAGE_BRIEF_BATCH_NOT_FOUND", "usage brief batch not found")
	}
	return batch, nil
}

func (s *UsageBriefService) RerunBatch(ctx context.Context, id int64) (*UsageBriefBatch, error) {
	batch, err := s.repo.RerunBatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if batch == nil {
		return nil, infraerrors.NotFound("USAGE_BRIEF_BATCH_NOT_FOUND", "usage brief batch not found")
	}
	return batch, nil
}

func (s *UsageBriefService) DeleteBatch(ctx context.Context, id int64) error {
	if err := s.repo.DeleteBatch(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.NotFound("USAGE_BRIEF_BATCH_NOT_FOUND", "usage brief batch not found")
		}
		return err
	}
	return nil
}

func (s *UsageBriefService) CancelJob(ctx context.Context, id int64) (*UsageBriefJob, error) {
	job, err := s.repo.CancelJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, infraerrors.NotFound("USAGE_BRIEF_JOB_NOT_FOUND", "usage brief job not found")
	}
	s.refreshJobBatchStats(ctx, *job)
	return job, nil
}

func (s *UsageBriefService) ResetJob(ctx context.Context, id int64) (*UsageBriefJob, error) {
	job, err := s.repo.ResetJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, infraerrors.NotFound("USAGE_BRIEF_JOB_NOT_FOUND", "usage brief job not found")
	}
	s.refreshJobBatchStats(ctx, *job)
	return job, nil
}

func (s *UsageBriefService) RerunJob(ctx context.Context, id int64) (*UsageBriefJob, error) {
	job, err := s.repo.RerunJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, infraerrors.NotFound("USAGE_BRIEF_JOB_NOT_FOUND", "usage brief job not found")
	}
	s.refreshJobBatchStats(ctx, *job)
	return job, nil
}

func (s *UsageBriefService) ListJobChunks(ctx context.Context, id int64, filter UsageBriefJobChunkFilter) ([]UsageBriefJobChunk, int64, error) {
	job, err := s.repo.GetJob(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	if job == nil {
		return nil, 0, infraerrors.NotFound("USAGE_BRIEF_JOB_NOT_FOUND", "usage brief job not found")
	}
	chunkType := strings.TrimSpace(filter.ChunkType)
	if chunkType == "" {
		chunkType = UsageBriefChunkTypeSource
	}
	if chunkType != UsageBriefChunkTypeSource && chunkType != UsageBriefChunkTypeMerge {
		return nil, 0, infraerrors.BadRequest("INVALID_USAGE_BRIEF_CHUNK_TYPE", "invalid usage brief chunk type")
	}
	filter.ChunkType = chunkType
	return s.repo.ListJobChunks(ctx, id, filter)
}

func (s *UsageBriefService) ListJobConversations(ctx context.Context, id int64, filter UsageBriefJobConversationFilter) ([]UsageBriefJobConversation, int64, error) {
	job, err := s.repo.GetJob(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	if job == nil {
		return nil, 0, infraerrors.NotFound("USAGE_BRIEF_JOB_NOT_FOUND", "usage brief job not found")
	}
	return s.repo.ListJobConversations(ctx, id, filter)
}

func (s *UsageBriefService) DeleteJob(ctx context.Context, id int64) error {
	job, _ := s.repo.GetJob(ctx, id)
	if err := s.repo.DeleteJob(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.NotFound("USAGE_BRIEF_JOB_NOT_FOUND", "usage brief job not found")
		}
		return err
	}
	if job != nil {
		s.refreshJobBatchStats(ctx, *job)
	}
	return nil
}

func (s *UsageBriefService) TriggerProduction(ctx context.Context, req UsageBriefTriggerProductionRequest) (*UsageBriefBatch, []UsageBriefJob, error) {
	if req.PeriodType == "" {
		req.PeriodType = UsageBriefPeriodDaily
	}
	if req.TriggerKind == "" {
		req.TriggerKind = UsageBriefTriggerManual
	}
	start, end, err := UsageBriefPeriodBounds(req.PeriodType, req.PeriodDate)
	if err != nil {
		return nil, nil, err
	}
	if isCurrentOrFuturePeriod(req.PeriodType, start, end, time.Now()) {
		return nil, nil, infraerrors.BadRequest("USAGE_BRIEF_PERIOD_NOT_CLOSED", "current or future period cannot be generated yet")
	}
	users, err := s.repo.ListNormalUsers(ctx)
	if err != nil {
		return nil, nil, err
	}
	pendingUsers := make([]User, 0, len(users))
	for _, user := range users {
		exists, err := s.repo.HasProductionJobOrReport(ctx, user.ID, req.PeriodType, start, end)
		if err != nil {
			return nil, nil, err
		}
		if !exists {
			pendingUsers = append(pendingUsers, user)
		}
	}
	if len(pendingUsers) == 0 {
		return nil, []UsageBriefJob{}, nil
	}
	periodType := req.PeriodType
	createdBy := req.CreatedBy
	batch, err := s.repo.CreateBatch(ctx, UsageBriefBatch{
		BatchScope:  UsageBriefJobScopeProduction,
		TriggerKind: req.TriggerKind,
		Status:      UsageBriefStatusQueued,
		Title:       productionBatchTitle(req.PeriodType, start, end),
		PeriodType:  &periodType,
		PeriodStart: &start,
		PeriodEnd:   &end,
		CreatedBy:   &createdBy,
	})
	if err != nil {
		return nil, nil, err
	}
	if batch == nil {
		return nil, nil, fmt.Errorf("failed to create usage brief batch")
	}
	created := make([]UsageBriefJob, 0)
	for _, user := range pendingUsers {
		batchID := batch.ID
		userID := user.ID
		job, err := s.repo.CreateJob(ctx, UsageBriefJob{
			BatchID:     &batchID,
			JobScope:    UsageBriefJobScopeProduction,
			JobType:     periodType,
			Status:      UsageBriefStatusQueued,
			UserID:      &userID,
			PeriodType:  &periodType,
			PeriodStart: &start,
			PeriodEnd:   &end,
			CreatedBy:   &createdBy,
		})
		if err != nil {
			return nil, nil, err
		}
		if job != nil {
			created = append(created, *job)
		}
	}
	_ = s.repo.UpdateBatchStats(ctx, batch.ID)
	batch, _ = s.repo.GetBatch(ctx, batch.ID)
	return batch, created, nil
}

func (s *UsageBriefService) CreateTestJob(ctx context.Context, req UsageBriefCreateTestJobRequest) (*UsageBriefBatch, *UsageBriefJob, error) {
	if req.UserID <= 0 {
		return nil, nil, infraerrors.BadRequest("USAGE_BRIEF_USER_REQUIRED", "user_id is required")
	}
	if req.RangeEnd.IsZero() || req.RangeStart.IsZero() || !req.RangeEnd.After(req.RangeStart) {
		return nil, nil, infraerrors.BadRequest("USAGE_BRIEF_RANGE_INVALID", "range_start and range_end are required")
	}
	userID := req.UserID
	createdBy := req.CreatedBy
	batch, err := s.repo.CreateBatch(ctx, UsageBriefBatch{
		BatchScope:  UsageBriefJobScopeTest,
		TriggerKind: UsageBriefTriggerManual,
		Status:      UsageBriefStatusQueued,
		Title:       fmt.Sprintf("测试简报 %s 至 %s", req.RangeStart.Format("2006-01-02 15:04"), req.RangeEnd.Format("2006-01-02 15:04")),
		RangeStart:  &req.RangeStart,
		RangeEnd:    &req.RangeEnd,
		CreatedBy:   &createdBy,
	})
	if err != nil {
		return nil, nil, err
	}
	if batch == nil {
		return nil, nil, fmt.Errorf("failed to create usage brief test batch")
	}
	batchID := batch.ID
	job, err := s.repo.CreateJob(ctx, UsageBriefJob{
		BatchID:    &batchID,
		JobScope:   UsageBriefJobScopeTest,
		JobType:    UsageBriefJobTypeCustom,
		Status:     UsageBriefStatusQueued,
		UserID:     &userID,
		GroupID:    req.GroupID,
		RangeStart: &req.RangeStart,
		RangeEnd:   &req.RangeEnd,
		CreatedBy:  &createdBy,
	})
	if err != nil {
		return nil, nil, err
	}
	_ = s.repo.UpdateBatchStats(ctx, batch.ID)
	batch, _ = s.repo.GetBatch(ctx, batch.ID)
	return batch, job, nil
}

func (s *UsageBriefService) ensureDueProductionJobs(ctx context.Context, now time.Time) {
	if !s.IsEnabled(ctx) {
		return
	}
	yesterday := now.AddDate(0, 0, -1)
	if _, _, err := s.TriggerProduction(ctx, UsageBriefTriggerProductionRequest{PeriodType: UsageBriefPeriodDaily, PeriodDate: yesterday, TriggerKind: UsageBriefTriggerAuto}); err != nil {
		logger.LegacyPrintf("service.usage_brief", "schedule daily jobs failed: %v", err)
	}
	if now.Weekday() == time.Monday {
		lastWeek := now.AddDate(0, 0, -7)
		if _, _, err := s.TriggerProduction(ctx, UsageBriefTriggerProductionRequest{PeriodType: UsageBriefPeriodWeekly, PeriodDate: lastWeek, TriggerKind: UsageBriefTriggerAuto}); err != nil {
			logger.LegacyPrintf("service.usage_brief", "schedule weekly jobs failed: %v", err)
		}
	}
	if now.Day() == 1 {
		lastMonth := now.AddDate(0, -1, 0)
		if _, _, err := s.TriggerProduction(ctx, UsageBriefTriggerProductionRequest{PeriodType: UsageBriefPeriodMonthly, PeriodDate: lastMonth, TriggerKind: UsageBriefTriggerAuto}); err != nil {
			logger.LegacyPrintf("service.usage_brief", "schedule monthly jobs failed: %v", err)
		}
	}
}

func (s *UsageBriefService) processJob(ctx context.Context, job UsageBriefJob) {
	if canceled, _ := s.repo.IsJobCancelRequested(ctx, job.ID); canceled {
		_ = s.repo.MarkJobCanceled(ctx, job.ID)
		s.refreshJobBatchStats(ctx, job)
		return
	}
	if err := s.repo.MarkJobRunning(ctx, job.ID); err != nil {
		logger.LegacyPrintf("service.usage_brief", "mark job running failed: job_id=%d err=%v", job.ID, err)
		return
	}
	s.refreshJobBatchStats(ctx, job)
	jobCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go s.watchJobCancel(jobCtx, job.ID, cancel, done)
	result, report, jobStatus, err := s.generateForJob(jobCtx, job)
	close(done)
	cancel()
	if err != nil {
		if s.finishCanceledOrResetJob(ctx, job.ID) {
			s.refreshJobBatchStats(ctx, job)
			return
		}
		if errors.Is(err, context.Canceled) {
			_ = s.repo.MarkJobCanceled(ctx, job.ID)
			s.refreshJobBatchStats(ctx, job)
			return
		}
		if isUsageBriefDependencyWaitError(err) {
			_ = s.repo.ScheduleJobDependencyWait(ctx, job.ID, err.Error(), time.Now().Add(2*time.Minute))
			s.refreshJobBatchStats(ctx, job)
			return
		}
		if s.shouldRetryUsageBriefJob(err, job) {
			_ = s.repo.ScheduleJobRetry(ctx, job.ID, err.Error(), s.nextRetryAt(job.RetryCount))
			s.refreshJobBatchStats(ctx, job)
			return
		}
		_ = s.repo.FailJob(ctx, job.ID, err.Error())
		s.refreshJobBatchStats(ctx, job)
		return
	}
	if s.finishCanceledOrResetJob(ctx, job.ID) {
		s.refreshJobBatchStats(ctx, job)
		return
	}
	if canceled, _ := s.repo.IsJobCancelRequested(ctx, job.ID); canceled {
		_ = s.repo.MarkJobCanceled(ctx, job.ID)
		s.refreshJobBatchStats(ctx, job)
		return
	}
	var reportID *int64
	if report != nil {
		reportID = &report.ID
	}
	status := strings.TrimSpace(jobStatus)
	if status == "" {
		status = UsageBriefStatusSucceeded
	}
	if err := s.repo.CompleteJob(ctx, job.ID, reportID, result, status); err != nil {
		logger.LegacyPrintf("service.usage_brief", "complete job failed: job_id=%d err=%v", job.ID, err)
	}
	s.sendProductionJobEmailIfNeeded(ctx, job.ID)
	s.refreshJobBatchStats(ctx, job)
}

func (s *UsageBriefService) refreshJobBatchStats(ctx context.Context, job UsageBriefJob) {
	if job.BatchID == nil || *job.BatchID <= 0 {
		return
	}
	if err := s.repo.UpdateBatchStats(ctx, *job.BatchID); err != nil {
		logger.LegacyPrintf("service.usage_brief", "update batch stats failed: batch_id=%d err=%v", *job.BatchID, err)
	}
}

func (s *UsageBriefService) updateJobProgress(ctx context.Context, jobID int64, current, total int, stage string, chunkCurrent, chunkTotal, tokenProcessed, tokenTotal, inputTokens, outputTokens int) {
	if err := s.repo.UpdateJobProgress(ctx, jobID, current, total, stage, chunkCurrent, chunkTotal, tokenProcessed, tokenTotal, inputTokens, outputTokens); err != nil {
		logger.LegacyPrintf("service.usage_brief", "update job progress failed: job_id=%d err=%v", jobID, err)
	}
	job, err := s.repo.GetJob(ctx, jobID)
	if err == nil && job != nil {
		s.refreshJobBatchStats(ctx, *job)
	}
}

func (s *UsageBriefService) shouldRetryUsageBriefJob(err error, job UsageBriefJob) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	return isRetryableUsageBriefError(err)
}

func (s *UsageBriefService) nextRetryAt(_ int) time.Time {
	return time.Now().Add(15 * time.Second)
}

func (s *UsageBriefService) finishCanceledOrResetJob(ctx context.Context, jobID int64) bool {
	current, err := s.repo.GetJob(ctx, jobID)
	if err != nil || current == nil || !current.CancelRequested {
		return false
	}
	if current.Status == UsageBriefStatusQueued {
		_ = s.repo.ClearJobCancelRequest(ctx, jobID)
		return true
	}
	_ = s.repo.MarkJobCanceled(ctx, jobID)
	return true
}

func (s *UsageBriefService) watchJobCancel(ctx context.Context, jobID int64, cancel context.CancelFunc, done <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			canceled, err := s.repo.IsJobCancelRequested(ctx, jobID)
			if err == nil && canceled {
				cancel()
				return
			}
		}
	}
}

func (s *UsageBriefService) generateForJob(ctx context.Context, job UsageBriefJob) (string, *UsageBriefReport, string, error) {
	if job.JobScope == UsageBriefJobScopeTest {
		return s.generateTestJob(ctx, job)
	}
	if job.UserID == nil || job.PeriodType == nil || job.PeriodStart == nil || job.PeriodEnd == nil {
		return "", nil, "", fmt.Errorf("production job missing period fields")
	}
	switch *job.PeriodType {
	case UsageBriefPeriodDaily:
		return s.generateDailyReport(ctx, job)
	case UsageBriefPeriodWeekly:
		return s.generateWeeklyReport(ctx, job)
	case UsageBriefPeriodMonthly:
		return s.generateMonthlyReport(ctx, job)
	default:
		return "", nil, "", fmt.Errorf("unsupported period type %q", *job.PeriodType)
	}
}

func (s *UsageBriefService) generateDailyReport(ctx context.Context, job UsageBriefJob) (string, *UsageBriefReport, string, error) {
	sourceRange := UsageBriefSourceRange{
		UserID: *job.UserID,
		Start:  *job.PeriodStart,
		End:    job.PeriodEnd.AddDate(0, 0, 1),
	}
	usageSummary, err := s.repo.SummarizeUsageRecords(ctx, UsageBriefUsageRecordFilter{
		UserID: sourceRange.UserID,
		Start:  sourceRange.Start,
		End:    sourceRange.End,
	})
	if err != nil {
		return "", nil, "", err
	}
	if usageSummary.RequestCount == 0 {
		content, report, err := s.saveNoWorkProductionReport(ctx, job, UsageBriefPeriodDaily, 0)
		return content, report, UsageBriefStatusSucceeded, err
	}
	meta := map[string]any{
		"user_id":      *job.UserID,
		"period_type":  UsageBriefPeriodDaily,
		"period_start": job.PeriodStart.Format("2006-01-02"),
		"period_end":   job.PeriodEnd.Format("2006-01-02"),
		"summary":      usageSummary.toMap(),
	}
	priorDailyReports, err := s.listPriorDailyReportsForReference(ctx, *job.UserID, *job.PeriodStart)
	if err != nil {
		return "", nil, "", err
	}
	if len(priorDailyReports) > 0 {
		meta["prior_daily_reports"] = usageBriefReportReferences(priorDailyReports)
	}
	content, inputTokens, outputTokens, reportStatus, err := s.generateRecordBrief(ctx, job, "日报", meta, sourceRange, workSummaryFinalInstruction("工作日报", "请基于当天 usage 记录和请求 JSON 的分片摘要生成普通用户工作日报。重点凸显用户当天完成的工作、模块/功能设计或实现、贡献线索、风险和阻塞；可参考 prior_daily_reports 识别连续工作背景，但当天事实必须来自本次分片摘要。"))
	if err != nil {
		return "", nil, "", err
	}
	if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
		return "", nil, "", err
	}
	generatedAt := time.Now()
	title := workReportTitle(UsageBriefPeriodDaily, *job.PeriodStart, *job.PeriodEnd)
	report, overwritten, err := s.repo.UpsertGeneratedReport(ctx, UsageBriefReport{
		UserID:          *job.UserID,
		PeriodType:      UsageBriefPeriodDaily,
		PeriodStart:     *job.PeriodStart,
		PeriodEnd:       *job.PeriodEnd,
		Status:          reportStatus,
		Title:           title,
		ContentMD:       content,
		SourceKind:      UsageBriefJobScopeProduction,
		GeneratedByJob:  &job.ID,
		InputUsageCount: usageSummary.RequestCount,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		GeneratedAt:     &generatedAt,
	})
	if err != nil {
		return "", nil, "", err
	}
	if !overwritten && report != nil && report.IsAdminEdited {
		return report.ContentMD, report, report.Status, nil
	}
	return content, report, reportStatus, nil
}

func (s *UsageBriefService) generateWeeklyReport(ctx context.Context, job UsageBriefJob) (string, *UsageBriefReport, string, error) {
	if err := s.waitForActiveDailyDependencies(ctx, job.ID, *job.UserID, *job.PeriodStart, *job.PeriodEnd); err != nil {
		return "", nil, "", err
	}
	dailyReports, err := s.repo.ListDailyReports(ctx, *job.UserID, *job.PeriodStart, *job.PeriodEnd)
	if err != nil {
		return "", nil, "", err
	}
	if len(dailyReports) == 0 || reportInputUsageTotal(dailyReports) == 0 {
		content, report, err := s.saveNoWorkProductionReport(ctx, job, UsageBriefPeriodWeekly, 0)
		return content, report, UsageBriefStatusSucceeded, err
	}
	meta := map[string]any{
		"user_id":      *job.UserID,
		"label":        "周报",
		"period_start": job.PeriodStart.Format("2006-01-02"),
		"period_end":   job.PeriodEnd.Format("2006-01-02"),
		"source_count": len(dailyReports),
	}
	content, inputTokens, outputTokens, err := s.generateReportBrief(ctx, job, "周报", meta, dailyReports, workSummaryFinalInstruction("工作周报", "请基于周一到周日 7 天工作日报的分片摘要生成普通用户工作周报。重点总结本周工作成果、模块推进、设计或实现进展、贡献度、风险和下周建议。"))
	if err != nil {
		return "", nil, "", err
	}
	if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
		return "", nil, "", err
	}
	generatedAt := time.Now()
	title := workReportTitle(UsageBriefPeriodWeekly, *job.PeriodStart, *job.PeriodEnd)
	report, _, err := s.repo.UpsertGeneratedReport(ctx, UsageBriefReport{
		UserID:          *job.UserID,
		PeriodType:      UsageBriefPeriodWeekly,
		PeriodStart:     *job.PeriodStart,
		PeriodEnd:       *job.PeriodEnd,
		Status:          UsageBriefStatusSucceeded,
		Title:           title,
		ContentMD:       content,
		SourceKind:      UsageBriefJobScopeProduction,
		GeneratedByJob:  &job.ID,
		InputUsageCount: len(dailyReports),
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		GeneratedAt:     &generatedAt,
	})
	if err != nil {
		return "", nil, "", err
	}
	return content, report, UsageBriefStatusSucceeded, nil
}

func (s *UsageBriefService) generateMonthlyReport(ctx context.Context, job UsageBriefJob) (string, *UsageBriefReport, string, error) {
	if err := s.waitForActiveDailyDependencies(ctx, job.ID, *job.UserID, *job.PeriodStart, *job.PeriodEnd); err != nil {
		return "", nil, "", err
	}
	if err := s.waitForActiveWeeklyDependencies(ctx, job.ID, *job.UserID, *job.PeriodStart, *job.PeriodEnd); err != nil {
		return "", nil, "", err
	}
	weeklyReports, err := s.repo.ListWeeklyReportsForMonth(ctx, *job.UserID, *job.PeriodStart, *job.PeriodEnd)
	if err != nil {
		return "", nil, "", err
	}
	dailyReports, err := s.repo.ListDailyReports(ctx, *job.UserID, *job.PeriodStart, *job.PeriodEnd)
	if err != nil {
		return "", nil, "", err
	}
	if reportsRepresentNoWork(dailyReports, weeklyReports) {
		content, report, err := s.saveNoWorkProductionReport(ctx, job, UsageBriefPeriodMonthly, 0)
		return content, report, UsageBriefStatusSucceeded, err
	}
	sources := make([]UsageBriefReport, 0, len(weeklyReports)+len(dailyReports))
	sources = append(sources, weeklyReports...)
	sources = append(sources, dailyReports...)
	meta := map[string]any{
		"user_id":        *job.UserID,
		"label":          "月报",
		"period_start":   job.PeriodStart.Format("2006-01-02"),
		"period_end":     job.PeriodEnd.Format("2006-01-02"),
		"weekly_count":   len(weeklyReports),
		"daily_count":    len(dailyReports),
		"monthly_scope":  "严格自然月",
		"rollup_comment": "如果周报跨月，只能采用属于本自然月的结论，不得把月外日期计入本月。",
	}
	content, inputTokens, outputTokens, err := s.generateReportBrief(ctx, job, "月报", meta, sources, workSummaryFinalInstruction("工作月报", "请基于严格自然月内的工作日报分片摘要生成普通用户工作月报，并参考周报提炼趋势。重点总结本月核心工作成果、模块设计贡献、工作连续性、主要风险和下月建议；统计口径必须严格限定在 period_start 到 period_end 之间。"))
	if err != nil {
		return "", nil, "", err
	}
	if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
		return "", nil, "", err
	}
	generatedAt := time.Now()
	title := workReportTitle(UsageBriefPeriodMonthly, *job.PeriodStart, *job.PeriodEnd)
	report, _, err := s.repo.UpsertGeneratedReport(ctx, UsageBriefReport{
		UserID:          *job.UserID,
		PeriodType:      UsageBriefPeriodMonthly,
		PeriodStart:     *job.PeriodStart,
		PeriodEnd:       *job.PeriodEnd,
		Status:          UsageBriefStatusSucceeded,
		Title:           title,
		ContentMD:       content,
		SourceKind:      UsageBriefJobScopeProduction,
		GeneratedByJob:  &job.ID,
		InputUsageCount: len(weeklyReports) + len(dailyReports),
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		GeneratedAt:     &generatedAt,
	})
	if err != nil {
		return "", nil, "", err
	}
	return content, report, UsageBriefStatusSucceeded, nil
}

func (s *UsageBriefService) saveNoWorkProductionReport(ctx context.Context, job UsageBriefJob, periodType string, inputUsageCount int) (string, *UsageBriefReport, error) {
	if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
		return "", nil, err
	}
	_ = s.repo.DeleteJobChunks(ctx, job.ID)
	s.updateJobProgress(ctx, job.ID, 1, 1, "no_usage_records", 0, 0, 0, 0, 0, 0)
	generatedAt := time.Now()
	title := workReportTitle(periodType, *job.PeriodStart, *job.PeriodEnd)
	content := noWorkReportContent(periodType)
	report, overwritten, err := s.repo.UpsertGeneratedReport(ctx, UsageBriefReport{
		UserID:          *job.UserID,
		PeriodType:      periodType,
		PeriodStart:     *job.PeriodStart,
		PeriodEnd:       *job.PeriodEnd,
		Status:          UsageBriefStatusSucceeded,
		Title:           title,
		ContentMD:       content,
		SourceKind:      UsageBriefJobScopeProduction,
		GeneratedByJob:  &job.ID,
		InputUsageCount: inputUsageCount,
		InputTokens:     0,
		OutputTokens:    0,
		GeneratedAt:     &generatedAt,
	})
	if err != nil {
		return "", nil, err
	}
	if !overwritten && report != nil && report.IsAdminEdited {
		return report.ContentMD, report, nil
	}
	if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
		return "", nil, err
	}
	return content, report, nil
}

func (s *UsageBriefService) waitForActiveDailyDependencies(ctx context.Context, jobID, userID int64, start, end time.Time) error {
	for day := dateOnlyTime(start); !day.After(dateOnlyTime(end)); day = day.AddDate(0, 0, 1) {
		if err := s.ensureJobStillActive(ctx, jobID); err != nil {
			return err
		}
		active, err := s.repo.GetActiveJobByPeriod(ctx, userID, UsageBriefPeriodDaily, day, day)
		if err != nil {
			return err
		}
		if active != nil {
			return newUsageBriefDependencyWaitError("waiting for daily report %s to finish", day.Format("2006-01-02"))
		}
	}
	return nil
}

func (s *UsageBriefService) waitForActiveWeeklyDependencies(ctx context.Context, jobID, userID int64, start, end time.Time) error {
	seen := map[string]bool{}
	for day := dateOnlyTime(start); !day.After(dateOnlyTime(end)); day = day.AddDate(0, 0, 1) {
		if err := s.ensureJobStillActive(ctx, jobID); err != nil {
			return err
		}
		weekStart, weekEnd, err := UsageBriefPeriodBounds(UsageBriefPeriodWeekly, day)
		if err != nil {
			return err
		}
		key := weekStart.Format("2006-01-02")
		if seen[key] {
			continue
		}
		seen[key] = true
		active, err := s.repo.GetActiveJobByPeriod(ctx, userID, UsageBriefPeriodWeekly, weekStart, weekEnd)
		if err != nil {
			return err
		}
		if active != nil {
			return newUsageBriefDependencyWaitError("waiting for weekly report %s to %s to finish", weekStart.Format("2006-01-02"), weekEnd.Format("2006-01-02"))
		}
	}
	return nil
}

func (s *UsageBriefService) generateTestJob(ctx context.Context, job UsageBriefJob) (string, *UsageBriefReport, string, error) {
	if job.UserID == nil || job.RangeStart == nil || job.RangeEnd == nil {
		return "", nil, "", fmt.Errorf("test job missing user or range")
	}
	sourceRange := UsageBriefSourceRange{
		UserID:  *job.UserID,
		GroupID: job.GroupID,
		Start:   *job.RangeStart,
		End:     *job.RangeEnd,
	}
	usageSummary, err := s.repo.SummarizeUsageRecords(ctx, UsageBriefUsageRecordFilter{
		UserID:  sourceRange.UserID,
		GroupID: sourceRange.GroupID,
		Start:   sourceRange.Start,
		End:     sourceRange.End,
	})
	if err != nil {
		return "", nil, "", err
	}
	meta := map[string]any{
		"user_id":     *job.UserID,
		"range_start": job.RangeStart.Format(time.RFC3339),
		"range_end":   job.RangeEnd.Format(time.RFC3339),
		"group_id":    job.GroupID,
		"summary":     usageSummary.toMap(),
	}
	content, _, _, status, err := s.generateRecordBrief(ctx, job, "测试简报", meta, sourceRange, "这是管理员测试生成任务。请基于分片摘要生成简洁的中文 Markdown 测试简报，重点凸显普通用户在选定时间范围内的工作情况、模块设计进展、贡献度、风险和建议；token 与成本只用一句话概括。")
	if err != nil {
		return "", nil, "", err
	}
	return content, nil, status, nil
}

type usageBriefPromptItem struct {
	Payload         any
	EstimatedTokens int
}

type usageBriefPromptChunk struct {
	Items           []usageBriefPromptItem
	EstimatedTokens int
}

type usageBriefChunkSummary struct {
	Title           string `json:"title"`
	ContentMD       string `json:"content_md"`
	EstimatedTokens int    `json:"estimated_tokens"`
}

func (s *UsageBriefService) generateRecordBrief(ctx context.Context, job UsageBriefJob, label string, meta map[string]any, sourceRange UsageBriefSourceRange, finalInstruction string) (string, int, int, string, error) {
	settings, err := s.requireUsageBriefAISettings(ctx)
	if err != nil {
		return "", 0, 0, "", err
	}
	budget := chunkInputTokenBudget(settings)
	chunkTotal, totalEstimated, err := s.prepareSourceRecordChunks(ctx, job, label, meta, sourceRange, budget)
	if err != nil {
		return "", 0, 0, "", err
	}
	if chunkTotal == 0 {
		return "", 0, 0, "", fmt.Errorf("usage brief source chunks are empty")
	}
	return s.generatePreparedSourceBrief(ctx, settings, job, label, meta, finalInstruction, chunkTotal, totalEstimated)
}

func (s *UsageBriefService) generateReportBrief(ctx context.Context, job UsageBriefJob, label string, meta map[string]any, reports []UsageBriefReport, finalInstruction string) (string, int, int, error) {
	settings, err := s.requireUsageBriefAISettings(ctx)
	if err != nil {
		return "", 0, 0, err
	}
	budget := chunkInputTokenBudget(settings)
	items := reportPromptItemsForChunks(reports, budget)
	return s.generateChunkedBrief(ctx, settings, job, label, "markdown_reports", meta, items, finalInstruction)
}

func (s *UsageBriefService) prepareSourceRecordChunks(ctx context.Context, job UsageBriefJob, label string, meta map[string]any, sourceRange UsageBriefSourceRange, budget int) (int, int, error) {
	if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
		return 0, 0, err
	}
	const usageRecordPageSize = 50
	var (
		chunkIndex          int
		totalEstimated      int
		current             usageBriefPromptChunk
		beforeCreatedAt     *time.Time
		beforeID            int64
		processedRecordRows int
		keptRecords         []usageBriefCompressedPromptRecord
	)
	flush := func() error {
		if len(current.Items) == 0 {
			return nil
		}
		chunkIndex++
		chunkPayload := map[string]any{
			"label":       label,
			"item_kind":   "usage_records",
			"chunk_index": chunkIndex,
			"meta":        meta,
			"items":       promptChunkPayloads(current.Items),
		}
		contentJSON := mustCompactJSON(chunkPayload)
		if _, ok, err := s.reusableSucceededJobChunk(ctx, job.ID, chunkIndex, UsageBriefChunkTypeSource, contentJSON); err != nil {
			return err
		} else if !ok {
			_, err := s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
				JobID:          job.ID,
				ChunkIndex:     chunkIndex,
				ChunkType:      UsageBriefChunkTypeSource,
				Status:         UsageBriefStatusQueued,
				TokenEstimated: current.EstimatedTokens,
				ContentJSON:    contentJSON,
			})
			if err != nil {
				return err
			}
		}
		totalEstimated += current.EstimatedTokens
		s.updateJobProgress(ctx, job.ID, chunkIndex, 0, "chunking", chunkIndex, 0, totalEstimated, totalEstimated, 0, 0)
		current = usageBriefPromptChunk{}
		return nil
	}

	beforeCreatedAt = &sourceRange.End
	beforeID = int64(^uint64(0) >> 1)
	for {
		if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
			return chunkIndex, totalEstimated, err
		}
		records, err := s.repo.FetchUsageRecordsPage(ctx, UsageBriefUsageRecordFilter{
			UserID:          sourceRange.UserID,
			GroupID:         sourceRange.GroupID,
			Start:           sourceRange.Start,
			End:             sourceRange.End,
			BeforeCreatedAt: beforeCreatedAt,
			BeforeID:        beforeID,
			Limit:           usageRecordPageSize,
		})
		if err != nil {
			return chunkIndex, totalEstimated, err
		}
		if len(records) == 0 {
			break
		}
		processedRecordRows += len(records)
		keptRecords, err = s.appendCompressedUsageBriefRecordsForPrompt(ctx, keptRecords, records)
		if err != nil {
			return chunkIndex, totalEstimated, err
		}
		last := records[len(records)-1]
		beforeCreatedAt = &last.CreatedAt
		beforeID = last.ID
		if len(records) < usageRecordPageSize {
			break
		}
	}
	recordsForPrompt := usageBriefPromptRecordsFromCompressedKept(keptRecords)
	conversationTotal, err := s.upsertJobConversations(ctx, job.ID, recordsForPrompt)
	if err != nil {
		return chunkIndex, totalEstimated, err
	}
	for _, item := range recordPromptItems(recordsForPrompt, budget) {
		if len(current.Items) > 0 && current.EstimatedTokens+item.EstimatedTokens > budget {
			if err := flush(); err != nil {
				return chunkIndex, totalEstimated, err
			}
		}
		current.Items = append(current.Items, item)
		current.EstimatedTokens += item.EstimatedTokens
		if current.EstimatedTokens >= budget {
			if err := flush(); err != nil {
				return chunkIndex, totalEstimated, err
			}
		}
	}
	if err := flush(); err != nil {
		return chunkIndex, totalEstimated, err
	}
	if chunkIndex == 0 && processedRecordRows > 0 {
		chunkIndex = 1
		chunkPayload := map[string]any{
			"label":       label,
			"item_kind":   "usage_records",
			"chunk_index": chunkIndex,
			"meta":        meta,
			"items":       []any{},
		}
		contentJSON := mustCompactJSON(chunkPayload)
		tokenEstimated := estimateTokenCount(contentJSON)
		if _, ok, err := s.reusableSucceededJobChunk(ctx, job.ID, chunkIndex, UsageBriefChunkTypeSource, contentJSON); err != nil {
			return 0, 0, err
		} else if !ok {
			if _, err := s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
				JobID:          job.ID,
				ChunkIndex:     chunkIndex,
				ChunkType:      UsageBriefChunkTypeSource,
				Status:         UsageBriefStatusQueued,
				TokenEstimated: tokenEstimated,
				ContentJSON:    contentJSON,
			}); err != nil {
				return 0, 0, err
			}
		}
		totalEstimated = tokenEstimated
	}
	_ = s.repo.DeleteStaleJobChunks(ctx, job.ID, UsageBriefChunkTypeSource, chunkIndex)
	_ = s.repo.DeleteStaleJobConversations(ctx, job.ID, conversationTotal)
	s.updateJobProgress(ctx, job.ID, 0, chunkIndex+1, "chunks_prepared", 0, chunkIndex, 0, totalEstimated, 0, 0)
	return chunkIndex, totalEstimated, nil
}

func (s *UsageBriefService) generatePreparedSourceBrief(ctx context.Context, settings *UsageBriefSettings, job UsageBriefJob, label string, meta map[string]any, finalInstruction string, chunkTotal, totalEstimated int) (string, int, int, string, error) {
	var processedEstimated, actualInput, actualOutput, skippedCount int
	for chunkIndex := 1; chunkIndex <= chunkTotal; chunkIndex++ {
		chunk, err := s.repo.GetJobChunk(ctx, job.ID, chunkIndex, UsageBriefChunkTypeSource)
		if err != nil {
			return "", actualInput, actualOutput, "", err
		}
		if chunk == nil {
			return "", actualInput, actualOutput, "", fmt.Errorf("usage brief source chunk %d is missing", chunkIndex)
		}
		if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
			return "", actualInput, actualOutput, "", err
		}
		if chunk.Status == UsageBriefStatusSucceeded && strings.TrimSpace(chunk.SummaryMD) != "" {
			processedEstimated += chunk.TokenEstimated
			actualInput += chunk.InputTokens
			actualOutput += chunk.OutputTokens
			s.updateJobProgress(ctx, job.ID, chunk.ChunkIndex, chunkTotal+1, "summarizing_chunks", chunk.ChunkIndex, chunkTotal, processedEstimated, totalEstimated, actualInput, actualOutput)
			continue
		}
		if chunk.Status == UsageBriefStatusFailed && chunk.RetryCount >= usageBriefSourceChunkRetryLimit {
			skippedCount++
			processedEstimated += chunk.TokenEstimated
			actualInput += chunk.InputTokens
			actualOutput += chunk.OutputTokens
			s.updateJobProgress(ctx, job.ID, chunk.ChunkIndex, chunkTotal+1, "summarizing_chunks", chunk.ChunkIndex, chunkTotal, processedEstimated, totalEstimated, actualInput, actualOutput)
			continue
		}
		summary, inTokens, outTokens, retryCount, err := s.summarizeSourceChunkWithRetries(ctx, settings, job.ID, label, chunk)
		actualInput += inTokens
		actualOutput += outTokens
		if err != nil {
			if activeErr := s.ensureJobStillActive(ctx, job.ID); activeErr != nil {
				return "", actualInput, actualOutput, "", activeErr
			}
			skippedCount++
			processedEstimated += chunk.TokenEstimated
			s.updateJobProgress(ctx, job.ID, chunk.ChunkIndex, chunkTotal+1, "summarizing_chunks", chunk.ChunkIndex, chunkTotal, processedEstimated, totalEstimated, actualInput, actualOutput)
			continue
		}
		if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
			return "", actualInput, actualOutput, "", err
		}
		finishedAt := time.Now()
		_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
			JobID:          job.ID,
			ChunkIndex:     chunk.ChunkIndex,
			ChunkType:      UsageBriefChunkTypeSource,
			Status:         UsageBriefStatusSucceeded,
			TokenEstimated: chunk.TokenEstimated,
			InputTokens:    inTokens,
			OutputTokens:   outTokens,
			RetryCount:     retryCount,
			ContentJSON:    chunk.ContentJSON,
			SummaryMD:      summary,
			StartedAt:      chunk.StartedAt,
			FinishedAt:     &finishedAt,
		})
		processedEstimated += chunk.TokenEstimated
		s.updateJobProgress(ctx, job.ID, chunk.ChunkIndex, chunkTotal+1, "summarizing_chunks", chunk.ChunkIndex, chunkTotal, processedEstimated, totalEstimated, actualInput, actualOutput)
	}
	mergeInstruction := finalInstruction
	status := UsageBriefStatusSucceeded
	if skippedCount > 0 {
		status = UsageBriefStatusPartial
		mergeInstruction = partialUsageBriefFinalInstruction(finalInstruction, skippedCount, chunkTotal)
	}
	content, mergeIn, mergeOut, err := s.mergeStoredSourceChunkSummaries(ctx, settings, job, label, meta, mergeInstruction, chunkTotal, totalEstimated, actualInput, actualOutput)
	actualInput += mergeIn
	actualOutput += mergeOut
	if err != nil {
		return "", actualInput, actualOutput, "", err
	}
	if skippedCount > 0 {
		content = partialUsageBriefReportPrefix(skippedCount, chunkTotal) + "\n\n" + content
	}
	s.updateJobProgress(ctx, job.ID, chunkTotal+1, chunkTotal+1, "merged", chunkTotal, chunkTotal, totalEstimated, totalEstimated, actualInput, actualOutput)
	return content, actualInput, actualOutput, status, nil
}

func (s *UsageBriefService) summarizeSourceChunkWithRetries(ctx context.Context, settings *UsageBriefSettings, jobID int64, label string, chunk *UsageBriefJobChunk) (string, int, int, int, error) {
	if chunk == nil {
		return "", 0, 0, 0, fmt.Errorf("usage brief source chunk is nil")
	}
	chunkPayload := chunk.ContentJSON
	prompt := promptWithPayload(
		fmt.Sprintf("请总结以下%s输入分片，输出中文 Markdown 分片摘要。只总结当前分片，优先提炼普通用户正在完成的工作、模块设计或实现进展、贡献线索、风险和阻塞；token 与成本只保留一句话级别的概括。", label),
		json.RawMessage(chunkPayload),
	)
	var totalInput, totalOutput int
	retryCount := maxInt(0, chunk.RetryCount)
	startAttempt := 0
	if retryCount > 0 {
		startAttempt = retryCount + 1
	} else if chunk.Status == UsageBriefStatusFailed && strings.TrimSpace(chunk.ErrorMessage) != "" {
		startAttempt = 1
	}
	startedAt := time.Now()
	if chunk.StartedAt != nil {
		startedAt = *chunk.StartedAt
	}
	for attempt := startAttempt; attempt <= usageBriefSourceChunkRetryLimit; attempt++ {
		if err := s.ensureJobStillActive(ctx, jobID); err != nil {
			return "", totalInput, totalOutput, retryCount, err
		}
		_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
			JobID:          jobID,
			ChunkIndex:     chunk.ChunkIndex,
			ChunkType:      UsageBriefChunkTypeSource,
			Status:         UsageBriefStatusRunning,
			TokenEstimated: chunk.TokenEstimated,
			InputTokens:    totalInput,
			OutputTokens:   totalOutput,
			RetryCount:     maxInt(0, attempt-1),
			ContentJSON:    chunkPayload,
			StartedAt:      &startedAt,
		})
		summary, inTokens, outTokens, err := s.generateMarkdownWithSettings(ctx, settings, prompt)
		totalInput += inTokens
		totalOutput += outTokens
		if err == nil {
			return summary, totalInput, totalOutput, retryCount, nil
		}
		lastErrorAt := time.Now()
		nextRetryCount := attempt
		if activeErr := s.ensureJobStillActive(ctx, jobID); activeErr != nil {
			return "", totalInput, totalOutput, nextRetryCount, activeErr
		}
		errMsg := detailedUsageBriefChunkError(err, nextRetryCount, usageBriefSourceChunkRetryLimit)
		_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
			JobID:          jobID,
			ChunkIndex:     chunk.ChunkIndex,
			ChunkType:      UsageBriefChunkTypeSource,
			Status:         UsageBriefStatusFailed,
			TokenEstimated: chunk.TokenEstimated,
			InputTokens:    totalInput,
			OutputTokens:   totalOutput,
			RetryCount:     nextRetryCount,
			ContentJSON:    chunkPayload,
			ErrorMessage:   errMsg,
			LastErrorAt:    &lastErrorAt,
			StartedAt:      &startedAt,
			FinishedAt:     &lastErrorAt,
		})
		retryCount = nextRetryCount
		if attempt >= usageBriefSourceChunkRetryLimit {
			return "", totalInput, totalOutput, retryCount, fmt.Errorf("%s", errMsg)
		}
		if err := sleepUsageBriefSourceChunkRetry(ctx); err != nil {
			return "", totalInput, totalOutput, retryCount, err
		}
	}
	return "", totalInput, totalOutput, retryCount, fmt.Errorf("usage brief source chunk %d failed after retries", chunk.ChunkIndex)
}

func (s *UsageBriefService) mergeStoredSourceChunkSummaries(ctx context.Context, settings *UsageBriefSettings, job UsageBriefJob, label string, meta map[string]any, finalInstruction string, chunkTotal, totalEstimated, sourceInputTokens, sourceOutputTokens int) (string, int, int, error) {
	summaries := make([]usageBriefChunkSummary, 0, minInt(chunkTotal, 50))
	for page := 1; ; page++ {
		chunks, total, err := s.repo.ListJobChunkSummaries(ctx, job.ID, UsageBriefJobChunkFilter{
			ChunkType: UsageBriefChunkTypeSource,
			Page:      page,
			PageSize:  50,
		})
		if err != nil {
			return "", 0, 0, err
		}
		if total > 0 {
			chunkTotal = int(total)
		}
		if len(chunks) == 0 {
			break
		}
		for _, chunk := range chunks {
			if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
				return "", 0, 0, err
			}
			if chunk.Status == UsageBriefStatusFailed {
				continue
			}
			if chunk.Status != UsageBriefStatusSucceeded || strings.TrimSpace(chunk.SummaryMD) == "" {
				return "", 0, 0, fmt.Errorf("usage brief source chunk %d is not summarized", chunk.ChunkIndex)
			}
			summaries = append(summaries, usageBriefChunkSummary{
				Title:           fmt.Sprintf("%s分片 %d/%d", label, chunk.ChunkIndex, chunkTotal),
				ContentMD:       chunk.SummaryMD,
				EstimatedTokens: estimateTokenCount(chunk.SummaryMD),
			})
		}
		if int64(page*50) >= total {
			break
		}
	}
	if len(summaries) == 0 {
		return "", 0, 0, fmt.Errorf("usage brief source summaries are empty")
	}
	s.updateJobProgress(ctx, job.ID, chunkTotal, chunkTotal+1, "merging", chunkTotal, chunkTotal, totalEstimated, totalEstimated, sourceInputTokens, sourceOutputTokens)
	return s.mergeChunkSummaries(ctx, settings, job, label, meta, finalInstruction, summaries, 1)
}

func (s *UsageBriefService) requireUsageBriefAISettings(ctx context.Context) (*UsageBriefSettings, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		return nil, fmt.Errorf("usage brief ai api key is not configured")
	}
	return settings, nil
}

func (s *UsageBriefService) generateChunkedBrief(ctx context.Context, settings *UsageBriefSettings, job UsageBriefJob, label, itemKind string, meta map[string]any, items []usageBriefPromptItem, finalInstruction string) (string, int, int, error) {
	if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
		return "", 0, 0, err
	}
	chunks := promptItemChunks(items, chunkInputTokenBudget(settings))
	if len(chunks) == 0 {
		chunks = []usageBriefPromptChunk{{}}
	}
	_ = s.repo.DeleteStaleJobChunks(ctx, job.ID, UsageBriefChunkTypeSource, len(chunks))
	totalEstimated := 0
	for _, item := range items {
		totalEstimated += item.EstimatedTokens
	}
	totalSteps := len(chunks) + 1
	s.updateJobProgress(ctx, job.ID, 0, totalSteps, "chunking", 0, len(chunks), 0, totalEstimated, 0, 0)

	summaries := make([]usageBriefChunkSummary, 0, len(chunks))
	var processedEstimated, actualInput, actualOutput int
	for i, chunk := range chunks {
		if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
			return "", actualInput, actualOutput, err
		}
		startedAt := time.Now()
		chunkPayload := map[string]any{
			"label":       label,
			"item_kind":   itemKind,
			"chunk_index": i + 1,
			"chunk_total": len(chunks),
			"meta":        meta,
			"items":       promptChunkPayloads(chunk.Items),
		}
		contentJSON := mustCompactJSON(chunkPayload)
		reused, ok, err := s.reusableSucceededJobChunk(ctx, job.ID, i+1, UsageBriefChunkTypeSource, contentJSON)
		if err != nil {
			return "", actualInput, actualOutput, err
		}
		if ok {
			processedEstimated += chunk.EstimatedTokens
			actualInput += reused.InputTokens
			actualOutput += reused.OutputTokens
			summaries = append(summaries, usageBriefChunkSummary{
				Title:           fmt.Sprintf("%s分片 %d/%d", label, i+1, len(chunks)),
				ContentMD:       reused.SummaryMD,
				EstimatedTokens: estimateTokenCount(reused.SummaryMD),
			})
			s.updateJobProgress(ctx, job.ID, i+1, totalSteps, "summarizing_chunks", i+1, len(chunks), processedEstimated, totalEstimated, actualInput, actualOutput)
			continue
		}
		_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
			JobID:          job.ID,
			ChunkIndex:     i + 1,
			ChunkType:      UsageBriefChunkTypeSource,
			Status:         UsageBriefStatusRunning,
			TokenEstimated: chunk.EstimatedTokens,
			ContentJSON:    contentJSON,
			StartedAt:      &startedAt,
		})
		prompt := promptWithPayload(
			fmt.Sprintf("请总结以下%s输入分片，输出中文 Markdown 分片摘要。只总结当前分片，优先提炼普通用户正在完成的工作、模块设计或实现进展、贡献线索、风险和阻塞；token 与成本只保留一句话级别的概括。", label),
			chunkPayload,
		)
		summary, inTokens, outTokens, err := s.generateMarkdownWithSettings(ctx, settings, prompt)
		actualInput += inTokens
		actualOutput += outTokens
		finishedAt := time.Now()
		if err != nil {
			if activeErr := s.ensureJobStillActive(ctx, job.ID); activeErr != nil {
				return "", actualInput, actualOutput, activeErr
			}
			_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
				JobID:          job.ID,
				ChunkIndex:     i + 1,
				ChunkType:      UsageBriefChunkTypeSource,
				Status:         UsageBriefStatusFailed,
				TokenEstimated: chunk.EstimatedTokens,
				InputTokens:    inTokens,
				OutputTokens:   outTokens,
				ContentJSON:    contentJSON,
				ErrorMessage:   err.Error(),
				StartedAt:      &startedAt,
				FinishedAt:     &finishedAt,
			})
			return "", actualInput, actualOutput, err
		}
		if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
			return "", actualInput, actualOutput, err
		}
		_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
			JobID:          job.ID,
			ChunkIndex:     i + 1,
			ChunkType:      UsageBriefChunkTypeSource,
			Status:         UsageBriefStatusSucceeded,
			TokenEstimated: chunk.EstimatedTokens,
			InputTokens:    inTokens,
			OutputTokens:   outTokens,
			ContentJSON:    contentJSON,
			SummaryMD:      summary,
			StartedAt:      &startedAt,
			FinishedAt:     &finishedAt,
		})
		processedEstimated += chunk.EstimatedTokens
		summaries = append(summaries, usageBriefChunkSummary{
			Title:           fmt.Sprintf("%s分片 %d/%d", label, i+1, len(chunks)),
			ContentMD:       summary,
			EstimatedTokens: estimateTokenCount(summary),
		})
		s.updateJobProgress(ctx, job.ID, i+1, totalSteps, "summarizing_chunks", i+1, len(chunks), processedEstimated, totalEstimated, actualInput, actualOutput)
	}

	content, mergeIn, mergeOut, err := s.mergeChunkSummaries(ctx, settings, job, label, meta, finalInstruction, summaries, 1)
	actualInput += mergeIn
	actualOutput += mergeOut
	if err != nil {
		return "", actualInput, actualOutput, err
	}
	s.updateJobProgress(ctx, job.ID, totalSteps, totalSteps, "merged", len(chunks), len(chunks), totalEstimated, totalEstimated, actualInput, actualOutput)
	return content, actualInput, actualOutput, nil
}

func (s *UsageBriefService) mergeChunkSummaries(ctx context.Context, settings *UsageBriefSettings, job UsageBriefJob, label string, meta map[string]any, instruction string, summaries []usageBriefChunkSummary, depth int) (string, int, int, error) {
	if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
		return "", 0, 0, err
	}
	budget := chunkInputTokenBudget(settings)
	groups := summaryGroups(summaries, budget)
	if len(groups) > 1 {
		compacted := make([]usageBriefChunkSummary, 0, len(groups))
		var totalIn, totalOut int
		for i, group := range groups {
			if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
				return "", totalIn, totalOut, err
			}
			payload := map[string]any{
				"label":       label,
				"merge_level": depth,
				"group_index": i + 1,
				"group_total": len(groups),
				"meta":        meta,
				"summaries":   group,
			}
			startedAt := time.Now()
			contentJSON := mustCompactJSON(payload)
			chunkIndex := 100000 + depth*1000 + i + 1
			reused, ok, err := s.reusableSucceededJobChunk(ctx, job.ID, chunkIndex, UsageBriefChunkTypeMerge, contentJSON)
			if err != nil {
				return "", totalIn, totalOut, err
			}
			if ok {
				totalIn += reused.InputTokens
				totalOut += reused.OutputTokens
				compacted = append(compacted, usageBriefChunkSummary{
					Title:           fmt.Sprintf("%s中间摘要 %d/%d", label, i+1, len(groups)),
					ContentMD:       reused.SummaryMD,
					EstimatedTokens: estimateTokenCount(reused.SummaryMD),
				})
				continue
			}
			_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
				JobID:          job.ID,
				ChunkIndex:     chunkIndex,
				ChunkType:      UsageBriefChunkTypeMerge,
				Status:         UsageBriefStatusRunning,
				TokenEstimated: estimateTokenCount(contentJSON),
				ContentJSON:    contentJSON,
				StartedAt:      &startedAt,
			})
			prompt := promptWithPayload("请将以下分片摘要合并为更短的中文 Markdown 中间摘要。优先保留工作内容、模块/设计进展、贡献度、风险和建议，去掉重复内容；token 与成本只保留一句话概括。", payload)
			content, inTokens, outTokens, err := s.generateMarkdownWithSettings(ctx, settings, prompt)
			totalIn += inTokens
			totalOut += outTokens
			finishedAt := time.Now()
			if err != nil {
				if activeErr := s.ensureJobStillActive(ctx, job.ID); activeErr != nil {
					return "", totalIn, totalOut, activeErr
				}
				_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
					JobID:          job.ID,
					ChunkIndex:     chunkIndex,
					ChunkType:      UsageBriefChunkTypeMerge,
					Status:         UsageBriefStatusFailed,
					TokenEstimated: estimateTokenCount(contentJSON),
					InputTokens:    inTokens,
					OutputTokens:   outTokens,
					ContentJSON:    contentJSON,
					ErrorMessage:   err.Error(),
					StartedAt:      &startedAt,
					FinishedAt:     &finishedAt,
				})
				return "", totalIn, totalOut, err
			}
			if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
				return "", totalIn, totalOut, err
			}
			_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
				JobID:          job.ID,
				ChunkIndex:     chunkIndex,
				ChunkType:      UsageBriefChunkTypeMerge,
				Status:         UsageBriefStatusSucceeded,
				TokenEstimated: estimateTokenCount(contentJSON),
				InputTokens:    inTokens,
				OutputTokens:   outTokens,
				ContentJSON:    contentJSON,
				SummaryMD:      content,
				StartedAt:      &startedAt,
				FinishedAt:     &finishedAt,
			})
			compacted = append(compacted, usageBriefChunkSummary{
				Title:           fmt.Sprintf("%s中间摘要 %d/%d", label, i+1, len(groups)),
				ContentMD:       content,
				EstimatedTokens: estimateTokenCount(content),
			})
		}
		final, inTokens, outTokens, err := s.mergeChunkSummaries(ctx, settings, job, label, meta, instruction, compacted, depth+1)
		return final, totalIn + inTokens, totalOut + outTokens, err
	}

	payload := map[string]any{
		"label":     label,
		"meta":      meta,
		"summaries": summaries,
	}
	startedAt := time.Now()
	contentJSON := mustCompactJSON(payload)
	chunkIndex := 200000 + depth
	reused, ok, err := s.reusableSucceededJobChunk(ctx, job.ID, chunkIndex, UsageBriefChunkTypeMerge, contentJSON)
	if err != nil {
		return "", 0, 0, err
	}
	if ok {
		return reused.SummaryMD, reused.InputTokens, reused.OutputTokens, nil
	}
	_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
		JobID:          job.ID,
		ChunkIndex:     chunkIndex,
		ChunkType:      UsageBriefChunkTypeMerge,
		Status:         UsageBriefStatusRunning,
		TokenEstimated: estimateTokenCount(contentJSON),
		ContentJSON:    contentJSON,
		StartedAt:      &startedAt,
	})
	content, inTokens, outTokens, err := s.generateMarkdownWithSettings(ctx, settings, promptWithPayload(instruction, payload))
	finishedAt := time.Now()
	if err != nil {
		if activeErr := s.ensureJobStillActive(ctx, job.ID); activeErr != nil {
			return "", inTokens, outTokens, activeErr
		}
		_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
			JobID:          job.ID,
			ChunkIndex:     chunkIndex,
			ChunkType:      UsageBriefChunkTypeMerge,
			Status:         UsageBriefStatusFailed,
			TokenEstimated: estimateTokenCount(contentJSON),
			InputTokens:    inTokens,
			OutputTokens:   outTokens,
			ContentJSON:    contentJSON,
			ErrorMessage:   err.Error(),
			StartedAt:      &startedAt,
			FinishedAt:     &finishedAt,
		})
		return "", inTokens, outTokens, err
	}
	if err := s.ensureJobStillActive(ctx, job.ID); err != nil {
		return "", inTokens, outTokens, err
	}
	_, _ = s.repo.UpsertJobChunk(ctx, UsageBriefJobChunk{
		JobID:          job.ID,
		ChunkIndex:     chunkIndex,
		ChunkType:      UsageBriefChunkTypeMerge,
		Status:         UsageBriefStatusSucceeded,
		TokenEstimated: estimateTokenCount(contentJSON),
		InputTokens:    inTokens,
		OutputTokens:   outTokens,
		ContentJSON:    contentJSON,
		SummaryMD:      content,
		StartedAt:      &startedAt,
		FinishedAt:     &finishedAt,
	})
	return content, inTokens, outTokens, nil
}

func (s *UsageBriefService) upsertJobConversations(ctx context.Context, jobID int64, records []usageBriefPromptRecord) (int, error) {
	for i, rec := range records {
		if err := s.ensureJobStillActive(ctx, jobID); err != nil {
			return i, err
		}
		conversationJSON := strings.TrimSpace(rec.MessagesJSON)
		if conversationJSON == "" {
			conversationJSON = mustCompactJSON(usageBriefFinalMessagesPayload{Messages: rec.Messages})
		}
		coveredIDs := rec.CoveredUsageLogIDs
		if len(coveredIDs) == 0 && rec.ID > 0 {
			coveredIDs = []int64{rec.ID}
		}
		coveredCount := rec.CoveredRequestCount
		if coveredCount <= 0 {
			coveredCount = 1
		}
		if _, err := s.repo.UpsertJobConversation(ctx, UsageBriefJobConversation{
			JobID:               jobID,
			ConversationIndex:   i + 1,
			UsageLogID:          rec.ID,
			CoveredUsageLogIDs:  coveredIDs,
			CoveredRequestCount: coveredCount,
			TokenEstimated:      estimateTokenCount(conversationJSON),
			ConversationJSON:    conversationJSON,
		}); err != nil {
			return i, err
		}
	}
	return len(records), nil
}

func recordPromptItems(records []usageBriefPromptRecord, budget int) []usageBriefPromptItem {
	items := make([]usageBriefPromptItem, 0, len(records))
	for _, rec := range records {
		estimated := estimateTokenCount(mustCompactJSON(rec))
		if estimated <= budget {
			items = append(items, usageBriefPromptItem{Payload: rec, EstimatedTokens: estimated})
			continue
		}
		payload := strings.TrimSpace(rec.MessagesJSON)
		if payload == "" {
			payload = strings.TrimSpace(rec.RequestPayload)
		}
		payloadParts := splitTextByRuneBudget(payload, budget*4)
		if len(payloadParts) == 0 {
			payloadParts = []string{""}
		}
		meta := recordMetaWithoutPayload(rec.UsageBriefSourceRecord)
		if rec.CoveredRequestCount > 1 {
			meta["covered_usage_log_ids"] = rec.CoveredUsageLogIDs
			meta["covered_request_count"] = rec.CoveredRequestCount
			meta["prompt_compacted"] = true
		}
		for i, part := range payloadParts {
			payload := map[string]any{
				"record":             meta,
				"messages_json_part": part,
				"part_index":         i + 1,
				"part_total":         len(payloadParts),
				"note":               "单条最终消息 JSON 超过分片预算，已仅按 messages JSON 字段切分，其他记录字段保持完整。",
			}
			items = append(items, usageBriefPromptItem{Payload: payload, EstimatedTokens: estimateTokenCount(mustCompactJSON(payload))})
		}
	}
	return items
}

type usageBriefPromptRecord struct {
	UsageBriefSourceRecord
	CoveredUsageLogIDs  []int64                       `json:"covered_usage_log_ids,omitempty"`
	CoveredRequestCount int                           `json:"covered_request_count,omitempty"`
	PromptCompacted     bool                          `json:"prompt_compacted,omitempty"`
	Messages            []usageBriefCompressedMessage `json:"messages,omitempty"`
	MessagesJSON        string                        `json:"-"`
}

type usageBriefCompressedMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type usageBriefFinalMessagesPayload struct {
	Messages []usageBriefCompressedMessage `json:"messages"`
}

type usageBriefRequestContext struct {
	raw           map[string]any
	chainKey      string
	generationKey string
	messages      []any
	staticContext map[string]any
	isCompact     bool
}

type usageBriefPromptRecordState struct {
	record usageBriefPromptRecord
	ctx    usageBriefRequestContext
}

type usageBriefCompressedPromptRecord struct {
	record     usageBriefPromptRecord
	comparison []usageBriefCompressedMessage
}

func (s *UsageBriefService) appendCompressedUsageBriefRecordsForPrompt(ctx context.Context, kept []usageBriefCompressedPromptRecord, records []UsageBriefSourceRecord) ([]usageBriefCompressedPromptRecord, error) {
	for _, rec := range records {
		if err := s.ensureUsageBriefRecordCompressedPayloads(ctx, &rec); err != nil {
			return nil, err
		}
		payload, messages := usageBriefFinalMessagesFromCompressedRecord(rec)
		if len(messages) == 0 {
			continue
		}
		comparison := coalesceUsageBriefCompressedMessages(messages)
		if len(comparison) == 0 {
			continue
		}
		containedBy := -1
		for i, existing := range kept {
			if usageBriefCompressedMessagesContained(comparison, existing.comparison) {
				containedBy = i
				break
			}
		}
		if containedBy >= 0 {
			existing := &kept[containedBy].record
			existing.CoveredUsageLogIDs = append(existing.CoveredUsageLogIDs, rec.ID)
			existing.CoveredRequestCount++
			existing.PromptCompacted = existing.CoveredRequestCount > 1
			continue
		}
		for i := len(kept) - 1; i >= 0; i-- {
			if usageBriefCompressedMessagesContained(kept[i].comparison, comparison) {
				kept = append(kept[:i], kept[i+1:]...)
			}
		}
		promptRec := usageBriefPromptRecord{
			UsageBriefSourceRecord: rec,
			CoveredUsageLogIDs:     []int64{rec.ID},
			CoveredRequestCount:    1,
			Messages:               messages,
			MessagesJSON:           payload,
		}
		promptRec.RequestPayload = ""
		promptRec.ResponsePayload = ""
		promptRec.CompressedRequestPayload = ""
		promptRec.CompressedResponsePayload = ""
		kept = append(kept, usageBriefCompressedPromptRecord{record: promptRec, comparison: comparison})
	}
	return kept, nil
}

func usageBriefPromptRecordsFromCompressedKept(kept []usageBriefCompressedPromptRecord) []usageBriefPromptRecord {
	out := make([]usageBriefPromptRecord, 0, len(kept))
	for _, item := range kept {
		record := item.record
		if record.CoveredRequestCount > 1 {
			record.PromptCompacted = true
			out = append(out, record)
			continue
		}
		record.CoveredUsageLogIDs = nil
		record.CoveredRequestCount = 0
		out = append(out, record)
	}
	return out
}

func (s *UsageBriefService) compressedUsageBriefRecordsForPrompt(ctx context.Context, records []UsageBriefSourceRecord) ([]usageBriefPromptRecord, error) {
	kept, err := s.appendCompressedUsageBriefRecordsForPrompt(ctx, nil, records)
	if err != nil {
		return nil, err
	}
	return usageBriefPromptRecordsFromCompressedKept(kept), nil
}

func (s *UsageBriefService) ensureUsageBriefRecordCompressedPayloads(ctx context.Context, rec *UsageBriefSourceRecord) error {
	if rec == nil {
		return nil
	}
	var (
		changed            bool
		compressedRequest  *string
		compressedResponse *string
	)
	if strings.TrimSpace(rec.CompressedRequestPayload) == "" && strings.TrimSpace(rec.RequestPayload) != "" {
		compressed := CompressUsageLogPayloadJSON(&rec.RequestPayload, UsageLogPayloadKindRequest)
		if compressed == nil {
			empty := usageLogDetailEmptyCompressedJSON
			compressed = &empty
		}
		rec.CompressedRequestPayload = *compressed
		compressedRequest = compressed
		changed = true
	}
	if strings.TrimSpace(rec.CompressedResponsePayload) == "" && strings.TrimSpace(rec.ResponsePayload) != "" {
		compressed := CompressUsageLogPayloadJSON(&rec.ResponsePayload, UsageLogPayloadKindResponse)
		if compressed == nil {
			empty := usageLogDetailEmptyCompressedJSON
			compressed = &empty
		}
		rec.CompressedResponsePayload = *compressed
		compressedResponse = compressed
		changed = true
	}
	if changed {
		return s.repo.UpdateUsageRecordCompressedPayloads(ctx, rec.ID, compressedRequest, compressedResponse)
	}
	return nil
}

func usageBriefFinalMessagesFromCompressedRecord(rec UsageBriefSourceRecord) (string, []usageBriefCompressedMessage) {
	messages := make([]usageBriefCompressedMessage, 0, 8)
	messages = append(messages, usageBriefCompressedMessagesFromJSON(rec.CompressedRequestPayload)...)
	messages = append(messages, usageBriefCompressedMessagesFromJSON(rec.CompressedResponsePayload)...)
	payload := usageBriefFinalMessagesPayload{Messages: messages}
	return mustCompactJSON(payload), messages
}

func usageBriefCompressedMessagesFromJSON(raw string) []usageBriefCompressedMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var payload usageBriefFinalMessagesPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	out := make([]usageBriefCompressedMessage, 0, len(payload.Messages))
	for _, message := range payload.Messages {
		message.Role = strings.ToLower(strings.TrimSpace(message.Role))
		message.Content = strings.TrimSpace(message.Content)
		if (message.Role == "user" || message.Role == "assistant") && message.Content != "" {
			out = append(out, message)
		}
	}
	return out
}

func coalesceUsageBriefCompressedMessages(messages []usageBriefCompressedMessage) []usageBriefCompressedMessage {
	out := make([]usageBriefCompressedMessage, 0, len(messages))
	for _, message := range messages {
		if len(out) > 0 && out[len(out)-1].Role == message.Role {
			out[len(out)-1].Content += message.Content
			continue
		}
		out = append(out, message)
	}
	return out
}

func usageBriefCompressedMessagesContained(needle, haystack []usageBriefCompressedMessage) bool {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}
	for start := 0; start <= len(haystack)-len(needle); start++ {
		matched := true
		for offset := range needle {
			n := needle[offset]
			h := haystack[start+offset]
			if n.Role != h.Role {
				matched = false
				break
			}
			if offset == len(needle)-1 {
				if h.Content != n.Content && !strings.HasPrefix(h.Content, n.Content) {
					matched = false
					break
				}
				continue
			}
			if h.Content != n.Content {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func compactUsageBriefRecordsForPrompt(records []UsageBriefSourceRecord) []usageBriefPromptRecord {
	if len(records) < 2 {
		out := make([]usageBriefPromptRecord, 0, len(records))
		for _, rec := range records {
			out = append(out, slimUsageBriefPromptRecord(usageBriefPromptRecord{
				UsageBriefSourceRecord: rec,
				CoveredUsageLogIDs:     []int64{rec.ID},
				CoveredRequestCount:    1,
			}))
		}
		return out
	}
	states := make([]usageBriefPromptRecordState, 0, len(records))
	activeByChain := map[string]int{}
	generationByChain := map[string]int{}
	for _, rec := range records {
		ctx := parseUsageBriefRequestContext(rec)
		if ctx.isCompact && ctx.chainKey != "" {
			generationByChain[ctx.chainKey]++
			delete(activeByChain, ctx.chainKey)
		}
		if ctx.chainKey != "" {
			ctx.generationKey = fmt.Sprintf("%s#%d", ctx.chainKey, generationByChain[ctx.chainKey])
		}
		state := usageBriefPromptRecordState{
			record: usageBriefPromptRecord{
				UsageBriefSourceRecord: rec,
				CoveredUsageLogIDs:     []int64{rec.ID},
				CoveredRequestCount:    1,
			},
			ctx: ctx,
		}
		if ctx.generationKey != "" {
			if prevIndex, ok := activeByChain[ctx.generationKey]; ok && usageBriefRecordStrictlyCovered(states[prevIndex].ctx, ctx) {
				state.record.CoveredUsageLogIDs = append(append([]int64{}, states[prevIndex].record.CoveredUsageLogIDs...), rec.ID)
				state.record.CoveredRequestCount = states[prevIndex].record.CoveredRequestCount + 1
				states[prevIndex].record.RequestPayload = ""
			}
			activeByChain[ctx.generationKey] = len(states)
		}
		states = append(states, state)
	}
	out := make([]usageBriefPromptRecord, 0, len(states))
	for _, state := range states {
		if strings.TrimSpace(state.record.RequestPayload) == "" {
			continue
		}
		out = append(out, slimUsageBriefPromptRecord(state.record))
	}
	return out
}

func slimUsageBriefPromptRecord(rec usageBriefPromptRecord) usageBriefPromptRecord {
	rec.RequestPayload = slimUsageBriefRequestPayload(rec.RequestPayload)
	rec.PromptCompacted = rec.CoveredRequestCount > 1
	if !rec.PromptCompacted {
		rec.CoveredUsageLogIDs = nil
		rec.CoveredRequestCount = 0
	}
	return rec
}

func parseUsageBriefRequestContext(rec UsageBriefSourceRecord) usageBriefRequestContext {
	ctx := usageBriefRequestContext{
		isCompact: isUsageBriefCompactEndpoint(rec.InboundEndpoint) || isUsageBriefCompactEndpoint(rec.UpstreamEndpoint),
	}
	payload := strings.TrimSpace(rec.RequestPayload)
	if payload == "" {
		return ctx
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(payload), &body); err != nil {
		return ctx
	}
	ctx.raw = body
	if isUsageBriefCompactRequest(body) {
		ctx.isCompact = true
	}
	ctx.chainKey = usageBriefPromptChainKey(body)
	ctx.messages = usageBriefPromptConversationItems(body)
	ctx.staticContext = usageBriefPromptStaticContext(body)
	return ctx
}

func isUsageBriefCompactEndpoint(endpoint string) bool {
	endpoint = strings.ToLower(strings.TrimSpace(endpoint))
	return strings.Contains(endpoint, "/compact") || strings.Contains(endpoint, "contextcompaction")
}

func isUsageBriefCompactRequest(body map[string]any) bool {
	kind := strings.ToLower(strings.TrimSpace(jsonStringAt(body, "client_metadata", "x-codex-turn-metadata", "request_kind")))
	if strings.Contains(kind, "compact") {
		return true
	}
	for _, value := range []string{
		jsonStringAt(body, "client_metadata", "request_kind"),
		jsonStringAt(body, "request_kind"),
		jsonStringAt(body, "type"),
	} {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), "compact") {
			return true
		}
	}
	return false
}

func usageBriefPromptChainKey(body map[string]any) string {
	codexThreadID := firstUsageBriefNonEmptyString(
		jsonStringAt(body, "client_metadata", "thread_id"),
		jsonStringAt(body, "client_metadata", "x-codex-turn-metadata", "thread_id"),
	)
	codexWindowID := firstUsageBriefNonEmptyString(
		jsonStringAt(body, "client_metadata", "x-codex-window-id"),
		jsonStringAt(body, "client_metadata", "x-codex-turn-metadata", "window_id"),
	)
	codexSessionID := firstUsageBriefNonEmptyString(
		jsonStringAt(body, "client_metadata", "session_id"),
		jsonStringAt(body, "client_metadata", "x-codex-turn-metadata", "session_id"),
	)
	if codexThreadID != "" && codexWindowID != "" {
		return "codex:" + codexThreadID + ":" + codexWindowID
	}
	if codexThreadID != "" {
		return "codex:" + codexThreadID
	}
	if codexSessionID != "" {
		return "codex:" + codexSessionID
	}
	if value := firstUsageBriefNonEmptyString(
		jsonStringAt(body, "conversation_id"),
		jsonStringAt(body, "thread_id"),
		jsonStringAt(body, "session_id"),
		jsonStringAt(body, "metadata", "session_id"),
		metadataUserIDSessionID(jsonStringAt(body, "metadata", "user_id")),
	); value != "" {
		return "generic:" + value
	}
	return ""
}

func firstUsageBriefNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !strings.EqualFold(value, "[REDACTED]") {
			return value
		}
	}
	return ""
}

func metadataUserIDSessionID(raw string) string {
	if parsed := ParseMetadataUserID(raw); parsed != nil {
		return parsed.SessionID
	}
	return ""
}

func usageBriefPromptConversationItems(body map[string]any) []any {
	if messages, ok := jsonArrayAt(body, "messages"); ok {
		return messages
	}
	if input, ok := jsonArrayAt(body, "input"); ok {
		return input
	}
	return nil
}

func usageBriefPromptStaticContext(body map[string]any) map[string]any {
	keys := []string{
		"model",
		"instructions",
		"system",
		"tools",
		"tool_choice",
		"parallel_tool_calls",
		"reasoning",
		"text",
		"response_format",
		"temperature",
		"top_p",
		"max_tokens",
		"max_output_tokens",
	}
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := body[key]; ok {
			out[key] = value
		}
	}
	return out
}

func usageBriefRecordStrictlyCovered(previous, current usageBriefRequestContext) bool {
	if previous.chainKey == "" || current.chainKey == "" || previous.generationKey != current.generationKey {
		return false
	}
	if previous.isCompact || current.isCompact {
		return false
	}
	if len(previous.messages) == 0 || len(current.messages) <= len(previous.messages) {
		return false
	}
	if !usageBriefJSONEqual(previous.staticContext, current.staticContext) {
		return false
	}
	for i, item := range previous.messages {
		if !usageBriefJSONEqual(item, current.messages[i]) {
			return false
		}
	}
	return true
}

func slimUsageBriefRequestPayload(payload string) string {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return ""
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(payload), &body); err != nil {
		return payload
	}
	slimUsageBriefCodexMetadata(body)
	if include, ok := body["include"].([]any); ok && len(include) == 1 {
		if value, ok := include[0].(string); ok && value == "reasoning.encrypted_content" {
			delete(body, "include")
		}
	}
	return mustCompactJSON(body)
}

func slimUsageBriefCodexMetadata(body map[string]any) {
	metadata, ok := body["client_metadata"].(map[string]any)
	if !ok {
		return
	}
	turnMeta := map[string]any{}
	if parsed, ok := parseUsageBriefEmbeddedJSONObject(asString(metadata["x-codex-turn-metadata"])); ok {
		for _, key := range []string{
			"installation_id",
			"session_id",
			"thread_id",
			"turn_id",
			"window_id",
			"request_kind",
			"workspace_kind",
			"turn_started_at_unix_ms",
		} {
			if value, exists := parsed[key]; exists {
				turnMeta[key] = value
			}
		}
		if workspaces, ok := parsed["workspaces"].(map[string]any); ok {
			turnMeta["workspace_count"] = len(workspaces)
			hasChanges := false
			latestCommit := ""
			for _, rawWorkspace := range workspaces {
				workspace, ok := rawWorkspace.(map[string]any)
				if !ok {
					continue
				}
				if value, ok := workspace["has_changes"].(bool); ok && value {
					hasChanges = true
				}
				if value := asString(workspace["latest_git_commit_hash"]); value != "" {
					latestCommit = value
				}
			}
			turnMeta["has_changes"] = hasChanges
			if latestCommit != "" {
				turnMeta["latest_git_commit_hash"] = latestCommit
			}
		}
	}
	delete(metadata, "x-codex-turn-metadata")
	for key, value := range metadata {
		if strings.TrimSpace(asString(value)) == "" || strings.EqualFold(strings.TrimSpace(asString(value)), "[REDACTED]") {
			delete(metadata, key)
		}
	}
	if len(turnMeta) > 0 {
		metadata["codex_turn_metadata"] = turnMeta
	}
	if len(metadata) == 0 {
		delete(body, "client_metadata")
	}
}

func parseUsageBriefEmbeddedJSONObject(raw string) (map[string]any, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, false
	}
	return out, true
}

func jsonStringAt(root any, path ...string) string {
	value, ok := jsonValueAt(root, path...)
	if !ok {
		return ""
	}
	if s, ok := value.(string); ok {
		if len(path) > 0 && path[len(path)-1] == "x-codex-turn-metadata" {
			return s
		}
		return strings.TrimSpace(s)
	}
	return ""
}

func jsonArrayAt(root any, path ...string) ([]any, bool) {
	value, ok := jsonValueAt(root, path...)
	if !ok {
		return nil, false
	}
	items, ok := value.([]any)
	return items, ok
}

func jsonValueAt(root any, path ...string) (any, bool) {
	var current any = root
	for _, part := range path {
		if part == "x-codex-turn-metadata" {
			obj, ok := current.(map[string]any)
			if !ok {
				return nil, false
			}
			parsed, ok := parseUsageBriefEmbeddedJSONObject(asString(obj[part]))
			if !ok {
				return nil, false
			}
			current = parsed
			continue
		}
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := obj[part]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func usageBriefJSONEqual(a, b any) bool {
	return reflect.DeepEqual(normalizeUsageBriefJSONValue(a), normalizeUsageBriefJSONValue(b))
}

func normalizeUsageBriefJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = normalizeUsageBriefJSONValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeUsageBriefJSONValue(item)
		}
		return out
	case float64:
		if v == float64(int64(v)) {
			return int64(v)
		}
		return v
	default:
		return v
	}
}

func reportPromptItemsForChunks(reports []UsageBriefReport, budget int) []usageBriefPromptItem {
	items := make([]usageBriefPromptItem, 0, len(reports))
	for _, report := range reports {
		payload := map[string]any{
			"id":                report.ID,
			"title":             report.Title,
			"period_type":       report.PeriodType,
			"period_start":      report.PeriodStart.Format("2006-01-02"),
			"period_end":        report.PeriodEnd.Format("2006-01-02"),
			"source_kind":       report.SourceKind,
			"input_usage_count": report.InputUsageCount,
			"input_tokens":      report.InputTokens,
			"output_tokens":     report.OutputTokens,
			"content_md":        report.ContentMD,
		}
		estimated := estimateTokenCount(mustCompactJSON(payload))
		if estimated <= budget {
			items = append(items, usageBriefPromptItem{Payload: payload, EstimatedTokens: estimated})
			continue
		}
		parts := splitTextByRuneBudget(report.ContentMD, budget*4)
		if len(parts) == 0 {
			parts = []string{""}
		}
		for i, part := range parts {
			partPayload := map[string]any{
				"id":           report.ID,
				"title":        report.Title,
				"period_type":  report.PeriodType,
				"period_start": report.PeriodStart.Format("2006-01-02"),
				"period_end":   report.PeriodEnd.Format("2006-01-02"),
				"content_part": part,
				"part_index":   i + 1,
				"part_total":   len(parts),
			}
			items = append(items, usageBriefPromptItem{Payload: partPayload, EstimatedTokens: estimateTokenCount(mustCompactJSON(partPayload))})
		}
	}
	return items
}

func promptItemChunks(items []usageBriefPromptItem, budget int) []usageBriefPromptChunk {
	if budget <= 0 {
		budget = 4000
	}
	chunks := make([]usageBriefPromptChunk, 0)
	var current usageBriefPromptChunk
	for _, item := range items {
		if len(current.Items) > 0 && current.EstimatedTokens+item.EstimatedTokens > budget {
			chunks = append(chunks, current)
			current = usageBriefPromptChunk{}
		}
		current.Items = append(current.Items, item)
		current.EstimatedTokens += item.EstimatedTokens
	}
	if len(current.Items) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

func promptChunkPayloads(items []usageBriefPromptItem) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, item.Payload)
	}
	return out
}

func summaryGroups(summaries []usageBriefChunkSummary, budget int) [][]usageBriefChunkSummary {
	if budget <= 0 {
		budget = 4000
	}
	groups := make([][]usageBriefChunkSummary, 0)
	current := make([]usageBriefChunkSummary, 0)
	var currentTokens int
	for _, summary := range summaries {
		estimated := summary.EstimatedTokens
		if estimated <= 0 {
			estimated = estimateTokenCount(summary.ContentMD)
		}
		if len(current) > 0 && currentTokens+estimated > budget {
			groups = append(groups, current)
			current = make([]usageBriefChunkSummary, 0)
			currentTokens = 0
		}
		current = append(current, summary)
		currentTokens += estimated
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	if len(groups) == 0 {
		return [][]usageBriefChunkSummary{{}}
	}
	return groups
}

func chunkInputTokenBudget(settings *UsageBriefSettings) int {
	if settings == nil {
		return 8000
	}
	available := settings.ContextTokens - settings.OutputReservedTokens
	if available < 8000 {
		available = 8000
	}
	budget := available / 2
	if budget < 4000 {
		budget = 4000
	}
	if budget > 120000 {
		budget = 120000
	}
	return budget
}

func estimateTokenCount(value any) int {
	var raw string
	switch v := value.(type) {
	case string:
		raw = v
	default:
		raw = mustCompactJSON(v)
	}
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	return len([]rune(raw))/4 + 1
}

func splitTextByRuneBudget(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = 16000
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	parts := make([]string, 0, len(runes)/maxRunes+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		parts = append(parts, string(runes[start:end]))
	}
	return parts
}

func recordMetaWithoutPayload(rec UsageBriefSourceRecord) map[string]any {
	return map[string]any{
		"id":              rec.ID,
		"created_at":      rec.CreatedAt,
		"model":           rec.Model,
		"requested_model": rec.RequestedModel,
		"upstream_model":  rec.UpstreamModel,
		"group_id":        rec.GroupID,
		"request_type":    rec.RequestType,
		"input_tokens":    rec.InputTokens,
		"output_tokens":   rec.OutputTokens,
		"total_cost":      rec.TotalCost,
		"actual_cost":     rec.ActualCost,
	}
}

func mustCompactJSON(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (s *UsageBriefService) reusableSucceededJobChunk(ctx context.Context, jobID int64, chunkIndex int, chunkType, contentJSON string) (*UsageBriefJobChunk, bool, error) {
	chunk, err := s.repo.GetJobChunk(ctx, jobID, chunkIndex, chunkType)
	if err != nil {
		return nil, false, err
	}
	if chunk == nil || chunk.Status != UsageBriefStatusSucceeded || strings.TrimSpace(chunk.SummaryMD) == "" {
		return nil, false, nil
	}
	if !sameUsageBriefChunkInput(chunk.ContentJSON, contentJSON) {
		return nil, false, nil
	}
	return chunk, true, nil
}

func sameUsageBriefChunkInput(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b {
		return true
	}
	var av any
	var bv any
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

func (s *UsageBriefService) ensureJobStillActive(ctx context.Context, jobID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	canceled, err := s.repo.IsJobCancelRequested(ctx, jobID)
	if err != nil {
		return err
	}
	if canceled {
		return context.Canceled
	}
	return nil
}

func (s *UsageBriefService) generateMarkdown(ctx context.Context, prompt string) (string, int, int, error) {
	settings, err := s.requireUsageBriefAISettings(ctx)
	if err != nil {
		return "", 0, 0, err
	}
	return s.generateMarkdownWithSettings(ctx, settings, prompt)
}

func (s *UsageBriefService) generateMarkdownWithSettings(ctx context.Context, settings *UsageBriefSettings, prompt string) (string, int, int, error) {
	text, usageIn, usageOut, err := s.callResponses(ctx, settings, prompt)
	if err != nil {
		return "", usageIn, usageOut, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", usageIn, usageOut, fmt.Errorf("empty response from usage brief model")
	}
	return text, usageIn, usageOut, nil
}

type usageBriefAIError struct {
	message   string
	retryable bool
}

func (e *usageBriefAIError) Error() string {
	return e.message
}

func isRetryableUsageBriefError(err error) bool {
	var aiErr *usageBriefAIError
	if errors.As(err, &aiErr) {
		return aiErr.retryable
	}
	return false
}

func newUsageBriefAIError(message string, retryable bool) error {
	return &usageBriefAIError{message: message, retryable: retryable}
}

type usageBriefDependencyWaitError struct {
	message string
}

func (e *usageBriefDependencyWaitError) Error() string {
	return e.message
}

func isUsageBriefDependencyWaitError(err error) bool {
	var waitErr *usageBriefDependencyWaitError
	return errors.As(err, &waitErr)
}

func newUsageBriefDependencyWaitError(format string, args ...any) error {
	return &usageBriefDependencyWaitError{message: fmt.Sprintf(format, args...)}
}

func (s *UsageBriefService) callResponses(ctx context.Context, settings *UsageBriefSettings, prompt string) (string, int, int, error) {
	endpoint := strings.TrimRight(settings.BaseURL, "/") + "/v1/responses"
	payload := map[string]any{
		"model": settings.Model,
		"input": []map[string]string{
			{"role": "system", "content": usageBriefSystemPrompt()},
			{"role": "user", "content": prompt},
		},
		"max_output_tokens": settings.OutputReservedTokens,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", 0, 0, ctx.Err()
		}
		return "", 0, 0, newUsageBriefAIError(fmt.Sprintf("responses api request failed: %v", err), true)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	if err != nil {
		return "", 0, 0, newUsageBriefAIError(fmt.Sprintf("responses api read failed: %v", err), true)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryable := resp.StatusCode == http.StatusRequestTimeout ||
			resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusInternalServerError ||
			resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusGatewayTimeout
		return "", 0, 0, newUsageBriefAIError(fmt.Sprintf("responses api returned %d: %s", resp.StatusCode, string(respBody)), retryable)
	}
	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", 0, 0, err
	}
	inputTokens, outputTokens := responseUsageTokens(parsed)
	return extractResponsesText(parsed), inputTokens, outputTokens, nil
}

func UsageBriefPeriodBounds(periodType string, date time.Time) (time.Time, time.Time, error) {
	d := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local)
	switch periodType {
	case UsageBriefPeriodDaily:
		return d, d, nil
	case UsageBriefPeriodWeekly:
		offset := (int(d.Weekday()) + 6) % 7
		start := d.AddDate(0, 0, -offset)
		return start, start.AddDate(0, 0, 6), nil
	case UsageBriefPeriodMonthly:
		start := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.Local)
		return start, start.AddDate(0, 1, -1), nil
	default:
		return time.Time{}, time.Time{}, infraerrors.BadRequest("USAGE_BRIEF_PERIOD_INVALID", "period_type must be daily, weekly or monthly")
	}
}

func isCurrentOrFuturePeriod(periodType string, start, end, now time.Time) bool {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	switch periodType {
	case UsageBriefPeriodDaily:
		return !end.Before(today)
	case UsageBriefPeriodWeekly, UsageBriefPeriodMonthly:
		return !end.Before(today)
	default:
		return true
	}
}

func currentPeriodMessage(periodType string) string {
	switch periodType {
	case UsageBriefPeriodDaily:
		return "请在明天后查看"
	case UsageBriefPeriodWeekly:
		return "请在下周后查看"
	case UsageBriefPeriodMonthly:
		return "请在下月后查看"
	default:
		return "请在周期结束后查看"
	}
}

func productionBatchTitle(periodType string, start, end time.Time) string {
	switch periodType {
	case UsageBriefPeriodDaily:
		return fmt.Sprintf("%s 工作日报批次", start.Format("2006-01-02"))
	case UsageBriefPeriodWeekly:
		return fmt.Sprintf("%s 至 %s 工作周报批次", start.Format("2006-01-02"), end.Format("2006-01-02"))
	case UsageBriefPeriodMonthly:
		return fmt.Sprintf("%s 工作月报批次", start.Format("2006-01"))
	default:
		return "用量简报批次"
	}
}

func workReportTitle(periodType string, start, end time.Time) string {
	switch periodType {
	case UsageBriefPeriodDaily:
		return fmt.Sprintf("%s 工作日报", start.Format("2006-01-02"))
	case UsageBriefPeriodWeekly:
		return fmt.Sprintf("%s 至 %s 工作周报", start.Format("2006-01-02"), end.Format("2006-01-02"))
	case UsageBriefPeriodMonthly:
		return fmt.Sprintf("%s 工作月报", start.Format("2006-01"))
	default:
		return "工作简报"
	}
}

func buildDailyPrompt(userID int64, start, end time.Time, records []UsageBriefSourceRecord) string {
	payload := map[string]any{
		"user_id":      userID,
		"period_type":  UsageBriefPeriodDaily,
		"period_start": start.Format("2006-01-02"),
		"period_end":   end.Format("2006-01-02"),
		"records":      compactUsageBriefRecordsForPrompt(records),
		"summary":      summarizeRecords(records),
	}
	return promptWithPayload(workSummaryFinalInstruction("工作日报", "请基于以下当日 API 使用记录和请求 JSON 生成普通用户工作日报。重点总结用户当天完成的工作、模块/功能设计或实现、贡献线索、风险和阻塞。"), payload)
}

func buildTestPrompt(userID int64, start, end time.Time, records []UsageBriefSourceRecord) string {
	payload := map[string]any{
		"user_id":     userID,
		"range_start": start.Format(time.RFC3339),
		"range_end":   end.Format(time.RFC3339),
		"records":     compactUsageBriefRecordsForPrompt(records),
		"summary":     summarizeRecords(records),
	}
	return promptWithPayload(workSummaryFinalInstruction("测试工作简报", "这是管理员测试生成任务。请基于给定时间范围内的 API 使用记录和请求 JSON 生成普通用户工作总结，重点总结工作成果、模块/设计进展、贡献线索、风险和建议。"), payload)
}

func buildRollupPrompt(label string, userID int64, start, end time.Time, reports []UsageBriefReport) string {
	items := make([]map[string]any, 0, len(reports))
	for _, report := range reports {
		items = append(items, map[string]any{
			"title":        report.Title,
			"period_start": report.PeriodStart.Format("2006-01-02"),
			"period_end":   report.PeriodEnd.Format("2006-01-02"),
			"content_md":   report.ContentMD,
		})
	}
	payload := map[string]any{
		"user_id":      userID,
		"label":        label,
		"period_start": start.Format("2006-01-02"),
		"period_end":   end.Format("2006-01-02"),
		"source":       items,
	}
	return promptWithPayload(workSummaryFinalInstruction("工作"+label, "请基于下级 Markdown 工作简报生成普通用户工作"+label+"。保留工作成果、模块/设计进展、贡献度变化、风险和改进建议。"), payload)
}

func buildMonthlyPrompt(userID int64, start, end time.Time, weeklyReports, dailyReports []UsageBriefReport) string {
	payload := map[string]any{
		"user_id":        userID,
		"label":          "月报",
		"period_start":   start.Format("2006-01-02"),
		"period_end":     end.Format("2006-01-02"),
		"weekly_reports": reportPromptItems(weeklyReports),
		"daily_reports":  reportPromptItems(dailyReports),
	}
	return promptWithPayload(workSummaryFinalInstruction("工作月报", "请基于自然月内的工作日报生成普通用户工作月报，并参考周报提炼趋势。月报统计口径必须严格限定在 period_start 到 period_end 之间；如果周报跨月，只能采用其中属于本自然月的结论，不得把月外日期计入本月。"), payload)
}

func reportPromptItems(reports []UsageBriefReport) []map[string]any {
	items := make([]map[string]any, 0, len(reports))
	for _, report := range reports {
		items = append(items, map[string]any{
			"title":        report.Title,
			"period_start": report.PeriodStart.Format("2006-01-02"),
			"period_end":   report.PeriodEnd.Format("2006-01-02"),
			"content_md":   report.ContentMD,
		})
	}
	return items
}

func promptWithPayload(instruction string, payload any) string {
	b, _ := json.MarshalIndent(payload, "", "  ")
	return instruction + "\n\n要求：\n- 只输出 Markdown 正文。\n- 使用清晰标题和列表，必要时才使用短表格。\n- 报告主体必须是普通用户工作总结，不要把模型、token、成本、请求次数作为主体。\n- 对请求 JSON 做工作主题归纳，不要逐条复述所有原文。\n- token、成本、模型等用量信息只允许在“用量概括”中用一句话总结。\n- 不要输出 API Key、Authorization、cookie 等敏感字段。\n\n数据：\n```json\n" + string(b) + "\n```"
}

func usageBriefSystemPrompt() string {
	return "你是严谨的用户工作简报分析助手。你只生成简洁的中文 Markdown 工作总结，重点判断普通用户完成了什么工作、模块设计或实现进展、贡献度、风险和建议；token、成本和模型等用量信息只做一句话概括。"
}

func workSummaryFinalInstruction(label, focus string) string {
	return focus + "\n\n请使用固定 Markdown 结构：\n## 工作概览\n## 完成内容\n## 模块与设计进展\n## 贡献度判断\n## 风险与建议\n## 用量概括\n\n如果输入为空或所有下级报告都明确表示无工作情况，只输出非常简短的无工作情况说明和一句用量概括，不要扩写空洞内容。“用量概括”只能用一句话概括 token、成本、模型或请求量；其他章节必须围绕普通用户的工作成果与贡献展开。报告标题语义为“" + label + "”。"
}

func partialUsageBriefFinalInstruction(base string, skipped, total int) string {
	return base + fmt.Sprintf("\n\n注意：本次共有 %d/%d 个 source 分片因 AI 总结失败达到重试上限被跳过。请只基于已成功的分片摘要生成报告，不要编造被跳过分片的内容，并在报告中保持结论克制。", skipped, total)
}

func partialUsageBriefReportPrefix(skipped, total int) string {
	return fmt.Sprintf("> 部分成功：本报告生成时有 %d/%d 个请求分片在重试 %d 次后仍失败并被跳过，跳过分片的详细错误可在管理员“查看分片”中查看。", skipped, total, usageBriefSourceChunkRetryLimit)
}

func noWorkReportContent(periodType string) string {
	scope := "本周期"
	switch periodType {
	case UsageBriefPeriodDaily:
		scope = "当天"
	case UsageBriefPeriodWeekly:
		scope = "本周"
	case UsageBriefPeriodMonthly:
		scope = "本月"
	}
	return fmt.Sprintf("## 工作概览\n%s未采集到任何使用记录，暂无可归纳的工作情况。\n\n## 用量概括\n本周期请求数为 0，暂无 token 或成本消耗。", scope)
}

func usageBriefReportReferences(reports []UsageBriefReport) []map[string]any {
	out := make([]map[string]any, 0, len(reports))
	for _, report := range reports {
		out = append(out, map[string]any{
			"id":                report.ID,
			"title":             report.Title,
			"period_start":      report.PeriodStart.Format("2006-01-02"),
			"period_end":        report.PeriodEnd.Format("2006-01-02"),
			"status":            report.Status,
			"input_usage_count": report.InputUsageCount,
			"content_md":        report.ContentMD,
		})
	}
	return out
}

func detailedUsageBriefChunkError(err error, retryCount, retryLimit int) string {
	msg := strings.TrimSpace(fmt.Sprintf("%v", err))
	if msg == "" {
		msg = "unknown usage brief source chunk error"
	}
	return fmt.Sprintf("source 分片 AI 总结失败：%s；已重试 %d/%d 次。", msg, minInt(retryCount, retryLimit), retryLimit)
}

func sleepUsageBriefSourceChunkRetry(ctx context.Context) error {
	timer := time.NewTimer(usageBriefSourceChunkRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *UsageBriefService) listPriorDailyReportsForReference(ctx context.Context, userID int64, periodStart time.Time) ([]UsageBriefReport, error) {
	end := periodStart.AddDate(0, 0, -1)
	start := periodStart.AddDate(0, 0, -7)
	reports, err := s.repo.ListDailyReports(ctx, userID, start, end)
	if err != nil {
		return nil, err
	}
	if len(reports) <= 7 {
		return reports, nil
	}
	return reports[len(reports)-7:], nil
}

func dateOnlyTime(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func reportInputUsageTotal(reports []UsageBriefReport) int {
	total := 0
	for _, report := range reports {
		total += report.InputUsageCount
	}
	return total
}

func reportsRepresentNoWork(dailyReports, weeklyReports []UsageBriefReport) bool {
	if len(dailyReports) > 0 {
		return reportInputUsageTotal(dailyReports) == 0
	}
	if len(weeklyReports) > 0 {
		return reportInputUsageTotal(weeklyReports) == 0
	}
	return true
}

func summarizeRecords(records []UsageBriefSourceRecord) map[string]any {
	modelCounts := map[string]int{}
	requestTypes := map[string]int{}
	var inputTokens, outputTokens int
	var cost float64
	for _, rec := range records {
		model := rec.RequestedModel
		if model == "" {
			model = rec.Model
		}
		modelCounts[model]++
		requestTypes[rec.RequestType]++
		inputTokens += rec.InputTokens
		outputTokens += rec.OutputTokens
		cost += rec.ActualCost
	}
	return map[string]any{
		"request_count": len(records),
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
		"actual_cost":   cost,
		"models":        sortedCounts(modelCounts),
		"request_types": sortedCounts(requestTypes),
	}
}

func (summary UsageBriefUsageSummary) toMap() map[string]any {
	return map[string]any{
		"request_count": summary.RequestCount,
		"input_tokens":  summary.InputTokens,
		"output_tokens": summary.OutputTokens,
		"total_cost":    summary.TotalCost,
		"actual_cost":   summary.ActualCost,
		"models":        sortedCounts(summary.ModelCounts),
		"request_types": sortedCounts(summary.RequestTypeCounts),
	}
}

func sortedCounts(values map[string]int) []map[string]any {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if values[keys[i]] == values[keys[j]] {
			return keys[i] < keys[j]
		}
		return values[keys[i]] > values[keys[j]]
	})
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		out = append(out, map[string]any{"name": key, "count": values[key]})
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func extractResponsesText(payload map[string]any) string {
	if v, ok := payload["output_text"].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	var parts []string
	if output, ok := payload["output"].([]any); ok {
		for _, item := range output {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if content, ok := obj["content"].([]any); ok {
				for _, c := range content {
					cobj, ok := c.(map[string]any)
					if !ok {
						continue
					}
					if text, ok := cobj["text"].(string); ok && text != "" {
						parts = append(parts, text)
					}
				}
			}
		}
	}
	return strings.Join(parts, "\n")
}

func responseUsageTokens(payload map[string]any) (int, int) {
	usage, _ := payload["usage"].(map[string]any)
	return intFromAny(usage["input_tokens"]), intFromAny(usage["output_tokens"])
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func parseIntDefault(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return v
}
