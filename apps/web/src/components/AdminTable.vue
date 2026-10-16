<script setup>
import AppEmpty from '@/components/AppEmpty.vue'

/**
 * 后台数据表外壳。
 * 只负责「卡片 + 表头 + 行分隔 + 空态 + 底部分页槽」，
 * 具体单元格由各页面用 <tr><td class="td-cell"> 写，保留最大灵活度。
 */
defineProps({
  columns: { type: Array, default: () => [] }, // [{ key, label, class? }]
  empty: { type: Boolean, default: false },
  emptyText: { type: String, default: '暂无数据' },
  minWidth: { type: String, default: '760px' },
})
</script>

<template>
  <div class="overflow-hidden rounded-card border border-ink-200/70 bg-white shadow-card">
    <div class="overflow-x-auto">
      <table class="w-full border-collapse text-sm" :style="{ minWidth }">
        <thead>
          <tr class="thead-row">
            <th v-for="column in columns" :key="column.key" class="th-cell" :class="column.class">
              {{ column.label }}
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-ink-100">
          <slot />
        </tbody>
      </table>
    </div>

    <AppEmpty v-if="empty" :title="emptyText" />

    <div v-if="$slots.footer" class="border-t border-ink-100 px-5 py-4">
      <slot name="footer" />
    </div>
  </div>
</template>
