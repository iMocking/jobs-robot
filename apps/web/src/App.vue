<script setup>
import { RouterView } from 'vue-router'

import AppToaster from '@/components/AppToaster.vue'

/**
 * 根组件。
 * 只做两件事：渲染当前外壳，以及挂一个全局提示条。
 * 外壳的切换（前台 / 认证 / 后台）完全由路由的顶级记录决定。
 */
</script>

<template>
  <RouterView />
  <AppToaster />
</template>
