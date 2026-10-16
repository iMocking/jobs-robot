<script setup>
import AppIcon from '@/components/AppIcon.vue'
import { formatNumber } from '@/utils/format'

/**
 * 统计卡：图标 + 指标名 + 大数字 + 一句口径说明。
 *
 * 设计稿这里原本是「环比 +12%」。后端的 StatsView 只有当前快照，没有历史对比，
 * 编一个百分比等于给运营一个假的判断依据，因此换成 hint ——
 * 只允许写「由同一份 stats 派生的另一个真实数字」（例如 已发布 128 / 启用 42）。
 */
const props = defineProps({
  label: { type: String, required: true },
  value: { type: [Number, String], default: 0 },
  hint: { type: String, default: '' },
  icon: { type: String, default: 'briefcase' },
  alert: { type: Boolean, default: false },
  suffix: { type: String, default: '' },
})
</script>

<template>
  <div class="card p-5">
    <div class="flex items-center gap-2.5">
      <span
        class="flex size-9 items-center justify-center rounded-lg"
        :class="alert ? 'bg-amber-50 text-amber-600' : 'bg-brand-50 text-brand-600'"
      >
        <AppIcon :name="icon" :size="18" />
      </span>
      <span class="text-[13.5px] text-ink-500">{{ label }}</span>
    </div>

    <div class="mt-4 flex items-end justify-between gap-3">
      <p class="tabular text-[30px] leading-none font-bold text-ink-900">
        {{ typeof value === 'number' ? formatNumber(value) : value }}
        <span v-if="suffix" class="text-[15px] font-medium text-ink-500">{{ suffix }}</span>
      </p>

      <p v-if="hint" class="shrink-0 text-right text-[12.5px] leading-snug text-ink-400">
        {{ hint }}
      </p>
    </div>
  </div>
</template>
