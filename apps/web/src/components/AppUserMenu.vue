<script setup>
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import AppIcon from '@/components/AppIcon.vue'
import { useSessionStore } from '@/stores/session'

/**
 * 前后台共用的「头像 + 下拉菜单」：
 * 圆形首字头像 + 点击展开的菜单卡片（用户信息 + 自定义菜单项 + 退出登录）。
 *
 * 前后台菜单项不同，通过 items prop 传入（{ key, label, icon }），
 * 点击后组件收起菜单并把该项通过 @item-click 交给父组件处理；
 * 退出登录统一在组件内完成（session.logout + 跳转到 logoutRoute）。
 */
const props = defineProps({
  /** 菜单项列表：{ key, label, icon }，点击由父组件通过 @item-click 处理 */
  items: { type: Array, default: () => [] },
  /** 昵称为空时的兜底文案（后台管理员用） */
  nicknameFallback: { type: String, default: '' },
  /** 退出登录后跳转的路由名（后台 login / 前台 home） */
  logoutRoute: { type: String, default: 'home' },
})

const emit = defineEmits(['item-click'])

const router = useRouter()
const session = useSessionStore()

const menuOpen = ref(false)

/** 头像图片加载失败时回退到首字母（文件被删 / URL 失效不显示裂图） */
const avatarFailed = ref(false)
watch(
  () => session.user?.avatar_url,
  () => {
    avatarFailed.value = false
  },
)
const showAvatarImage = computed(() => Boolean(session.user?.avatar_url) && !avatarFailed.value)

const name = computed(() =>
  props.nicknameFallback
    ? session.user?.nickname || props.nicknameFallback
    : session.displayName,
)
const email = computed(() => session.user?.email ?? '')

function pick(item) {
  menuOpen.value = false
  emit('item-click', item)
}

async function logout() {
  menuOpen.value = false
  await session.logout()
  router.push({ name: props.logoutRoute })
}
</script>

<template>
  <div class="relative">
    <button
      type="button"
      class="flex size-9 items-center justify-center overflow-hidden rounded-full bg-brand-600 text-[14px] font-semibold text-white transition hover:bg-brand-700"
      :aria-expanded="menuOpen"
      aria-haspopup="menu"
      :aria-label="`用户菜单：${name}`"
      @click="menuOpen = !menuOpen"
    >
      <img
        v-if="showAvatarImage"
        :src="session.user.avatar_url"
        :alt="name"
        class="size-full object-cover"
        @error="avatarFailed = true"
      />
      <template v-else>{{ session.initial }}</template>
    </button>

    <template v-if="menuOpen">
      <div class="fixed inset-0 z-10" @click="menuOpen = false" />
      <div
        class="absolute right-0 z-20 mt-2 w-56 overflow-hidden rounded-xl border border-ink-200 bg-white py-1.5 shadow-panel"
        role="menu"
      >
        <div class="text-center border-b border-ink-100 px-4 py-2.5">
          <p class="truncate text-[13.5px] font-medium text-ink-800">{{ name }}</p>
          <p class="truncate text-[12px] text-ink-400">{{ email }}</p>
        </div>
        <button
          v-for="item in items"
          :key="item.key"
          type="button"
          class="flex w-full items-center gap-2.5 px-4 py-2.5 text-[13.5px] text-ink-600 transition hover:bg-ink-50"
          role="menuitem"
          @click="pick(item)"
        >
          <AppIcon :name="item.icon" :size="16" />
          <span class="truncate">{{ item.label }}</span>
          <AppIcon name="chevron-right" :size="16" class="ml-auto shrink-0 text-ink-300" />
        </button>
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
</template>
