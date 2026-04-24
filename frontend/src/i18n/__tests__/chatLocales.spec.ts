import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const chatKeys = [
  'title',
  'description',
  'sidebarEyebrow',
  'headerEyebrow',
  'apiKeyLabel',
  'modelLabel',
  'emptyConversationsTitle',
  'emptyConversationsDesc',
  'untitledConversation',
  'emptyTitle',
  'emptyDesc',
  'youLabel',
  'assistantLabel',
  'failed',
  'stopped',
  'streaming',
  'stopGenerating',
  'waitingResponse',
  'inputPlaceholder',
  'inputPlaceholderNoImage',
  'imageHint',
  'noImageHint',
  'selectApiKeyFirst',
  'createConversationFailed',
  'updateConversationFailed',
  'renamePrompt',
  'renameConversationFailed',
  'deleteConversationConfirm',
  'deleteConversationFailed',
  'onlyImageSupported',
  'uploadFailed',
  'removeAttachmentFailed',
  'streamFailed',
  'loadFailed',
] as const

describe('chat locale keys', () => {
  it('contains the chat navigation label in zh and en', () => {
    expect(zh.nav.chat).toBe('聊天')
    expect(en.nav.chat).toBe('Chat')
  })

  it('contains the chat bundle required by ChatView in zh and en', () => {
    for (const key of chatKeys) {
      expect(zh.chat[key]).toBeTruthy()
      expect(en.chat[key]).toBeTruthy()
    }
  })
})
