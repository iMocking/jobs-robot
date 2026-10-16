<script setup>
import { computed } from 'vue'

/**
 * 柱状图（近 7 日抓取趋势）。
 *
 * 用 DOM + flex 而不是 SVG：柱子宽度自适应容器，标签换行由浏览器处理，
 * 比在 SVG 里手算文字位置稳。
 */
const props = defineProps({
  items: { type: Array, default: () => [] }, // [{ label, value }]
  height: { type: Number, default: 200 },
})

/** 取一个「好看的」上限，让 4 条网格线落在整数刻度上 */
const axisMax = computed(() => {
  const max = Math.max(...props.items.map((item) => item.value || 0), 1)
  const magnitude = 10 ** Math.floor(Math.log10(max))
  const step = [1, 2, 2.5, 5, 10].map((m) => m * magnitude).find((s) => s * 4 >= max) ?? magnitude * 10
  return { max: step * 4, ticks: [4, 3, 2, 1, 0].map((i) => step * i) }
})
</script>

<template>
  <div class="flex gap-4">
    <!-- 纵轴刻度 -->
    <div
      class="flex flex-col justify-between pt-px pb-7 text-right text-[11.5px] text-ink-400 tabular"
      :style="{ height: `${height + 28}px` }"
    >
      <span v-for="tick in axisMax.ticks" :key="tick">{{ tick }}</span>
    </div>

    <div class="min-w-0 flex-1">
      <div class="relative" :style="{ height: `${height}px` }">
        <!-- 网格线 -->
        <div class="absolute inset-0 flex flex-col justify-between">
          <div v-for="tick in axisMax.ticks" :key="tick" class="h-px w-full bg-ink-100" />
        </div>

        <!-- 柱体 -->
        <div class="relative flex h-full items-end justify-between gap-2">
          <div
            v-for="item in items"
            :key="item.label"
            class="group flex h-full min-w-0 flex-1 flex-col items-center justify-end"
          >
            <span class="tabular mb-1.5 text-[12px] font-medium text-ink-600">{{ item.value }}</span>
            <div
              class="w-full max-w-[46px] rounded-t-md bg-brand-600 transition group-hover:bg-brand-500"
              :style="{ height: `${Math.max(2, ((item.value || 0) / axisMax.max) * (height - 26))}px` }"
              :title="`${item.label}：${item.value}`"
            />
          </div>
        </div>
      </div>

      <!-- 横轴标签 -->
      <div class="mt-2 flex justify-between gap-2">
        <span
          v-for="item in items"
          :key="`label-${item.label}`"
          class="min-w-0 flex-1 truncate text-center text-[11.5px] text-ink-400"
        >
          {{ item.label }}
        </span>
      </div>
    </div>
  </div>
</template>
