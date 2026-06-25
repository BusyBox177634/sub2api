<template>
  <QuickMonitorLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.groups.searchGroups')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>

            <Select
              v-model="filters.platform"
              :options="platformFilterOptions"
              :placeholder="t('admin.groups.allPlatforms')"
              class="w-44"
              @change="reload"
            />
            <Select
              v-model="filters.status"
              :options="statusOptions"
              :placeholder="t('admin.groups.allStatus')"
              class="w-40"
              @change="reload"
            />
            <Select
              v-model="filters.is_exclusive"
              :options="exclusiveOptions"
              :placeholder="t('admin.groups.allGroups')"
              class="w-44"
              @change="reload"
            />
          </div>

          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="loading"
              :title="t('common.refresh')"
              @click="loadGroups"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="groups"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="sort_order"
          default-sort-order="asc"
          @sort="handleSort"
        >
          <template #cell-name="{ row, value }">
            <div>
              <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
              <p
                v-if="row.description"
                class="mt-1 max-w-xs truncate text-xs text-gray-500 dark:text-gray-400"
              >
                {{ row.description }}
              </p>
            </div>
          </template>

          <template #cell-platform="{ value }">
            <span
              :class="[
                'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
                value === 'anthropic'
                  ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
                  : value === 'openai'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                    : value === 'antigravity'
                      ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                      : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
              ]"
            >
              <PlatformIcon :platform="value" size="xs" />
              {{ t(`admin.groups.platforms.${value}`) }}
            </span>
          </template>

          <template #cell-billing_type="{ row }">
            <div class="space-y-1">
              <span
                :class="[
                  'inline-block rounded-full px-2 py-0.5 text-xs font-medium',
                  row.subscription_type === 'subscription'
                    ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400'
                    : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
                ]"
              >
                {{
                  row.subscription_type === 'subscription'
                    ? t('admin.groups.subscription.subscription')
                    : t('admin.groups.subscription.standard')
                }}
              </span>
              <div
                v-if="row.subscription_type === 'subscription'"
                class="text-xs text-gray-500 dark:text-gray-400"
              >
                <template v-if="row.daily_limit_usd || row.weekly_limit_usd || row.monthly_limit_usd">
                  <span v-if="row.daily_limit_usd">${{ row.daily_limit_usd }}/{{ t('admin.groups.limitDay') }}</span>
                  <span
                    v-if="row.daily_limit_usd && (row.weekly_limit_usd || row.monthly_limit_usd)"
                    class="mx-1 text-gray-300 dark:text-gray-600"
                  >·</span>
                  <span v-if="row.weekly_limit_usd">${{ row.weekly_limit_usd }}/{{ t('admin.groups.limitWeek') }}</span>
                  <span
                    v-if="row.weekly_limit_usd && row.monthly_limit_usd"
                    class="mx-1 text-gray-300 dark:text-gray-600"
                  >·</span>
                  <span v-if="row.monthly_limit_usd">${{ row.monthly_limit_usd }}/{{ t('admin.groups.limitMonth') }}</span>
                </template>
                <span v-else class="text-gray-400 dark:text-gray-500">
                  {{ t('admin.groups.subscription.noLimit') }}
                </span>
              </div>
            </div>
          </template>

          <template #cell-rate_multiplier="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ value }}x</span>
          </template>

          <template #cell-is_exclusive="{ value }">
            <span :class="['badge', value ? 'badge-primary' : 'badge-gray']">
              {{ value ? t('admin.groups.exclusive') : t('admin.groups.public') }}
            </span>
          </template>

          <template #cell-account_count="{ row }">
            <div class="space-y-0.5 text-xs">
              <div>
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.groups.accountsAvailable') }}</span>
                <span class="ml-1 font-medium text-emerald-600 dark:text-emerald-400">
                  {{ row.active_account_count || 0 }}
                </span>
                <span class="ml-1 inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300">
                  {{ t('admin.groups.accountsUnit') }}
                </span>
              </div>
              <div v-if="row.rate_limited_account_count">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.groups.accountsRateLimited') }}</span>
                <span class="ml-1 font-medium text-amber-600 dark:text-amber-400">
                  {{ row.rate_limited_account_count }}
                </span>
                <span class="ml-1 inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300">
                  {{ t('admin.groups.accountsUnit') }}
                </span>
              </div>
              <div>
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.groups.accountsTotal') }}</span>
                <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">
                  {{ row.account_count || 0 }}
                </span>
                <span class="ml-1 inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300">
                  {{ t('admin.groups.accountsUnit') }}
                </span>
              </div>
            </div>
          </template>

          <template #cell-capacity="{ row }">
            <GroupCapacityBadge
              v-if="capacityMap.get(row.id)"
              :concurrency-used="capacityMap.get(row.id)!.concurrencyUsed"
              :concurrency-max="capacityMap.get(row.id)!.concurrencyMax"
              :sessions-used="capacityMap.get(row.id)!.sessionsUsed"
              :sessions-max="capacityMap.get(row.id)!.sessionsMax"
              :rpm-used="capacityMap.get(row.id)!.rpmUsed"
              :rpm-max="capacityMap.get(row.id)!.rpmMax"
            />
            <span v-else class="text-xs text-gray-400">—</span>
          </template>

          <template #cell-usage="{ row }">
            <div v-if="usageLoading" class="text-xs text-gray-400">—</div>
            <div v-else class="space-y-0.5 text-xs">
              <div class="text-gray-500 dark:text-gray-400">
                <span class="text-gray-400 dark:text-gray-500">{{ t('admin.groups.usageToday') }}</span>
                <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">
                  ${{ formatCost(usageMap.get(row.id)?.today_cost ?? 0) }}
                </span>
              </div>
              <div class="text-gray-500 dark:text-gray-400">
                <span class="text-gray-400 dark:text-gray-500">{{ t('admin.groups.usageTotal') }}</span>
                <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">
                  ${{ formatCost(usageMap.get(row.id)?.total_cost ?? 0) }}
                </span>
              </div>
            </div>
          </template>

          <template #cell-status="{ value }">
            <span :class="['badge', value === 'active' ? 'badge-success' : 'badge-danger']">
              {{ t(`admin.accounts.status.${value}`) }}
            </span>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.groups.noGroupsYet')"
              :description="t('admin.groups.noGroupsDescription')"
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
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import GroupCapacityBadge from '@/components/common/GroupCapacityBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import QuickMonitorLayout from '@/components/layout/QuickMonitorLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import { quickMonitorAPI } from '@/api/quickMonitor'
import type { AdminGroup, GroupPlatform } from '@/types'
import type { Column } from '@/components/common/types'

const { t } = useI18n()
const route = useRoute()
const suffix = computed(() => String(route.params.quickSuffix || ''))

const groups = ref<AdminGroup[]>([])
const loading = ref(false)
const usageLoading = ref(false)
const searchQuery = ref('')
const usageMap = ref(new Map<number, { today_cost: number; total_cost: number }>())
const capacityMap = ref(new Map<number, {
  concurrencyUsed: number
  concurrencyMax: number
  sessionsUsed: number
  sessionsMax: number
  rpmUsed: number
  rpmMax: number
}>())

const filters = reactive<{
  platform: GroupPlatform | ''
  status: 'active' | 'inactive' | ''
  is_exclusive: boolean | ''
  sort_by: string
  sort_order: 'asc' | 'desc'
}>({
  platform: '',
  status: '',
  is_exclusive: '',
  sort_by: 'sort_order',
  sort_order: 'asc',
})

const pagination = reactive({
  page: 1,
  page_size: 50,
  total: 0,
})

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.groups.columns.name'), sortable: true },
  { key: 'platform', label: t('admin.groups.columns.platform') },
  { key: 'billing_type', label: t('admin.groups.columns.billingType') },
  { key: 'rate_multiplier', label: t('admin.groups.columns.rateMultiplier'), sortable: true },
  { key: 'is_exclusive', label: t('admin.groups.columns.type') },
  { key: 'account_count', label: t('admin.groups.columns.accounts') },
  { key: 'capacity', label: t('admin.groups.columns.capacity') },
  { key: 'usage', label: t('admin.groups.columns.usage') },
  { key: 'status', label: t('admin.groups.columns.status') },
])

const platformFilterOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.groups.allPlatforms') },
  { value: 'openai', label: t('admin.groups.platforms.openai') },
  { value: 'anthropic', label: t('admin.groups.platforms.anthropic') },
  { value: 'gemini', label: t('admin.groups.platforms.gemini') },
  { value: 'antigravity', label: t('admin.groups.platforms.antigravity') },
])

const statusOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.groups.allStatus') },
  { value: 'active', label: t('admin.accounts.status.active') },
  { value: 'inactive', label: t('admin.accounts.status.inactive') },
])

const exclusiveOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.groups.allGroups') },
  { value: true, label: t('admin.groups.exclusive') },
  { value: false, label: t('admin.groups.public') },
])

async function loadGroups() {
  loading.value = true
  try {
    const data = await quickMonitorAPI.groups.list(suffix.value, {
      page: pagination.page,
      page_size: pagination.page_size,
      search: searchQuery.value || undefined,
      platform: filters.platform || undefined,
      status: filters.status || undefined,
      is_exclusive: filters.is_exclusive === '' ? undefined : filters.is_exclusive,
      sort_by: filters.sort_by,
      sort_order: filters.sort_order,
    })
    groups.value = data.items || []
    pagination.total = data.total || 0
    void loadUsageSummary()
    void loadCapacitySummary()
  } finally {
    loading.value = false
  }
}

async function loadUsageSummary() {
  usageLoading.value = true
  try {
    const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
    const data = await quickMonitorAPI.groups.getUsageSummary(suffix.value, timezone)
    const next = new Map<number, { today_cost: number; total_cost: number }>()
    data.forEach((item) => {
      next.set(item.group_id, { today_cost: item.today_cost, total_cost: item.total_cost })
    })
    usageMap.value = next
  } finally {
    usageLoading.value = false
  }
}

async function loadCapacitySummary() {
  const data = await quickMonitorAPI.groups.getCapacitySummary(suffix.value)
  const next = new Map<number, {
    concurrencyUsed: number
    concurrencyMax: number
    sessionsUsed: number
    sessionsMax: number
    rpmUsed: number
    rpmMax: number
  }>()
  data.forEach((item) => {
    next.set(item.group_id, {
      concurrencyUsed: item.concurrency_used,
      concurrencyMax: item.concurrency_max,
      sessionsUsed: item.sessions_used,
      sessionsMax: item.sessions_max,
      rpmUsed: item.rpm_used,
      rpmMax: item.rpm_max,
    })
  })
  capacityMap.value = next
}

function reload() {
  pagination.page = 1
  void loadGroups()
}

let searchTimer: ReturnType<typeof setTimeout> | undefined
function handleSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(reload, 300)
}

function handleSort(key: string, order: 'asc' | 'desc') {
  filters.sort_by = key
  filters.sort_order = order
  void loadGroups()
}

function handlePageChange(page: number) {
  pagination.page = page
  void loadGroups()
}

function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  void loadGroups()
}

function formatCost(cost: number) {
  if (cost >= 1000) return cost.toFixed(0)
  if (cost >= 100) return cost.toFixed(1)
  return cost.toFixed(2)
}

onMounted(loadGroups)
</script>
