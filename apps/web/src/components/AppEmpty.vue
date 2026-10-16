<script setup>
import AppIcon from '@/components/AppIcon.vue'

/** 空状态 / 加载失败态的统一外观，避免每个列表各画一套 */
defineProps({
  icon: { type: String, default: 'search' },
  title: { type: String, default: '暂无数据' },
  description: { type: String, default: '' },
  tone: { type: String, default: 'ink' },
})
</script>

<template>
  <div class="flex flex-col items-center justify-center px-6 py-16 text-center">
    <span
      class="mb-4 flex size-14 items-center justify-center rounded-2xl"
      :class="tone === 'danger' ? 'bg-red-50 text-red-400' : 'bg-ink-100 text-ink-400'"
    >
      <AppIcon :name="icon" :size="26" />
    </span>
    <p class="text-[15px] font-medium text-ink-700">{{ title }}</p>
    <p v-if="description" class="mt-1.5 max-w-md text-[13px] leading-relaxed text-ink-400">
      {{ description }}
    </p>
    <div v-if="$slots.default" class="mt-5">
      <slot />
    </div>
  </div>
</template>
