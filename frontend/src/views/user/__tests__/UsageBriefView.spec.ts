import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { User } from '@/types'
import UsageBriefView from '../UsageBriefView.vue'
import userAPI from '@/api/user'
import { usageBriefAPI } from '@/api/usageBrief'

vi.mock('@/api/user', () => ({
  default: {
    getProfile: vi.fn(),
    updateProfile: vi.fn()
  }
}))

vi.mock('@/api/usageBrief', () => ({
  usageBriefAPI: {
    getPeriod: vi.fn(),
    listReports: vi.fn(),
    getReport: vi.fn()
  }
}))

const userAPIMock = vi.mocked(userAPI)
const usageBriefAPIMock = vi.mocked(usageBriefAPI)

function profile(overrides: Record<string, unknown> = {}): User {
  return {
    id: 1,
    username: 'user',
    email: 'user@example.com',
    role: 'user',
    balance: 0,
    concurrency: 5,
    status: 'active',
    allowed_groups: [],
    balance_notify_enabled: true,
    balance_notify_threshold: null,
    balance_notify_extra_emails: [],
    usage_brief_auto_email_enabled: false,
    usage_brief_page_enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides
  } as User
}

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function mountView() {
  setActivePinia(createPinia())
  return mount(UsageBriefView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        MarkdownContent: { props: ['content'], template: '<article>{{ content }}</article>' },
        Toggle: {
          props: ['modelValue', 'disabled'],
          emits: ['update:modelValue'],
          template: '<button type="button" :disabled="disabled" @click="$emit(\'update:modelValue\', !modelValue)">{{ modelValue ? "on" : "off" }}</button>'
        }
      }
    }
  })
}

describe('UsageBriefView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    usageBriefAPIMock.getPeriod.mockResolvedValue({
      available: false,
      period_type: 'daily',
      period_start: '2026-01-01',
      period_end: '2026-01-01',
      reason: 'not_generated',
      message: '报告不可用'
    } as any)
    usageBriefAPIMock.listReports.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 } as any)
  })

  it('shows a loading switch state and skips brief APIs before profile resolves', async () => {
    const deferred = createDeferred<User>()
    userAPIMock.getProfile.mockReturnValue(deferred.promise)

    const wrapper = mountView()

    expect(wrapper.text()).toContain('启用用量简报')
    expect(wrapper.text()).toContain('加载中')
    expect(wrapper.text()).not.toContain('已开启')
    expect(wrapper.text()).not.toContain('已关闭')
    expect(wrapper.text()).not.toContain('自动发送到邮箱')
    expect(usageBriefAPIMock.getPeriod).not.toHaveBeenCalled()
    expect(usageBriefAPIMock.listReports).not.toHaveBeenCalled()

    deferred.resolve(profile())
    await flushPromises()

    expect(wrapper.text()).toContain('已开启')
    expect(usageBriefAPIMock.getPeriod).toHaveBeenCalledTimes(1)
    expect(usageBriefAPIMock.listReports).toHaveBeenCalledTimes(1)
  })

  it('does not load briefs on profile failure even though the fallback switch is enabled', async () => {
    userAPIMock.getProfile.mockRejectedValue(new Error('profile failed'))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('启用用量简报')
    expect(wrapper.text()).toContain('已开启')
    expect(usageBriefAPIMock.getPeriod).not.toHaveBeenCalled()
    expect(usageBriefAPIMock.listReports).not.toHaveBeenCalled()
  })

  it('only renders the page toggle and skips usage brief APIs when disabled', async () => {
    userAPIMock.getProfile.mockResolvedValue(profile({ usage_brief_page_enabled: false }))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('启用用量简报')
    expect(wrapper.text()).toContain('已关闭')
    expect(wrapper.text()).not.toContain('自动发送到邮箱')
    expect(wrapper.text()).not.toContain('历史简报')
    expect(usageBriefAPIMock.getPeriod).not.toHaveBeenCalled()
    expect(usageBriefAPIMock.listReports).not.toHaveBeenCalled()
  })

  it('treats a missing page preference as enabled and loads briefs', async () => {
    const legacyProfile = profile()
    delete legacyProfile.usage_brief_page_enabled
    userAPIMock.getProfile.mockResolvedValue(legacyProfile)

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('启用用量简报')
    expect(wrapper.text()).toContain('已开启')
    expect(wrapper.text()).toContain('自动发送到邮箱')
    expect(wrapper.text()).toContain('历史简报')
    expect(usageBriefAPIMock.getPeriod).toHaveBeenCalledTimes(1)
    expect(usageBriefAPIMock.listReports).toHaveBeenCalledTimes(1)
  })

  it('enables the page preference and loads current and historical briefs', async () => {
    userAPIMock.getProfile.mockResolvedValue(profile({ usage_brief_page_enabled: false }))
    userAPIMock.updateProfile.mockResolvedValue(profile({ usage_brief_page_enabled: true }))

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(userAPIMock.updateProfile).toHaveBeenCalledWith({ usage_brief_page_enabled: true })
    expect(usageBriefAPIMock.getPeriod).toHaveBeenCalledTimes(1)
    expect(usageBriefAPIMock.listReports).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('自动发送到邮箱')
    expect(wrapper.text()).toContain('历史简报')
  })

  it('keeps the page enabled when update response omits the page preference', async () => {
    userAPIMock.getProfile.mockResolvedValue(profile({ usage_brief_page_enabled: false }))
    const updated = profile()
    delete (updated as Partial<User>).usage_brief_page_enabled
    userAPIMock.updateProfile.mockResolvedValue(updated)

    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(userAPIMock.updateProfile).toHaveBeenCalledWith({ usage_brief_page_enabled: true })
    expect(wrapper.text()).toContain('已开启')
    expect(wrapper.text()).toContain('自动发送到邮箱')
    expect(usageBriefAPIMock.getPeriod).toHaveBeenCalledTimes(1)
    expect(usageBriefAPIMock.listReports).toHaveBeenCalledTimes(1)
  })

  it('shows enabled content on refresh after profile returns an enabled page preference', async () => {
    userAPIMock.getProfile.mockResolvedValue(profile({
      usage_brief_page_enabled: true,
      usage_brief_auto_email_enabled: false
    }))

    const wrapper = mountView()
    await flushPromises()

    expect(userAPIMock.getProfile).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('已开启')
    expect(wrapper.text()).toContain('自动发送到邮箱')
    expect(wrapper.text()).toContain('历史简报')
    expect(usageBriefAPIMock.getPeriod).toHaveBeenCalledTimes(1)
    expect(usageBriefAPIMock.listReports).toHaveBeenCalledTimes(1)
  })

  it('hides brief content and reflects auto-email disabled after turning the page off', async () => {
    userAPIMock.getProfile.mockResolvedValue(profile({
      usage_brief_page_enabled: true,
      usage_brief_auto_email_enabled: true
    }))
    userAPIMock.updateProfile.mockResolvedValue(profile({
      usage_brief_page_enabled: false,
      usage_brief_auto_email_enabled: false
    }))

    const wrapper = mountView()
    await flushPromises()
    expect(usageBriefAPIMock.getPeriod).toHaveBeenCalledTimes(1)
    expect(usageBriefAPIMock.listReports).toHaveBeenCalledTimes(1)

    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(userAPIMock.updateProfile).toHaveBeenCalledWith({ usage_brief_page_enabled: false })
    expect(wrapper.text()).toContain('已关闭')
    expect(wrapper.text()).not.toContain('自动发送到邮箱')
    expect(wrapper.text()).not.toContain('历史简报')
    expect(usageBriefAPIMock.getPeriod).toHaveBeenCalledTimes(1)
    expect(usageBriefAPIMock.listReports).toHaveBeenCalledTimes(1)
  })
})
