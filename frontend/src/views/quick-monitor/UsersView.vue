<template>
  <QuickMonitorLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex min-w-0 max-w-full flex-wrap items-center gap-3">
          <div class="flex min-w-0 flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full md:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.users.searchUsers')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>

            <div class="w-full sm:w-32">
              <Select
                v-model="filters.role"
                :options="roleOptions"
                @change="applyFilter"
              />
            </div>

            <div class="w-full sm:w-32">
              <Select
                v-model="filters.status"
                :options="statusOptions"
                @change="applyFilter"
              />
            </div>
          </div>

          <div class="flex w-full flex-wrap items-center justify-end gap-2 sm:w-auto">
            <button
              type="button"
              class="btn btn-secondary w-full px-2 sm:w-auto md:px-3"
              :disabled="loading"
              :title="t('common.refresh')"
              @click="loadUsers"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="users"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-email="{ value }">
            <div class="flex min-w-0 items-center gap-2">
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
                <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
                  {{ String(value || '?').charAt(0).toUpperCase() }}
                </span>
              </div>
              <span class="min-w-0 break-all font-medium text-gray-900 dark:text-white">{{ value }}</span>
            </div>
          </template>

          <template #cell-username="{ value }">
            <span class="break-words text-sm text-gray-700 dark:text-gray-300">{{ value || '-' }}</span>
          </template>

          <template #cell-role="{ value }">
            <span :class="['badge', value === 'admin' ? 'badge-purple' : 'badge-gray']">
              {{ t(`admin.users.roles.${value}`) }}
            </span>
          </template>

          <template #cell-subscriptions="{ row }">
            <div v-if="row.subscriptions && row.subscriptions.length > 0" class="flex min-w-0 max-w-full flex-wrap justify-start gap-1.5 sm:justify-end">
              <GroupBadge
                v-for="sub in row.subscriptions"
                :key="sub.id"
                class="min-w-0 max-w-full"
                :name="sub.group?.name || ''"
                :platform="sub.group?.platform"
                :subscription-type="sub.group?.subscription_type"
                :rate-multiplier="sub.group?.rate_multiplier"
                :days-remaining="sub.expires_at ? getDaysRemaining(sub.expires_at) : null"
                :title="sub.expires_at ? formatDateTime(sub.expires_at) : ''"
              />
            </div>
            <span
              v-else
              class="inline-flex max-w-full items-center gap-1.5 rounded-md bg-gray-50 px-2 py-1 text-xs text-gray-400 dark:bg-dark-700/50 dark:text-dark-500"
            >
              <Icon name="ban" size="xs" class="h-3.5 w-3.5" />
              <span class="break-words">{{ t('admin.users.noSubscription') }}</span>
            </span>
          </template>

          <template #cell-balance="{ value }">
            <span class="font-medium text-gray-900 dark:text-white">${{ Number(value || 0).toFixed(2) }}</span>
          </template>

          <template #cell-usage="{ row }">
            <div class="min-w-0 max-w-full overflow-hidden">
              <PlatformUsageBreakdown
                :today="usageStats[row.id]?.today_actual_cost ?? 0"
                :total="usageStats[row.id]?.total_actual_cost ?? 0"
                :by-platform="usageStats[row.id]?.by_platform"
              />
            </div>
          </template>

          <template #cell-concurrency="{ row }">
            <UserConcurrencyCell
              :current="row.current_concurrency ?? 0"
              :max="row.concurrency"
            />
          </template>

          <template #cell-status="{ value }">
            <div class="flex min-w-0 items-center gap-1.5">
              <span
                :class="[
                  'inline-block h-2 w-2 rounded-full',
                  value === 'active' ? 'bg-green-500' : 'bg-red-500',
                ]"
              ></span>
              <span class="break-words text-sm text-gray-700 dark:text-gray-300">
                {{ value === 'active' ? t('common.active') : t('admin.users.disabled') }}
              </span>
            </div>
          </template>

          <template #cell-last_active_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ value ? formatDateTime(value) : '-' }}</span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.users.noUsersYet')"
              :description="t('quickMonitor.noFilteredUsers')"
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
import { quickMonitorAPI } from '@/api/quickMonitor'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import QuickMonitorLayout from '@/components/layout/QuickMonitorLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import PlatformUsageBreakdown from '@/components/user/PlatformUsageBreakdown.vue'
import UserConcurrencyCell from '@/components/user/UserConcurrencyCell.vue'
import { formatDateTime } from '@/utils/format'
import type { AdminUser } from '@/types'
import type { BatchUserUsageStats } from '@/api/admin/dashboard'
import type { Column } from '@/components/common/types'

const { t } = useI18n()
const route = useRoute()
const suffix = computed(() => String(route.params.quickSuffix || ''))

const users = ref<AdminUser[]>([])
const usageStats = ref<Record<string, BatchUserUsageStats>>({})
const loading = ref(false)
const searchQuery = ref('')

const filters = reactive({
  role: '' as '' | 'admin' | 'user',
  status: '' as '' | 'active' | 'disabled',
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
  { key: 'email', label: t('admin.users.columns.user'), sortable: true },
  { key: 'id', label: t('admin.users.columns.id'), sortable: true },
  { key: 'username', label: t('admin.users.columns.username'), sortable: true },
  { key: 'role', label: t('admin.users.columns.role'), sortable: true },
  { key: 'subscriptions', label: t('admin.users.columns.subscriptions'), sortable: false },
  { key: 'balance', label: t('admin.users.columns.balance'), sortable: true },
  { key: 'usage', label: t('admin.users.columns.usage'), sortable: false },
  { key: 'concurrency', label: t('admin.users.columns.concurrency'), sortable: true },
  { key: 'status', label: t('admin.users.columns.status'), sortable: true },
  { key: 'last_active_at', label: t('admin.users.columns.lastActive'), sortable: true },
  { key: 'created_at', label: t('admin.users.columns.created'), sortable: true },
])

const roleOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.users.allRoles') },
  { value: 'admin', label: t('admin.users.admin') },
  { value: 'user', label: t('admin.users.user') },
])

const statusOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.users.allStatus') },
  { value: 'active', label: t('common.active') },
  { value: 'disabled', label: t('admin.users.disabled') },
])

async function loadUsers() {
  loading.value = true
  try {
    const data = await quickMonitorAPI.users.list(suffix.value, {
      page: pagination.page,
      page_size: pagination.page_size,
      search: searchQuery.value || undefined,
      role: filters.role || undefined,
      status: filters.status || undefined,
      sort_by: sortState.sort_by,
      sort_order: sortState.sort_order,
    })
    users.value = data.items || []
    pagination.total = data.total || 0
    await loadUsageStats(users.value.map((user) => user.id))
  } finally {
    loading.value = false
  }
}

async function loadUsageStats(userIds: number[]) {
  if (userIds.length === 0) {
    usageStats.value = {}
    return
  }
  try {
    const response = await quickMonitorAPI.dashboard.getBatchUsersUsage(suffix.value, userIds)
    usageStats.value = response.stats || {}
  } catch {
    usageStats.value = {}
  }
}

function applyFilter() {
  pagination.page = 1
  void loadUsers()
}

let searchTimer: ReturnType<typeof setTimeout> | undefined
function handleSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(applyFilter, 300)
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  void loadUsers()
}

function handlePageChange(page: number) {
  pagination.page = page
  void loadUsers()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  void loadUsers()
}

function getDaysRemaining(expiresAt: string): number | null {
  const diff = new Date(expiresAt).getTime() - Date.now()
  if (diff < 0) return null
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
}

onMounted(loadUsers)
</script>
