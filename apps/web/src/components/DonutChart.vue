<script setup>
import { computed } from 'vue'

/**
 * 环形图（岗位状态分布）。
 *
 * 手写 SVG 而不是引入图表库：这里只需要一个环形 + 引线百分比，
 * 图表库的体积与默认样式（tooltip、图例、动画）反而是负担。
 * 颜色与 @theme 中的语义色保持一致。
 *
 * 画布留白的取舍：外圈引线要占到环外约 30px，左右还要放得下「65%」这样的文本，
 * 因此 viewBox 比环本身宽出不少（360×320 装一个半径 76 的环）。
 * 早期版本把环画满 300×300，左侧凸出的百分比会被 viewBox 直接裁掉 —— 表现为只剩一个「%」。
 */
const props = defineProps({
  items: { type: Array, default: () => [] }, // [{ label, count, tone }]
  thickness: { type: Number, default: 26 },
})

const TONE_HEX = {
  brand: '#2563eb',
  accent: '#10b981',
  amber: '#f59e0b',
  danger: '#ef4444',
  ink: '#d1d5db',
  violet: '#7c3aed',
}

const WIDTH = 360
const HEIGHT = 320
const CENTER_X = WIDTH / 2
const CENTER_Y = HEIGHT / 2
const RADIUS = 76
const CIRCUMFERENCE = 2 * Math.PI * RADIUS

const total = computed(() => props.items.reduce((sum, item) => sum + (item.count || 0), 0))

/** 每段弧的 dash 参数与百分比引线坐标 */
const segments = computed(() => {
  const sum = total.value || 1
  let cursor = 0

  return props.items.map((item) => {
    const fraction = (item.count || 0) / sum
    const start = cursor
    cursor += fraction

    // 从 12 点方向顺时针铺开
    const angle = (start + fraction / 2) * 360 - 90
    const rad = (angle * Math.PI) / 180
    const cos = Math.cos(rad)
    const sin = Math.sin(rad)

    const inner = RADIUS + props.thickness / 2 + 4
    const outer = inner + 13
    const p1 = { x: CENTER_X + cos * inner, y: CENTER_Y + sin * inner }
    const p2 = { x: CENTER_X + cos * outer, y: CENTER_Y + sin * outer }
    const elbow = cos >= 0 ? 14 : -14

    return {
      ...item,
      percent: Math.round(fraction * 100),
      color: TONE_HEX[item.tone] ?? TONE_HEX.brand,
      dash: `${fraction * CIRCUMFERENCE} ${CIRCUMFERENCE}`,
      offset: -start * CIRCUMFERENCE,
      leader: { p1, p2, p3: { x: p2.x + elbow, y: p2.y } },
      anchor: cos >= 0 ? 'start' : 'end',
      textX: p2.x + elbow + (cos >= 0 ? 5 : -5),
      // 计数为 0 的段不画引线与百分比：角度为零时它们会全部堆在同一处，
      // 表现为两个标签重叠（例如「8% 0%」挤在一起），既难看也读不出信息
      showLeader: (item.count || 0) > 0,
    }
  })
})
</script>

<template>
  <div class="flex flex-col items-center gap-6 sm:flex-row sm:gap-7">
    <svg
      :viewBox="`0 0 ${WIDTH} ${HEIGHT}`"
      class="w-full max-w-[280px] shrink-0"
      role="img"
      aria-label="岗位状态分布"
    >
      <g :transform="`rotate(-90 ${CENTER_X} ${CENTER_Y})`">
        <circle
          :cx="CENTER_X"
          :cy="CENTER_Y"
          :r="RADIUS"
          fill="none"
          stroke="#f3f4f6"
          :stroke-width="thickness"
        />
        <circle
          v-for="segment in segments"
          :key="segment.label"
          :cx="CENTER_X"
          :cy="CENTER_Y"
          :r="RADIUS"
          fill="none"
          :stroke="segment.color"
          :stroke-width="thickness"
          :stroke-dasharray="segment.dash"
          :stroke-dashoffset="segment.offset"
          stroke-linecap="butt"
        />
      </g>

      <g v-for="segment in segments" :key="`leader-${segment.label}`">
        <template v-if="segment.showLeader">
          <polyline
            :points="`${segment.leader.p1.x},${segment.leader.p1.y} ${segment.leader.p2.x},${segment.leader.p2.y} ${segment.leader.p3.x},${segment.leader.p3.y}`"
            fill="none"
            :stroke="segment.color"
            stroke-width="1.2"
            stroke-linejoin="round"
          />
          <text
            :x="segment.textX"
            :y="segment.leader.p3.y + 4"
            :text-anchor="segment.anchor"
            class="fill-ink-500 text-[12.5px]"
          >
            {{ segment.percent }}%
          </text>
        </template>
      </g>
    </svg>

    <ul class="w-full space-y-3">
      <li v-for="segment in segments" :key="segment.label" class="flex items-center gap-2.5 text-sm">
        <span class="size-2.5 shrink-0 rounded-sm" :style="{ backgroundColor: segment.color }" />
        <span class="text-ink-600">{{ segment.label }}</span>
        <span class="tabular ml-auto font-medium text-ink-900">{{ segment.count }}</span>
      </li>
    </ul>
  </div>
</template>
