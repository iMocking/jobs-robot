<script setup>
import { computed } from 'vue'

import { AVATAR_TONE } from '@/utils/format'

/**
 * 公司与用户头像。
 * 设计稿刻意不使用真实品牌 Logo，统一用「圆角色块 + 首字」，
 * 既回避商标问题，也让不同公司在一屏里保持同样的视觉重量。
 */
const props = defineProps({
  name: { type: String, default: '' },
  initial: { type: String, default: '' },
  tone: { type: String, default: 'brand' },
  size: { type: String, default: 'md' }, // sm | md | lg | xl
})

const SIZES = {
  sm: 'size-8 rounded-lg text-[13px]',
  md: 'size-11 rounded-xl text-[17px]',
  lg: 'size-14 rounded-2xl text-[22px]',
  xl: 'size-[68px] rounded-2xl text-[30px]',
}

const label = computed(() => props.initial || props.name.slice(0, 1).toUpperCase() || '?')
</script>

<template>
  <span
    class="inline-flex shrink-0 items-center justify-center font-semibold text-white"
    :class="[SIZES[size], AVATAR_TONE[tone] ?? AVATAR_TONE.brand]"
    :title="name || undefined"
  >
    {{ label }}
  </span>
</template>
