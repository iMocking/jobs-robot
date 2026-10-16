<script setup>
/**
 * 信任徽章「100% 企业官网《加入我们》直招 · 信息真实可溯源」。
 * 设计稿里出现在两处，形态不同但语义唯一，因此抽成一个组件两种变体：
 *   glass —— Hero 深蓝渐变上的半透明白胶囊
 *   green —— 认证页品牌区上的绿色胶囊（绿色在本产品里专属于「直招」）
 */
defineProps({
  variant: { type: String, default: 'glass' }, // glass | green
})
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-full px-4 py-2 text-[13.5px] font-medium"
    :class="
      variant === 'green'
        ? 'bg-accent-100 text-accent-700 ring-1 ring-accent-200'
        : 'bg-white/12 text-white ring-1 ring-white/25 backdrop-blur-sm'
    "
  >
    <slot>100% 企业官网《加入我们》直招 · 信息真实可溯源</slot>
  </span>
</template>
