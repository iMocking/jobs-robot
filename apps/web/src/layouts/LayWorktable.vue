<script setup>
import { onMounted, provide, ref } from 'vue'
import { RouterView, useRouter } from 'vue-router'

import { adminApi } from '@/api'
import AdminSidebar from '@/components/AdminSidebar.vue'
import AdminTopbar from '@/components/AdminTopbar.vue'
import { ADMIN_REFRESH_KEY } from '@/composables/useAdminRefresh'
import { useSessionStore } from '@/stores/session'

/**
 * 后台外壳：固定深蓝侧边栏 + 顶栏 + 可滚动内容区。
 *
 * 顶栏的「刷新」不做整页重载，而是把一个自增的 tick 通过 provide 下发，
 * 页面用 useAdminRefresh 订阅 —— 这样刷新只重取数据，不丢筛选条件与滚动位置。
 */
const session = useSessionStore()
const router = useRouter()

const refreshTick = ref(0)
const pendingCount = ref(0)

provide(ADMIN_REFRESH_KEY, refreshTick)

function onRefresh() {
  refreshTick.value += 1
}

async function onLogout() {
  await session.logout()
  router.push({ name: 'login' })
}

onMounted(async () => {
  // 侧边栏的待审核角标：静默失败即可，不阻塞后台其余部分。
  // 口径取自 StatsView 的嵌套结构 jobs.pending_review（后端不返回扁平字段）。
  try {
    const stats = await adminApi.stats()
    pendingCount.value = stats?.jobs?.pending_review ?? 0
  } catch {
    pendingCount.value = 0
  }
})
</script>

<template>
  <div class="flex h-screen overflow-hidden bg-page">
    <AdminSidebar :user="session.user ?? {}" :pending-count="pendingCount" @logout="onLogout" />

    <div class="flex min-w-0 flex-1 flex-col">
      <AdminTopbar @refresh="onRefresh" />

      <main class="flex-1 overflow-y-auto">
        <div class="w-full p-2">
          <RouterView />
        </div>
      </main>
    </div>
  </div>
</template>
