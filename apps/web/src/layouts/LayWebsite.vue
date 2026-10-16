<script setup>
import { RouterView } from 'vue-router'

import WebsiteFooter from '@/components/WebsiteFooter.vue'
import WebsiteHeader from '@/components/WebsiteHeader.vue'

/**
 * 前台外壳：顶部导航 + 内容 + 页脚。
 * 页面组件只关心自己的主体内容，不需要重复拼导航与页脚。
 */
</script>

<template>
  <div class="flex min-h-screen flex-col bg-page">
    <WebsiteHeader />
    <main class="flex-1">
      <RouterView />
    </main>
    <WebsiteFooter />
  </div>
</template>
