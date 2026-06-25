<template>
  <QuickMonitorLayout>
    <div class="max-w-full space-y-6 overflow-hidden">
      <UsageStatsCards :stats="stats" />

      <div class="space-y-4">
        <div class="card p-4">
          <div class="quick-usage-range-toolbar flex min-w-0 max-w-full flex-col gap-4 md:flex-row md:flex-wrap md:items-center">
            <div class="quick-date-range-control flex min-w-0 max-w-full flex-col gap-2 sm:flex-row sm:items-center">
              <span class="shrink-0 text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.timeRange') }}:</span>
              <DateRangePicker
                v-model:start-date="startDate"
                v-model:end-date="endDate"
                @change="onDateRangeChange"
              />
            </div>
            <div class="quick-granularity-control flex min-w-0 max-w-full flex-col gap-2 md:ml-auto sm:flex-row sm:items-center">
              <span class="shrink-0 text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.granularity') }}:</span>
              <div class="min-w-0 md:w-28">
                <Select v-model="granularity" :options="granularityOptions" @change="loadChartData" />
              </div>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <ModelDistributionChart
            v-model:source="modelDistributionSource"
            v-model:metric="modelDistributionMetric"
            :model-stats="requestedModelStats"
            :upstream-model-stats="upstreamModelStats"
            :mapping-model-stats="mappingModelStats"
            :loading="modelStatsLoading"
            :show-source-toggle="true"
            :show-metric-toggle="true"
            :allow-breakdown="false"
            :start-date="startDate"
            :end-date="endDate"
            :filters="breakdownFilters"
          />
          <GroupDistributionChart
            v-model:metric="groupDistributionMetric"
            :group-stats="groupStats"
            :loading="chartsLoading"
            :show-metric-toggle="true"
            :allow-breakdown="false"
            :start-date="startDate"
            :end-date="endDate"
            :filters="breakdownFilters"
          />
        </div>

        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <EndpointDistributionChart
            v-model:source="endpointDistributionSource"
            v-model:metric="endpointDistributionMetric"
            :endpoint-stats="inboundEndpointStats"
            :upstream-endpoint-stats="upstreamEndpointStats"
            :endpoint-path-stats="endpointPathStats"
            :loading="endpointStatsLoading"
            :show-source-toggle="true"
            :show-metric-toggle="true"
            :allow-breakdown="false"
            :title="t('usage.endpointDistribution')"
            :start-date="startDate"
            :end-date="endDate"
            :filters="breakdownFilters"
          />
          <TokenUsageTrend :trend-data="trendData" :loading="chartsLoading" />
        </div>
      </div>

      <div class="card p-6">
        <div class="flex flex-wrap items-end justify-between gap-4">
          <div class="flex flex-1 flex-wrap items-end gap-4">
            <SearchDropdown
              v-model="userKeyword"
              :label="t('admin.usage.userFilter')"
              :placeholder="t('admin.usage.searchUserPlaceholder')"
              :items="userResults"
              :loading="userLoading"
              :display="formatUserOption"
              @search="searchUsers"
              @select="selectUser"
              @clear="clearUser"
            />

            <SearchDropdown
              v-model="apiKeyKeyword"
              :label="t('usage.apiKeyFilter')"
              :placeholder="t('admin.usage.searchApiKeyPlaceholder')"
              :items="apiKeyResults"
              :display="formatApiKeyOption"
              @search="searchApiKeys"
              @select="selectApiKey"
              @clear="clearApiKey"
            />

            <div class="w-full sm:w-auto sm:min-w-[220px]">
              <label class="input-label">{{ t('usage.model') }}</label>
              <Select
                v-model="filters.model"
                :options="modelSelectOptions"
                searchable
                clearable
                :placeholder="t('admin.usage.allModels')"
                @change="applyFilters"
              />
            </div>

            <SearchDropdown
              v-model="accountKeyword"
              :label="t('admin.usage.account')"
              :placeholder="t('admin.usage.searchAccountPlaceholder')"
              :items="accountResults"
              :display="formatAccountOption"
              @search="searchAccounts"
              @select="selectAccount"
              @clear="clearAccount"
            />

            <div class="w-full sm:w-auto sm:min-w-[180px]">
              <label class="input-label">{{ t('usage.type') }}</label>
              <Select v-model="filters.request_type" :options="requestTypeOptions" @change="applyFilters" />
            </div>

            <div class="w-full sm:w-auto sm:min-w-[200px]">
              <label class="input-label">{{ t('admin.usage.billingType') }}</label>
              <Select v-model="filters.billing_type" :options="billingTypeOptions" @change="applyFilters" />
            </div>

            <div class="w-full sm:w-auto sm:min-w-[200px]">
              <label class="input-label">{{ t('admin.usage.billingMode') }}</label>
              <Select v-model="filters.billing_mode" :options="billingModeOptions" @change="applyFilters" />
            </div>

            <div class="w-full sm:w-auto sm:min-w-[200px]">
              <label class="input-label">{{ t('admin.usage.group') }}</label>
              <Select v-model="filters.group_id" :options="groupOptions" searchable @change="applyFilters" />
            </div>
          </div>

          <div class="flex w-full flex-wrap items-center justify-end gap-3 sm:w-auto">
            <button type="button" class="btn btn-secondary" :disabled="loading || errLoading" @click="refreshData">
              {{ t('common.refresh') }}
            </button>
            <button type="button" class="btn btn-secondary" @click="resetFilters">
              {{ t('common.reset') }}
            </button>
          </div>
        </div>
      </div>

      <div class="mb-4 flex gap-2 border-b border-gray-200 dark:border-dark-700">
        <button type="button" class="tab" :class="{ 'tab-active': activeTab === 'usage' }" @click="activeTab = 'usage'">
          {{ t('usage.tabs.usage') }}
        </button>
        <button type="button" class="tab" :class="{ 'tab-active': activeTab === 'errors' }" @click="switchToErrorsTab">
          {{ t('usage.tabs.errors') }}
        </button>
      </div>

      <div v-show="activeTab === 'usage'">
        <UsageTable
          :data="logs"
          :loading="loading"
          :columns="columns"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
          @detailClick="openDetail"
        />

        <Pagination
          v-if="pagination.total > 0"
          :total="pagination.total"
          :page="pagination.page"
          :page-size="pagination.page_size"
          @update:page="handlePage"
          @update:pageSize="handlePageSize"
        />
      </div>

      <div v-show="activeTab === 'errors'" class="card overflow-hidden">
        <OpsErrorLogTable
          :rows="errRows"
          :total="errTotal"
          :loading="errLoading"
          :page="errPage"
          :page-size="errPageSize"
          @openErrorDetail="openError"
          @update:page="handleErrPage"
          @update:pageSize="handleErrPageSize"
        />
      </div>
    </div>
  </QuickMonitorLayout>

  <UsageLogDetailDialog
    :show="detailDialogVisible"
    :loading="detailLoading"
    :detail="detailData"
    :record="detailRecord"
    :admin="true"
    @close="closeDetail"
  />

  <OpsErrorDetailModal
    v-model:show="showErrorModal"
    :error-id="selectedErrorId"
    :error-type="'request'"
    :api="quickOpsErrorAPI"
  />
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import Pagination from '@/components/common/Pagination.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import QuickMonitorLayout from '@/components/layout/QuickMonitorLayout.vue'
import UsageStatsCards from '@/components/admin/usage/UsageStatsCards.vue'
import UsageTable from '@/components/admin/usage/UsageTable.vue'
import UsageLogDetailDialog from '@/components/usage/UsageLogDetailDialog.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import GroupDistributionChart from '@/components/charts/GroupDistributionChart.vue'
import EndpointDistributionChart from '@/components/charts/EndpointDistributionChart.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import OpsErrorLogTable from '@/views/admin/ops/components/OpsErrorLogTable.vue'
import OpsErrorDetailModal from '@/views/admin/ops/components/OpsErrorDetailModal.vue'
import { quickMonitorAPI } from '@/api/quickMonitor'
import { useAppStore } from '@/stores/app'
import { requestTypeToLegacyStream } from '@/utils/usageRequestType'
import type {
  Account,
  AdminGroup,
  AdminUsageLog,
  EndpointStat,
  GroupStat,
  ModelStat,
  TrendDataPoint,
  UsageLogDetailResponse,
} from '@/types'
import type { AdminUsageStatsResponse, AdminUsageQueryParams, SimpleApiKey, SimpleUser } from '@/api/admin/usage'
import type { OpsErrorLog, OpsErrorListQueryParams } from '@/api/admin/ops'
import type { Column } from '@/components/common/types'

type DistributionMetric = 'tokens' | 'actual_cost'
type EndpointSource = 'inbound' | 'upstream' | 'path'
type ModelDistributionSource = 'requested' | 'upstream' | 'mapping'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const suffix = computed(() => String(route.params.quickSuffix || ''))

const logs = ref<AdminUsageLog[]>([])
const stats = ref<AdminUsageStatsResponse | null>(null)
const groups = ref<AdminGroup[]>([])
const loading = ref(false)
const sortBy = ref('created_at')
const sortOrder = ref<'asc' | 'desc'>('desc')
const pagination = reactive({ page: 1, page_size: 50, total: 0 })
let abortController: AbortController | null = null

const trendData = ref<TrendDataPoint[]>([])
const requestedModelStats = ref<ModelStat[]>([])
const upstreamModelStats = ref<ModelStat[]>([])
const mappingModelStats = ref<ModelStat[]>([])
const groupStats = ref<GroupStat[]>([])
const inboundEndpointStats = ref<EndpointStat[]>([])
const upstreamEndpointStats = ref<EndpointStat[]>([])
const endpointPathStats = ref<EndpointStat[]>([])
const chartsLoading = ref(false)
const modelStatsLoading = ref(false)
const endpointStatsLoading = ref(false)
const granularity = ref<'day' | 'hour'>('hour')
const modelDistributionMetric = ref<DistributionMetric>('tokens')
const modelDistributionSource = ref<ModelDistributionSource>('requested')
const groupDistributionMetric = ref<DistributionMetric>('tokens')
const endpointDistributionMetric = ref<DistributionMetric>('tokens')
const endpointDistributionSource = ref<EndpointSource>('inbound')
const loadedModelSources = reactive<Record<ModelDistributionSource, boolean>>({
  requested: false,
  upstream: false,
  mapping: false,
})
let chartReqSeq = 0
let statsReqSeq = 0
let modelStatsReqSeq = 0

const activeTab = ref<'usage' | 'errors'>('usage')
const errRows = ref<OpsErrorLog[]>([])
const errLoading = ref(false)
const errPage = ref(1)
const errPageSize = ref(50)
const errTotal = ref(0)
const showErrorModal = ref(false)
const selectedErrorId = ref<number | null>(null)

const detailDialogVisible = ref(false)
const detailLoading = ref(false)
const detailData = ref<UsageLogDetailResponse | null>(null)
const detailRecord = ref<AdminUsageLog | null>(null)

const userKeyword = ref('')
const userResults = ref<SimpleUser[]>([])
const userLoading = ref(false)
const apiKeyKeyword = ref('')
const apiKeyResults = ref<SimpleApiKey[]>([])
const accountKeyword = ref('')
const accountResults = ref<Account[]>([])

const formatLocalDate = (date: Date) => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const getLast24HoursRangeDates = () => {
  const end = new Date()
  const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
  return { start: formatLocalDate(start), end: formatLocalDate(end) }
}

const getGranularityForRange = (start: string, end: string): 'day' | 'hour' => {
  const startTime = new Date(`${start}T00:00:00`).getTime()
  const endTime = new Date(`${end}T00:00:00`).getTime()
  const daysDiff = Math.ceil((endTime - startTime) / (1000 * 60 * 60 * 24))
  return daysDiff <= 1 ? 'hour' : 'day'
}

const defaultRange = getLast24HoursRangeDates()
const startDate = ref(defaultRange.start)
const endDate = ref(defaultRange.end)

const filters = reactive<AdminUsageQueryParams>({
  user_id: undefined,
  api_key_id: undefined,
  account_id: undefined,
  group_id: undefined,
  model: undefined,
  request_type: undefined,
  billing_type: null,
  billing_mode: undefined,
  start_date: startDate.value,
  end_date: endDate.value,
})

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('quickMonitor.usage.columns.user'), sortable: false },
  { key: 'api_key', label: t('quickMonitor.usage.columns.apiKey'), sortable: false },
  { key: 'account', label: t('quickMonitor.usage.columns.account'), sortable: false },
  { key: 'model', label: t('quickMonitor.usage.columns.model'), sortable: true },
  { key: 'endpoint', label: t('usage.endpoint'), sortable: false },
  { key: 'group', label: t('admin.usage.group'), sortable: false },
  { key: 'stream', label: t('quickMonitor.usage.columns.type'), sortable: false },
  { key: 'billing_mode', label: t('admin.usage.billingMode'), sortable: false },
  { key: 'tokens', label: t('quickMonitor.usage.columns.tokens'), sortable: false },
  { key: 'cost', label: t('quickMonitor.usage.columns.cost'), sortable: false },
  { key: 'first_token', label: t('usage.firstToken'), sortable: false },
  { key: 'duration', label: t('quickMonitor.usage.columns.duration'), sortable: false },
  { key: 'created_at', label: t('quickMonitor.usage.columns.time'), sortable: true },
  { key: 'actions', label: t('quickMonitor.usage.columns.detail'), sortable: false },
])

const granularityOptions = computed<SelectOption[]>(() => [
  { value: 'day', label: t('admin.dashboard.day') },
  { value: 'hour', label: t('admin.dashboard.hour') },
])

const requestTypeOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allTypes') },
  { value: 'ws_v2', label: t('usage.ws') },
  { value: 'stream', label: t('usage.stream') },
  { value: 'sync', label: t('usage.sync') },
  { value: 'cyber', label: t('usage.cyber') },
])

const billingTypeOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allBillingTypes') },
  { value: 0, label: t('admin.usage.billingTypeBalance') },
  { value: 1, label: t('admin.usage.billingTypeSubscription') },
])

const billingModeOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allBillingModes') },
  { value: 'token', label: t('admin.usage.billingModeToken') },
  { value: 'per_request', label: t('admin.usage.billingModePerRequest') },
  { value: 'image', label: t('admin.usage.billingModeImage') },
])

const groupOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allGroups') },
  ...groups.value.map((group) => ({ value: group.id, label: group.name })),
])

const modelNameOptions = computed(() =>
  Array.from(new Set([
    ...requestedModelStats.value,
    ...upstreamModelStats.value,
    ...mappingModelStats.value,
  ].map((model) => model.model).filter(Boolean))).sort()
)

const modelSelectOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allModels') },
  ...modelNameOptions.value.map((model) => ({ value: model, label: model })),
])

const breakdownFilters = computed(() => {
  const result: Record<string, unknown> = {}
  if (filters.user_id) result.user_id = filters.user_id
  if (filters.api_key_id) result.api_key_id = filters.api_key_id
  if (filters.account_id) result.account_id = filters.account_id
  if (filters.group_id) result.group_id = filters.group_id
  if (filters.request_type != null) result.request_type = filters.request_type
  if (filters.billing_type != null) result.billing_type = filters.billing_type
  return result
})

const quickOpsErrorAPI = computed(() => ({
  getRequestErrorDetail: (id: number) => quickMonitorAPI.ops.getRequestErrorDetail(suffix.value, id),
  getUpstreamErrorDetail: (id: number) => quickMonitorAPI.ops.getUpstreamErrorDetail(suffix.value, id),
  listRequestErrorUpstreamErrors: (
    id: number,
    params?: OpsErrorListQueryParams,
    options?: { include_detail?: boolean }
  ) => quickMonitorAPI.ops.listRequestErrorUpstreamErrors(suffix.value, id, params, options),
}))

function buildBaseParams() {
  const requestType = filters.request_type
  const legacyStream = requestType ? requestTypeToLegacyStream(requestType) : filters.stream
  return {
    ...filters,
    stream: legacyStream === null ? undefined : legacyStream,
  }
}

function buildListParams(): AdminUsageQueryParams {
  return {
    ...buildBaseParams(),
    page: pagination.page,
    page_size: pagination.page_size,
    exact_total: false,
    sort_by: sortBy.value,
    sort_order: sortOrder.value,
  }
}

async function loadLogs() {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  loading.value = true
  try {
    const data = await quickMonitorAPI.usage.list(suffix.value, buildListParams(), { signal: controller.signal })
    if (!controller.signal.aborted) {
      logs.value = data.items || []
      pagination.total = data.total || 0
    }
  } catch (error: any) {
    if (error?.name !== 'AbortError') {
      console.error('[QuickMonitorUsage] Failed to load usage logs', error)
      appStore.showError(t('usage.loadFailed'))
    }
  } finally {
    if (abortController === controller) loading.value = false
  }
}

async function loadStats(force = false) {
  const seq = ++statsReqSeq
  endpointStatsLoading.value = true
  try {
    const data = await quickMonitorAPI.usage.getStats(suffix.value, {
      ...buildBaseParams(),
      nocache: force ? 1 : undefined,
    })
    if (seq !== statsReqSeq) return
    stats.value = data
    inboundEndpointStats.value = data.endpoints || []
    upstreamEndpointStats.value = data.upstream_endpoints || []
    endpointPathStats.value = data.endpoint_paths || []
  } catch (error) {
    if (seq !== statsReqSeq) return
    console.error('[QuickMonitorUsage] Failed to load usage stats', error)
    stats.value = null
    inboundEndpointStats.value = []
    upstreamEndpointStats.value = []
    endpointPathStats.value = []
  } finally {
    if (seq === statsReqSeq) endpointStatsLoading.value = false
  }
}

function invalidateModelStatsCache() {
  loadedModelSources.requested = false
  loadedModelSources.upstream = false
  loadedModelSources.mapping = false
}

async function loadModelStats(source: ModelDistributionSource, force = false) {
  if (!force && loadedModelSources[source]) return
  const seq = ++modelStatsReqSeq
  modelStatsLoading.value = true
  try {
    const data = await quickMonitorAPI.dashboard.getModelStats(suffix.value, {
      ...buildBaseParams(),
      model_source: source,
    })
    if (seq !== modelStatsReqSeq) return
    const models = data.models || []
    if (source === 'requested') requestedModelStats.value = models
    else if (source === 'upstream') upstreamModelStats.value = models
    else mappingModelStats.value = models
    loadedModelSources[source] = true
  } catch (error) {
    if (seq !== modelStatsReqSeq) return
    console.error('[QuickMonitorUsage] Failed to load model stats', error)
    if (source === 'requested') requestedModelStats.value = []
    else if (source === 'upstream') upstreamModelStats.value = []
    else mappingModelStats.value = []
    loadedModelSources[source] = false
  } finally {
    if (seq === modelStatsReqSeq) modelStatsLoading.value = false
  }
}

async function loadChartData() {
  const seq = ++chartReqSeq
  chartsLoading.value = true
  try {
    const snapshot = await quickMonitorAPI.dashboard.getSnapshotV2(suffix.value, {
      ...buildBaseParams(),
      granularity: granularity.value,
      include_stats: false,
      include_trend: true,
      include_model_stats: false,
      include_group_stats: true,
      include_users_trend: false,
    })
    if (seq !== chartReqSeq) return
    trendData.value = snapshot.trend || []
    groupStats.value = snapshot.groups || []
  } catch (error) {
    if (seq !== chartReqSeq) return
    console.error('[QuickMonitorUsage] Failed to load chart data', error)
    trendData.value = []
    groupStats.value = []
  } finally {
    if (seq === chartReqSeq) chartsLoading.value = false
  }
}

async function loadGroups() {
  groups.value = await quickMonitorAPI.groups.getAll(suffix.value)
}

function applyFilters() {
  pagination.page = 1
  errPage.value = 1
  invalidateModelStatsCache()
  void loadLogs()
  void loadStats()
  void loadModelStats(modelDistributionSource.value, true)
  void loadChartData()
  if (activeTab.value === 'errors') void loadErrors()
  else errRows.value = []
}

function refreshData() {
  invalidateModelStatsCache()
  void loadLogs()
  void loadStats(true)
  void loadModelStats(modelDistributionSource.value, true)
  void loadChartData()
  if (activeTab.value === 'errors') void loadErrors()
}

function resetFilters() {
  const range = getLast24HoursRangeDates()
  startDate.value = range.start
  endDate.value = range.end
  filters.user_id = undefined
  filters.api_key_id = undefined
  filters.account_id = undefined
  filters.group_id = undefined
  filters.model = undefined
  filters.request_type = undefined
  filters.stream = undefined
  filters.billing_type = null
  filters.billing_mode = undefined
  filters.start_date = startDate.value
  filters.end_date = endDate.value
  granularity.value = getGranularityForRange(startDate.value, endDate.value)
  userKeyword.value = ''
  apiKeyKeyword.value = ''
  accountKeyword.value = ''
  userResults.value = []
  apiKeyResults.value = []
  accountResults.value = []
  applyFilters()
}

function onDateRangeChange(range: { startDate: string; endDate: string; preset: string | null }) {
  startDate.value = range.startDate
  endDate.value = range.endDate
  filters.start_date = range.startDate
  filters.end_date = range.endDate
  granularity.value = getGranularityForRange(range.startDate, range.endDate)
  applyFilters()
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortBy.value = key
  sortOrder.value = order
  applyFilters()
}

function handlePage(nextPage: number) {
  pagination.page = nextPage
  void loadLogs()
}

function handlePageSize(size: number) {
  pagination.page_size = size
  pagination.page = 1
  void loadLogs()
}

async function openDetail(record: AdminUsageLog) {
  detailRecord.value = record
  detailData.value = null
  detailDialogVisible.value = true
  detailLoading.value = true
  try {
    detailData.value = await quickMonitorAPI.usage.getDetail(suffix.value, record.id)
  } catch (error) {
    console.error('[QuickMonitorUsage] Failed to load usage detail', error)
    appStore.showError(t('usage.detailLoadFailed'))
    closeDetail()
  } finally {
    detailLoading.value = false
  }
}

function closeDetail() {
  detailDialogVisible.value = false
  detailRecord.value = null
  detailData.value = null
}

const toRFC3339 = (date: string | undefined, endOfDay = false) =>
  date ? new Date(`${date}${endOfDay ? 'T23:59:59.999' : 'T00:00:00'}`).toISOString() : undefined

async function loadErrors() {
  errLoading.value = true
  try {
    const data = await quickMonitorAPI.ops.listErrorLogs(suffix.value, {
      page: errPage.value,
      page_size: errPageSize.value,
      view: 'all',
      start_time: toRFC3339(filters.start_date),
      end_time: toRFC3339(filters.end_date, true),
      user_id: filters.user_id ?? undefined,
      api_key_id: filters.api_key_id ?? undefined,
      account_id: filters.account_id ?? undefined,
      group_id: filters.group_id ?? undefined,
      model: filters.model || undefined,
    })
    errRows.value = data.items || []
    errTotal.value = data.total || 0
  } catch (error) {
    console.error('[QuickMonitorUsage] Failed to load error logs', error)
    appStore.showError(t('usage.errors.failedToLoad'))
  } finally {
    errLoading.value = false
  }
}

function handleErrPage(page: number) {
  errPage.value = page
  void loadErrors()
}

function handleErrPageSize(size: number) {
  errPageSize.value = size
  errPage.value = 1
  void loadErrors()
}

function openError(id: number) {
  selectedErrorId.value = id
  showErrorModal.value = true
}

function switchToErrorsTab() {
  activeTab.value = 'errors'
  if (errRows.value.length === 0) void loadErrors()
}

async function searchUsers(keyword: string) {
  if (!keyword.trim()) {
    userResults.value = []
    return
  }
  userLoading.value = true
  try {
    userResults.value = await quickMonitorAPI.usage.searchUsers(suffix.value, keyword)
  } finally {
    userLoading.value = false
  }
}

async function searchApiKeys(keyword: string) {
  apiKeyResults.value = await quickMonitorAPI.usage.searchApiKeys(suffix.value, filters.user_id, keyword)
}

async function searchAccounts(keyword: string) {
  if (!keyword.trim()) {
    accountResults.value = []
    return
  }
  const data = await quickMonitorAPI.accounts.list(suffix.value, {
    page: 1,
    page_size: 20,
    search: keyword,
    sort_by: 'name',
    sort_order: 'asc',
  })
  accountResults.value = data.items || []
}

function selectUser(user: SimpleUser) {
  filters.user_id = user.id
  userKeyword.value = user.email
  clearApiKey(false)
  applyFilters()
}

function clearUser(apply = true) {
  filters.user_id = undefined
  userKeyword.value = ''
  userResults.value = []
  clearApiKey(false)
  if (apply) applyFilters()
}

function selectApiKey(key: SimpleApiKey) {
  filters.api_key_id = key.id
  apiKeyKeyword.value = key.name || `#${key.id}`
  applyFilters()
}

function clearApiKey(apply = true) {
  filters.api_key_id = undefined
  apiKeyKeyword.value = ''
  apiKeyResults.value = []
  if (apply) applyFilters()
}

function selectAccount(account: Account) {
  filters.account_id = account.id
  accountKeyword.value = account.name || `#${account.id}`
  applyFilters()
}

function clearAccount() {
  filters.account_id = undefined
  accountKeyword.value = ''
  accountResults.value = []
  applyFilters()
}

function formatUserOption(item: unknown) {
  const user = item as SimpleUser
  return `${user.email} #${user.id}${user.deleted ? ` (${t('admin.usage.userDeletedBadge')})` : ''}`
}

function formatApiKeyOption(item: unknown) {
  const key = item as SimpleApiKey
  return `${key.name || `#${key.id}`} #${key.id}`
}

function formatAccountOption(item: unknown) {
  const account = item as Account
  return `${account.name || `#${account.id}`} #${account.id}`
}

const SearchDropdown = defineComponent({
  name: 'SearchDropdown',
  props: {
    modelValue: { type: String, default: '' },
    label: { type: String, required: true },
    placeholder: { type: String, default: '' },
    items: { type: Array as () => unknown[], default: () => [] },
    loading: { type: Boolean, default: false },
    display: { type: Function as unknown as () => (item: unknown) => string, required: true },
  },
  emits: ['update:modelValue', 'search', 'select', 'clear'],
  setup(props, { emit }) {
    const open = ref(false)
    let timer: ReturnType<typeof setTimeout> | undefined
    const update = (value: string) => {
      emit('update:modelValue', value)
      open.value = true
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => emit('search', value), 300)
    }
    const clear = () => {
      open.value = false
      emit('update:modelValue', '')
      emit('clear')
    }
    return () =>
      h('div', { class: 'usage-filter-dropdown relative w-full sm:w-auto sm:min-w-[240px]' }, [
        h('label', { class: 'input-label' }, props.label),
        h('input', {
          value: props.modelValue,
          type: 'text',
          class: 'input pr-8',
          placeholder: props.placeholder,
          onInput: (event: Event) => update((event.target as HTMLInputElement).value),
          onFocus: () => {
            open.value = true
          },
          onKeyup: (event: KeyboardEvent) => {
            if (event.key === 'Enter') emit('search', props.modelValue)
          },
        }),
        props.modelValue
          ? h('button', {
              type: 'button',
              class: 'absolute right-2 top-9 text-gray-400',
              'aria-label': t('quickMonitor.usage.clearFilter'),
              onClick: clear,
            }, 'x')
          : null,
        open.value && (props.items.length > 0 || props.modelValue)
          ? h('div', { class: 'absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:bg-gray-800' }, [
              props.loading
                ? h('div', { class: 'px-4 py-3 text-sm text-gray-500 dark:text-gray-400' }, t('common.loading'))
                : props.items.length === 0
                  ? h('div', { class: 'px-4 py-3 text-sm text-gray-500 dark:text-gray-400' }, t('common.noOptionsFound'))
                  : props.items.map((item) =>
                      h('button', {
                        type: 'button',
                        class: 'w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700',
                        onClick: () => {
                          open.value = false
                          emit('select', item)
                        },
                      }, props.display(item)),
                    ),
            ])
          : null,
      ])
  },
})

onMounted(async () => {
  await loadGroups()
  void loadLogs()
  void loadStats()
  void loadModelStats(modelDistributionSource.value, true)
  window.setTimeout(() => {
    void loadChartData()
  }, 120)
})

onUnmounted(() => {
  abortController?.abort()
})

watch(modelDistributionSource, (source) => {
  void loadModelStats(source)
})
</script>

<style scoped>
.quick-date-range-control :deep(.relative),
.quick-granularity-control :deep(.relative) {
  min-width: 0;
  max-width: 100%;
}

.quick-date-range-control :deep(.date-picker-trigger) {
  box-sizing: border-box;
  max-width: 100%;
  width: 100%;
}

.quick-date-range-control :deep(.date-picker-value) {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 639px) {
  .quick-date-range-control :deep(.date-picker-dropdown) {
    left: 0;
    max-width: calc(100vw - 2rem);
    min-width: 0;
    width: min(320px, calc(100vw - 2rem));
  }

  .quick-date-range-control :deep(.date-picker-custom) {
    align-items: stretch;
    flex-direction: column;
  }

  .quick-date-range-control :deep(.date-picker-separator) {
    display: none;
  }
}
</style>
