<script setup>
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'

import AppAvatar from '@/components/AppAvatar.vue'
import AppButton from '@/components/AppButton.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppLogo from '@/components/AppLogo.vue'
import { useSessionStore } from '@/stores/session'

/**
 * 前台顶部导航。
 * 结构与设计稿一致：左 Logo、中菜单、右搜索 + 登录态。
 * 详情页会把所属的一级栏目点亮（职位详情 → 职位），避免用户失去位置感。
 */
const route = useRoute()
const router = useRouter()
const session = useSessionStore()

const NAV = [
  { name: 'home', label: '首页' },
  { name: 'jobs', label: '职位' },
  { name: 'companies', label: '公司库' },
  { name: 'about', label: '关于直招' },
]

/**
 * 用户菜单与后台顶栏（AdminTopbar）同款交互：
 * 圆形深蓝头像 + 点击展开下拉（个人中心 / 账号设置 / 退出登录）。
 * 「个人中心」与「账号设置」分别落到个人中心的资料 / 账号安全两个分区。
 */
const userMenuOpen = ref(false)

const USER_MENU = [
  { key: 'profile', label: '个人中心', icon: 'user', tab: 'profile' },
  { key: 'security', label: '账号设置', icon: 'lock', tab: 'security' },
]

function openAccountTab(tab) {
  userMenuOpen.value = false
  router.push({ name: 'account', query: tab === 'profile' ? {} : { tab } })
}

async function handleLogout() {
  userMenuOpen.value = false
  await session.logout()
  router.push({ name: 'home' })
}

const keyword = ref(String(route.query.keyword ?? ''))

// 从职位页返回首页时，搜索框内容要跟着地址栏走
watch(
  () => route.query.keyword,
  (value) => {
    keyword.value = String(value ?? '')
  },
)

/** 详情页归属到它的一级栏目，导航高亮才不会掉 */
const activeName = computed(() => {
  if (route.name === 'job') return 'jobs'
  if (route.name === 'company') return 'companies'
  return route.name
})

function submit() {
  router.push({ name: 'jobs', query: keyword.value ? { keyword: keyword.value } : {} })
}
</script>

<template>
  <header class="sticky top-0 z-50 border-b border-ink-100 bg-white/95 backdrop-blur">
    <div class="container-page flex h-16 items-center gap-8">
      <RouterLink :to="{ name: 'home' }" class="shrink-0">
        <AppLogo />
      </RouterLink>

      <nav class="hidden items-center gap-1 lg:flex">
        <RouterLink
          v-for="item in NAV"
          :key="item.name"
          :to="{ name: item.name }"
          class="relative px-3.5 py-2 text-[15px] transition"
          :class="
            activeName === item.name
              ? 'font-medium text-brand-700'
              : 'text-ink-600 hover:text-ink-900'
          "
        >
          {{ item.label }}
          <span
            v-if="activeName === item.name"
            class="absolute inset-x-3.5 -bottom-[1px] h-[2.5px] rounded-full bg-brand-600"
          />
        </RouterLink>
      </nav>

      <div class="ml-auto flex items-center gap-3">
        <div
          class="hidden h-10 w-[260px] items-center gap-2 rounded-full bg-ink-100 px-4 transition focus-within:bg-white focus-within:ring-2 focus-within:ring-brand-500/20 md:flex"
        >
          <AppIcon name="search" :size="16" class="text-ink-400" />
          <input
            v-model="keyword"
            type="search"
            placeholder="搜索职位/公司"
            class="h-full min-w-0 flex-1 bg-transparent text-[13.5px] text-ink-800 placeholder:text-ink-400 focus:outline-none"
            @keyup.enter="submit"
          />
        </div>

        <template v-if="session.isAuthenticated">
          <div class="relative">
            <button
              type="button"
              class="flex size-9 items-center justify-center rounded-full bg-brand-600 text-[14px] font-semibold text-white transition hover:bg-brand-700"
              :aria-expanded="userMenuOpen"
              aria-haspopup="menu"
              :aria-label="`用户菜单：${session.displayName}`"
              @click="userMenuOpen = !userMenuOpen"
            >
              {{ session.initial }}
            </button>

            <template v-if="userMenuOpen">
              <div class="fixed inset-0 z-10" @click="userMenuOpen = false" />
              <div
                class="absolute right-0 z-20 mt-2 w-56 overflow-hidden rounded-xl border border-ink-200 bg-white py-1.5 shadow-panel"
                role="menu"
              >
                <div class="border-b border-ink-100 px-4 py-2.5">
                  <p class="truncate text-[13.5px] font-medium text-ink-800">
                    {{ session.displayName }}
                  </p>
                  <p class="truncate text-[12px] text-ink-400">{{ session.user?.email }}</p>
                </div>
                <button
                  v-for="item in USER_MENU"
                  :key="item.key"
                  type="button"
                  class="flex w-full items-center gap-2.5 px-4 py-2.5 text-[13.5px] text-ink-600 transition hover:bg-ink-50"
                  role="menuitem"
                  @click="openAccountTab(item.tab)"
                >
                  <AppIcon :name="item.icon" :size="16" />
                  {{ item.label }}
                </button>
                <button
                  type="button"
                  class="flex w-full items-center gap-2.5 px-4 py-2.5 text-[13.5px] text-red-600 transition hover:bg-red-50"
                  role="menuitem"
                  @click="handleLogout"
                >
                  <AppIcon name="logout" :size="16" />
                  退出登录
                </button>
              </div>
            </template>
          </div>
        </template>
        <AppButton v-else :to="{ name: 'login' }" size="md" class="px-6">登录</AppButton>
      </div>
    </div>
  </header>
</template>
