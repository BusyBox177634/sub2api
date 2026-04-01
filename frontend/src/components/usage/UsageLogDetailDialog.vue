<template>
  <BaseDialog :show="show" :title="t('usage.detailTitle')" width="wide" @close="emit('close')">
    <div class="space-y-4">
      <div v-if="record" class="grid gap-3 rounded-2xl border border-gray-200 bg-gray-50 p-4 text-sm dark:border-dark-700 dark:bg-dark-900/60 md:grid-cols-2">
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.model') }}</div>
          <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ record.model || '-' }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.type') }}</div>
          <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ requestTypeLabel }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.time') }}</div>
          <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ formatDateTime(record.created_at) }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.usage.requestId') }}</div>
          <div class="mt-1 break-all font-mono text-xs text-gray-700 dark:text-gray-300">{{ record.request_id || '-' }}</div>
        </div>
        <div v-if="showAdminMeta">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.usage.user') }}</div>
          <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ adminRecord?.user?.email || '-' }}</div>
        </div>
        <div v-if="showAdminMeta">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.apiKeyFilter') }}</div>
          <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ adminRecord?.api_key?.name || '-' }}</div>
        </div>
        <div v-if="showAdminMeta">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.usage.account') }}</div>
          <div class="mt-1 font-medium text-gray-900 dark:text-white">{{ adminRecord?.account?.name || '-' }}</div>
        </div>
      </div>

      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="rounded-full px-4 py-2 text-sm font-medium transition"
          :class="activeTab === 'messages'
            ? 'bg-primary-600 text-white'
            : 'bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700'"
          @click="activeTab = 'messages'"
        >
          {{ t('usage.detailTitle') }}
        </button>
        <button
          type="button"
          class="rounded-full px-4 py-2 text-sm font-medium transition"
          :class="activeTab === 'request'
            ? 'bg-primary-600 text-white'
            : 'bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700'"
          @click="activeTab = 'request'"
        >
          {{ t('usage.requestJson') }}
        </button>
        <button
          type="button"
          class="rounded-full px-4 py-2 text-sm font-medium transition"
          :class="activeTab === 'response'
            ? 'bg-primary-600 text-white'
            : 'bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700'"
          @click="activeTab = 'response'"
        >
          {{ t('usage.responseJson') }}
        </button>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-14 text-sm text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>

      <div v-else-if="activeTab === 'messages'" class="space-y-5">
        <div v-if="!detail?.available" class="rounded-2xl border border-dashed border-gray-200 px-5 py-10 text-center dark:border-dark-700">
          <div class="text-sm font-medium text-gray-800 dark:text-gray-100">{{ t('usage.unavailable') }}</div>
          <div class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ unavailableMessage }}</div>
        </div>
        <template v-else>
          <section>
            <div class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('usage.sentMessages') }}</div>
            <div v-if="detail.request_messages.length === 0" class="rounded-2xl border border-gray-200 bg-gray-50 px-4 py-5 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900/60 dark:text-gray-400">
              {{ t('usage.noMessages') }}
            </div>
            <div v-else class="space-y-3">
              <article v-for="(message, index) in detail.request_messages" :key="`req-${index}`" class="rounded-2xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/60">
                <div class="mb-2 flex items-center gap-2">
                  <span class="rounded-full bg-primary-100 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-wide text-primary-700 dark:bg-primary-900/30 dark:text-primary-200">
                    {{ message.role }}
                  </span>
                </div>
                <pre class="whitespace-pre-wrap break-words text-sm text-gray-800 dark:text-gray-100">{{ message.text }}</pre>
              </article>
            </div>
          </section>

          <section>
            <div class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('usage.receivedMessages') }}</div>
            <div v-if="detail.response_messages.length === 0" class="rounded-2xl border border-gray-200 bg-gray-50 px-4 py-5 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900/60 dark:text-gray-400">
              {{ t('usage.noMessages') }}
            </div>
            <div v-else class="space-y-3">
              <article v-for="(message, index) in detail.response_messages" :key="`resp-${index}`" class="rounded-2xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
                <div class="mb-2 flex items-center gap-2">
                  <span class="rounded-full bg-emerald-100 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-wide text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-200">
                    {{ message.role }}
                  </span>
                </div>
                <pre class="whitespace-pre-wrap break-words text-sm text-gray-800 dark:text-gray-100">{{ message.text }}</pre>
              </article>
            </div>
          </section>
        </template>
      </div>

      <div v-else class="space-y-3">
        <div class="flex items-center justify-end">
          <button
            v-if="activeJson"
            type="button"
            class="btn btn-secondary btn-sm"
            @click="handleCopyJson(activeJson)"
          >
            {{ t('usage.copyJson') }}
          </button>
        </div>
        <div class="rounded-2xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/60">
          <pre class="max-h-[460px] overflow-auto whitespace-pre-wrap break-words text-xs text-gray-800 dark:text-gray-100">{{ prettyJson(activeJson) }}</pre>
        </div>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminUsageLog, UsageLog, UsageLogDetailResponse } from '@/types'
import { useClipboard } from '@/composables/useClipboard'
import { formatDateTime } from '@/utils/format'
import { resolveUsageRequestType } from '@/utils/usageRequestType'
import BaseDialog from '@/components/common/BaseDialog.vue'

type UsageRecord = UsageLog | AdminUsageLog

const props = withDefaults(defineProps<{
  show: boolean
  loading?: boolean
  detail: UsageLogDetailResponse | null
  record: UsageRecord | null
  admin?: boolean
}>(), {
  loading: false,
  admin: false
})

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const activeTab = ref<'messages' | 'request' | 'response'>('messages')

watch(() => props.show, (show) => {
  if (show) {
    activeTab.value = 'messages'
  }
})

const showAdminMeta = computed(() => props.admin && !!props.record)
const adminRecord = computed(() => props.record as AdminUsageLog | null)

const requestTypeLabel = computed(() => {
  if (!props.record) return t('usage.unknown')
  const requestType = resolveUsageRequestType(props.record)
  if (requestType === 'ws_v2') return t('usage.ws')
  if (requestType === 'stream') return t('usage.stream')
  if (requestType === 'sync') return t('usage.sync')
  return t('usage.unknown')
})

const activeJson = computed(() => {
  if (!props.detail) return ''
  if (activeTab.value === 'request') return props.detail.request_payload_json || ''
  return props.detail.response_payload_json || ''
})

const unavailableMessage = computed(() => {
  switch (props.detail?.reason) {
    case 'disabled':
      return t('usage.unavailableDisabled')
    case 'historical':
      return t('usage.unavailableHistorical')
    case 'not_captured':
      return t('usage.unavailableNotCaptured')
    default:
      return t('usage.unavailable')
  }
})

const prettyJson = (raw?: string) => {
  if (!raw) return '-'
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

const handleCopyJson = async (raw: string) => {
  await copyToClipboard(prettyJson(raw), t('usage.jsonCopied'))
}
</script>
