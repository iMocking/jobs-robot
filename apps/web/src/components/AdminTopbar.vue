<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AppIcon from '@/components/AppIcon.vue'
import AppUserMenu from '@/components/AppUserMenu.vue'

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

const pageTitle = computed(() => String(route.meta?.title ?? '').split(' · ')[0] || '仪表盘')

/** 后台菜单项：个人中心进前台账户页；访问前台由顶栏交给 AppUserMenu，点击后跳首页 */
const ADMIN_MENU = [
  { key: 'profile', label: '个人中心', icon: 'user' },
  { key: 'front', label: '访问前台', icon: 'external-link' },
]

function onMenuClick(item) {
  if (item.key === 'profile') router.push({ name: 'account' })
  else if (item.key === 'front') router.push({ name: 'home' })
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

      <AppUserMenu
        :items="ADMIN_MENU"
        nickname-fallback="管理员"
        logout-route="login"
        @item-click="onMenuClick"
      />
    </div>
  </header>
</template>
