import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const usageDetailKeys = [
  'detailTitle',
  'requestJson',
  'responseJson',
  'compressedRequestJson',
  'compressedResponseJson',
  'copyJson',
  'jsonCopied',
  'fullJsonCleaned',
  'detailLoadFailed',
  'unavailable',
  'unavailableDisabled',
  'unavailableHistorical',
  'unavailableNotCaptured',
  'sentMessages',
  'receivedMessages',
  'noMessages',
] as const

describe('usage detail locale keys', () => {
  it('contains request detail labels in zh and en', () => {
    for (const key of usageDetailKeys) {
      expect(zh.usage[key]).toBeTruthy()
      expect(en.usage[key]).toBeTruthy()
    }
  })

  it('keeps the request detail tab labels localized', () => {
    expect(zh.usage.detailTitle).toBe('请求详情')
    expect(zh.usage.requestJson).toBe('请求 JSON')
    expect(zh.usage.responseJson).toBe('响应 JSON')
    expect(zh.usage.compressedRequestJson).toBe('压缩请求 JSON')
    expect(zh.usage.compressedResponseJson).toBe('压缩响应 JSON')
    expect(en.usage.detailTitle).toBe('Request Detail')
  })
})
