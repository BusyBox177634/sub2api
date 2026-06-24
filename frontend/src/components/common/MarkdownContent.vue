<template>
  <article class="markdown-content" v-html="renderedHtml"></article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'

const props = withDefaults(defineProps<{
  content?: string | null
}>(), {
  content: ''
})

const renderedHtml = computed(() => {
  const html = marked.parse(props.content || '') as string
  return DOMPurify.sanitize(html)
})
</script>

<style scoped>
.markdown-content {
  @apply max-w-none text-[15px] leading-7 text-gray-700 dark:text-gray-300;
}

.markdown-content :deep(h1) {
  @apply mb-5 mt-7 border-b border-gray-200 pb-3 text-2xl font-bold leading-tight text-gray-900 dark:border-dark-700 dark:text-white;
}

.markdown-content :deep(h2) {
  @apply mb-4 mt-6 border-b border-gray-100 pb-2 text-xl font-semibold leading-tight text-gray-900 dark:border-dark-700 dark:text-white;
}

.markdown-content :deep(h3) {
  @apply mb-3 mt-5 text-lg font-semibold leading-snug text-gray-900 dark:text-white;
}

.markdown-content :deep(h4) {
  @apply mb-2 mt-4 text-base font-semibold text-gray-900 dark:text-white;
}

.markdown-content :deep(p) {
  @apply mb-4;
}

.markdown-content :deep(p:last-child) {
  @apply mb-0;
}

.markdown-content :deep(ul),
.markdown-content :deep(ol) {
  @apply mb-4 space-y-1 pl-6;
}

.markdown-content :deep(ul) {
  @apply list-disc;
}

.markdown-content :deep(ol) {
  @apply list-decimal;
}

.markdown-content :deep(li) {
  @apply pl-1;
}

.markdown-content :deep(li::marker) {
  @apply text-primary-600 dark:text-primary-300;
}

.markdown-content :deep(a) {
  @apply font-medium text-primary-600 underline underline-offset-2 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200;
}

.markdown-content :deep(blockquote) {
  @apply my-4 border-l-4 border-primary-300 bg-primary-50/60 py-3 pl-4 pr-3 text-gray-700 dark:border-primary-700 dark:bg-primary-950/20 dark:text-gray-300;
}

.markdown-content :deep(strong) {
  @apply font-semibold text-gray-900 dark:text-white;
}

.markdown-content :deep(em) {
  @apply italic text-gray-600 dark:text-gray-400;
}

.markdown-content :deep(code) {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[13px] text-pink-700 dark:bg-dark-700 dark:text-pink-300;
}

.markdown-content :deep(pre) {
  @apply my-4 overflow-x-auto rounded-lg border border-gray-200 bg-gray-950 p-4 text-gray-100 dark:border-dark-600 dark:bg-dark-950;
}

.markdown-content :deep(pre code) {
  @apply bg-transparent p-0 text-[13px] text-inherit;
}

.markdown-content :deep(table) {
  @apply my-4 w-full border-collapse overflow-hidden rounded-lg text-sm;
}

.markdown-content :deep(th),
.markdown-content :deep(td) {
  @apply border border-gray-200 px-3 py-2 text-left align-top dark:border-dark-600;
}

.markdown-content :deep(th) {
  @apply bg-gray-50 font-semibold text-gray-900 dark:bg-dark-800 dark:text-white;
}

.markdown-content :deep(tbody tr:nth-child(even)) {
  @apply bg-gray-50/60 dark:bg-dark-800/40;
}

.markdown-content :deep(hr) {
  @apply my-6 border-0 border-t border-gray-200 dark:border-dark-700;
}

.markdown-content :deep(img) {
  @apply my-4 max-w-full rounded-lg border border-gray-200 dark:border-dark-600;
}
</style>
