<script setup>
import { RouterView } from 'vue-router'

import AppToaster from '@/components/AppToaster.vue'
import BackendDownBanner from '@/components/BackendDownBanner.vue'

/**
 * 根组件。
 * 只做三件事：渲染当前外壳、挂全局「后端未启动」红字提示、挂全局提示条。
 * 外壳的切换（前台 / 认证 / 后台）完全由路由的顶级记录决定。
 */
</script>

<template>
  <BackendDownBanner />
  <RouterView />
  <AppToaster />
</template>
