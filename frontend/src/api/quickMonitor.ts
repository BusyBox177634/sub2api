import { apiClient } from './client'
import type {
  AdminGroup,
  AdminUser,
  Account,
  GroupPlatform,
  PaginatedResponse,
  SubscriptionProgress,
  UsageLogDetailResponse,
  UsageRequestType,
  UserSpendingRankingResponse,
  UserSubscription,
} from '@/types'
import type {
  DashboardSnapshotV2Params,
  DashboardSnapshotV2Response,
  BatchUsersUsageResponse,
  ModelStatsParams,
  ModelStatsResponse,
  UserTrendParams,
  UserTrendResponse,
  UserSpendingRankingParams,
} from './admin/dashboard'
import type { AdminUsageQueryParams, AdminUsageStatsResponse, SimpleApiKey, SimpleUser } from './admin/usage'
import type { AdminUsageLog } from '@/types'
import type {
  OpsDashboardOverview,
  OpsErrorDetail,
  OpsErrorListQueryParams,
  OpsErrorLogsResponse,
  OpsQueryMode,
} from './admin/ops'
import type { UsageBriefReport, UsageBriefReportGroup, UsageBriefReportGroupQuery, UsageBriefReportQuery } from './usageBrief'

const encodedSuffix = (suffix: string) => encodeURIComponent(suffix)
const base = (suffix: string) => `/quick-monitor/${encodedSuffix(suffix)}`

export interface QuickUserListParams {
  page?: number
  page_size?: number
  status?: 'active' | 'disabled' | ''
  role?: 'admin' | 'user' | ''
  search?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface QuickGroupListParams {
  page?: number
  page_size?: number
  platform?: GroupPlatform | ''
  status?: 'active' | 'inactive' | ''
  is_exclusive?: boolean
  search?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface QuickGroupUsageSummary {
  group_id: number
  today_cost: number
  total_cost: number
}

export interface QuickGroupCapacitySummary {
  group_id: number
  concurrency_used: number
  concurrency_max: number
  sessions_used: number
  sessions_max: number
  rpm_used: number
  rpm_max: number
}

export interface QuickSubscriptionListParams {
  page?: number
  page_size?: number
  status?: 'active' | 'expired' | 'revoked' | ''
  user_id?: number
  group_id?: number
  platform?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface QuickOpsOverviewParams {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
}

export interface QuickUsageStatsParams {
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
  billing_mode?: string
  period?: string
  start_date?: string
  end_date?: string
  timezone?: string
  nocache?: number
}

export interface QuickAccountListParams {
  page?: number
  page_size?: number
  platform?: string
  type?: string
  status?: string
  search?: string
  group_id?: number
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export const quickMonitorAPI = {
  async status(suffix: string): Promise<{ enabled: boolean }> {
    const { data } = await apiClient.get<{ enabled: boolean }>(`${base(suffix)}/status`)
    return data
  },

  dashboard: {
    async getSnapshotV2(suffix: string, params?: DashboardSnapshotV2Params): Promise<DashboardSnapshotV2Response> {
      const { data } = await apiClient.get<DashboardSnapshotV2Response>(`${base(suffix)}/dashboard/snapshot-v2`, { params })
      return data
    },
    async getModelStats(suffix: string, params?: ModelStatsParams): Promise<ModelStatsResponse> {
      const { data } = await apiClient.get<ModelStatsResponse>(`${base(suffix)}/dashboard/models`, { params })
      return data
    },
    async getUserUsageTrend(suffix: string, params?: UserTrendParams): Promise<UserTrendResponse> {
      const { data } = await apiClient.get<UserTrendResponse>(`${base(suffix)}/dashboard/users-trend`, { params })
      return data
    },
    async getUserSpendingRanking(suffix: string, params?: UserSpendingRankingParams): Promise<UserSpendingRankingResponse> {
      const { data } = await apiClient.get<UserSpendingRankingResponse>(`${base(suffix)}/dashboard/users-ranking`, { params })
      return data
    },
    async getBatchUsersUsage(suffix: string, userIds: number[]): Promise<BatchUsersUsageResponse> {
      const { data } = await apiClient.post<BatchUsersUsageResponse>(`${base(suffix)}/dashboard/users-usage`, {
        user_ids: userIds,
      })
      return data
    },
  },

  ops: {
    async getDashboardOverview(suffix: string, params: QuickOpsOverviewParams = {}): Promise<OpsDashboardOverview> {
      const { data } = await apiClient.get<OpsDashboardOverview>(`${base(suffix)}/ops/dashboard/overview`, { params })
      return data
    },
    async listErrorLogs(suffix: string, params: OpsErrorListQueryParams = {}): Promise<OpsErrorLogsResponse> {
      const { data } = await apiClient.get<OpsErrorLogsResponse>(`${base(suffix)}/ops/errors`, { params })
      return data
    },
    async getRequestErrorDetail(suffix: string, id: number): Promise<OpsErrorDetail> {
      const { data } = await apiClient.get<OpsErrorDetail>(`${base(suffix)}/ops/request-errors/${id}`)
      return data
    },
    async getUpstreamErrorDetail(suffix: string, id: number): Promise<OpsErrorDetail> {
      const { data } = await apiClient.get<OpsErrorDetail>(`${base(suffix)}/ops/upstream-errors/${id}`)
      return data
    },
    async listRequestErrorUpstreamErrors(
      suffix: string,
      id: number,
      params: OpsErrorListQueryParams = {},
      options: { include_detail?: boolean } = {}
    ): Promise<PaginatedResponse<OpsErrorDetail>> {
      const query: Record<string, unknown> = { ...params }
      if (options.include_detail) query.include_detail = '1'
      const { data } = await apiClient.get<PaginatedResponse<OpsErrorDetail>>(
        `${base(suffix)}/ops/request-errors/${id}/upstream-errors`,
        { params: query }
      )
      return data
    },
  },

  users: {
    async list(suffix: string, params: QuickUserListParams = {}, options?: { signal?: AbortSignal }): Promise<PaginatedResponse<AdminUser>> {
      const { data } = await apiClient.get<PaginatedResponse<AdminUser>>(`${base(suffix)}/users`, {
        params,
        signal: options?.signal,
      })
      return data
    },
    async getById(suffix: string, id: number): Promise<AdminUser> {
      const { data } = await apiClient.get<AdminUser>(`${base(suffix)}/users/${id}`)
      return data
    },
  },

  accounts: {
    async list(suffix: string, params: QuickAccountListParams = {}, options?: { signal?: AbortSignal }): Promise<PaginatedResponse<Account>> {
      const { data } = await apiClient.get<PaginatedResponse<Account>>(`${base(suffix)}/accounts`, {
        params,
        signal: options?.signal,
      })
      return data
    },
    async getById(suffix: string, id: number): Promise<Account> {
      const { data } = await apiClient.get<Account>(`${base(suffix)}/accounts/${id}`)
      return data
    },
  },

  groups: {
    async list(suffix: string, params: QuickGroupListParams = {}, options?: { signal?: AbortSignal }): Promise<PaginatedResponse<AdminGroup>> {
      const { data } = await apiClient.get<PaginatedResponse<AdminGroup>>(`${base(suffix)}/groups`, {
        params,
        signal: options?.signal,
      })
      return data
    },
    async getAll(suffix: string, platform?: GroupPlatform): Promise<AdminGroup[]> {
      const { data } = await apiClient.get<AdminGroup[]>(`${base(suffix)}/groups/all`, {
        params: platform ? { platform } : undefined,
      })
      return data
    },
    async getById(suffix: string, id: number): Promise<AdminGroup> {
      const { data } = await apiClient.get<AdminGroup>(`${base(suffix)}/groups/${id}`)
      return data
    },
    async getUsageSummary(suffix: string, timezone?: string): Promise<QuickGroupUsageSummary[]> {
      const { data } = await apiClient.get<QuickGroupUsageSummary[]>(`${base(suffix)}/groups/usage-summary`, {
        params: timezone ? { timezone } : undefined,
      })
      return data
    },
    async getCapacitySummary(suffix: string): Promise<QuickGroupCapacitySummary[]> {
      const { data } = await apiClient.get<QuickGroupCapacitySummary[]>(`${base(suffix)}/groups/capacity-summary`)
      return data
    },
  },

  subscriptions: {
    async list(suffix: string, params: QuickSubscriptionListParams = {}, options?: { signal?: AbortSignal }): Promise<PaginatedResponse<UserSubscription>> {
      const { data } = await apiClient.get<PaginatedResponse<UserSubscription>>(`${base(suffix)}/subscriptions`, {
        params,
        signal: options?.signal,
      })
      return data
    },
    async getById(suffix: string, id: number): Promise<UserSubscription> {
      const { data } = await apiClient.get<UserSubscription>(`${base(suffix)}/subscriptions/${id}`)
      return data
    },
    async getProgress(suffix: string, id: number): Promise<SubscriptionProgress> {
      const { data } = await apiClient.get<SubscriptionProgress>(`${base(suffix)}/subscriptions/${id}/progress`)
      return data
    },
  },

  usage: {
    async list(suffix: string, params: AdminUsageQueryParams, options?: { signal?: AbortSignal }): Promise<PaginatedResponse<AdminUsageLog>> {
      const { data } = await apiClient.get<PaginatedResponse<AdminUsageLog>>(`${base(suffix)}/usage`, {
        params,
        signal: options?.signal,
      })
      return data
    },
    async getDetail(suffix: string, id: number): Promise<UsageLogDetailResponse> {
      const { data } = await apiClient.get<UsageLogDetailResponse>(`${base(suffix)}/usage/${id}/detail`)
      return data
    },
    async getStats(suffix: string, params: QuickUsageStatsParams): Promise<AdminUsageStatsResponse> {
      const { data } = await apiClient.get<AdminUsageStatsResponse>(`${base(suffix)}/usage/stats`, { params })
      return data
    },
    async searchUsers(suffix: string, keyword: string): Promise<SimpleUser[]> {
      const { data } = await apiClient.get<SimpleUser[]>(`${base(suffix)}/usage/search-users`, { params: { q: keyword } })
      return data
    },
    async searchApiKeys(suffix: string, userId?: number, keyword?: string): Promise<SimpleApiKey[]> {
      const params: Record<string, unknown> = {}
      if (userId !== undefined) params.user_id = userId
      if (keyword) params.q = keyword
      const { data } = await apiClient.get<SimpleApiKey[]>(`${base(suffix)}/usage/search-api-keys`, { params })
      return data
    },
  },

  usageBrief: {
    async listReportGroups(suffix: string, params: UsageBriefReportGroupQuery = {}): Promise<PaginatedResponse<UsageBriefReportGroup>> {
      const { data } = await apiClient.get<PaginatedResponse<UsageBriefReportGroup>>(`${base(suffix)}/usage-brief/report-groups`, { params })
      return data
    },
    async listReports(suffix: string, params: UsageBriefReportQuery = {}): Promise<PaginatedResponse<UsageBriefReport>> {
      const { data } = await apiClient.get<PaginatedResponse<UsageBriefReport>>(`${base(suffix)}/usage-brief/reports`, { params })
      return data
    },
    async getReport(suffix: string, id: number): Promise<UsageBriefReport> {
      const { data } = await apiClient.get<UsageBriefReport>(`${base(suffix)}/usage-brief/reports/${id}`)
      return data
    },
  },
}

export default quickMonitorAPI
