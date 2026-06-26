import { apiClient } from './client'
import type { PaginatedResponse } from '@/types'

export type UsageBriefPeriodType = 'daily' | 'weekly' | 'monthly'
export type UsageBriefStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'paused' | 'partial'
export type UsageBriefEmailStatus = 'pending' | 'sent' | 'failed' | 'skipped'

export interface UsageBriefReport {
  id: number
  user_id: number
  user_email?: string
  username?: string
  period_type: UsageBriefPeriodType
  period_start: string
  period_end: string
  status: UsageBriefStatus
  title: string
  content_md: string
  source_kind: 'production' | 'test'
  is_admin_edited: boolean
  edited_by?: number
  generated_by_job_id?: number
  input_usage_count: number
  input_tokens: number
  output_tokens: number
  error_message?: string
  generated_at?: string
  created_at: string
  updated_at: string
}

export type UsageBriefReportGroupBy = 'period' | 'user'

export interface UsageBriefReportGroup {
  key: string
  group_by: UsageBriefReportGroupBy
  label: string
  period_type?: UsageBriefPeriodType
  period_start?: string
  period_end?: string
  user_id?: number
  user_email?: string
  username?: string
  report_count: number
  user_count: number
  input_usage_count: number
  input_tokens: number
  output_tokens: number
  reports: UsageBriefReport[]
}

export interface UsageBriefJob {
  id: number
  batch_id?: number
  job_scope: 'production' | 'test'
  job_type: 'daily' | 'weekly' | 'monthly' | 'custom'
  status: UsageBriefStatus
  user_id?: number
  user_email?: string
  username?: string
  group_id?: number
  group_name?: string
  period_type?: UsageBriefPeriodType
  period_start?: string
  period_end?: string
  range_start?: string
  range_end?: string
  progress_current: number
  progress_total: number
  cancel_requested: boolean
  retry_count: number
  next_retry_at?: string
  stage?: string
  chunk_current: number
  chunk_total: number
  token_estimated_total: number
  token_estimated_processed: number
  input_tokens: number
  output_tokens: number
  report_id?: number
  result_md?: string
  error_message?: string
  email_status: UsageBriefEmailStatus
  email_sent_at?: string
  email_error_message?: string
  email_attempt_count: number
  started_at?: string
  finished_at?: string
  created_at: string
  updated_at: string
}

export interface UsageBriefJobChunk {
  id: number
  job_id: number
  chunk_index: number
  chunk_type: 'source' | 'merge'
  status: UsageBriefStatus
  token_estimated: number
  input_tokens: number
  output_tokens: number
  retry_count: number
  content_json?: string
  conversation_json?: string
  summary_md?: string
  error_message?: string
  last_error_at?: string
  started_at?: string
  finished_at?: string
  created_at: string
  updated_at: string
}

export interface UsageBriefJobConversation {
  id: number
  job_id: number
  conversation_index: number
  usage_log_id: number
  covered_usage_log_ids?: number[]
  covered_request_count: number
  token_estimated: number
  conversation_json: string
  created_at: string
  updated_at: string
}

export interface UsageBriefBatch {
  id: number
  batch_scope: 'production' | 'test'
  trigger_kind: 'manual' | 'auto'
  status: UsageBriefStatus
  title: string
  notes?: string
  period_type?: UsageBriefPeriodType
  period_start?: string
  period_end?: string
  range_start?: string
  range_end?: string
  job_count: number
  succeeded_count: number
  failed_count: number
  canceled_count: number
  running_count: number
  queued_count: number
  retrying_count: number
  token_estimated_total: number
  token_estimated_processed: number
  input_tokens: number
  output_tokens: number
  cancel_requested: boolean
  paused_at?: string
  deleted_at?: string
  created_by?: number
  created_at: string
  updated_at: string
}

export interface UsageBriefPeriodView {
  available: boolean
  reason?: string
  message?: string
  period_type: UsageBriefPeriodType
  period_start: string
  period_end: string
  status?: UsageBriefStatus
  report?: UsageBriefReport
  job?: UsageBriefJob
}

export interface UsageBriefSettings {
  enabled: boolean
  base_url: string
  api_key_configured: boolean
  model: string
  context_tokens: number
  output_reserved_tokens: number
  concurrency: number
}

export interface UpdateUsageBriefSettingsRequest {
  base_url?: string
  api_key?: string
  clear_api_key?: boolean
  model?: string
  context_tokens?: number
  output_reserved_tokens?: number
  concurrency?: number
}

export interface UsageBriefReportQuery {
  page?: number
  page_size?: number
  user_id?: number
  period_type?: UsageBriefPeriodType | ''
  source_kind?: string
  status?: string
  search?: string
  search_scope?: 'user' | ''
  start_date?: string
  end_date?: string
}

export interface UsageBriefReportGroupQuery extends UsageBriefReportQuery {
  group_by?: UsageBriefReportGroupBy
}

export interface UsageBriefJobQuery {
  page?: number
  page_size?: number
  scope?: string
  status?: string
  user_id?: number
  batch_id?: number
}

export interface UsageBriefBatchQuery {
  page?: number
  page_size?: number
  scope?: string
  status?: string
}

export interface UsageBriefJobChunkQuery {
  page?: number
  page_size?: number
  chunk_type?: 'source' | 'merge'
}

export interface UsageBriefJobConversationQuery {
  page?: number
  page_size?: number
}

export const usageBriefAPI = {
  async listReports(params: UsageBriefReportQuery = {}): Promise<PaginatedResponse<UsageBriefReport>> {
    const { data } = await apiClient.get<PaginatedResponse<UsageBriefReport>>('/usage-brief/reports', { params })
    return data
  },

  async getReport(id: number): Promise<UsageBriefReport> {
    const { data } = await apiClient.get<UsageBriefReport>(`/usage-brief/reports/${id}`)
    return data
  },

  async getPeriod(periodType: UsageBriefPeriodType, date: string): Promise<UsageBriefPeriodView> {
    const { data } = await apiClient.get<UsageBriefPeriodView>('/usage-brief/period', {
      params: { period_type: periodType, date }
    })
    return data
  }
}

export const adminUsageBriefAPI = {
  async getSettings(): Promise<UsageBriefSettings> {
    const { data } = await apiClient.get<UsageBriefSettings>('/admin/usage-brief/settings')
    return data
  },

  async updateSettings(payload: UpdateUsageBriefSettingsRequest): Promise<UsageBriefSettings> {
    const { data } = await apiClient.put<UsageBriefSettings>('/admin/usage-brief/settings', payload)
    return data
  },

  async listReports(params: UsageBriefReportQuery = {}): Promise<PaginatedResponse<UsageBriefReport>> {
    const { data } = await apiClient.get<PaginatedResponse<UsageBriefReport>>('/admin/usage-brief/reports', { params })
    return data
  },

  async listReportGroups(params: UsageBriefReportGroupQuery = {}): Promise<PaginatedResponse<UsageBriefReportGroup>> {
    const { data } = await apiClient.get<PaginatedResponse<UsageBriefReportGroup>>('/admin/usage-brief/report-groups', { params })
    return data
  },

  async deleteReportGroup(params: UsageBriefReportGroupQuery & { group_key: string }): Promise<{ deleted_count: number }> {
    const { data } = await apiClient.delete<{ deleted_count: number }>('/admin/usage-brief/report-groups', { params })
    return data
  },

  async getReport(id: number): Promise<UsageBriefReport> {
    const { data } = await apiClient.get<UsageBriefReport>(`/admin/usage-brief/reports/${id}`)
    return data
  },

  async updateReport(id: number, payload: { title: string; content_md: string }): Promise<UsageBriefReport> {
    const { data } = await apiClient.put<UsageBriefReport>(`/admin/usage-brief/reports/${id}`, payload)
    return data
  },

  async deleteReport(id: number): Promise<{ id: number }> {
    const { data } = await apiClient.delete<{ id: number }>(`/admin/usage-brief/reports/${id}`)
    return data
  },

  async listBatches(params: UsageBriefBatchQuery = {}): Promise<PaginatedResponse<UsageBriefBatch>> {
    const { data } = await apiClient.get<PaginatedResponse<UsageBriefBatch>>('/admin/usage-brief/batches', { params })
    return data
  },

  async listBatchJobs(id: number, params: UsageBriefJobQuery = {}): Promise<PaginatedResponse<UsageBriefJob>> {
    const { data } = await apiClient.get<PaginatedResponse<UsageBriefJob>>(`/admin/usage-brief/batches/${id}/jobs`, { params })
    return data
  },

  async listJobs(params: UsageBriefJobQuery = {}): Promise<PaginatedResponse<UsageBriefJob>> {
    const { data } = await apiClient.get<PaginatedResponse<UsageBriefJob>>('/admin/usage-brief/jobs', { params })
    return data
  },

  async triggerProduction(payload: { period_type: UsageBriefPeriodType; period_date: string }): Promise<{ batch: UsageBriefBatch; items: UsageBriefJob[]; count: number }> {
    const { data } = await apiClient.post<{ batch: UsageBriefBatch; items: UsageBriefJob[]; count: number }>('/admin/usage-brief/jobs/production', payload)
    return data
  },

  async createTestJob(payload: { user_id: number; group_id?: number; range_start: string; range_end: string }): Promise<{ batch: UsageBriefBatch; job: UsageBriefJob }> {
    const { data } = await apiClient.post<{ batch: UsageBriefBatch; job: UsageBriefJob }>('/admin/usage-brief/jobs/test', payload)
    return data
  },

  async pauseBatch(id: number): Promise<UsageBriefBatch> {
    const { data } = await apiClient.post<UsageBriefBatch>(`/admin/usage-brief/batches/${id}/pause`)
    return data
  },

  async resumeBatch(id: number): Promise<UsageBriefBatch> {
    const { data } = await apiClient.post<UsageBriefBatch>(`/admin/usage-brief/batches/${id}/resume`)
    return data
  },

  async cancelBatch(id: number): Promise<UsageBriefBatch> {
    const { data } = await apiClient.post<UsageBriefBatch>(`/admin/usage-brief/batches/${id}/cancel`)
    return data
  },

  async resetBatch(id: number): Promise<UsageBriefBatch> {
    const { data } = await apiClient.post<UsageBriefBatch>(`/admin/usage-brief/batches/${id}/reset`)
    return data
  },

  async rerunBatch(id: number): Promise<UsageBriefBatch> {
    const { data } = await apiClient.post<UsageBriefBatch>(`/admin/usage-brief/batches/${id}/rerun`)
    return data
  },

  async deleteBatch(id: number): Promise<{ id: number }> {
    const { data } = await apiClient.delete<{ id: number }>(`/admin/usage-brief/batches/${id}`)
    return data
  },

  async cancelJob(id: number): Promise<UsageBriefJob> {
    const { data } = await apiClient.post<UsageBriefJob>(`/admin/usage-brief/jobs/${id}/cancel`)
    return data
  },

  async resetJob(id: number): Promise<UsageBriefJob> {
    const { data } = await apiClient.post<UsageBriefJob>(`/admin/usage-brief/jobs/${id}/reset`)
    return data
  },

  async rerunJob(id: number): Promise<UsageBriefJob> {
    const { data } = await apiClient.post<UsageBriefJob>(`/admin/usage-brief/jobs/${id}/rerun`)
    return data
  },

  async sendJobEmail(id: number): Promise<UsageBriefJob> {
    const { data } = await apiClient.post<UsageBriefJob>(`/admin/usage-brief/jobs/${id}/send-email`)
    return data
  },

  async listJobChunks(id: number, params: UsageBriefJobChunkQuery = {}): Promise<PaginatedResponse<UsageBriefJobChunk>> {
    const { data } = await apiClient.get<PaginatedResponse<UsageBriefJobChunk>>(`/admin/usage-brief/jobs/${id}/chunks`, {
      params
    })
    return data
  },

  async listJobConversations(id: number, params: UsageBriefJobConversationQuery = {}): Promise<PaginatedResponse<UsageBriefJobConversation>> {
    const { data } = await apiClient.get<PaginatedResponse<UsageBriefJobConversation>>(`/admin/usage-brief/jobs/${id}/conversations`, {
      params
    })
    return data
  },

  async deleteJob(id: number): Promise<{ id: number }> {
    const { data } = await apiClient.delete<{ id: number }>(`/admin/usage-brief/jobs/${id}`)
    return data
  }
}
