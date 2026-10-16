<script setup>
import { computed } from 'vue'

import { TONE_RING } from '@/utils/format'

/**
 * 语义标签。
 * 「官网直招」用 accent（绿），状态用 amber/danger/ink，全部走 TONE_RING。
 */
const props = defineProps({
  tone: { type: String, default: 'ink' },
  size: { type: String, default: 'md' }, // sm | md
  dot: { type: Boolean, default: false },
})

const classes = computed(() => [
  'inline-flex items-center gap-1 rounded-md font-medium whitespace-nowrap',
  props.size === 'sm' ? 'px-1.5 py-0.5 text-[11.5px]' : 'px-2 py-1 text-xs',
  TONE_RING[props.tone] ?? TONE_RING.ink,
])

const DOT_TONE = {
  brand: 'bg-brand-500',
  accent: 'bg-accent-500',
  amber: 'bg-amber-500',
  danger: 'bg-red-500',
  ink: 'bg-ink-400',
  violet: 'bg-violet-500',
}
</script>

<template>
  <span :class="classes">
    <span v-if="dot" class="size-1.5 rounded-full" :class="DOT_TONE[tone] ?? DOT_TONE.ink" />
    <slot />
  </span>
</template>
