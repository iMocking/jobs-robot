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
  /**
   * 圆形形态：用户头像用（与后台顶栏头像同款 rounded-full），
   * 公司/职位头像保持圆角方形，避免「官网直招」语境被圆形弱化。
   */
  circle: { type: Boolean, default: false },
})

const SIZE_CLASS = {
  sm: { base: 'size-8 text-[13px]', radius: 'rounded-lg' },
  md: { base: 'size-11 text-[17px]', radius: 'rounded-xl' },
  lg: { base: 'size-14 text-[22px]', radius: 'rounded-2xl' },
  xl: { base: 'size-[68px] text-[30px]', radius: 'rounded-2xl' },
}

const sizeClass = computed(() => {
  const item = SIZE_CLASS[props.size] ?? SIZE_CLASS.md
  return `${item.base} ${props.circle ? 'rounded-full' : item.radius}`
})

const label = computed(() => props.initial || props.name.slice(0, 1).toUpperCase() || '?')
</script>

<template>
  <span
    class="inline-flex shrink-0 items-center justify-center font-semibold text-white"
    :class="[sizeClass, AVATAR_TONE[tone] ?? AVATAR_TONE.brand]"
    :title="name || undefined"
  >
    {{ label }}
  </span>
</template>
