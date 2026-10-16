<script setup>
/**
 * 页签。
 * 系统配置页用它切换「基础设置 / 收集范围 / 爬虫设置」，
 * 视觉沿用设计稿：浅灰底槽 + 白色胶囊高亮。
 */
defineProps({
  modelValue: { type: String, default: '' },
  tabs: { type: Array, default: () => [] }, // [{ key, label, badge? }]
})

defineEmits(['update:modelValue'])
</script>

<template>
  <div class="inline-flex items-center gap-1 rounded-xl bg-ink-100 p-1">
    <button
      v-for="tab in tabs"
      :key="tab.key"
      type="button"
      class="inline-flex items-center gap-1.5 rounded-lg px-4 py-2 text-[13.5px] transition"
      :class="
        tab.key === modelValue
          ? 'bg-white font-medium text-brand-700 shadow-sm'
          : 'text-ink-500 hover:text-ink-800'
      "
      @click="$emit('update:modelValue', tab.key)"
    >
      {{ tab.label }}
      <span
        v-if="tab.badge !== undefined"
        class="rounded bg-ink-100 px-1.5 text-[11px] text-ink-500"
      >
        {{ tab.badge }}
      </span>
    </button>
  </div>
</template>
