package service

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

func (s *UsageBriefService) sendProductionJobEmailIfNeeded(ctx context.Context, jobID int64) {
	if s == nil || jobID <= 0 {
		return
	}
	job, err := s.repo.GetJob(ctx, jobID)
	if err != nil || job == nil {
		if err != nil {
			logger.LegacyPrintf("service.usage_brief", "load completed job for email failed: job_id=%d err=%v", jobID, err)
		}
		return
	}
	if job.JobScope != UsageBriefJobScopeProduction {
		return
	}
	if job.Status != UsageBriefStatusSucceeded && job.Status != UsageBriefStatusPartial {
		return
	}
	if !s.shouldAutoSendProductionJobEmail(ctx, *job) {
		return
	}
	if _, err := s.SendJobEmail(ctx, jobID); err != nil {
		logger.LegacyPrintf("service.usage_brief", "send usage brief email failed: job_id=%d err=%v", jobID, err)
	}
}

func (s *UsageBriefService) shouldAutoSendProductionJobEmail(ctx context.Context, job UsageBriefJob) bool {
	if s == nil || s.repo == nil {
		return false
	}
	if job.JobScope != UsageBriefJobScopeProduction {
		return false
	}
	if job.Status != UsageBriefStatusSucceeded && job.Status != UsageBriefStatusPartial {
		return false
	}
	if job.UserID == nil || *job.UserID <= 0 {
		return false
	}
	enabled, err := s.repo.IsUserUsageBriefAutoEmailEnabled(ctx, *job.UserID)
	if err != nil {
		logger.LegacyPrintf("service.usage_brief", "load usage brief auto email preference failed: job_id=%d user_id=%d err=%v", job.ID, *job.UserID, err)
		return false
	}
	return enabled
}

func (s *UsageBriefService) SendJobEmail(ctx context.Context, jobID int64) (*UsageBriefJob, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("USAGE_BRIEF_UNAVAILABLE", "usage brief service unavailable")
	}
	job, err := s.repo.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, infraerrors.NotFound("USAGE_BRIEF_JOB_NOT_FOUND", "usage brief job not found")
	}
	if job.Status != UsageBriefStatusSucceeded && job.Status != UsageBriefStatusPartial {
		return nil, infraerrors.BadRequest("USAGE_BRIEF_EMAIL_JOB_NOT_READY", "usage brief job is not completed")
	}
	if job.EmailStatus == UsageBriefEmailStatusSent {
		return job, nil
	}

	mail, err := s.usageBriefEmailPayload(ctx, *job)
	if err != nil {
		_ = s.repo.MarkJobEmailSkipped(ctx, job.ID, err.Error())
		updated, _ := s.repo.GetJob(ctx, job.ID)
		if updated != nil {
			job = updated
		}
		return job, err
	}
	if s.notificationEmailService == nil {
		err := infraerrors.ServiceUnavailable("USAGE_BRIEF_EMAIL_NOT_CONFIGURED", "notification email service is not configured")
		_ = s.repo.MarkJobEmailFailed(ctx, job.ID, err.Error())
		updated, _ := s.repo.GetJob(ctx, job.ID)
		if updated != nil {
			job = updated
		}
		return job, err
	}

	sendErr := s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventUsageBriefReport,
		RecipientEmail: mail.RecipientEmail,
		RecipientName:  mail.RecipientName,
		UserID:         mail.UserID,
		SourceType:     "usage_brief_job",
		SourceID:       fmt.Sprintf("%d", job.ID),
		ReminderKey:    usageBriefJobEmailReminderKey(*job),
		Variables: map[string]string{
			"brief_title":        mail.Title,
			"brief_period_type":  usageBriefEmailPeriodLabel(mail.PeriodType),
			"brief_period_start": mail.PeriodStart,
			"brief_period_end":   mail.PeriodEnd,
			"brief_html":         mail.HTML,
		},
		RawHTMLVariables: map[string]string{
			"brief_html": mail.HTML,
		},
	})
	if sendErr != nil {
		_ = s.repo.MarkJobEmailFailed(ctx, job.ID, sendErr.Error())
		updated, _ := s.repo.GetJob(ctx, job.ID)
		if updated != nil {
			job = updated
		}
		return job, sendErr
	}
	if err := s.repo.MarkJobEmailSent(ctx, job.ID); err != nil {
		return nil, err
	}
	return s.repo.GetJob(ctx, job.ID)
}

func usageBriefJobEmailReminderKey(job UsageBriefJob) string {
	if job.FinishedAt != nil && !job.FinishedAt.IsZero() {
		return job.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	return job.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

type usageBriefEmailPayload struct {
	UserID         int64
	RecipientEmail string
	RecipientName  string
	Title          string
	PeriodType     string
	PeriodStart    string
	PeriodEnd      string
	HTML           string
}

func (s *UsageBriefService) usageBriefEmailPayload(ctx context.Context, job UsageBriefJob) (usageBriefEmailPayload, error) {
	if job.UserID == nil || *job.UserID <= 0 {
		return usageBriefEmailPayload{}, fmt.Errorf("usage brief job has no user")
	}
	if strings.TrimSpace(job.UserEmail) == "" {
		return usageBriefEmailPayload{}, fmt.Errorf("usage brief job has no recipient email")
	}
	out := usageBriefEmailPayload{
		UserID:         *job.UserID,
		RecipientEmail: strings.TrimSpace(job.UserEmail),
		RecipientName:  usageBriefRecipientName(job),
	}
	if job.JobScope == UsageBriefJobScopeProduction {
		if job.ReportID == nil || *job.ReportID <= 0 {
			return usageBriefEmailPayload{}, fmt.Errorf("usage brief production job has no report")
		}
		report, err := s.repo.GetReport(ctx, *job.ReportID)
		if err != nil {
			return usageBriefEmailPayload{}, err
		}
		if report == nil {
			return usageBriefEmailPayload{}, fmt.Errorf("usage brief report not found")
		}
		out.Title = strings.TrimSpace(report.Title)
		out.PeriodType = report.PeriodType
		out.PeriodStart = dateString(report.PeriodStart)
		out.PeriodEnd = dateString(report.PeriodEnd)
		out.HTML = usageBriefMarkdownToEmailHTML(report.ContentMD)
	} else {
		out.Title = "测试简报"
		out.PeriodType = UsageBriefJobTypeCustom
		if job.RangeStart != nil {
			out.PeriodStart = timeString(*job.RangeStart)
		}
		if job.RangeEnd != nil {
			out.PeriodEnd = timeString(*job.RangeEnd)
		}
		out.HTML = usageBriefMarkdownToEmailHTML(job.ResultMD)
	}
	if out.Title == "" {
		out.Title = "用量简报"
	}
	if strings.TrimSpace(out.HTML) == "" {
		return usageBriefEmailPayload{}, fmt.Errorf("usage brief email content is empty")
	}
	return out, nil
}

func usageBriefRecipientName(job UsageBriefJob) string {
	if strings.TrimSpace(job.Username) != "" {
		return strings.TrimSpace(job.Username)
	}
	return emailRecipientName(job.UserEmail)
}

func usageBriefEmailPeriodLabel(periodType string) string {
	switch periodType {
	case UsageBriefPeriodDaily:
		return "日报"
	case UsageBriefPeriodWeekly:
		return "周报"
	case UsageBriefPeriodMonthly:
		return "月报"
	case UsageBriefJobTypeCustom:
		return "测试简报"
	default:
		return "用量简报"
	}
}

func dateString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format("2006-01-02")
}

func timeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format("2006-01-02 15:04")
}

func usageBriefMarkdownToEmailHTML(markdown string) string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	var builder strings.Builder
	inCode := false
	inList := false
	paragraph := make([]string, 0, 2)

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		builder.WriteString(`<p>`)
		builder.WriteString(strings.Join(paragraph, "<br>"))
		builder.WriteString(`</p>`)
		paragraph = paragraph[:0]
	}
	closeList := func() {
		if inList {
			builder.WriteString(`</ul>`)
			inList = false
		}
	}

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "```") {
			flushParagraph()
			closeList()
			if inCode {
				builder.WriteString(`</code></pre>`)
				inCode = false
			} else {
				builder.WriteString(`<pre><code>`)
				inCode = true
			}
			continue
		}
		if inCode {
			builder.WriteString(html.EscapeString(rawLine))
			builder.WriteByte('\n')
			continue
		}
		if line == "" {
			flushParagraph()
			closeList()
			continue
		}
		if level, text := markdownHeading(line); level > 0 {
			flushParagraph()
			closeList()
			builder.WriteString(fmt.Sprintf(`<h%d>%s</h%d>`, level, html.EscapeString(text), level))
			continue
		}
		if item, ok := markdownListItem(line); ok {
			flushParagraph()
			if !inList {
				builder.WriteString(`<ul>`)
				inList = true
			}
			builder.WriteString(`<li>`)
			builder.WriteString(html.EscapeString(item))
			builder.WriteString(`</li>`)
			continue
		}
		if strings.HasPrefix(line, ">") {
			flushParagraph()
			closeList()
			builder.WriteString(`<blockquote>`)
			builder.WriteString(html.EscapeString(strings.TrimSpace(strings.TrimPrefix(line, ">"))))
			builder.WriteString(`</blockquote>`)
			continue
		}
		paragraph = append(paragraph, html.EscapeString(line))
	}
	flushParagraph()
	closeList()
	if inCode {
		builder.WriteString(`</code></pre>`)
	}
	return usageBriefEmailHTMLWrapper(builder.String())
}

func markdownHeading(line string) (int, string) {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return 0, ""
	}
	return level, strings.TrimSpace(line[level+1:])
}

func markdownListItem(line string) (string, bool) {
	for _, prefix := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
		}
	}
	if idx := strings.Index(line, ". "); idx > 0 {
		number := line[:idx]
		for _, r := range number {
			if r < '0' || r > '9' {
				return "", false
			}
		}
		return strings.TrimSpace(line[idx+2:]), true
	}
	return "", false
}

func usageBriefEmailHTMLWrapper(body string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	return `<div class="usage-brief-content">` + body + `</div>`
}
