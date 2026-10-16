<script setup>
import { computed } from 'vue'

import AppIcon from '@/components/AppIcon.vue'

const props = defineProps({
  page: { type: Number, default: 1 },
  pages: { type: Number, default: 1 },
  total: { type: Number, default: 0 },
  pageSize: { type: Number, default: 10 },
})

const emit = defineEmits(['update:page'])

/** 页码窗口：始终保留首末页，中间用省略号收起，避免 129 页时排出一整行 */
const items = computed(() => {
  const total = Math.max(1, props.pages)
  const current = Math.min(Math.max(1, props.page), total)
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)

  const list = [1]
  const start = Math.max(2, current - 1)
  const end = Math.min(total - 1, current + 1)
  if (start > 2) list.push('gap-start')
  for (let i = start; i <= end; i += 1) list.push(i)
  if (end < total - 1) list.push('gap-end')
  list.push(total)
  return list
})

function go(target) {
  if (typeof target !== 'number') return
  if (target < 1 || target > props.pages || target === props.page) return
  emit('update:page', target)
}
</script>

<template>
  <nav v-if="pages > 1" class="flex items-center justify-center gap-2" aria-label="分页">
    <button
      type="button"
      class="flex size-9 items-center justify-center rounded-lg border border-ink-200 bg-white text-ink-500 transition hover:border-brand-300 hover:text-brand-600 disabled:cursor-not-allowed disabled:opacity-40"
      :disabled="page <= 1"
      aria-label="上一页"
      @click="go(page - 1)"
    >
      <AppIcon name="chevron-left" :size="16" />
    </button>

    <template v-for="item in items" :key="String(item)">
      <span v-if="typeof item !== 'number'" class="px-1 text-ink-400">…</span>
      <button
        v-else
        type="button"
        class="tabular flex h-9 min-w-9 items-center justify-center rounded-lg border px-2.5 text-sm transition"
        :class="
          item === page
            ? 'border-brand-600 bg-brand-600 font-medium text-white'
            : 'border-ink-200 bg-white text-ink-600 hover:border-brand-300 hover:text-brand-600'
        "
        :aria-current="item === page ? 'page' : undefined"
        @click="go(item)"
      >
        {{ item }}
      </button>
    </template>

    <button
      type="button"
      class="flex size-9 items-center justify-center rounded-lg border border-ink-200 bg-white text-ink-500 transition hover:border-brand-300 hover:text-brand-600 disabled:cursor-not-allowed disabled:opacity-40"
      :disabled="page >= pages"
      aria-label="下一页"
      @click="go(page + 1)"
    >
      <AppIcon name="chevron-right" :size="16" />
    </button>

    <span v-if="total" class="ml-2 text-[13px] text-ink-400">共 {{ total }} 条</span>
  </nav>
</template>
