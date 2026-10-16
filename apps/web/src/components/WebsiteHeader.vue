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
          <RouterLink
            :to="{ name: 'account' }"
            class="flex items-center gap-2 rounded-lg py-1 pr-2.5 pl-1 transition hover:bg-ink-100"
          >
            <AppAvatar :name="session.displayName" :initial="session.initial" size="sm" />
            <span class="hidden max-w-[110px] truncate text-[13.5px] text-ink-700 sm:block">
              {{ session.displayName }}
            </span>
          </RouterLink>
        </template>
        <AppButton v-else :to="{ name: 'login' }" size="md" class="px-6">登录</AppButton>
      </div>
    </div>
  </header>
</template>
