import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'
import type { OpsDashboardOverview } from '@/api/admin/ops'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/api', () => ({
  adminAPI: {
    groups: {
      getAll: vi.fn().mockResolvedValue([]),
    },
  },
}))

vi.mock('@/api/admin/ops', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/admin/ops')>()
  return {
    ...actual,
    opsAPI: {
      getRealtimeTrafficSummary: vi.fn().mockResolvedValue({
        request_count: 0,
        token_consumed: 0,
        qps: 0,
        tps: 0,
        switch_count: 0,
      }),
    },
  }
})

vi.mock('@/stores', () => ({
  useAdminSettingsStore: () => ({
    opsMonitoringEnabled: true,
  }),
}))

const SelectStub = defineComponent({
  name: 'SelectStub',
  template: '<div class="select-stub" />',
})

const HelpTooltipStub = defineComponent({
  name: 'HelpTooltip',
  template: '<span class="tooltip-stub" />',
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: Boolean,
    title: String,
  },
  template: '<section v-if="show" class="dialog-stub"><h2>{{ title }}</h2><slot /></section>',
})

const IconStub = defineComponent({
  name: 'Icon',
  template: '<span class="icon-stub" />',
})

function buildOverview(): OpsDashboardOverview {
  return {
    start_time: '2026-06-23T12:00:00Z',
    end_time: '2026-06-23T13:00:00Z',
    platform: '',
    health_score: 100,
    success_count: 10,
    error_count_total: 0,
    business_limited_count: 0,
    error_count_sla: 0,
    request_count_total: 10,
    request_count_sla: 10,
    token_consumed: 1000,
    sla: 100,
    error_rate: 0,
    upstream_error_rate: 0,
    upstream_error_count_excl_429_529: 0,
    upstream_429_count: 0,
    upstream_529_count: 0,
    qps: { current: 0, peak: 0, avg: 0 },
    tps: { current: 0, peak: 0, avg: 0 },
    duration: {},
    ttft: {},
    system_metrics: {
      id: 1,
      created_at: '2026-06-23T13:00:00Z',
      window_minutes: 1,
      cpu_usage_percent: 12.3,
      memory_usage_percent: 45.6,
      disk_mounts: [
        {
          mount_point: '/app/data',
          device: '/run/host_mark/Users',
          fstype: 'fakeowner',
          role: 'data_dir',
          total_mb: 948634,
          used_mb: 825958,
          free_mb: 122676,
          usage_percent: 87,
        },
        {
          mount_point: '/',
          device: 'overlay',
          fstype: 'overlay',
          role: 'container_root',
          total_mb: 102400,
          used_mb: 51200,
          free_mb: 51200,
          usage_percent: 50,
        },
        {
          mount_point: '/data',
          device: '/dev/vdb1',
          fstype: 'xfs',
          role: 'mount',
          total_mb: 204800,
          used_mb: 190464,
          free_mb: 14336,
          usage_percent: 93,
        },
      ],
    },
    job_heartbeats: [],
  }
}

function mountHeader(overview = buildOverview()) {
  return mount(OpsDashboardHeader, {
    props: {
      overview,
      platform: '',
      groupId: null,
      timeRange: '1h',
      queryMode: 'auto',
      loading: false,
      lastUpdated: new Date('2026-06-23T13:00:00Z'),
      fullscreen: false,
    },
    global: {
      stubs: {
        Select: SelectStub,
        HelpTooltip: HelpTooltipStub,
        BaseDialog: BaseDialogStub,
        Icon: IconStub,
      },
    },
  })
}

describe('OpsDashboardHeader disk metrics', () => {
  it('prioritizes the application data directory in the system health card', () => {
    const wrapper = mountHeader()

    expect(wrapper.text()).toContain('admin.ops.disk')
    expect(wrapper.text()).toContain('87.0%')
    expect(wrapper.text()).toContain('/app/data')
    expect(wrapper.text()).toContain('119.8 GB')
    expect(wrapper.text()).toContain('926.4 GB')
  })

  it('opens disk details with all visible mount points', async () => {
    const wrapper = mountHeader()
    const diskCard = wrapper
      .findAll('.rounded-xl')
      .find((card) => card.text().includes('admin.ops.disk') && card.text().includes('/data'))
    expect(diskCard).toBeTruthy()
    const detailsButton = diskCard!.find('button')
    expect(detailsButton).toBeTruthy()

    await detailsButton.trigger('click')

    expect(wrapper.text()).toContain('admin.ops.diskDetails')
    expect(wrapper.text()).toContain('/')
    expect(wrapper.text()).toContain('/data')
    expect(wrapper.text()).toContain('overlay')
    expect(wrapper.text()).toContain('/dev/vdb1')
    expect(wrapper.text()).toContain('/app/data')
    expect(wrapper.text()).toContain('/run/host_mark/Users')
    expect(wrapper.text()).toContain('fakeowner')
    expect(wrapper.text()).toContain('admin.ops.diskRoleDataDir')
    expect(wrapper.text()).toContain('admin.ops.diskRoleContainerRoot')
    expect(wrapper.text()).toContain('admin.ops.diskRoleMount')
  })

  it('keeps legacy app data fakeowner mounts but filters unrelated host bind mounts', async () => {
    const overview = buildOverview()
    overview.system_metrics!.disk_mounts = [
      {
        mount_point: '/app/data',
        device: '/run/host_mark/Users',
        fstype: 'fakeowner',
        total_mb: 948634,
        used_mb: 825958,
        free_mb: 122676,
        usage_percent: 87,
      },
      {
        mount_point: '/host/other',
        device: '/run/host_mark/Users',
        fstype: 'fakeowner',
        total_mb: 948634,
        used_mb: 825958,
        free_mb: 122676,
        usage_percent: 87,
      },
    ]
    const wrapper = mountHeader(overview)

    expect(wrapper.text()).toContain('/app/data')
    expect(wrapper.text()).not.toContain('/host/other')
  })
})
