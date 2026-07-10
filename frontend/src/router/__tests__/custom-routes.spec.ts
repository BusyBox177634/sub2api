import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: false,
  isAdmin: false,
  isSimpleMode: false,
  hasPendingAuthSession: false,
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: true,
  cachedPublicSettings: null as null | Record<string, unknown>,
  publicSettingsLoaded: true,
  fetchPublicSettings: vi.fn(),
}))

const quickMonitorStatus = vi.hoisted(() => vi.fn())

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    customMenuItems: [],
  }),
}))

vi.mock('@/stores/adminCompliance', () => ({
  useAdminComplianceStore: () => ({
    initialized: true,
    fetchStatus: vi.fn(),
    requireAcknowledgement: vi.fn(),
  }),
}))

vi.mock('@/api/quickMonitor', () => ({
  quickMonitorAPI: {
    status: quickMonitorStatus,
  },
}))

vi.mock('@/api/setup', () => ({
  getSetupStatus: vi.fn(),
}))

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  }),
}))

describe('custom README_USER_CUSTOM routes', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'scrollTo', { value: vi.fn(), writable: true })
    authStore.checkAuth.mockReset()
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    authStore.isSimpleMode = false
    authStore.hasPendingAuthSession = false
    appStore.backendModeEnabled = true
    appStore.cachedPublicSettings = null
    appStore.publicSettingsLoaded = true
    appStore.fetchPublicSettings.mockReset()
    quickMonitorStatus.mockReset()
  })

  afterEach(() => {
    vi.resetModules()
  })

  it('registers the admin usage brief route', async () => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find((record) => record.name === 'AdminUsageBrief')

    expect(route?.path).toBe('/admin/usage-brief')
    expect(route?.meta.requiresAdmin).toBe(true)
    expect(route?.meta.requiresUsageBrief).toBe(true)
  })

  it('allows valid quick-monitor routes through status validation before backend-mode auth blocking', async () => {
    quickMonitorStatus.mockResolvedValueOnce({ enabled: true })
    const { default: router } = await import('@/router')

    await router.push('/opsview/dashboard')
    await router.isReady()

    expect(quickMonitorStatus).toHaveBeenCalledWith('opsview')
    expect(router.currentRoute.value.name).toBe('QuickMonitorDashboard')
    expect(router.currentRoute.value.fullPath).toBe('/opsview/dashboard')
  })

  it('redirects invalid or disabled quick-monitor suffixes to 404', async () => {
    quickMonitorStatus.mockResolvedValueOnce({ enabled: false })
    const { default: router } = await import('@/router')

    await router.push('/opsview/dashboard')
    await router.isReady()

    expect(quickMonitorStatus).toHaveBeenCalledWith('opsview')
    expect(router.currentRoute.value.fullPath).toBe('/404')
  })

  it('rejects reserved quick-monitor suffixes without calling status', async () => {
    const { default: router } = await import('@/router')

    await router.push('/admin/dashboard')
    await router.isReady()

    expect(quickMonitorStatus).not.toHaveBeenCalled()
    expect(router.currentRoute.value.fullPath).toBe('/login?redirect=/admin/dashboard')
  })
})
