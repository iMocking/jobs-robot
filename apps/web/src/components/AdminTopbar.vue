<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AppIcon from '@/components/AppIcon.vue'
import { useSessionStore } from '@/stores/session'
import { userInitial } from '@/utils/format'

/**
 * 后台顶栏：面包屑 + 刷新 + 管理员菜单。
 * 面包屑直接由当前路由的 meta.title 推导（去掉「· JobHub 直招」后缀），
 * 不再让每个页面自己传一遍标题，避免标题两处维护。
 *
 * 设计稿这里还有一个全局搜索框，但后端没有跨资源搜索接口 ——
 * 关键词筛选是各列表自己的参数（/admin/jobs?keyword=、/admin/users?keyword=、
 * /admin/companies?keyword=），范围与权限各不相同。摆一个永远搜不出结果的输入框
 * 比没有更糟，所以移除；要搜索请进入对应列表页。
 */
const props = defineProps({
  /** 关闭刷新按钮（例如纯静态页面） */
  refreshable: { type: Boolean, default: true },
})

const emit = defineEmits(['refresh'])

const route = useRoute()
const router = useRouter()
const session = useSessionStore()

const menuOpen = ref(false)

const pageTitle = computed(() => String(route.meta?.title ?? '').split(' · ')[0] || '仪表盘')

async function logout() {
  menuOpen.value = false
  await session.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <header class="flex h-16 shrink-0 items-center gap-4 border-b border-ink-200/70 bg-white px-6">
    <nav class="flex items-center gap-2 text-[13.5px]">
      <RouterLink :to="{ name: 'dashboard' }" class="text-ink-400 transition hover:text-brand-600">
        首页
      </RouterLink>
      <span class="text-ink-300">/</span>
      <span class="font-medium text-ink-800">{{ pageTitle }}</span>
    </nav>

    <div class="ml-auto flex items-center gap-3">
      <button
        v-if="refreshable"
        type="button"
        class="inline-flex h-9 items-center gap-1.5 rounded-full bg-white px-4 text-[13px] text-ink-600 ring-1 ring-ink-200 transition hover:text-brand-600 hover:ring-brand-300"
        @click="emit('refresh')"
      >
        <AppIcon name="refresh" :size="15" />
        刷新
      </button>

      <div class="relative">
        <button
          type="button"
          class="flex size-9 items-center justify-center rounded-full bg-brand-600 text-[14px] font-semibold text-white transition hover:bg-brand-700"
          :aria-expanded="menuOpen"
          aria-haspopup="menu"
          @click="menuOpen = !menuOpen"
        >
          {{ userInitial(session.user) }}
        </button>

        <template v-if="menuOpen">
          <div class="fixed inset-0 z-10" @click="menuOpen = false" />
          <div
            class="absolute right-0 z-20 mt-2 w-56 overflow-hidden rounded-xl border border-ink-200 bg-white py-1.5 shadow-panel"
            role="menu"
          >
            <div class="border-b border-ink-100 px-4 py-2.5">
              <p class="truncate text-[13.5px] font-medium text-ink-800">
                {{ session.user?.nickname || '管理员' }}
              </p>
              <p class="truncate text-[12px] text-ink-400">{{ session.user?.email }}</p>
            </div>
            <RouterLink
              :to="{ name: 'home' }"
              class="flex items-center gap-2.5 px-4 py-2.5 text-[13.5px] text-ink-600 transition hover:bg-ink-50"
              role="menuitem"
              @click="menuOpen = false"
            >
              <AppIcon name="external-link" :size="16" />
              访问前台
            </RouterLink>
            <button
              type="button"
              class="flex w-full items-center gap-2.5 px-4 py-2.5 text-[13.5px] text-red-600 transition hover:bg-red-50"
              role="menuitem"
              @click="logout"
            >
              <AppIcon name="logout" :size="16" />
              退出登录
            </button>
          </div>
        </template>
      </div>
    </div>
  </header>
</template>
