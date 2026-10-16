<script setup>
import { RouterView } from 'vue-router'

import AuthBrandPanel from '@/components/AuthBrandPanel.vue'

/**
 * 认证外壳：左品牌墙 + 右表单区。
 * 与前台/后台并列作为第三个顶级分支，是因为设计稿里的登录类页面
 * 并不带站点导航与页脚，硬塞进 LayWebsite 会让外壳充满条件判断。
 */
</script>

<template>
  <div class="grid min-h-screen lg:grid-cols-2">
    <AuthBrandPanel />

    <main class="flex items-center justify-center bg-[#f6f7fb] px-5 py-10 sm:px-8 lg:py-14">
      <div class="w-full max-w-[520px]">
        <RouterView />
      </div>
    </main>
  </div>
</template>
