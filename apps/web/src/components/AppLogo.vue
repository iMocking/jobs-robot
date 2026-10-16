<script setup>
import { computed } from 'vue'

import AppIcon from '@/components/AppIcon.vue'

/**
 * 品牌标识：蓝色圆角方块 + 公文包图形 + 「JobHub 直招」字标。
 * 三个外壳（前台 / 认证 / 后台）共用，保证跨屏一致。
 */
const props = defineProps({
  size: { type: String, default: 'md' }, // sm | md | lg
  /** light = 深色背景上使用（字标转白） */
  variant: { type: String, default: 'dark' },
  subtitle: { type: String, default: '' },
})

const SIZES = {
  sm: { box: 'size-8 rounded-lg', icon: 17, text: 'text-[15px]' },
  md: { box: 'size-9 rounded-[10px]', icon: 19, text: 'text-[19px]' },
  lg: { box: 'size-14 rounded-2xl', icon: 30, text: 'text-[28px]' },
}

const style = computed(() => SIZES[props.size] ?? SIZES.md)
</script>

<template>
  <span class="inline-flex items-center gap-2.5">
    <span
      class="inline-flex shrink-0 items-center justify-center bg-brand-600 text-white shadow-sm"
      :class="style.box"
    >
      <AppIcon name="briefcase" :size="style.icon" :stroke-width="1.9" />
    </span>
    <span class="flex flex-col leading-none">
      <span
        class="font-bold tracking-tight"
        :class="[style.text, variant === 'light' ? 'text-white' : 'text-ink-900']"
      >
        JobHub<span class="ml-1 font-semibold">直招</span>
      </span>
      <span v-if="subtitle" class="mt-1 text-[11.5px] text-ink-400">{{ subtitle }}</span>
    </span>
  </span>
</template>
