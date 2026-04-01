<template>
  <AppLayout>
    <div v-if="pageLoading" class="mx-auto flex h-[calc(100dvh-6rem)] min-h-0 w-full max-w-7xl items-center justify-center md:h-[calc(100dvh-7rem)] lg:h-[calc(100dvh-8rem)]">
      <div class="flex items-center gap-3 rounded-2xl border border-gray-200 bg-white/90 px-5 py-4 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
        <div class="h-5 w-5 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
        <span class="text-sm text-gray-600 dark:text-gray-300">{{ t('common.loading') }}</span>
      </div>
    </div>

    <div v-else class="mx-auto flex h-[calc(100dvh-6rem)] min-h-0 w-full max-w-7xl flex-col gap-6 overflow-hidden md:h-[calc(100dvh-7rem)] lg:h-[calc(100dvh-8rem)] lg:flex-row">
      <aside class="flex max-h-[35vh] min-h-0 w-full shrink-0 flex-col overflow-hidden rounded-3xl border border-gray-200 bg-white/90 shadow-sm backdrop-blur dark:border-dark-700 dark:bg-dark-900/90 lg:h-full lg:max-h-none lg:w-[320px]">
        <div class="border-b border-gray-100 p-4 dark:border-dark-700">
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-xs font-semibold uppercase tracking-[0.24em] text-primary-500">{{ t('chat.sidebarEyebrow') }}</p>
              <h2 class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ t('chat.title') }}</h2>
            </div>
            <button class="btn btn-primary btn-sm" :disabled="busyCreatingConversation || !canCreateConversation" @click="handleCreateConversation">
              <Icon name="plus" size="sm" />
            </button>
          </div>
          <div class="mt-4 space-y-3">
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('chat.apiKeyLabel') }}</label>
              <select v-model.number="selectedApiKeyId" class="input w-full" @change="handleApiKeyChange">
                <option v-for="item in apiKeys" :key="item.id" :value="item.id">
                  {{ item.name }} · {{ item.group_name }}
                </option>
              </select>
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('chat.modelLabel') }}</label>
              <select v-model="selectedModel" class="input w-full" :disabled="modelsLoading || models.length === 0" @change="handleModelChange">
                <option v-for="item in models" :key="item.id" :value="item.id">
                  {{ item.display_name }}
                </option>
              </select>
            </div>
          </div>
        </div>

        <div class="min-h-0 flex-1 overflow-y-auto p-3">
          <div v-if="conversationsLoading" class="space-y-3">
            <div v-for="n in 6" :key="n" class="h-20 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-800"></div>
          </div>

          <div v-else-if="conversations.length === 0" class="rounded-2xl border border-dashed border-gray-200 bg-gray-50/80 p-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900/60 dark:text-gray-400">
            <div class="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-2xl bg-primary-50 text-primary-500 dark:bg-primary-900/30">
              <Icon name="chat" size="lg" />
            </div>
            <p class="font-medium text-gray-900 dark:text-white">{{ t('chat.emptyConversationsTitle') }}</p>
            <p class="mt-1">{{ t('chat.emptyConversationsDesc') }}</p>
          </div>

          <div v-else class="space-y-2">
            <button
              v-for="conversation in conversations"
              :key="conversation.id"
              class="w-full rounded-2xl border px-4 py-3 text-left transition-all"
              :class="conversation.id === highlightedConversationId
                ? 'border-primary-300 bg-primary-50 shadow-sm dark:border-primary-700 dark:bg-primary-900/20'
                : 'border-transparent bg-gray-50 hover:border-gray-200 hover:bg-white dark:bg-dark-900/60 dark:hover:border-dark-700 dark:hover:bg-dark-900'"
              @click="openConversation(conversation.id)"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ conversation.title || t('chat.untitledConversation') }}</p>
                  <p class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ conversation.model }}</p>
                  <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">{{ formatConversationTime(conversation.last_message_at || conversation.updated_at) }}</p>
                </div>
                <div class="flex shrink-0 items-center gap-1">
                  <button class="rounded-lg p-1.5 text-gray-400 transition hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-800 dark:hover:text-gray-200" @click.stop="renameConversation(conversation)">
                    <Icon name="edit" size="sm" />
                  </button>
                  <button class="rounded-lg p-1.5 text-gray-400 transition hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-300" @click.stop="removeConversation(conversation)">
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </div>
            </button>
          </div>
        </div>
      </aside>

      <section class="flex h-full min-h-0 flex-1 flex-col overflow-hidden rounded-[2rem] border border-gray-200 bg-white/90 shadow-sm backdrop-blur dark:border-dark-700 dark:bg-dark-900/90">
        <header class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p class="text-xs font-semibold uppercase tracking-[0.24em] text-gray-400 dark:text-gray-500">{{ t('chat.headerEyebrow') }}</p>
              <h1 class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">
                {{ activeConversation?.title || t('chat.title') }}
              </h1>
            </div>
            <div class="flex items-center gap-2">
              <span v-if="isStreaming" class="rounded-full bg-amber-50 px-3 py-1 text-xs font-medium text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                {{ t('chat.streaming') }}
              </span>
              <button v-if="isStreaming" class="btn btn-secondary btn-sm" @click="stopStreaming">
                <Icon name="x" size="sm" />
                <span>{{ t('chat.stopGenerating') }}</span>
              </button>
            </div>
          </div>
        </header>

        <div class="relative min-h-0 flex-1 overflow-y-auto px-4 py-5 md:px-6" ref="messageScroller">
          <div v-if="showMessageSkeleton" class="space-y-4">
            <div v-for="n in 4" :key="n" class="animate-pulse rounded-3xl bg-gray-100 p-6 dark:bg-dark-800">
              <div class="h-4 w-20 rounded bg-gray-200 dark:bg-dark-700"></div>
              <div class="mt-4 h-4 rounded bg-gray-200 dark:bg-dark-700"></div>
              <div class="mt-2 h-4 w-4/5 rounded bg-gray-200 dark:bg-dark-700"></div>
            </div>
          </div>

          <div v-else-if="messages.length === 0" class="flex h-full min-h-full items-center justify-center py-10">
            <div class="max-w-lg text-center">
              <div class="mx-auto flex h-16 w-16 items-center justify-center rounded-3xl bg-primary-50 text-primary-500 shadow-sm dark:bg-primary-900/20">
                <Icon name="sparkles" size="xl" />
              </div>
              <h2 class="mt-6 text-2xl font-semibold text-gray-900 dark:text-white">{{ t('chat.emptyTitle') }}</h2>
              <p class="mt-3 text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('chat.emptyDesc') }}</p>
            </div>
          </div>

          <div v-else class="space-y-5">
            <article
              v-for="message in messages"
              :key="message.id"
              class="flex"
              :class="message.role === 'user' ? 'justify-end' : 'justify-start'"
            >
              <div
                class="max-w-3xl rounded-[2rem] border px-5 py-4 shadow-sm"
                :class="message.role === 'user'
                  ? 'border-primary-300 bg-primary-50 text-gray-900 dark:border-primary-700 dark:bg-primary-900/20 dark:text-white'
                  : 'border-gray-200 bg-white text-gray-900 dark:border-dark-700 dark:bg-dark-900 dark:text-white'"
              >
                <div class="mb-3 flex items-center justify-between gap-4">
                  <div class="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-gray-400 dark:text-gray-500">
                    <span class="inline-flex h-8 w-8 items-center justify-center rounded-2xl" :class="message.role === 'user' ? 'bg-primary-100 text-primary-600 dark:bg-primary-800/50 dark:text-primary-200' : 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-gray-200'">
                      <Icon :name="message.role === 'user' ? 'user' : 'chat'" size="sm" />
                    </span>
                    <span>{{ message.role === 'user' ? t('chat.youLabel') : t('chat.assistantLabel') }}</span>
                  </div>
                  <span
                    v-if="message.status === 'failed' || message.status === 'stopped'"
                    class="rounded-full px-2.5 py-1 text-[11px] font-medium"
                    :class="message.status === 'failed'
                      ? 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300'
                      : 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'"
                  >
                    {{ message.status === 'failed' ? t('chat.failed') : t('chat.stopped') }}
                  </span>
                </div>

                <div v-if="message.attachments?.length" class="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-3">
                  <button
                    v-for="attachment in message.attachments"
                    :key="attachment.id"
                    type="button"
                    class="group overflow-hidden rounded-2xl border border-gray-200 bg-gray-50 transition hover:border-primary-300 hover:shadow-sm dark:border-dark-700 dark:bg-dark-800"
                    @click="openImagePreview(attachment)"
                  >
                    <img :src="attachment.data_url" :alt="attachment.original_name" class="h-28 w-full object-cover transition duration-300 group-hover:scale-[1.03]" />
                    <div class="flex items-center justify-between gap-2 px-3 py-2 text-left">
                      <span class="truncate text-xs font-medium text-gray-700 dark:text-gray-200">{{ attachment.original_name }}</span>
                      <Icon name="arrowRight" size="xs" class="text-gray-400" />
                    </div>
                  </button>
                </div>

                <div v-if="message.text" class="chat-markdown prose prose-sm max-w-none dark:prose-invert" v-html="renderMarkdown(message.text)"></div>
                <p v-else-if="message.role === 'assistant' && message.status === 'streaming'" class="text-sm text-gray-500 dark:text-gray-400">{{ t('chat.waitingResponse') }}</p>
                <p v-if="message.error_message" class="mt-3 rounded-2xl bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
                  {{ message.error_message }}
                </p>
              </div>
            </article>
          </div>

          <transition name="fade">
            <div
              v-if="isConversationSwitching"
              class="pointer-events-none absolute inset-0 flex items-center justify-center bg-white/45 backdrop-blur-[1px] dark:bg-dark-950/45"
            >
              <div class="flex items-center gap-3 rounded-2xl border border-gray-200 bg-white/95 px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-900/95">
                <div class="h-4 w-4 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
                <span class="text-sm text-gray-600 dark:text-gray-300">{{ t('common.loading') }}</span>
              </div>
            </div>
          </transition>
        </div>

        <footer class="border-t border-gray-100 px-4 py-4 dark:border-dark-700 md:px-6">
          <div
            class="rounded-[2rem] border-2 border-dashed p-4 transition"
            :class="dragActive
              ? 'border-primary-400 bg-primary-50/80 dark:border-primary-600 dark:bg-primary-900/10'
              : 'border-transparent bg-gray-50 dark:bg-dark-900/60'"
            @dragenter.prevent="dragActive = true"
            @dragover.prevent="dragActive = true"
            @dragleave.prevent="dragActive = false"
            @drop.prevent="handleDrop"
          >
            <div v-if="draftAttachments.length" class="mb-4 flex flex-wrap gap-3">
              <div
                v-for="attachment in draftAttachments"
                :key="attachment.id"
                class="group relative overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800"
              >
                <img :src="attachment.data_url" :alt="attachment.original_name" class="h-24 w-24 object-cover" />
                <button
                  type="button"
                  class="absolute right-2 top-2 rounded-full bg-black/65 p-1 text-white opacity-0 transition group-hover:opacity-100"
                  @click="removeDraftAttachment(attachment)"
                >
                  <Icon name="x" size="xs" />
                </button>
              </div>
            </div>

            <div class="flex items-end gap-3">
              <input ref="fileInputRef" type="file" accept="image/*" class="hidden" multiple @change="handleFileSelection" />
              <button class="btn btn-secondary shrink-0" type="button" :disabled="uploadingAttachments || !activeModelSupportsImages" @click="fileInputRef?.click()">
                <Icon name="upload" size="sm" />
              </button>

              <div class="min-w-0 flex-1">
                <textarea
                  ref="textareaRef"
                  v-model="draftText"
                  rows="1"
                  class="input min-h-[56px] w-full resize-none rounded-[1.5rem] px-5 py-4 leading-6"
                  :placeholder="activeModelSupportsImages ? t('chat.inputPlaceholder') : t('chat.inputPlaceholderNoImage')"
                  @input="autoResizeTextarea"
                  @keydown="handleComposerKeydown"
                  @paste="handlePaste"
                ></textarea>
                <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">
                  {{ activeModelSupportsImages ? t('chat.imageHint') : t('chat.noImageHint') }}
                </p>
              </div>

              <button class="btn btn-primary shrink-0" type="button" :disabled="sendDisabled" @click="sendMessage">
                <Icon name="arrowUp" size="sm" />
              </button>
            </div>
          </div>
        </footer>
      </section>
    </div>

    <div v-if="previewAttachment" class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-6" @click="previewAttachment = null">
      <img :src="previewAttachment.data_url" :alt="previewAttachment.original_name" class="max-h-full max-w-full rounded-3xl shadow-2xl" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { chatAPI } from '@/api/chat'
import { useAppStore } from '@/stores'
import type { ChatApiKeyOption, ChatAttachment, ChatConversation, ChatMessage, ChatModel } from '@/types'

marked.setOptions({
  breaks: true,
  gfm: true
})

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const conversations = ref<ChatConversation[]>([])
const messages = ref<ChatMessage[]>([])
const apiKeys = ref<ChatApiKeyOption[]>([])
const models = ref<ChatModel[]>([])
const messagesCache = ref<Record<number, ChatMessage[]>>({})

const activeConversationId = ref<number | null>(null)
const pendingConversationId = ref<number | null>(null)
const selectedApiKeyId = ref<number | null>(null)
const selectedModel = ref('')
const draftText = ref('')
const draftAttachments = ref<ChatAttachment[]>([])
const previewAttachment = ref<ChatAttachment | null>(null)

const pageLoading = ref(true)
const conversationsLoading = ref(false)
const messagesLoading = ref(false)
const modelsLoading = ref(false)
const uploadingAttachments = ref(false)
const busyCreatingConversation = ref(false)
const isStreaming = ref(false)
const dragActive = ref(false)

const textareaRef = ref<HTMLTextAreaElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
const messageScroller = ref<HTMLElement | null>(null)
const streamAbortController = ref<AbortController | null>(null)

const activeConversation = computed(() =>
  conversations.value.find((item) => item.id === activeConversationId.value) ?? null
)
const highlightedConversationId = computed(() => pendingConversationId.value ?? activeConversationId.value)
const isConversationSwitching = computed(() =>
  pendingConversationId.value !== null && pendingConversationId.value !== activeConversationId.value
)

const activeModelSupportsImages = computed(() =>
  models.value.find((item) => item.id === selectedModel.value)?.supports_image_input ?? false
)

const canCreateConversation = computed(() => Boolean(selectedApiKeyId.value && selectedModel.value))
const showMessageSkeleton = computed(() => messagesLoading.value && messages.value.length === 0)
const sendDisabled = computed(() =>
  isStreaming.value ||
  uploadingAttachments.value ||
  (!draftText.value.trim() && draftAttachments.value.length === 0)
)

let conversationSwitchToken = 0

function renderMarkdown(content: string) {
  const html = enhanceMarkdownCodeBlocks(marked.parse(content || '') as string)
  return DOMPurify.sanitize(html)
}

function enhanceMarkdownCodeBlocks(html: string) {
  return html.replace(
    /<pre><code(?: class="language-([^"]+)")?>([\s\S]*?)<\/code><\/pre>/g,
    (_match, language: string | undefined, encodedCode: string) =>
      renderHighlightedCodeBlock(decodeHtmlEntities(encodedCode), language)
  )
}

function renderHighlightedCodeBlock(code: string, language?: string) {
  const normalizedLanguage = normalizeCodeLanguage(language)
  const languageLabel = normalizedLanguage ? normalizedLanguage.toUpperCase() : 'CODE'
  const normalizedCode = code.replace(/\r\n/g, '\n')
  const highlightedLines = normalizedCode.split('\n').map((line) => syntaxHighlightLine(line, normalizedLanguage))
  const lineNumbers = highlightedLines
    .map((_line, index) => `<span class="chat-code-line-number">${index + 1}</span>`)
    .join('')
  const codeLines = highlightedLines
    .map((line) => `<span class="chat-code-line">${line || '&nbsp;'}</span>`)
    .join('')

  return `
    <div class="chat-code-shell">
      <div class="chat-code-header">
        <span class="chat-code-lang">${escapeHtml(languageLabel)}</span>
      </div>
      <div class="chat-code-body">
        <div class="chat-code-gutter" aria-hidden="true">${lineNumbers}</div>
        <pre class="chat-code-block"><code class="chat-code language-${escapeHtml(normalizedLanguage || 'plain')}">${codeLines}</code></pre>
      </div>
    </div>
  `
}

function normalizeCodeLanguage(language?: string) {
  const normalized = (language || '').trim().toLowerCase()
  if (!normalized) return ''

  const aliasMap: Record<string, string> = {
    javascript: 'js',
    jsx: 'js',
    typescript: 'ts',
    tsx: 'ts',
    shell: 'bash',
    sh: 'bash',
    zsh: 'bash',
    shellscript: 'bash',
    py: 'python',
    cxx: 'cpp',
    cc: 'cpp',
    'c++': 'cpp',
    hpp: 'cpp',
    hxx: 'cpp',
    yml: 'yaml',
    md: 'markdown'
  }

  return aliasMap[normalized] || normalized
}

function syntaxHighlightLine(code: string, language: string) {
  let highlighted = escapeHtml(code)
  const placeholders: string[] = []

  const stash = (value: string) => {
    const index = placeholders.push(value) - 1
    return `__CHAT_TOKEN_${index}__`
  }

  highlighted = highlighted.replace(
    /(\/\*[\s\S]*?\*\/|\/\/.*|#.*)$/g,
    (value) => stash(`<span class="chat-token comment">${value}</span>`)
  )

  highlighted = highlighted.replace(
    /("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`])*`)/g,
    (value) => stash(`<span class="chat-token string">${value}</span>`)
  )

  highlighted = highlighted.replace(
    /\b(\d+(?:\.\d+)?)\b/g,
    (value) => stash(`<span class="chat-token number">${value}</span>`)
  )

  const keywordPattern = buildKeywordPattern(language)
  if (keywordPattern) {
    highlighted = highlighted.replace(
      keywordPattern,
      (value) => stash(`<span class="chat-token keyword">${value}</span>`)
    )
  }

  highlighted = highlighted.replace(
    /\b([A-Za-z_][\w$]*)(?=\s*\()/g,
    (value) => stash(`<span class="chat-token function">${value}</span>`)
  )

  highlighted = highlighted.replace(
    /(&lt;\/?|\/?&gt;|===|!==|==|!=|=&gt;|-&gt;|:=|&&|\|\||[=+\-*/%<>!]+)/g,
    (value) => stash(`<span class="chat-token operator">${value}</span>`)
  )

  highlighted = highlighted.replace(
    /__CHAT_TOKEN_(\d+)__/g,
    (_token, index) => placeholders[Number(index)] || ''
  )

  return highlighted
}

function buildKeywordPattern(language: string) {
  const keywordMap: Record<string, string[]> = {
    js: ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'switch', 'case', 'break', 'continue', 'async', 'await', 'try', 'catch', 'throw', 'new', 'class', 'extends', 'import', 'from', 'export', 'default', 'true', 'false', 'null', 'undefined'],
    ts: ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'switch', 'case', 'break', 'continue', 'async', 'await', 'try', 'catch', 'throw', 'new', 'class', 'extends', 'import', 'from', 'export', 'default', 'interface', 'type', 'implements', 'public', 'private', 'protected', 'readonly', 'true', 'false', 'null', 'undefined'],
    python: ['def', 'return', 'if', 'elif', 'else', 'for', 'while', 'in', 'import', 'from', 'as', 'class', 'try', 'except', 'finally', 'raise', 'with', 'lambda', 'yield', 'True', 'False', 'None'],
    go: ['func', 'package', 'import', 'return', 'if', 'else', 'for', 'range', 'switch', 'case', 'break', 'continue', 'type', 'struct', 'interface', 'map', 'var', 'const', 'go', 'defer', 'select', 'chan', 'true', 'false', 'nil'],
    cpp: ['using', 'namespace', 'class', 'struct', 'template', 'typename', 'public', 'private', 'protected', 'return', 'if', 'else', 'for', 'while', 'switch', 'case', 'break', 'continue', 'enum', 'void', 'int', 'bool', 'const', 'nullptr', 'new', 'delete', 'this'],
    c: ['struct', 'enum', 'typedef', 'return', 'if', 'else', 'for', 'while', 'switch', 'case', 'break', 'continue', 'void', 'int', 'char', 'bool', 'const', 'NULL'],
    bash: ['if', 'then', 'else', 'fi', 'for', 'do', 'done', 'case', 'esac', 'function', 'export', 'local', 'echo', 'while', 'in'],
    json: ['true', 'false', 'null'],
    yaml: ['true', 'false', 'null'],
    html: ['div', 'span', 'script', 'style', 'head', 'body', 'html', 'meta', 'link'],
    css: ['display', 'position', 'color', 'background', 'border', 'padding', 'margin', 'flex', 'grid'],
    sql: ['select', 'from', 'where', 'join', 'left', 'right', 'inner', 'outer', 'insert', 'into', 'update', 'delete', 'create', 'table', 'group', 'by', 'order', 'limit', 'and', 'or', 'not', 'null', 'values', 'set']
  }

  const fallbackKeywords = ['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'async', 'await', 'class', 'import', 'export', 'true', 'false', 'null']
  const keywords = keywordMap[language] || fallbackKeywords
  if (!keywords.length) return null
  return new RegExp(`\\b(${keywords.join('|')})\\b`, 'g')
}

function escapeHtml(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function decodeHtmlEntities(value: string) {
  return value
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, '\'')
    .replace(/&amp;/g, '&')
}

function formatConversationTime(value?: string | null) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function updateRouteConversation(id: number | null) {
  const nextQuery = { ...route.query }
  if (id) {
    nextQuery.c = String(id)
  } else {
    delete nextQuery.c
  }
  void router.replace({ query: nextQuery })
}

function syncConversationSelection(conversation: ChatConversation | null) {
  if (!conversation) return
  activeConversationId.value = conversation.id
  selectedApiKeyId.value = conversation.api_key_id
  selectedModel.value = conversation.model
  updateRouteConversation(conversation.id)
}

async function loadAPIKeys() {
  apiKeys.value = await chatAPI.listAPIKeys()
  if (!selectedApiKeyId.value && apiKeys.value.length > 0) {
    selectedApiKeyId.value = apiKeys.value[0].id
  }
}

async function loadModels(apiKeyId: number, preferredModel?: string) {
  modelsLoading.value = true
  try {
    const modelItems = await chatAPI.listModels(apiKeyId)
    models.value = modelItems
    const targetModel = preferredModel && modelItems.some((item) => item.id === preferredModel)
      ? preferredModel
      : modelItems[0]?.id || ''
    selectedModel.value = targetModel
  } finally {
    modelsLoading.value = false
  }
}

async function fetchModelsData(apiKeyId: number, preferredModel?: string) {
  const modelItems = await chatAPI.listModels(apiKeyId)
  const targetModel = preferredModel && modelItems.some((item) => item.id === preferredModel)
    ? preferredModel
    : modelItems[0]?.id || ''
  return { modelItems, targetModel }
}

async function loadConversations(options?: { preserveSelection?: boolean }) {
  conversationsLoading.value = true
  try {
    conversations.value = await chatAPI.listConversations()
    const queryConversationId = Number(route.query.c)
    const preferredConversationId = options?.preserveSelection ? activeConversationId.value : null
    const targetConversation = conversations.value.find((item) => item.id === queryConversationId)
      ?? conversations.value.find((item) => item.id === preferredConversationId)
      ?? conversations.value[0]

    if (targetConversation) {
      syncConversationSelection(targetConversation)
      await loadModels(targetConversation.api_key_id, targetConversation.model)
      const shouldReloadMessages = !options?.preserveSelection || targetConversation.id !== activeConversationId.value || messages.value.length === 0
      if (shouldReloadMessages) {
        await loadMessages(targetConversation.id)
      }
    } else {
      activeConversationId.value = null
      messages.value = []
      if (selectedApiKeyId.value) {
        await loadModels(selectedApiKeyId.value)
      }
    }
  } finally {
    conversationsLoading.value = false
  }
}

async function loadMessages(conversationId: number) {
  messagesLoading.value = true
  try {
    const nextMessages = await chatAPI.listMessages(conversationId)
    messagesCache.value[conversationId] = cloneChatMessages(nextMessages)
    messages.value = nextMessages
  } finally {
    messagesLoading.value = false
    await nextTick()
    scrollMessagesToBottom()
  }
}

async function ensureConversation(): Promise<ChatConversation | null> {
  if (activeConversation.value) return activeConversation.value
  if (!selectedApiKeyId.value || !selectedModel.value) return null
  return handleCreateConversation()
}

async function handleCreateConversation() {
  if (!selectedApiKeyId.value || !selectedModel.value) {
    appStore.showError(t('chat.selectApiKeyFirst'))
    return null
  }
  busyCreatingConversation.value = true
  try {
    const conversation = await chatAPI.createConversation({
      api_key_id: selectedApiKeyId.value,
      model: selectedModel.value
    })
    conversations.value = [conversation, ...conversations.value]
    syncConversationSelection(conversation)
    messages.value = []
    draftText.value = ''
    draftAttachments.value = []
    await nextTick()
    scrollMessagesToBottom()
    return conversation
  } catch (error: any) {
    appStore.showError(error.message || t('chat.createConversationFailed'))
    return null
  } finally {
    busyCreatingConversation.value = false
  }
}

async function openConversation(conversationId: number) {
  const target = conversations.value.find((item) => item.id === conversationId)
  if (!target) return
  if (target.id === activeConversationId.value && pendingConversationId.value == null) return

  draftText.value = ''
  draftAttachments.value = []

  const switchToken = ++conversationSwitchToken
  pendingConversationId.value = target.id

  try {
    const cachedMessages = messagesCache.value[target.id]
    const [modelData, nextMessages] = await Promise.all([
      fetchModelsData(target.api_key_id, target.model),
      cachedMessages
        ? Promise.resolve(cloneChatMessages(cachedMessages))
        : chatAPI.listMessages(target.id)
    ])

    if (!cachedMessages) {
      messagesCache.value[target.id] = cloneChatMessages(nextMessages)
    }
    if (switchToken !== conversationSwitchToken) return

    models.value = modelData.modelItems
    selectedApiKeyId.value = target.api_key_id
    selectedModel.value = modelData.targetModel
    messages.value = nextMessages
    syncConversationSelection(target)
  } catch (error: any) {
    if (switchToken === conversationSwitchToken) {
      appStore.showError(error.message || t('chat.loadFailed'))
    }
  } finally {
    if (switchToken === conversationSwitchToken) {
      pendingConversationId.value = null
      await nextTick()
      scrollMessagesToBottom()
    }
  }
}

async function handleApiKeyChange() {
  if (!selectedApiKeyId.value) return
  const previousModel = selectedModel.value
  await loadModels(selectedApiKeyId.value, previousModel)
  if (!activeConversation.value) return
  try {
    const updated = await chatAPI.updateConversation(activeConversation.value.id, {
      api_key_id: selectedApiKeyId.value,
      model: selectedModel.value
    })
    patchConversation(updated)
  } catch (error: any) {
    appStore.showError(error.message || t('chat.updateConversationFailed'))
  }
}

async function handleModelChange() {
  if (!activeConversation.value || !selectedModel.value) return
  try {
    const updated = await chatAPI.updateConversation(activeConversation.value.id, {
      model: selectedModel.value
    })
    patchConversation(updated)
  } catch (error: any) {
    appStore.showError(error.message || t('chat.updateConversationFailed'))
  }
}

function patchConversation(updated: ChatConversation) {
  const index = conversations.value.findIndex((item) => item.id === updated.id)
  if (index >= 0) {
    conversations.value[index] = updated
  } else {
    conversations.value.unshift(updated)
  }
  if (activeConversationId.value === updated.id) {
    syncConversationSelection(updated)
  }
}

async function renameConversation(conversation: ChatConversation) {
  const nextTitle = window.prompt(t('chat.renamePrompt'), conversation.title)
  if (nextTitle == null) return
  try {
    const updated = await chatAPI.updateConversation(conversation.id, { title: nextTitle })
    patchConversation(updated)
  } catch (error: any) {
    appStore.showError(error.message || t('chat.renameConversationFailed'))
  }
}

async function removeConversation(conversation: ChatConversation) {
  if (!window.confirm(t('chat.deleteConversationConfirm'))) return
  try {
    await chatAPI.deleteConversation(conversation.id)
    conversations.value = conversations.value.filter((item) => item.id !== conversation.id)
    if (activeConversationId.value === conversation.id) {
      const nextConversation = conversations.value[0] ?? null
      if (nextConversation) {
        await openConversation(nextConversation.id)
      } else {
        activeConversationId.value = null
        messages.value = []
      }
    }
  } catch (error: any) {
    appStore.showError(error.message || t('chat.deleteConversationFailed'))
  }
}

async function uploadFiles(files: File[]) {
  if (files.length === 0) return
  if (!activeModelSupportsImages.value) {
    appStore.showError(t('chat.noImageHint'))
    return
  }
  const conversation = await ensureConversation()
  if (!conversation) {
    appStore.showError(t('chat.createConversationFailed'))
    return
  }

  const imageFiles = files.filter((file) => file.type.startsWith('image/'))
  if (imageFiles.length !== files.length) {
    appStore.showError(t('chat.onlyImageSupported'))
  }
  if (!imageFiles.length) return

  uploadingAttachments.value = true
  try {
    for (const file of imageFiles) {
      const attachment = await chatAPI.uploadAttachment(conversation.id, file)
      draftAttachments.value.push(attachment)
    }
  } catch (error: any) {
    appStore.showError(error.message || t('chat.uploadFailed'))
  } finally {
    uploadingAttachments.value = false
    dragActive.value = false
  }
}

async function removeDraftAttachment(attachment: ChatAttachment) {
  try {
    await chatAPI.deleteAttachment(attachment.id)
    draftAttachments.value = draftAttachments.value.filter((item) => item.id !== attachment.id)
  } catch (error: any) {
    appStore.showError(error.message || t('chat.removeAttachmentFailed'))
  }
}

function handleFileSelection(event: Event) {
  const target = event.target as HTMLInputElement
  const files = Array.from(target.files || [])
  void uploadFiles(files)
  target.value = ''
}

function handleDrop(event: DragEvent) {
  dragActive.value = false
  const files = Array.from(event.dataTransfer?.files || [])
  void uploadFiles(files)
}

function handlePaste(event: ClipboardEvent) {
  if (!activeModelSupportsImages.value) return
  const files = Array.from(event.clipboardData?.items || [])
    .filter((item) => item.kind === 'file' && item.type.startsWith('image/'))
    .map((item) => item.getAsFile())
    .filter((file): file is File => Boolean(file))
  if (files.length > 0) {
    event.preventDefault()
    void uploadFiles(files)
  }
}

function autoResizeTextarea() {
  const textarea = textareaRef.value
  if (!textarea) return
  textarea.style.height = 'auto'
  textarea.style.height = `${Math.min(textarea.scrollHeight, 220)}px`
}

function handleComposerKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    void sendMessage()
  }
}

function scrollMessagesToBottom() {
  const scroller = messageScroller.value
  if (!scroller) return
  scroller.scrollTop = scroller.scrollHeight
}

function openImagePreview(attachment: ChatAttachment) {
  previewAttachment.value = attachment
}

function consumeSSEBuffer(
  buffer: string,
  onEvent: (event: Record<string, any>) => void
) {
  let rest = buffer
  while (true) {
    const boundary = rest.indexOf('\n\n')
    if (boundary === -1) break
    const rawEvent = rest.slice(0, boundary)
    rest = rest.slice(boundary + 2)
    const payload = rawEvent
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice(5).trim())
      .join('')
    if (!payload || payload === '[DONE]') continue
    try {
      onEvent(JSON.parse(payload))
    } catch {
      // ignore malformed chunks
    }
  }
  return rest
}

async function extractStreamError(response: Response) {
  const raw = await response.text()
  try {
    const parsed = JSON.parse(raw)
    return parsed.error?.message || parsed.message || raw || t('chat.streamFailed')
  } catch {
    return raw || t('chat.streamFailed')
  }
}

async function sendMessage() {
  const conversation = await ensureConversation()
  if (!conversation) return

  const text = draftText.value.trim()
  const attachments = [...draftAttachments.value]
  if (!text && attachments.length === 0) return

  const userTempId = `temp-user-${Date.now()}`
  const assistantTempId = `temp-assistant-${Date.now()}`

  messages.value.push({
    id: userTempId,
    conversation_id: conversation.id,
    user_id: 0,
    role: 'user',
    status: 'completed',
    text,
    model: selectedModel.value,
    attachment_ids: attachments.map((item) => item.id),
    attachments,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    is_temporary: true
  })
  messages.value.push({
    id: assistantTempId,
    conversation_id: conversation.id,
    user_id: 0,
    role: 'assistant',
    status: 'streaming',
    text: '',
    model: selectedModel.value,
    attachment_ids: [],
    attachments: [],
    error_message: '',
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    is_temporary: true
  })

  draftText.value = ''
  draftAttachments.value = []
  autoResizeTextarea()
  await nextTick()
  scrollMessagesToBottom()

  isStreaming.value = true
  streamAbortController.value = new AbortController()

  try {
    const response = await chatAPI.streamConversationMessage(
      conversation.id,
      {
        text,
        attachment_ids: attachments.map((item) => item.id)
      },
      streamAbortController.value.signal
    )

    const assistantMessage = messages.value.find((item) => item.id === assistantTempId)
    if (!assistantMessage) return

    if (!response.ok) {
      assistantMessage.status = 'failed'
      assistantMessage.error_message = await extractStreamError(response)
      appStore.showError(assistantMessage.error_message || t('chat.streamFailed'))
      return
    }

    if (!response.body) {
      assistantMessage.status = 'failed'
      assistantMessage.error_message = t('chat.streamFailed')
      return
    }

    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      buffer = consumeSSEBuffer(buffer, (event) => {
        switch (event.type) {
          case 'response.output_text.delta':
            assistantMessage.text += String(event.delta || '')
            assistantMessage.status = 'streaming'
            break
          case 'response.completed':
          case 'response.done':
            assistantMessage.status = 'completed'
            if (!assistantMessage.text && Array.isArray(event.response?.output)) {
              assistantMessage.text = extractCompletedText(event.response.output)
            }
            break
          case 'response.failed':
          case 'response.incomplete':
          case 'error':
            assistantMessage.status = 'failed'
            assistantMessage.error_message = event.error?.message || event.message || t('chat.streamFailed')
            break
          default:
            break
        }
      })
      await nextTick()
      scrollMessagesToBottom()
    }

    if (assistantMessage.status === 'streaming') {
      assistantMessage.status = 'completed'
    }
  } catch (error: any) {
    const assistantMessage = messages.value.find((item) => item.id === assistantTempId)
    if (assistantMessage) {
      if (error?.name === 'AbortError') {
        assistantMessage.status = 'stopped'
      } else {
        assistantMessage.status = 'failed'
        assistantMessage.error_message = error?.message || t('chat.streamFailed')
        appStore.showError(assistantMessage.error_message || t('chat.streamFailed'))
      }
    }
  } finally {
    isStreaming.value = false
    streamAbortController.value = null
    await loadMessages(conversation.id)
    await loadConversations({ preserveSelection: true })
  }
}

function extractCompletedText(output: Array<Record<string, any>>) {
  const blocks: string[] = []
  for (const item of output) {
    if (item.role !== 'assistant' || !Array.isArray(item.content)) continue
    for (const content of item.content) {
      if (content.type === 'output_text' && typeof content.text === 'string' && content.text.trim()) {
        blocks.push(content.text)
      }
    }
  }
  return blocks.join('\n\n')
}

function cloneChatMessages(source: ChatMessage[]) {
  return source.map((message) => ({
    ...message,
    attachment_ids: Array.isArray(message.attachment_ids) ? [...message.attachment_ids] : [],
    attachments: Array.isArray(message.attachments)
      ? message.attachments.map((attachment) => ({ ...attachment }))
      : []
  }))
}

function stopStreaming() {
  streamAbortController.value?.abort()
}

watch(activeConversationId, (id) => {
  updateRouteConversation(id)
})

onMounted(async () => {
  try {
    pageLoading.value = true
    await loadAPIKeys()
    if (selectedApiKeyId.value) {
      await loadModels(selectedApiKeyId.value)
    }
    await loadConversations()
    if (!conversations.value.length && canCreateConversation.value) {
      const conversation = await handleCreateConversation()
      if (conversation) {
        await loadMessages(conversation.id)
      }
    }
  } catch (error: any) {
    appStore.showError(error.message || t('chat.loadFailed'))
  } finally {
    pageLoading.value = false
  }
})
</script>

<style scoped>
.chat-markdown :deep(p) {
  margin-bottom: 0.85rem;
  line-height: 1.8;
}

.chat-markdown :deep(p:last-child) {
  margin-bottom: 0;
}

.chat-markdown :deep(pre) {
  margin: 0;
  padding: 0;
  background: transparent;
}

.chat-markdown :deep(code) {
  border-radius: 0.5rem;
}

.chat-markdown :deep(img) {
  border-radius: 1rem;
}

.chat-markdown :deep(.chat-code-shell) {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 1rem;
  background: #0b1220;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
}

.chat-markdown :deep(.chat-code-header) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.65rem 1rem;
  border-bottom: 1px solid rgba(148, 163, 184, 0.14);
  background: rgba(15, 23, 42, 0.98);
}

.chat-markdown :deep(.chat-code-lang) {
  font-size: 0.72rem;
  font-weight: 700;
  letter-spacing: 0.18em;
  color: #94a3b8;
}

.chat-markdown :deep(.chat-code-block) {
  overflow-x: auto;
  margin: 0;
  padding: 1rem 1.1rem 1.15rem;
  background: #0b1220;
}

.chat-markdown :deep(.chat-code-body) {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
}

.chat-markdown :deep(.chat-code-gutter) {
  padding: 1rem 0 1.15rem 0.9rem;
  border-right: 1px solid rgba(148, 163, 184, 0.14);
  background: rgba(15, 23, 42, 0.98);
  user-select: none;
}

.chat-markdown :deep(.chat-code-line-number) {
  display: block;
  min-width: 2.75rem;
  padding-right: 0.8rem;
  text-align: right;
  color: #475569;
  font-size: 0.78rem;
  line-height: 1.7;
  font-family: "SFMono-Regular", "JetBrains Mono", "Fira Code", "Menlo", "Monaco", monospace;
}

.chat-markdown :deep(.chat-code) {
  display: block;
  color: #e2e8f0;
  font-size: 0.875rem;
  line-height: 1.7;
  white-space: pre;
  font-family: "SFMono-Regular", "JetBrains Mono", "Fira Code", "Menlo", "Monaco", monospace;
}

.chat-markdown :deep(.chat-code-line) {
  display: block;
  min-height: 1.7em;
}

.chat-markdown :deep(.chat-token.comment) {
  color: #64748b;
}

.chat-markdown :deep(.chat-token.string) {
  color: #fbbf24;
}

.chat-markdown :deep(.chat-token.number) {
  color: #f472b6;
}

.chat-markdown :deep(.chat-token.keyword) {
  color: #8b5cf6;
}

.chat-markdown :deep(.chat-token.function) {
  color: #60a5fa;
}

.chat-markdown :deep(.chat-token.operator) {
  color: #cbd5e1;
}
</style>
