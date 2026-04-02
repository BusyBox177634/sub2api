<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.editProfile') }}
      </h2>
    </div>
    <div class="space-y-4 px-6 py-6">
      <div>
        <label class="input-label">
          {{ t('profile.username') }}
        </label>
        <div class="rounded-xl border border-gray-200 bg-gray-50 px-4 py-3 text-sm font-medium text-gray-900 dark:border-dark-700 dark:bg-dark-800 dark:text-white">
          {{ displayUsername }}
        </div>
      </div>
      <p class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.usernameManagedByAdmin') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  initialUsername: string
}>()

const { t } = useI18n()
const displayUsername = computed(() => props.initialUsername || '-')
</script>
