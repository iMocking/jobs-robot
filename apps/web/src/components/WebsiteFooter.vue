<script setup>
import { RouterLink } from 'vue-router'

import { USE_MOCK } from '@/api/http'

/**
 * 前台页脚。
 * 设计稿右侧是「帮助中心 / 数据来源说明 / 企业入驻」三个入口；
 * 演示数据模式下额外标注一行提示，避免把演示岗位误当成真实招聘信息。
 */
const LINKS = [
  { label: '帮助中心', name: 'about' },
  { label: '数据来源说明', name: 'about', hash: '#source' },
  { label: '企业入驻', name: 'about', hash: '#join' },
]
</script>

<template>
  <footer class="mt-auto border-t border-ink-100 bg-white">
    <div class="container-page flex flex-col items-center justify-between gap-3 py-6 sm:flex-row">
      <p class="text-[13px] text-ink-400">
        © 2026 JobHub 直招 · 聚合企业官网《加入我们》岗位，求职信息真实可溯源
        <span v-if="USE_MOCK" class="ml-1 text-ink-300">（界面演示数据）</span>
      </p>
      <nav class="flex items-center gap-6">
        <RouterLink
          v-for="link in LINKS"
          :key="link.label"
          :to="{ name: link.name, hash: link.hash }"
          class="text-[13px] text-ink-500 transition hover:text-brand-600"
        >
          {{ link.label }}
        </RouterLink>
      </nav>
    </div>
  </footer>
</template>
