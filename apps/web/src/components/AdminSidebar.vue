<script setup>
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'

import AppIcon from '@/components/AppIcon.vue'
import AppLogo from '@/components/AppLogo.vue'
import { userInitial } from '@/utils/format'

/**
 * 后台侧边栏。
 * 导航项与设计稿一致（仪表盘 / 岗位管理 / 审核队列 / 公司管理 / 用户管理 /
 * 角色权限 / 系统配置 / 系统监控 / 审计日志），底部固定当前管理员信息。
 */
const props = defineProps({
  user: { type: Object, default: () => ({}) },
  /** 审核队列的待处理数量，用角标提示运营优先处理 */
  pendingCount: { type: Number, default: 0 },
})

defineEmits(['logout'])

const route = useRoute()

const NAV = [
  { name: 'dashboard', label: '仪表盘', icon: 'gauge' },
  { name: 'postings', label: '岗位管理', icon: 'briefcase' },
  { name: 'review', label: '审核队列', icon: 'clipboard', badge: 'pending' },
  { name: 'sources', label: '公司管理', icon: 'building' },
  { name: 'users', label: '用户管理', icon: 'users' },
  { name: 'roles', label: '角色权限', icon: 'shield-check' },
  { name: 'settings', label: '系统配置', icon: 'sliders' },
  { name: 'monitor', label: '系统监控', icon: 'activity' },
  { name: 'logs', label: '审计日志', icon: 'document' },
]

/** 详情页归属到列表项：/postings/12 也要点亮「岗位管理」 */
const PARENT = {
  posting: 'postings',
  source: 'sources',
  user: 'users',
  log: 'logs',
}

const activeName = computed(() => {
  const current = route.name
  return PARENT[current] ?? current
})

const pending = computed(() => props.pendingCount)
</script>

<template>
  <aside class="flex w-[236px] shrink-0 flex-col bg-sidebar">
    <div class="flex h-16 items-center px-6">
      <RouterLink :to="{ name: 'dashboard' }">
        <AppLogo variant="light" />
      </RouterLink>
    </div>

    <nav class="flex-1 space-y-1 overflow-y-auto px-3 py-4">
      <RouterLink
        v-for="item in NAV"
        :key="item.name"
        :to="{ name: item.name }"
        class="relative flex items-center gap-3 rounded-xl px-3.5 py-2.5 text-[14px] transition"
        :class="
          activeName === item.name
            ? 'bg-white/15 font-medium text-white'
            : 'text-white/70 hover:bg-white/10 hover:text-white'
        "
      >
        <span
          v-if="activeName === item.name"
          class="absolute top-1/2 -left-3 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-white"
        />
        <AppIcon :name="item.icon" :size="18" />
        <span class="flex-1">{{ item.label }}</span>
        <span
          v-if="item.badge === 'pending' && pending"
          class="tabular rounded-full bg-white/20 px-2 py-0.5 text-[11px] text-white"
        >
          {{ pending }}
        </span>
      </RouterLink>
    </nav>

    <div class="border-t border-white/15 px-4 py-4">
      <div class="flex items-center gap-3">
        <span
          class="flex size-9 shrink-0 items-center justify-center rounded-full bg-white/20 text-[15px] font-semibold text-white"
        >
          {{ userInitial(user) }}
        </span>
        <div class="min-w-0 flex-1">
          <p class="truncate text-[13.5px] font-medium text-white">
            {{ user.nickname || '管理员' }}
          </p>
          <p class="truncate text-[11.5px] text-white/60">{{ user.email || 'admin@jobhub.cn' }}</p>
        </div>
        <button
          type="button"
          class="rounded-lg p-1.5 text-white/60 transition hover:bg-white/15 hover:text-white"
          title="退出登录"
          aria-label="退出登录"
          @click="$emit('logout')"
        >
          <AppIcon name="logout" :size="17" />
        </button>
      </div>
    </div>
  </aside>
</template>
