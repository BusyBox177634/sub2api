<template>
  <QuickMonitorLayout>
    <div class="max-w-full space-y-5 overflow-hidden">
      <section class="card p-4">
        <div class="quick-brief-filters grid min-w-0 max-w-full grid-cols-1 gap-3 lg:grid-cols-[180px_160px_160px_160px_minmax(0,1fr)_auto]">
          <div class="quick-filter-field">
            <select v-model="groupBy" class="input w-full min-w-0" :aria-label="t('quickMonitor.usageBrief.groupBy')">
              <option value="user">{{ t('quickMonitor.usageBrief.groupByUser') }}</option>
              <option value="period">{{ t('quickMonitor.usageBrief.groupByPeriod') }}</option>
            </select>
          </div>
          <div class="quick-filter-field">
            <select v-model="periodType" class="input w-full min-w-0">
              <option value="">{{ t('quickMonitor.usageBrief.allPeriods') }}</option>
              <option value="daily">{{ t('quickMonitor.usageBrief.daily') }}</option>
              <option value="weekly">{{ t('quickMonitor.usageBrief.weekly') }}</option>
              <option value="monthly">{{ t('quickMonitor.usageBrief.monthly') }}</option>
            </select>
          </div>
          <div class="quick-filter-field">
            <input v-model="startDate" class="input w-full min-w-0" type="date" />
          </div>
          <div class="quick-filter-field">
            <input v-model="endDate" class="input w-full min-w-0" type="date" />
          </div>
          <div class="quick-filter-field">
            <input v-model.trim="search" class="input w-full min-w-0" type="search" :placeholder="t('quickMonitor.searchByUser')" @keyup.enter="reload" />
          </div>
          <div class="quick-filter-field">
            <button class="btn btn-secondary w-full lg:w-auto" type="button" :disabled="loading" @click="reload">
              {{ t('common.refresh') }}
            </button>
          </div>
        </div>
      </section>

      <section class="space-y-3">
        <div v-if="loading" class="card p-8 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('common.loading') }}</div>
        <div v-else-if="groups.length === 0" class="card p-8 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('quickMonitor.usageBrief.noReports') }}</div>
        <article v-for="group in groups" v-else :key="group.key" class="card overflow-hidden">
          <button
            class="flex w-full min-w-0 flex-col items-start justify-between gap-2 px-4 py-4 text-left hover:bg-gray-50 dark:hover:bg-dark-800 sm:flex-row sm:items-center sm:gap-4 sm:px-5"
            type="button"
            @click="toggleGroup(group.key)"
          >
            <div class="min-w-0 max-w-full">
              <h2 class="break-words text-base font-semibold text-gray-900 dark:text-white">{{ groupTitle(group) }}</h2>
              <p class="mt-1 break-words text-xs text-gray-500 dark:text-dark-400">
                <span v-if="groupBy === 'user' && group.username && group.user_email">{{ group.user_email }} · </span>
                {{ groupMeta(group) }}
              </p>
            </div>
            <span class="shrink-0 text-sm text-primary-600 dark:text-primary-300">
              {{ expandedKeys.has(group.key) ? t('common.collapse') : t('common.expand') }}
            </span>
          </button>

          <div v-if="expandedKeys.has(group.key)" class="divide-y divide-gray-100 border-t border-gray-100 dark:divide-dark-700 dark:border-dark-700">
            <button
              v-for="report in group.reports"
              :key="report.id"
              type="button"
              class="flex w-full min-w-0 flex-col items-start justify-between gap-2 px-4 py-3 text-left hover:bg-gray-50 dark:hover:bg-dark-800 sm:flex-row sm:items-center sm:gap-4 sm:px-5"
              @click="selectReport(report.id)"
            >
              <div class="min-w-0 max-w-full">
                <div class="break-words font-medium text-gray-900 dark:text-white">{{ report.title }}</div>
                <div class="mt-1 break-words text-xs text-gray-500 dark:text-dark-400">
                  <template v-if="groupBy === 'period'">
                    {{ reportUserTitle(report) }} ·
                  </template>
                  {{ periodTypeLabel(report.period_type) }} · {{ formatPeriod(report) }} · {{ reportStatusLabel(report.status) }}
                </div>
              </div>
              <div class="shrink-0 text-left text-xs text-gray-500 dark:text-dark-400 sm:text-right">
                <div>{{ formatDate(report.generated_at || report.created_at) }}</div>
                <div>{{ formatInteger(report.input_usage_count) }} {{ t('quickMonitor.recordsUnit') }}</div>
              </div>
            </button>
          </div>
        </article>
      </section>

      <Pagination
        v-if="total > 0"
        :total="total"
        :page="page"
        :page-size="pageSize"
        :page-size-options="[20, 50, 100]"
        @update:page="page = $event"
        @update:pageSize="handlePageSize"
      />
    </div>

    <div v-if="selectedReport" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-2 sm:p-4" @click.self="selectedReport = null">
      <div class="max-h-[90vh] w-full max-w-5xl overflow-hidden rounded-lg bg-white shadow-xl dark:bg-dark-900">
        <div class="flex flex-col items-start justify-between gap-3 border-b border-gray-200 px-4 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:px-5">
          <div class="min-w-0 max-w-full">
            <h2 class="break-words text-base font-semibold text-gray-900 dark:text-white sm:text-lg">{{ selectedReport.title }}</h2>
            <p class="break-words text-xs text-gray-500 dark:text-dark-400">
              {{ reportUserTitle(selectedReport) }} · {{ formatPeriod(selectedReport) }}
            </p>
          </div>
          <button class="btn btn-secondary btn-sm w-full sm:w-auto" type="button" @click="selectedReport = null">{{ t('common.close') }}</button>
        </div>
        <div class="max-h-[calc(90vh-112px)] overflow-y-auto p-4 sm:max-h-[calc(90vh-80px)] sm:p-6">
          <MarkdownContent :content="selectedReport.content_md" />
        </div>
      </div>
    </div>
  </QuickMonitorLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import MarkdownContent from '@/components/common/MarkdownContent.vue'
import Pagination from '@/components/common/Pagination.vue'
import QuickMonitorLayout from '@/components/layout/QuickMonitorLayout.vue'
import { quickMonitorAPI } from '@/api/quickMonitor'
import type { UsageBriefPeriodType, UsageBriefReport, UsageBriefReportGroup, UsageBriefReportGroupBy, UsageBriefStatus } from '@/api/usageBrief'

const { t } = useI18n()
const route = useRoute()
const suffix = computed(() => String(route.params.quickSuffix || ''))
const groupByStorageKey = computed(() => `quick_monitor_usage_brief_group_by:${suffix.value}`)

const groups = ref<UsageBriefReportGroup[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const periodType = ref<'' | UsageBriefPeriodType>('')
const savedGroupBy = localStorage.getItem(groupByStorageKey.value)
const groupBy = ref<UsageBriefReportGroupBy>(savedGroupBy === 'period' || savedGroupBy === 'user' ? savedGroupBy : 'user')
const startDate = ref('')
const endDate = ref('')
const search = ref('')
const loading = ref(false)
const expandedKeys = ref<Set<string>>(new Set())
const selectedReport = ref<UsageBriefReport | null>(null)

async function loadGroups() {
  loading.value = true
  try {
    const data = await quickMonitorAPI.usageBrief.listReportGroups(suffix.value, {
      page: page.value,
      page_size: pageSize.value,
      group_by: groupBy.value,
      period_type: periodType.value || undefined,
      source_kind: 'production',
      start_date: startDate.value || undefined,
      end_date: endDate.value || undefined,
      search: search.value || undefined,
      search_scope: 'user',
    })
    groups.value = data.items || []
    total.value = data.total || 0
    expandedKeys.value = new Set(groups.value.slice(0, 1).map((item) => item.key))
  } finally {
    loading.value = false
  }
}

function reload() {
  page.value = 1
  void loadGroups()
}

function handlePageSize(size: number) {
  pageSize.value = size
  page.value = 1
}

function toggleGroup(key: string) {
  const next = new Set(expandedKeys.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expandedKeys.value = next
}

async function selectReport(id: number) {
  selectedReport.value = await quickMonitorAPI.usageBrief.getReport(suffix.value, id)
}

function reportStatusLabel(status: UsageBriefStatus) {
  return t(`quickMonitor.usageBrief.status.${status}`)
}

function periodTypeLabel(type: UsageBriefPeriodType) {
  return t(`quickMonitor.usageBrief.${type}`)
}

function reportUserTitle(report: UsageBriefReport) {
  return report.username || report.user_email || t('quickMonitor.usageBrief.userFallback', { id: report.user_id })
}

function groupTitle(group: UsageBriefReportGroup) {
  if (group.group_by === 'period') {
    return `${periodTypeLabel(group.period_type || 'daily')} · ${formatPeriod(group)}`
  }
  return groupUserTitle(group)
}

function groupUserTitle(group: UsageBriefReportGroup) {
  return group.username || group.user_email || (group.user_id ? t('quickMonitor.usageBrief.userFallback', { id: group.user_id }) : group.label)
}

function groupMeta(group: UsageBriefReportGroup) {
  if (group.group_by === 'period') {
    return `${t('quickMonitor.usageBrief.reportCount', { count: group.report_count })} · ${t('quickMonitor.usageBrief.userCount', { count: group.user_count })}`
  }
  return `${t('quickMonitor.usageBrief.reportCount', { count: group.report_count })} · ${formatUserGroupRange(group)}`
}

function formatPeriod(value: UsageBriefReportGroup | UsageBriefReport) {
  return t('quickMonitor.dateRange', {
    start: formatDate(value.period_start),
    end: formatDate(value.period_end),
  })
}

function formatUserGroupRange(group: UsageBriefReportGroup) {
  const starts = group.reports
    .map((item) => item.period_start)
    .filter(Boolean)
    .sort()
  const ends = group.reports
    .map((item) => item.period_end)
    .filter(Boolean)
    .sort()
  if (starts.length === 0 || ends.length === 0) return t('quickMonitor.usageBrief.noPeriod')
  return t('quickMonitor.dateRange', {
    start: formatDate(starts[0]),
    end: formatDate(ends[ends.length - 1]),
  })
}

function formatDate(value?: string | null) {
  return value ? new Date(value).toLocaleDateString() : '-'
}

function formatInteger(value?: number | null) {
  return Math.round(Number(value || 0)).toLocaleString()
}

watch([page, pageSize, groupBy, periodType, startDate, endDate], () => {
  void loadGroups()
})

watch(groupBy, (value) => {
  localStorage.setItem(groupByStorageKey.value, value)
  page.value = 1
})

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(reload, 300)
})

onMounted(() => {
  void loadGroups()
})
</script>

<style scoped>
.quick-filter-field {
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
}

.quick-filter-field :deep(select),
.quick-filter-field :deep(input),
.quick-filter-field :deep(button) {
  box-sizing: border-box;
  max-width: 100%;
  min-width: 0;
}

.quick-filter-field :deep(input[type='date']) {
  min-inline-size: 0;
  width: 100%;
}
</style>
