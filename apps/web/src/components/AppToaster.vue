<script setup>
import { storeToRefs } from 'pinia'

import AppIcon from '@/components/AppIcon.vue'
import { useToastStore } from '@/stores/toast'

const toast = useToastStore()
const { items } = storeToRefs(toast)

const TONE = {
  success: { icon: 'check-circle', cls: 'border-accent-200 bg-accent-50 text-accent-700' },
  error: { icon: 'alert', cls: 'border-red-200 bg-red-50 text-red-700' },
  info: { icon: 'info', cls: 'border-brand-200 bg-brand-50 text-brand-700' },
}
</script>

<template>
  <div class="pointer-events-none fixed top-5 left-1/2 z-[100] flex w-full max-w-[440px] -translate-x-1/2 flex-col gap-2 px-4">
    <TransitionGroup
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="-translate-y-2 opacity-0"
      leave-active-class="transition duration-150 ease-in"
      leave-to-class="-translate-y-1 opacity-0"
    >
      <div
        v-for="item in items"
        :key="item.id"
        class="pointer-events-auto flex items-start gap-2.5 rounded-xl border px-4 py-3 text-[13.5px] shadow-panel"
        :class="(TONE[item.tone] ?? TONE.info).cls"
        role="status"
      >
        <AppIcon :name="(TONE[item.tone] ?? TONE.info).icon" :size="17" class="mt-px" />
        <!-- whitespace-pre-line：接口的失败文案可能是多行排查步骤（1) 2) 3)…），
             按 ordinary 空白折叠会挤成一整段，读不出层次 -->
        <span class="flex-1 leading-snug whitespace-pre-line">{{ item.message }}</span>
        <button
          type="button"
          class="rounded p-0.5 opacity-60 transition hover:opacity-100"
          aria-label="关闭"
          @click="toast.dismiss(item.id)"
        >
          <AppIcon name="x" :size="14" />
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>
