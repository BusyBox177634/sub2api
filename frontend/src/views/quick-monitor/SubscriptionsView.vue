<template>
  <QuickMonitorLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.status"
                :options="statusOptions"
                :placeholder="t('admin.subscriptions.allStatus')"
                @change="applyFilters"
              />
            </div>
            <div class="w-full sm:w-48">
              <Select
                v-model="filters.group_id"
                :options="groupOptions"
                :placeholder="t('admin.subscriptions.allGroups')"
                searchable
                @change="applyFilters"
              />
            </div>
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.platform"
                :options="platformFilterOptions"
                :placeholder="t('admin.subscriptions.allPlatforms')"
                @change="applyFilters"
              />
            </div>
          </div>

          <div class="ml-auto flex flex-wrap items-center justify-end gap-3">
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="loading"
              :title="t('common.refresh')"
              @click="loadSubscriptions"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="subscriptions"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-user="{ row }">
            <div class="flex items-center gap-2">
              <div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
                <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
                  {{ (row.user?.email || `#${row.user_id}`).charAt(0).toUpperCase() }}
                </span>
              </div>
              <div>
                <div class="font-medium text-gray-900 dark:text-white">
                  {{ row.user?.email || `#${row.user_id}` }}
                </div>
                <div v-if="row.user?.username" class="text-xs text-gray-500 dark:text-dark-400">
                  {{ row.user.username }}
                </div>
              </div>
            </div>
          </template>

          <template #cell-group="{ row }">
            <GroupBadge
              v-if="row.group"
              :name="row.group.name"
              :platform="row.group.platform"
              :subscription-type="row.group.subscription_type"
              :rate-multiplier="row.group.rate_multiplier"
              :show-rate="false"
            />
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-usage="{ row }">
            <div class="min-w-[280px] space-y-2">
              <UsageProgressRow
                v-if="row.group?.daily_limit_usd"
                :label="t('admin.subscriptions.daily')"
                :used="row.daily_usage_usd"
                :limit="row.group.daily_limit_usd"
                :hint="formatResetTime(row.daily_window_start, 'daily')"
              />
              <UsageProgressRow
                v-if="row.group?.weekly_limit_usd"
                :label="t('admin.subscriptions.weekly')"
                :used="row.weekly_usage_usd"
                :limit="row.group.weekly_limit_usd"
                :hint="formatResetTime(row.weekly_window_start, 'weekly')"
              />
              <UsageProgressRow
                v-if="row.group?.monthly_limit_usd"
                :label="t('admin.subscriptions.monthly')"
                :used="row.monthly_usage_usd"
                :limit="row.group.monthly_limit_usd"
                :hint="formatResetTime(row.monthly_window_start, 'monthly')"
              />
              <div
                v-if="!row.group?.daily_limit_usd && !row.group?.weekly_limit_usd && !row.group?.monthly_limit_usd"
                class="flex items-center gap-2 rounded-lg bg-gradient-to-r from-emerald-50 to-teal-50 px-3 py-2 dark:from-emerald-900/20 dark:to-teal-900/20"
              >
                <span class="text-lg text-emerald-600 dark:text-emerald-400">∞</span>
                <span class="text-xs font-medium text-emerald-700 dark:text-emerald-300">
                  {{ t('admin.subscriptions.unlimited') }}
                </span>
              </div>
            </div>
          </template>

          <template #cell-expires_at="{ value }">
            <div v-if="value">
              <span
                class="text-sm"
                :class="isExpiringSoon(value) ? 'text-orange-600 dark:text-orange-400' : 'text-gray-700 dark:text-gray-300'"
              >
                {{ formatDateOnly(value) }}
              </span>
              <div v-if="getDaysRemaining(value) !== null" class="text-xs text-gray-500">
                {{ getDaysRemaining(value) }} {{ t('admin.subscriptions.daysRemaining') }}
              </div>
            </div>
            <span v-else class="text-sm text-gray-500">
              {{ t('admin.subscriptions.noExpiration') }}
            </span>
          </template>

          <template #cell-status="{ value }">
            <span
              :class="[
                'badge',
                value === 'active'
                  ? 'badge-success'
                  : value === 'expired'
                    ? 'badge-warning'
                    : 'badge-danger',
              ]"
            >
              {{ t(`admin.subscriptions.status.${value}`) }}
            </span>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.subscriptions.noSubscriptionsYet')"
              :description="t('quickMonitor.noFilteredSubscriptions')"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>
  </QuickMonitorLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { quickMonitorAPI } from '@/api/quickMonitor'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import QuickMonitorLayout from '@/components/layout/QuickMonitorLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import { formatDateOnly } from '@/utils/format'
import type { AdminGroup, GroupPlatform, UserSubscription } from '@/types'
import type { Column } from '@/components/common/types'

const { t } = useI18n()
const route = useRoute()
const suffix = computed(() => String(route.params.quickSuffix || ''))

const subscriptions = ref<UserSubscription[]>([])
const groups = ref<AdminGroup[]>([])
const loading = ref(false)

const filters = reactive({
  status: 'active',
  group_id: '',
  platform: '',
})

const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc',
})

const pagination = reactive({
  page: 1,
  page_size: 50,
  total: 0,
})

const columns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.subscriptions.columns.user'), sortable: false },
  { key: 'group', label: t('admin.subscriptions.columns.group'), sortable: false },
  { key: 'usage', label: t('admin.subscriptions.columns.usage'), sortable: false },
  { key: 'expires_at', label: t('admin.subscriptions.columns.expires'), sortable: true },
  { key: 'status', label: t('admin.subscriptions.columns.status'), sortable: true },
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allStatus') },
  { value: 'active', label: t('admin.subscriptions.status.active') },
  { value: 'expired', label: t('admin.subscriptions.status.expired') },
  { value: 'revoked', label: t('admin.subscriptions.status.revoked') },
])

const groupOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allGroups') },
  ...groups.value.map((group) => ({ value: String(group.id), label: group.name })),
])

const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allPlatforms') },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' },
])

async function loadSubscriptions() {
  loading.value = true
  try {
    const data = await quickMonitorAPI.subscriptions.list(suffix.value, {
      page: pagination.page,
      page_size: pagination.page_size,
      status: (filters.status as '' | 'active' | 'expired' | 'revoked') || undefined,
      group_id: filters.group_id ? Number(filters.group_id) : undefined,
      platform: (filters.platform as GroupPlatform | '') || undefined,
      sort_by: sortState.sort_by,
      sort_order: sortState.sort_order,
    })
    subscriptions.value = data.items || []
    pagination.total = data.total || 0
  } finally {
    loading.value = false
  }
}

async function loadGroups() {
  groups.value = await quickMonitorAPI.groups.getAll(suffix.value)
}

function applyFilters() {
  pagination.page = 1
  void loadSubscriptions()
}

function handlePageChange(page: number) {
  pagination.page = page
  void loadSubscriptions()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  void loadSubscriptions()
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  void loadSubscriptions()
}

function getDaysRemaining(expiresAt: string): number | null {
  const diff = new Date(expiresAt).getTime() - Date.now()
  if (diff < 0) return null
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
}

function isExpiringSoon(expiresAt: string): boolean {
  const days = getDaysRemaining(expiresAt)
  return days !== null && days <= 7
}

function getProgressWidth(used: number | null | undefined, limit: number | null): string {
  if (!limit || limit === 0) return '0%'
  return `${Math.min(((used ?? 0) / limit) * 100, 100)}%`
}

function getProgressClass(used: number | null | undefined, limit: number | null): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = ((used ?? 0) / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function formatResetTime(windowStart: string | null, period: 'daily' | 'weekly' | 'monthly'): string {
  if (!windowStart) return t('admin.subscriptions.windowNotActive')
  const start = new Date(windowStart)
  const now = new Date()
  const resetTime = new Date(start)
  if (period === 'daily') resetTime.setDate(resetTime.getDate() + 1)
  if (period === 'weekly') resetTime.setDate(resetTime.getDate() + 7)
  if (period === 'monthly') resetTime.setMonth(resetTime.getMonth() + 1)
  if (resetTime <= now) return t('admin.subscriptions.resetNow')
  const diffMs = resetTime.getTime() - now.getTime()
  const hours = Math.floor(diffMs / (1000 * 60 * 60))
  const minutes = Math.ceil((diffMs % (1000 * 60 * 60)) / (1000 * 60))
  if (hours >= 24) {
    return t('admin.subscriptions.resetInDaysHours', {
      days: Math.floor(hours / 24),
      hours: hours % 24,
    })
  }
  if (hours > 0) {
    return t('admin.subscriptions.resetInHoursMinutes', { hours, minutes })
  }
  return t('admin.subscriptions.resetInMinutes', { minutes })
}

const UsageProgressRow = defineComponent({
  name: 'UsageProgressRow',
  props: {
    label: { type: String, required: true },
    used: { type: Number, default: 0 },
    limit: { type: Number, required: true },
    hint: { type: String, default: '' },
  },
  setup(props) {
    return () =>
      h('div', { class: 'usage-row' }, [
        h('div', { class: 'flex items-center gap-2' }, [
          h('span', { class: 'usage-label' }, props.label),
          h('div', { class: 'h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600' }, [
            h('div', {
              class: ['h-1.5 rounded-full transition-all', getProgressClass(props.used, props.limit)],
              style: { width: getProgressWidth(props.used, props.limit) },
            }),
          ]),
          h('span', { class: 'usage-amount' }, [
            `$${Number(props.used || 0).toFixed(2)}`,
            h('span', { class: 'text-gray-400' }, ' / '),
            `$${Number(props.limit || 0).toFixed(2)}`,
          ]),
        ]),
        props.hint
          ? h('div', { class: 'reset-info' }, [
              h('span', props.hint),
            ])
          : null,
      ])
  },
})

onMounted(async () => {
  await Promise.all([loadGroups(), loadSubscriptions()])
})
</script>

<style scoped>
.usage-row {
  @apply space-y-1;
}

.usage-label {
  @apply w-10 flex-shrink-0 text-xs font-medium text-gray-500 dark:text-gray-400;
}

.usage-amount {
  @apply w-28 flex-shrink-0 text-right text-xs font-mono text-gray-700 dark:text-gray-300;
}

.reset-info {
  @apply ml-12 text-xs text-gray-400 dark:text-gray-500;
}
</style>
