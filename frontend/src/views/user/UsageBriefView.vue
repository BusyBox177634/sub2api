<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="card p-5">
        <div class="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">启用用量简报</h2>
          </div>
          <div class="flex items-center gap-3">
            <span class="text-sm text-gray-500 dark:text-gray-400">{{ pageEnabledStatusLabel }}</span>
            <Toggle :model-value="pageEnabled" :disabled="!profileLoaded || profileLoading || savingPageEnabled" @update:model-value="togglePageEnabled" />
          </div>
        </div>
      </div>

      <template v-if="profileLoaded && pageEnabled">
      <div class="flex flex-wrap items-end gap-3">
        <div>
          <label class="input-label">类型</label>
          <select v-model="periodType" class="input w-32" @change="loadPeriod">
            <option value="daily">日报</option>
            <option value="weekly">周报</option>
            <option value="monthly">月报</option>
          </select>
        </div>
        <div>
          <label class="input-label">日期</label>
          <input v-model="periodDate" type="date" class="input w-44" @change="loadPeriod" />
        </div>
        <button class="btn btn-primary" :disabled="loading" @click="loadPeriod">刷新</button>
      </div>

      <div class="card p-5">
        <div class="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">自动发送到邮箱</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              开启后，新生成的日报、周报和月报会自动发送到 {{ profileEmail || '当前账号邮箱' }}。
            </p>
          </div>
          <div class="flex items-center gap-3">
            <span class="text-sm text-gray-500 dark:text-gray-400">{{ autoEmailEnabled ? '已开启' : '已关闭' }}</span>
            <Toggle :model-value="autoEmailEnabled" :disabled="profileLoading || savingAutoEmail" @update:model-value="toggleAutoEmail" />
          </div>
        </div>
      </div>

      <div class="card p-5">
        <div v-if="loading" class="py-16 text-center text-sm text-gray-500 dark:text-gray-400">
          加载中...
        </div>
        <div v-else-if="view?.available && view.report" class="space-y-4">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ view.report.title }}</h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ formatDate(view.period_start) }} 至 {{ formatDate(view.period_end) }}
              </p>
            </div>
            <span class="rounded bg-green-100 px-2 py-1 text-xs font-medium text-green-700 dark:bg-green-900/30 dark:text-green-300">
              已生成
            </span>
          </div>
          <MarkdownContent :content="view.report.content_md" />
        </div>
        <div v-else class="py-16 text-center">
          <p class="text-base font-medium text-gray-900 dark:text-white">{{ view?.message || '报告不可用' }}</p>
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ statusHint }}</p>
        </div>
      </div>

      <div class="card overflow-hidden">
        <div class="border-b border-gray-200 px-5 py-3 dark:border-dark-700">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">历史简报</h2>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">标题</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">类型</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">周期</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">状态</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="report in reports" :key="report.id" class="hover:bg-gray-50 dark:hover:bg-dark-800">
                <td class="px-4 py-3">
                  <button class="text-left text-sm font-medium text-primary-600 hover:text-primary-700" @click="openReport(report.id)">
                    {{ report.title }}
                  </button>
                </td>
                <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{{ periodLabel(report.period_type) }}</td>
                <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                  {{ formatDate(report.period_start) }} 至 {{ formatDate(report.period_end) }}
                </td>
                <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-300">{{ statusLabel(report.status) }}</td>
              </tr>
              <tr v-if="!reports.length">
                <td colspan="4" class="px-4 py-8 text-center text-sm text-gray-500">暂无历史简报</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import MarkdownContent from '@/components/common/MarkdownContent.vue'
import Toggle from '@/components/common/Toggle.vue'
import { usageBriefAPI, type UsageBriefPeriodType, type UsageBriefPeriodView, type UsageBriefReport } from '@/api/usageBrief'
import userAPI from '@/api/user'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const periodType = ref<UsageBriefPeriodType>('daily')
const periodDate = ref(new Date().toISOString().slice(0, 10))
const loading = ref(false)
const view = ref<UsageBriefPeriodView | null>(null)
const reports = ref<UsageBriefReport[]>([])
const pageEnabledPreference = ref<boolean | null>(null)
const profileLoaded = ref(false)
const savingPageEnabled = ref(false)
const autoEmailEnabled = ref(false)
const savingAutoEmail = ref(false)
const profileLoading = ref(false)
const profileEmail = ref('')

const pageEnabled = computed(() => pageEnabledPreference.value !== false)
const pageEnabledStatusLabel = computed(() => {
  if (!profileLoaded.value || profileLoading.value) return '加载中'
  return pageEnabled.value ? '已开启' : '已关闭'
})

const statusHint = computed(() => {
  if (view.value?.reason === 'generating') return '后台正在生成，请稍后刷新。'
  if (view.value?.reason === 'current_period_unavailable') return '当前周期结束后才会生成对应简报。'
  if (view.value?.reason === 'not_generated') return '如果这是已结束周期，管理员可以手动触发生成。'
  return ''
})

function formatDate(value?: string) {
  if (!value) return '-'
  return value.slice(0, 10)
}

function periodLabel(value: string) {
  return value === 'weekly' ? '周报' : value === 'monthly' ? '月报' : '日报'
}

function statusLabel(value: string) {
  const labels: Record<string, string> = {
    queued: '排队中',
    running: '生成中',
    succeeded: '已生成',
    failed: '失败',
    canceled: '已取消'
  }
  return labels[value] || value
}

async function loadPeriod() {
  if (!pageEnabled.value) return
  loading.value = true
  try {
    view.value = await usageBriefAPI.getPeriod(periodType.value, periodDate.value)
  } catch (error: any) {
    appStore.showError(error?.message || '加载用量简报失败')
  } finally {
    loading.value = false
  }
}

async function loadReports() {
  if (!pageEnabled.value) return
  try {
    const res = await usageBriefAPI.listReports({ page: 1, page_size: 20, period_type: periodType.value })
    reports.value = res.items || []
  } catch (error) {
    reports.value = []
  }
}

async function loadProfile(): Promise<boolean> {
  profileLoading.value = true
  let shouldLoadBriefs = false
  try {
    const profile = await userAPI.getProfile()
    pageEnabledPreference.value = profile.usage_brief_page_enabled === false ? false : true
    shouldLoadBriefs = pageEnabled.value
    autoEmailEnabled.value = profile.usage_brief_auto_email_enabled === true
    profileEmail.value = profile.email || ''
  } catch (error: any) {
    pageEnabledPreference.value = true
    appStore.showError(error?.message || '加载邮箱发送设置失败')
  } finally {
    profileLoaded.value = true
    profileLoading.value = false
  }
  return shouldLoadBriefs
}

async function togglePageEnabled(value: boolean) {
  if (savingPageEnabled.value) return
  const previousPageEnabledPreference = pageEnabledPreference.value
  const previousAutoEmailEnabled = autoEmailEnabled.value
  pageEnabledPreference.value = value
  savingPageEnabled.value = true
  if (!value) {
    autoEmailEnabled.value = false
    view.value = null
    reports.value = []
  }
  try {
    const updated = await userAPI.updateProfile({ usage_brief_page_enabled: value })
    pageEnabledPreference.value = updated.usage_brief_page_enabled === false ? false : true
    autoEmailEnabled.value = updated.usage_brief_auto_email_enabled === true
    profileEmail.value = updated.email || profileEmail.value
    appStore.showSuccess(value ? '已开启用量简报' : '已关闭用量简报')
    if (pageEnabled.value) {
      await Promise.all([loadPeriod(), loadReports()])
    }
  } catch (error: any) {
    pageEnabledPreference.value = previousPageEnabledPreference
    autoEmailEnabled.value = previousAutoEmailEnabled
    appStore.showError(error?.message || '保存用量简报设置失败')
  } finally {
    savingPageEnabled.value = false
  }
}

async function toggleAutoEmail(value: boolean) {
  if (savingAutoEmail.value) return
  const previous = autoEmailEnabled.value
  autoEmailEnabled.value = value
  savingAutoEmail.value = true
  try {
    const updated = await userAPI.updateProfile({ usage_brief_auto_email_enabled: value })
    autoEmailEnabled.value = updated.usage_brief_auto_email_enabled === true
    profileEmail.value = updated.email || profileEmail.value
    appStore.showSuccess(value ? '已开启自动发送到邮箱' : '已关闭自动发送到邮箱')
  } catch (error: any) {
    autoEmailEnabled.value = previous
    appStore.showError(error?.message || '保存邮箱发送设置失败')
  } finally {
    savingAutoEmail.value = false
  }
}

async function openReport(id: number) {
  if (!pageEnabled.value) return
  loading.value = true
  try {
    const report = await usageBriefAPI.getReport(id)
    view.value = {
      available: report.status === 'succeeded' || report.status === 'partial',
      period_type: report.period_type,
      period_start: report.period_start,
      period_end: report.period_end,
      status: report.status,
      report
    }
  } catch (error: any) {
    appStore.showError(error?.message || '加载报告失败')
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  const shouldLoadBriefs = await loadProfile()
  if (shouldLoadBriefs) {
    await Promise.all([loadPeriod(), loadReports()])
  }
})
</script>
