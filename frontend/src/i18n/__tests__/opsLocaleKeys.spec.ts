import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

function flattenKeys(obj: Record<string, any>, prefix = ''): string[] {
  const keys: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    const fullKey = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'object' && v !== null && !Array.isArray(v)) {
      keys.push(...flattenKeys(v, fullKey))
    } else {
      keys.push(fullKey)
    }
  }
  return keys
}

describe('ops locale key completeness', () => {
  const requiredKeys = [
    'admin.ops.result',
    'admin.ops.disk',
    'admin.ops.diskDetails',
    'admin.ops.diskRoleDataDir',
    'admin.ops.diskRoleContainerRoot',
    'admin.ops.diskRoleMount',
    'admin.ops.usageDetailRetention',
    'admin.ops.retentionRunning',
    'admin.ops.retentionWaiting',
    'admin.ops.retentionIdle',
    'admin.ops.retentionStopped',
    'admin.ops.remaining',
    'admin.ops.processed',
    'admin.ops.cleaned',
    'admin.ops.compressed',
    'admin.ops.fallback',
    'admin.ops.skipped',
    'admin.ops.failed',
    'admin.ops.range',
    'admin.ops.nextRun',
    'admin.ops.startedAt',
    'admin.ops.finishedAt',
    'admin.ops.tooltips.usageDetailRetention',
    'admin.ops.timeRange.custom',
    'admin.ops.customTimeRange.startTime',
    'admin.ops.customTimeRange.endTime',
  ]

  for (const key of requiredKeys) {
    it(`en locale has ${key}`, () => {
      const enKeys = flattenKeys(en)
      expect(enKeys).toContain(key)
    })

    it(`zh locale has ${key}`, () => {
      const zhKeys = flattenKeys(zh)
      expect(zhKeys).toContain(key)
    })
  }
})

describe('groups locale key completeness', () => {
  it('en locale has admin.groups.failedToSave', () => {
    const enKeys = flattenKeys(en)
    expect(enKeys).toContain('admin.groups.failedToSave')
  })
})
