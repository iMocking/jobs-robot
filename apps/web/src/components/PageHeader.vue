<script setup>
/**
 * 后台页面标题栏：左侧标题 + 描述，右侧操作区。
 * 所有后台页面统一用它起头，保证纵向节奏一致。
 */
defineProps({
  title: { type: String, required: true },
  description: { type: String, default: '' },
})
</script>

<template>
  <div class="mb-5 flex flex-wrap items-start justify-between gap-4">
    <div>
      <h1 class="text-[20px] font-semibold text-ink-900">{{ title }}</h1>
      <p v-if="description" class="mt-1 text-[13px] text-ink-500">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="flex flex-wrap items-center gap-2.5">
      <slot name="actions" />
    </div>
  </div>
</template>
