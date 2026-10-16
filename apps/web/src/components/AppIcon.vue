<script setup>
import { computed } from 'vue'

/**
 * AppIcon —— 全站唯一的图标来源。
 *
 * 刻意不引入图标库：设计稿需要的图标数量有限，手写 SVG 可以保证
 * 线宽、圆角与视觉重心完全统一（stroke-linecap/linejoin 统一 round），
 * 也不会为了几个图标把整包图标字体或 tree-shaking 不可靠的库装进来。
 *
 * filled 类图标（品牌图形、分页点）用 currentColor 填充，其余一律描边。
 */
const props = defineProps({
  name: { type: String, required: true },
  size: { type: [Number, String], default: 20 },
  strokeWidth: { type: [Number, String], default: 1.7 },
})

const ICONS = {
  // —— 品牌与导航 ——
  briefcase: {
    body: '<rect x="3" y="7.6" width="18" height="12.2" rx="2.6" /><path d="M9 7.6V6.2A2.2 2.2 0 0 1 11.2 4h1.6A2.2 2.2 0 0 1 15 6.2v1.4" /><path d="M3 12.6h18" />',
  },
  search: { body: '<circle cx="10.8" cy="10.8" r="6.6" /><path d="m15.8 15.8 4.6 4.6" />' },
  gauge: {
    body: '<path d="M3.6 18.8a8.9 8.9 0 1 1 16.8 0" /><path d="m12 14.2 4.2-4.4" /><circle cx="12" cy="15.4" r="1.5" />',
  },
  clipboard: {
    body: '<path d="M9 4.6H7.6a2.1 2.1 0 0 0-2.1 2.1v12.2a2.1 2.1 0 0 0 2.1 2.1h8.8a2.1 2.1 0 0 0 2.1-2.1V6.7a2.1 2.1 0 0 0-2.1-2.1H15" /><rect x="9" y="2.9" width="6" height="3.6" rx="1.3" /><path d="m9.6 13.4 1.7 1.7 3.3-3.4" />',
  },
  building: {
    body: '<path d="M4 20.6V5.4A1.4 1.4 0 0 1 5.4 4h7.2A1.4 1.4 0 0 1 14 5.4v15.2" /><path d="M14 20.6V9.8h4.6A1.4 1.4 0 0 1 20 11.2v9.4" /><path d="M3 20.6h18" /><path d="M7.2 7.8h1.8M7.2 11.4h1.8M7.2 15h1.8M10.8 7.8h1.6M10.8 11.4h1.6M10.8 15h1.6" />',
  },
  users: {
    body: '<circle cx="9.4" cy="8.4" r="3.4" /><path d="M3.2 19.8a6.2 6.2 0 0 1 12.4 0" /><path d="M16.4 5.6a3.4 3.4 0 0 1 0 6.5" /><path d="M17.9 13.9a6.2 6.2 0 0 1 3 5.9" />',
  },
  'shield-check': {
    body: '<path d="M12 3.2 5.5 5.6v5.7c0 4.2 2.7 7.4 6.5 8.8 3.8-1.4 6.5-4.6 6.5-8.8V5.6L12 3.2Z" /><path d="m9.3 11.6 1.9 2 3.6-3.8" />',
  },
  sliders: {
    body: '<path d="M4 7.5h8.5M17 7.5h3M4 16.5h3M11.5 16.5H20" /><circle cx="15" cy="7.5" r="2.1" /><circle cx="9.2" cy="16.5" r="2.1" />',
  },
  activity: { body: '<path d="M3 12.4h3.6l2.5-6.4 4.2 12.6 2.4-6.2H21" />' },
  document: {
    body: '<path d="M14 3.6H7.6a2.1 2.1 0 0 0-2.1 2.1v12.6a2.1 2.1 0 0 0 2.1 2.1h8.8a2.1 2.1 0 0 0 2.1-2.1V8.1L14 3.6Z" /><path d="M13.8 3.6v4.5h4.6" /><path d="M8.6 12.6h6.8M8.6 16.1h4.4" />',
  },

  // —— 信息维度：工作地址 / 来源 / 时间 / 联系方式 ——
  'map-pin': {
    body: '<path d="M12 20.8s6.4-5.2 6.4-10A6.4 6.4 0 0 0 5.6 10.8c0 4.8 6.4 10 6.4 10Z" /><circle cx="12" cy="10.6" r="2.3" />',
  },
  globe: {
    body: '<circle cx="12" cy="12" r="8.6" /><path d="M3.5 9.4h17M3.5 14.6h17" /><path d="M12 3.4c2.2 2.3 3.4 5.3 3.4 8.6S14.2 18.3 12 20.6C9.8 18.3 8.6 15.3 8.6 12S9.8 5.7 12 3.4Z" />',
  },
  clock: { body: '<circle cx="12" cy="12" r="8.6" /><path d="M12 7.4V12l3.1 2" />' },
  mail: {
    body: '<rect x="3" y="5.6" width="18" height="12.8" rx="2.4" /><path d="m4.2 7.6 6.9 4.8a1.6 1.6 0 0 0 1.8 0l6.9-4.8" />',
  },
  phone: {
    body: '<path d="M6.4 3.6h2.9l1.5 3.9-2 1.4a11.2 11.2 0 0 0 5.9 5.9l1.4-2 3.9 1.5v2.9a2.1 2.1 0 0 1-2.3 2.1A16.7 16.7 0 0 1 4.3 5.9a2.1 2.1 0 0 1 2.1-2.3Z" />',
  },
  calendar: {
    body: '<rect x="3.5" y="5.6" width="17" height="14.9" rx="2.5" /><path d="M3.5 10.4h17M8.6 3.6v4M15.4 3.6v4" />',
  },
  bookmark: { body: '<path d="M6.6 4.4h10.8v15.2L12 16l-5.4 3.6V4.4Z" />' },

  // —— 认证表单 ——
  lock: {
    body: '<rect x="4.6" y="10.1" width="14.8" height="9.9" rx="2.5" /><path d="M8.1 10.1V7.9a3.9 3.9 0 0 1 7.8 0v2.2" />',
  },
  'lock-open': {
    body: '<rect x="4.6" y="10.1" width="14.8" height="9.9" rx="2.5" /><path d="M8.1 10.1V7.9a3.9 3.9 0 0 1 7.4-1.6" />',
  },
  key: {
    body: '<circle cx="8.2" cy="15" r="3.6" /><path d="m10.8 12.5 7.4-7.4" /><path d="m15.6 7.7 2.3 2.3M13.2 10.1l2.3 2.3" />',
  },
  eye: {
    body: '<path d="M2.6 12S6.2 5.9 12 5.9 21.4 12 21.4 12 17.8 18.1 12 18.1 2.6 12 2.6 12Z" /><circle cx="12" cy="12" r="2.9" />',
  },
  'eye-off': {
    body: '<path d="m4.2 4.2 15.6 15.6" /><path d="M9.9 6.1A10.2 10.2 0 0 1 12 5.9c5.8 0 9.4 6.1 9.4 6.1a17.6 17.6 0 0 1-3.3 4.1" /><path d="M6.4 7.9A17 17 0 0 0 2.6 12s3.6 6.1 9.4 6.1a10 10 0 0 0 3.3-.5" /><path d="M10.1 10.1a2.9 2.9 0 0 0 4 4" />',
  },
  shield: {
    body: '<path d="M12 3.2 5.5 5.6v5.7c0 4.2 2.7 7.4 6.5 8.8 3.8-1.4 6.5-4.6 6.5-8.8V5.6L12 3.2Z" />',
  },
  user: { body: '<circle cx="12" cy="8.4" r="3.7" /><path d="M4.8 20.1a7.4 7.4 0 0 1 14.4 0" />' },

  // —— 操作 ——
  plus: { body: '<path d="M12 5.2v13.6M5.2 12h13.6" />' },
  pencil: {
    body: '<path d="M4.6 19.4h3.9l9.9-9.9a2.1 2.1 0 0 0-3-3l-9.9 9.9v3Z" /><path d="m14.3 6.6 3 3" />',
  },
  trash: {
    body: '<path d="M4.6 6.6h14.8" /><path d="M9.6 6.6V4.9a1.3 1.3 0 0 1 1.3-1.3h2.2a1.3 1.3 0 0 1 1.3 1.3v1.7" /><path d="m6.6 6.6.8 12.1a1.9 1.9 0 0 0 1.9 1.7h5.4a1.9 1.9 0 0 0 1.9-1.7l.8-12.1" /><path d="M10.5 10.4v6.1M13.5 10.4v6.1" />',
  },
  refresh: {
    body: '<path d="M20.4 12a8.4 8.4 0 1 1-2.6-6.1" /><path d="M20.4 4.4v5.7h-5.6" />',
  },
  logout: {
    body: '<path d="M9.4 20.4H6.6a2.1 2.1 0 0 1-2.1-2.1V5.7a2.1 2.1 0 0 1 2.1-2.1h2.8" /><path d="M15.6 16.4 20 12l-4.4-4.4" /><path d="M20 12H9.4" />',
  },
  'external-link': {
    body: '<path d="M14.2 4.6h5.2v5.2" /><path d="M19.4 4.6 11 13" /><path d="M17.8 14v4.5a2.1 2.1 0 0 1-2.1 2.1H6.1A2.1 2.1 0 0 1 4 18.5V8.9a2.1 2.1 0 0 1 2.1-2.1H10" />',
  },
  filter: { body: '<path d="M4 6.6h16l-6.2 7.2v5.4l-3.6-2v-3.4L4 6.6Z" />' },
  menu: { body: '<path d="M4 7h16M4 12h16M4 17h16" />' },
  x: { body: '<path d="M6.2 6.2 17.8 17.8M17.8 6.2 6.2 17.8" />' },
  check: { body: '<path d="m5.2 12.6 4.5 4.5L18.8 7" />' },
  'check-circle': {
    body: '<circle cx="12" cy="12" r="8.6" /><path d="m8.4 12.2 2.5 2.5 4.7-4.9" />',
  },
  info: { body: '<circle cx="12" cy="12" r="8.6" /><path d="M12 11.2v5.4M12 7.7v.1" />' },
  alert: { body: '<circle cx="12" cy="12" r="8.6" /><path d="M12 7.6v5.5M12 16.3v.1" />' },
  'arrow-right': { body: '<path d="M4.6 12h14.8" /><path d="m13.2 5.8 6.2 6.2-6.2 6.2" />' },
  'arrow-left': { body: '<path d="M19.4 12H4.6" /><path d="m10.8 5.8-6.2 6.2 6.2 6.2" />' },
  'chevron-down': { body: '<path d="m6.4 9.6 5.6 5.6 5.6-5.6" />' },
  'chevron-left': { body: '<path d="M14.4 6 8.4 12l6 6" />' },
  'chevron-right': { body: '<path d="m9.6 6 6 6-6 6" />' },

  // —— 品牌图形：刻意用通用图形表示，不复制真实商标 ——
  wechat: {
    body: '<path d="M11.8 4.2c-4.5 0-8.2 2.9-8.2 6.5 0 2.1 1.2 3.9 3.1 5.1l-.8 2.5 2.8-1.4c1 .3 2 .4 3.1.4 4.5 0 8.2-2.9 8.2-6.6S16.3 4.2 11.8 4.2Z" /><circle cx="9.1" cy="9.9" r="1.15" fill="currentColor" stroke="none" /><circle cx="14.5" cy="9.9" r="1.15" fill="currentColor" stroke="none" />',
  },
  douyin: {
    body: '<path d="M14.6 4.4v9.9a3.7 3.7 0 1 1-3.7-3.7c.3 0 .6 0 .9.1" /><path d="M14.6 4.4c.4 2.4 2 4 4.4 4.2" />',
  },
  dots: {
    filled: true,
    body: '<circle cx="12" cy="5.6" r="1.5" /><circle cx="12" cy="12" r="1.5" /><circle cx="12" cy="18.4" r="1.5" />',
  },
  bolt: { body: '<path d="M13.4 3.4 5.6 13.4h5.2l-.9 7.2 7.8-10h-5.2l.9-7.2Z" />' },
}

const icon = computed(() => ICONS[props.name] ?? ICONS.info)
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    :fill="icon.filled ? 'currentColor' : 'none'"
    :stroke="icon.filled ? 'none' : 'currentColor'"
    :stroke-width="strokeWidth"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    class="shrink-0"
    v-html="icon.body"
  />
</template>
