<template>
  <QuickMonitorLayout>
    <div class="space-y-6">
      <section class="rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
        <div class="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h1 class="text-xl font-black text-gray-900 dark:text-white">{{ t('quickMonitor.ops.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
              {{ overview ? t('quickMonitor.dateRange', { start: formatDate(overview.start_time), end: formatDate(overview.end_time) }) : t('quickMonitor.ops.waitingData') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-3">
            <Select v-model="timeRange" :options="timeRangeOptions" class="w-40" @change="loadAll" />
            <Select v-model="platform" :options="platformOptions" class="w-36" @change="onPlatformChange" />
            <Select v-model="groupId" :options="groupOptions" searchable class="w-48" @change="loadAll" />
            <Select v-model="queryMode" :options="queryModeOptions" class="w-36" @change="loadAll" />
            <button class="btn btn-secondary" type="button" :disabled="loading" @click="loadAll">
              {{ t('common.refresh') }}
            </button>
          </div>
        </div>
      </section>

      <section class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <div v-for="card in metricCards" :key="card.label" class="card p-5">
          <div class="text-sm text-gray-500 dark:text-dark-400">{{ card.label }}</div>
          <div class="mt-2 text-2xl font-semibold" :class="card.className || 'text-gray-900 dark:text-white'">
            {{ card.value }}
          </div>
          <div v-if="card.hint" class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ card.hint }}</div>
        </div>
      </section>

      <section class="grid gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)]">
        <div class="card p-5">
          <div class="flex items-center justify-between gap-3">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('quickMonitor.ops.systemResources') }}</h2>
            <span class="text-xs text-gray-500 dark:text-dark-400">
              {{ systemMetrics?.created_at ? formatDate(systemMetrics.created_at) : '-' }}
            </span>
          </div>
          <div v-if="systemMetrics" class="mt-4 grid gap-3 sm:grid-cols-2">
            <ResourceTile label="CPU" :value="formatPercent(systemMetrics.cpu_usage_percent)" :tone="metricTone(systemMetrics.cpu_usage_percent, 80, 95)" />
            <ResourceTile :label="t('quickMonitor.ops.metrics.memory')" :value="formatPercent(systemMetrics.memory_usage_percent)" :hint="`${formatMB(systemMetrics.memory_used_mb)} / ${formatMB(systemMetrics.memory_total_mb)}`" :tone="metricTone(systemMetrics.memory_usage_percent, 85, 95)" />
            <ResourceTile :label="t('quickMonitor.ops.metrics.database')" :value="systemMetrics.db_ok === false ? t('quickMonitor.ops.status.abnormal') : t('quickMonitor.ops.status.normal')" :hint="dbHint" :tone="systemMetrics.db_ok === false ? 'danger' : 'success'" />
            <ResourceTile label="Redis" :value="systemMetrics.redis_ok === false ? t('quickMonitor.ops.status.abnormal') : t('quickMonitor.ops.status.normal')" :hint="redisHint" :tone="systemMetrics.redis_ok === false ? 'danger' : 'success'" />
            <ResourceTile label="Goroutine" :value="formatInteger(systemMetrics.goroutine_count)" :hint="t('quickMonitor.ops.metrics.queueDepth', { value: formatInteger(systemMetrics.concurrency_queue_depth) })" />
            <ResourceTile :label="t('quickMonitor.ops.metrics.accountSwitch')" :value="formatInteger(systemMetrics.account_switch_count)" />
          </div>
          <p v-else class="mt-4 text-sm text-gray-500 dark:text-dark-400">{{ t('quickMonitor.ops.noSystemSnapshot') }}</p>
        </div>

        <div class="card p-5">
          <div class="flex items-center justify-between gap-3">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('quickMonitor.ops.usageDetailRetention') }}</h2>
            <span :class="['text-sm font-semibold', retentionTone]">{{ retentionStatusLabel }}</span>
          </div>
          <div v-if="retention" class="mt-4 space-y-3">
            <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
              <div class="h-full rounded-full bg-primary-500 transition-all" :style="{ width: `${retentionProgress}%` }"></div>
            </div>
            <div class="grid grid-cols-2 gap-2 text-xs text-gray-600 dark:text-dark-300">
              <InfoLine :label="t('quickMonitor.ops.retention.progress')" :value="formatPercent(retention.progress_percent)" />
              <InfoLine :label="t('quickMonitor.ops.retention.pending')" :value="`${formatInteger(retention.remaining_pending)} / ${formatInteger(retention.total_pending_at_start)}`" />
              <InfoLine :label="t('quickMonitor.ops.retention.processed')" :value="formatInteger(retention.processed)" />
              <InfoLine :label="t('quickMonitor.ops.retention.cleaned')" :value="formatInteger(retention.cleaned)" />
              <InfoLine :label="t('quickMonitor.ops.retention.compressedRequest')" :value="formatInteger(retention.compressed_request)" />
              <InfoLine :label="t('quickMonitor.ops.retention.compressedResponse')" :value="formatInteger(retention.compressed_response)" />
              <InfoLine :label="t('quickMonitor.ops.retention.fallbackSkipped')" :value="`${formatInteger(retention.fallback_empty)} / ${formatInteger(retention.skipped)}`" />
              <InfoLine :label="t('quickMonitor.ops.retention.failed')" :value="formatInteger(retention.failed)" />
              <InfoLine :label="t('quickMonitor.ops.retention.startedAt')" :value="formatDate(retention.started_at)" />
              <InfoLine :label="t('quickMonitor.ops.retention.finishedAt')" :value="formatDate(retention.finished_at)" />
              <InfoLine :label="t('quickMonitor.ops.retention.nextRun')" :value="formatDate(retention.next_run_at)" />
              <InfoLine :label="t('quickMonitor.ops.retention.window')" :value="retentionWindow" />
            </div>
            <p v-if="retention.last_error" class="rounded-lg bg-red-50 p-3 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-300">
              {{ retention.last_error }}
            </p>
          </div>
          <p v-else class="mt-4 text-sm text-gray-500 dark:text-dark-400">{{ t('quickMonitor.ops.noRetentionStatus') }}</p>
        </div>
      </section>

      <section class="card overflow-hidden">
        <div class="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('quickMonitor.ops.diskMountDetails') }}</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('quickMonitor.ops.diskMountHint') }}</p>
          </div>
          <span v-if="primaryDisk" class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ primaryDisk.mount_point }} · {{ formatPercent(primaryDisk.usage_percent) }}
          </span>
        </div>
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="table-th">{{ t('quickMonitor.ops.disk.mountPoint') }}</th>
                <th class="table-th">{{ t('quickMonitor.ops.disk.role') }}</th>
                <th class="table-th">{{ t('quickMonitor.ops.disk.device') }}</th>
                <th class="table-th">{{ t('quickMonitor.ops.disk.filesystem') }}</th>
                <th class="table-th">{{ t('quickMonitor.ops.disk.used') }}</th>
                <th class="table-th">{{ t('quickMonitor.ops.disk.free') }}</th>
                <th class="table-th">{{ t('quickMonitor.ops.disk.total') }}</th>
                <th class="table-th">{{ t('quickMonitor.ops.disk.usage') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
              <tr v-if="diskMounts.length === 0">
                <td colspan="8" class="table-empty">{{ t('quickMonitor.ops.noDiskMounts') }}</td>
              </tr>
              <tr v-for="mount in diskMounts" :key="`${mount.mount_point}-${mount.device}`" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                <td class="table-td font-mono">{{ mount.mount_point }}</td>
                <td class="table-td">{{ diskRoleLabel(mount) }}</td>
                <td class="table-td max-w-xs truncate">{{ mount.device }}</td>
                <td class="table-td">{{ mount.fstype }}</td>
                <td class="table-td">{{ formatMB(mount.used_mb) }}</td>
                <td class="table-td">{{ formatMB(mount.free_mb) }}</td>
                <td class="table-td">{{ formatMB(mount.total_mb) }}</td>
                <td class="table-td">{{ formatPercent(mount.usage_percent) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

    </div>
  </QuickMonitorLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import Select from '@/components/common/Select.vue'
import QuickMonitorLayout from '@/components/layout/QuickMonitorLayout.vue'
import { quickMonitorAPI } from '@/api/quickMonitor'
import type { AdminGroup } from '@/types'
import type { OpsDashboardOverview, OpsDiskMountMetric, OpsQueryMode } from '@/api/admin/ops'

const { t } = useI18n()
const route = useRoute()
const suffix = computed(() => String(route.params.quickSuffix || ''))

const timeRange = ref<'5m' | '30m' | '1h' | '6h' | '24h'>('1h')
const platform = ref('')
const groupId = ref<number | null>(null)
const queryMode = ref<OpsQueryMode>('auto')
const groups = ref<AdminGroup[]>([])
const overview = ref<OpsDashboardOverview | null>(null)
const loading = ref(false)

const timeRangeOptions = computed(() => [
  { value: '5m', label: t('quickMonitor.ops.ranges.fiveMinutes') },
  { value: '30m', label: t('quickMonitor.ops.ranges.thirtyMinutes') },
  { value: '1h', label: t('quickMonitor.ops.ranges.oneHour') },
  { value: '6h', label: t('quickMonitor.ops.ranges.sixHours') },
  { value: '24h', label: t('quickMonitor.ops.ranges.twentyFourHours') },
])

const platformOptions = computed(() => [
  { value: '', label: t('quickMonitor.ops.filters.allPlatforms') },
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' },
])

const queryModeOptions = computed(() => [
  { value: 'auto', label: t('quickMonitor.ops.filters.auto') },
  { value: 'raw', label: t('quickMonitor.ops.filters.raw') },
  { value: 'preagg', label: t('quickMonitor.ops.filters.preagg') },
])

const groupOptions = computed(() => [
  { value: null, label: t('quickMonitor.ops.filters.allGroups') },
  ...groups.value
    .filter((group) => !platform.value || group.platform === platform.value)
    .map((group) => ({ value: group.id, label: group.name })),
])

const systemMetrics = computed(() => overview.value?.system_metrics || null)
const retention = computed(() => overview.value?.usage_detail_retention || null)

const diskMounts = computed<OpsDiskMountMetric[]>(() => {
  const mounts = systemMetrics.value?.disk_mounts || []
  return mounts.filter(isValidDiskMount)
})

const primaryDisk = computed(() => {
  return diskMounts.value.find((mount) => mount.role === 'data_dir' || mount.mount_point === '/app/data') || diskMounts.value[0] || null
})

const dbHint = computed(() => {
  const metrics = systemMetrics.value
  if (!metrics) return ''
  const active = formatInteger(metrics.db_conn_active)
  const idle = formatInteger(metrics.db_conn_idle)
  const max = formatInteger(metrics.db_max_open_conns)
  return `active ${active} / idle ${idle} / max ${max}`
})

const redisHint = computed(() => {
  const metrics = systemMetrics.value
  if (!metrics) return ''
  const total = formatInteger(metrics.redis_conn_total)
  const idle = formatInteger(metrics.redis_conn_idle)
  const pool = formatInteger(metrics.redis_pool_size)
  return `total ${total} / idle ${idle} / pool ${pool}`
})

const retentionProgress = computed(() => clampPercent(retention.value?.progress_percent || 0))
const retentionStatusLabel = computed(() => {
  const status = retention.value
  if (!status) return t('quickMonitor.ops.status.noData')
  if (!status.enabled || status.phase === 'stopped') return t('quickMonitor.ops.status.stopped')
  if (status.running) return t('quickMonitor.ops.status.running')
  if (status.remaining_pending > 0) return t('quickMonitor.ops.status.pending')
  return t('quickMonitor.ops.status.idle')
})
const retentionTone = computed(() => {
  const status = retention.value
  if (!status) return 'text-gray-500'
  if (status.failed > 0 || status.last_error) return 'text-red-600 dark:text-red-400'
  if (status.running) return 'text-blue-600 dark:text-blue-400'
  if (status.remaining_pending > 0) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-emerald-600 dark:text-emerald-400'
})
const retentionWindow = computed(() => {
  const status = retention.value
  if (!status?.window_start || !status?.window_end) return '-'
  return `${formatDate(status.window_start)} - ${formatDate(status.window_end)}`
})

const metricCards = computed(() => {
  const data = overview.value
  return [
    {
      label: t('quickMonitor.ops.metrics.healthScore'),
      value: data?.health_score !== undefined ? String(Math.round(data.health_score)) : '-',
      hint: data?.qps ? `QPS ${formatNumber(data.qps.current)} / TPS ${formatNumber(data.tps?.current)}` : t('quickMonitor.ops.metrics.readonlyOverview'),
      className: healthScoreClass(data?.health_score),
    },
    {
      label: t('quickMonitor.ops.metrics.requestCount'),
      value: formatInteger(data?.request_count_total),
      hint: t('quickMonitor.ops.metrics.successError', {
        success: formatInteger(data?.success_count),
        error: formatInteger(data?.error_count_total),
      }),
    },
    {
      label: t('quickMonitor.ops.metrics.errorRate'),
      value: formatPercent((data?.error_rate || 0) * 100),
      hint: t('quickMonitor.ops.metrics.upstreamSla', {
        upstream: formatPercent((data?.upstream_error_rate || 0) * 100),
        sla: formatPercent((data?.sla || 0) * 100),
      }),
      className: metricToneClass((data?.error_rate || 0) * 100, 0.5, 3),
    },
    {
      label: t('quickMonitor.ops.metrics.latencyP99'),
      value: data?.duration?.p99_ms != null ? `${Math.round(data.duration.p99_ms)} ms` : '-',
      hint: t('quickMonitor.ops.metrics.latencyHint', {
        p95: data?.duration?.p95_ms != null ? Math.round(data.duration.p95_ms) : '-',
        avg: data?.duration?.avg_ms != null ? Math.round(data.duration.avg_ms) : '-',
      }),
    },
    {
      label: 'TTFT P99',
      value: data?.ttft?.p99_ms != null ? `${Math.round(data.ttft.p99_ms)} ms` : '-',
      hint: `P95 ${data?.ttft?.p95_ms != null ? Math.round(data.ttft.p95_ms) : '-'} ms`,
    },
    {
      label: 'Token',
      value: formatInteger(data?.token_consumed),
      hint: t('quickMonitor.ops.metrics.tpsPeak', { value: formatNumber(data?.tps?.peak) }),
    },
    {
      label: t('quickMonitor.ops.metrics.disk'),
      value: primaryDisk.value ? formatPercent(primaryDisk.value.usage_percent) : '-',
      hint: primaryDisk.value
        ? t('quickMonitor.ops.metrics.diskFree', {
            mount: primaryDisk.value.mount_point,
            free: formatMB(primaryDisk.value.free_mb),
          })
        : t('quickMonitor.ops.status.noData'),
      className: metricToneClass(primaryDisk.value?.usage_percent || 0, 85, 95),
    },
    {
      label: t('quickMonitor.ops.metrics.cleanupTask'),
      value: retentionStatusLabel.value,
      hint: retention.value
        ? t('quickMonitor.ops.metrics.cleanupHint', {
            pending: formatInteger(retention.value.remaining_pending),
            failed: formatInteger(retention.value.failed),
          })
        : t('quickMonitor.ops.noRetentionStatus'),
      className: retentionTone.value,
    },
  ]
})

async function loadAll() {
  await loadOverview()
}

async function loadOverview() {
  loading.value = true
  try {
    overview.value = await quickMonitorAPI.ops.getDashboardOverview(suffix.value, {
      time_range: timeRange.value,
      platform: platform.value || undefined,
      group_id: groupId.value,
      mode: queryMode.value,
    })
  } finally {
    loading.value = false
  }
}

async function loadGroups() {
  groups.value = await quickMonitorAPI.groups.getAll(suffix.value)
}

function onPlatformChange() {
  const current = groups.value.find((group) => group.id === groupId.value)
  if (current && current.platform !== platform.value) groupId.value = null
  void loadAll()
}

function isValidDiskMount(mount: OpsDiskMountMetric): boolean {
  if (!mount?.mount_point) return false
  if (mount.role === 'data_dir' || mount.mount_point.replace(/\/+$/, '') === '/app/data') return true
  const fstype = String(mount.fstype || '').trim().toLowerCase()
  const device = String(mount.device || '').trim().replace(/\/+$/, '')
  if (fstype === 'fakeowner') return false
  return !['/run/host_mark', '/run/desktop/mnt/host', '/host_mnt'].some((prefix) => device === prefix || device.startsWith(`${prefix}/`))
}

function diskRoleLabel(mount: OpsDiskMountMetric): string {
  if (mount.role === 'data_dir' || mount.mount_point.replace(/\/+$/, '') === '/app/data') return t('quickMonitor.ops.disk.dataDir')
  if (mount.role === 'container_root' || mount.mount_point === '/') return t('quickMonitor.ops.disk.containerRoot')
  return t('quickMonitor.ops.disk.mount')
}

function metricTone(value: number | null | undefined, warning: number, danger: number) {
  const n = Number(value)
  if (!Number.isFinite(n)) return 'default'
  if (n >= danger) return 'danger'
  if (n >= warning) return 'warning'
  return 'success'
}

function metricToneClass(value: number | null | undefined, warning: number, danger: number) {
  const tone = metricTone(value, warning, danger)
  if (tone === 'danger') return 'text-red-600 dark:text-red-400'
  if (tone === 'warning') return 'text-yellow-600 dark:text-yellow-400'
  if (tone === 'success') return 'text-emerald-600 dark:text-emerald-400'
  return 'text-gray-900 dark:text-white'
}

function healthScoreClass(value: number | null | undefined) {
  const n = Number(value)
  if (!Number.isFinite(n)) return 'text-gray-900 dark:text-white'
  if (n < 60) return 'text-red-600 dark:text-red-400'
  if (n < 90) return 'text-yellow-600 dark:text-yellow-400'
  return 'text-emerald-600 dark:text-emerald-400'
}

function clampPercent(value: number) {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, value))
}

function formatDate(value?: string | null) {
  return value ? new Date(value).toLocaleString() : '-'
}

function formatInteger(value?: number | null) {
  return Math.round(Number(value || 0)).toLocaleString()
}

function formatNumber(value?: number | null, digits = 2) {
  return Number(value || 0).toLocaleString(undefined, { maximumFractionDigits: digits })
}

function formatPercent(value?: number | null) {
  return `${formatNumber(value || 0, 1)}%`
}

function formatMB(value?: number | null) {
  const bytes = Number(value || 0) * 1024 * 1024
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, index)).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

const ResourceTile = defineComponent({
  name: 'ResourceTile',
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    hint: { type: String, default: '' },
    tone: { type: String, default: 'default' },
  },
  setup(props) {
    const toneClass = () => {
      if (props.tone === 'danger') return 'text-red-600 dark:text-red-400'
      if (props.tone === 'warning') return 'text-yellow-600 dark:text-yellow-400'
      if (props.tone === 'success') return 'text-emerald-600 dark:text-emerald-400'
      return 'text-gray-900 dark:text-white'
    }
    return () =>
      h('div', { class: 'rounded-xl bg-gray-50 p-3 dark:bg-dark-900' }, [
        h('div', { class: 'text-xs text-gray-500 dark:text-dark-400' }, props.label),
        h('div', { class: ['mt-1 text-lg font-semibold', toneClass()] }, props.value),
        props.hint ? h('div', { class: 'mt-1 text-xs text-gray-500 dark:text-dark-400' }, props.hint) : null,
      ])
  },
})

const InfoLine = defineComponent({
  name: 'InfoLine',
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
  },
  setup(props) {
    return () =>
      h('div', { class: 'flex items-center justify-between gap-3 rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-900' }, [
        h('span', props.label),
        h('strong', { class: 'text-right font-semibold text-gray-900 dark:text-white' }, props.value),
      ])
  },
})

onMounted(async () => {
  await Promise.all([loadGroups(), loadAll()])
})
</script>

<style scoped>
.table-th {
  @apply whitespace-nowrap px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400;
}

.table-td {
  @apply px-4 py-3 text-sm text-gray-700 dark:text-dark-200;
}

.table-empty {
  @apply px-4 py-10 text-center text-sm text-gray-500 dark:text-dark-400;
}
</style>
