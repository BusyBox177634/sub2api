<template>
  <div class="min-h-screen w-full max-w-full overflow-x-hidden bg-gray-50 dark:bg-dark-950">
    <div class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>
    <aside
      class="fixed inset-y-0 left-0 z-40 hidden border-r border-gray-200 bg-white/90 backdrop-blur transition-all duration-200 dark:border-dark-700 dark:bg-dark-900/90 lg:flex lg:flex-col"
      :class="sidebarCollapsed ? 'w-[72px]' : 'w-64'"
    >
      <div
        class="flex h-16 items-center border-b border-gray-100 dark:border-dark-700"
        :class="sidebarCollapsed ? 'justify-center px-3' : 'gap-3 px-5'"
      >
        <div class="flex h-9 w-9 items-center justify-center overflow-hidden rounded-lg bg-primary-100 dark:bg-primary-900/30">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
        </div>
        <div v-if="!sidebarCollapsed" class="min-w-0">
          <div class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ siteName }}</div>
          <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('quickMonitor.subtitle') }}</div>
        </div>
      </div>

      <nav class="flex-1 space-y-1 overflow-y-auto p-3">
        <router-link
          v-for="item in navItems"
          :key="item.path"
          :to="item.path"
          class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          :class="[
            route.path === item.path ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : '',
            sidebarCollapsed ? 'justify-center px-2' : '',
          ]"
          :title="sidebarCollapsed ? item.label : undefined"
        >
          <Icon :name="item.icon" size="sm" />
          <span v-if="!sidebarCollapsed" class="truncate">{{ item.label }}</span>
        </router-link>
      </nav>

      <div class="border-t border-gray-100 p-3 dark:border-dark-800">
        <button
          type="button"
          class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          :class="sidebarCollapsed ? 'justify-center px-2' : ''"
          :title="isDark ? t('nav.lightMode') : t('nav.darkMode')"
          @click="toggleTheme"
        >
          <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
          <span v-if="!sidebarCollapsed">{{ isDark ? t('nav.lightMode') : t('nav.darkMode') }}</span>
        </button>
        <button
          type="button"
          class="mt-1 flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          :class="sidebarCollapsed ? 'justify-center px-2' : ''"
          :title="sidebarCollapsed ? t('nav.expand') : t('nav.collapse')"
          @click="toggleSidebar"
        >
          <Icon :name="sidebarCollapsed ? 'chevronRight' : 'chevronLeft'" size="sm" />
          <span v-if="!sidebarCollapsed">{{ t('nav.collapse') }}</span>
        </button>
      </div>
    </aside>

    <div
      class="relative flex min-h-screen min-w-0 max-w-full flex-col overflow-x-hidden transition-[margin] duration-200"
      :class="sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64'"
    >
      <header class="sticky top-0 z-30 border-b border-gray-200/60 bg-white/90 backdrop-blur dark:border-dark-700 dark:bg-dark-900/90">
        <div class="flex h-14 min-w-0 items-center justify-between gap-3 px-3 sm:px-4 md:px-6">
          <div class="min-w-0">
            <h1 class="text-base font-semibold text-gray-900 dark:text-white">{{ pageTitle }}</h1>
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('quickMonitor.readOnly') }}</p>
          </div>
          <div class="flex shrink-0 items-center gap-3">
            <LocaleSwitcher />
            <div class="hidden text-xs text-gray-500 dark:text-dark-400 sm:block" :title="t('quickMonitor.suffix')">
              {{ suffix }}
            </div>
          </div>
        </div>
        <nav class="flex gap-2 overflow-x-auto border-t border-gray-100 px-4 py-2 dark:border-dark-700 lg:hidden">
          <router-link
            v-for="item in navItems"
            :key="`mobile-${item.path}`"
            :to="item.path"
            class="inline-flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
            :class="route.path === item.path ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : ''"
          >
            <Icon :name="item.icon" size="xs" />
            <span>{{ item.label }}</span>
          </router-link>
        </nav>
      </header>
      <main class="min-h-0 min-w-0 max-w-full flex-1 overflow-x-hidden p-3 sm:p-4 md:p-6 lg:p-8">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import { useAppStore } from '@/stores/app'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const suffix = computed(() => String(route.params.quickSuffix || ''))
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isDark = ref(document.documentElement.classList.contains('dark'))

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.siteLogo || '')
const pageTitle = computed(() => {
  const titleKey = route.meta.titleKey
  if (typeof titleKey === 'string' && titleKey) {
    return t(titleKey)
  }
  return String(route.meta.title || t('quickMonitor.title'))
})

const navItems = computed(() => {
  const base = `/${suffix.value}`
  return [
    { path: `${base}/dashboard`, label: t('quickMonitor.nav.dashboard'), icon: 'chart' as const },
    { path: `${base}/ops`, label: t('quickMonitor.nav.ops'), icon: 'bolt' as const },
    { path: `${base}/users`, label: t('quickMonitor.nav.users'), icon: 'users' as const },
    { path: `${base}/groups`, label: t('quickMonitor.nav.groups'), icon: 'grid' as const },
    { path: `${base}/subscriptions`, label: t('quickMonitor.nav.subscriptions'), icon: 'creditCard' as const },
    { path: `${base}/usage`, label: t('quickMonitor.nav.usage'), icon: 'clipboard' as const },
    { path: `${base}/usage-brief`, label: t('quickMonitor.nav.usageBrief'), icon: 'document' as const },
  ]
})

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function toggleSidebar() {
  appStore.toggleSidebar()
}

onMounted(() => {
  void appStore.fetchPublicSettings()
})
</script>
