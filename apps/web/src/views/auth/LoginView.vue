<script setup>
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { USE_MOCK } from '@/api/http'
import AppButton from '@/components/AppButton.vue'
import AppCheckbox from '@/components/AppCheckbox.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AuthCard from '@/components/AuthCard.vue'
import { useSessionStore } from '@/stores/session'
import { useSystemStore } from '@/stores/system'
import { useToastStore } from '@/stores/toast'

/**
 * 邮箱登录 = 设计稿第 3 屏。
 *
 * 「记住我」不是装饰：勾选后把邮箱留在本地，下次打开直接回填。
 * 注意它只记邮箱，绝不记密码或令牌 —— 令牌由 http 层单独管理，
 * 与「记住我」是两个生命周期不同的东西，混在一起会导致退出登录后仍被自动登录。
 */
const EMAIL_KEY = 'jobhub.remembered-email'

const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const system = useSystemStore()
const toast = useToastStore()

const remembered = localStorage.getItem(EMAIL_KEY) ?? ''

const form = reactive({
  email: remembered,
  password: '',
  remember: Boolean(remembered),
})

const errors = reactive({ email: '', password: '' })
const submitting = ref(false)
const showPassword = ref(false)

function validate() {
  errors.email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email) ? '' : '请输入有效的邮箱地址'
  errors.password = form.password.length >= 8 ? '' : '密码至少 8 位'
  return !errors.email && !errors.password
}

async function submit() {
  if (!validate()) return
  submitting.value = true
  try {
    await session.login({ email: form.email.trim(), password: form.password })
    if (form.remember) localStorage.setItem(EMAIL_KEY, form.email.trim())
    else localStorage.removeItem(EMAIL_KEY)

    toast.success('登录成功')
    // 守卫会把来源地址放在 redirect 里，登录后原路返回而不是统一丢到「我的」
    const redirect = route.query.redirect
    router.replace(redirect ? String(redirect) : { name: 'account' })
  } catch (err) {
    errors.password = err.message
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthCard title="欢迎回来" subtitle="使用邮箱登录 JobHub 直招">
    <!--
      系统尚未初始化时给出显眼的入口。
      正常情况下路由守卫已经把访问者推到 /setup 了，但这里仍然要提示：
      运维可能是通过书签 / 历史记录直接落到登录页的，对着一个永远登不进去的
      表单找不到北，比多一行提示糟糕得多。
    -->
    <div
      v-if="system.canSetup"
      class="mb-5 flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 px-3.5 py-3 text-[13px] leading-relaxed text-amber-800"
    >
      <AppIcon name="alert" :size="16" class="mt-0.5 shrink-0" />
      <span>
        系统尚未完成初始化，还没有任何账号可以登录。
        <RouterLink
          :to="{ name: 'setup' }"
          class="font-medium text-amber-900 underline decoration-amber-400 underline-offset-2 transition hover:text-amber-950"
        >
          前往初始化 →
        </RouterLink>
      </span>
    </div>

    <form class="space-y-4" novalidate @submit.prevent="submit">
      <AppInput
        v-model="form.email"
        icon="mail"
        type="email"
        autocomplete="email"
        placeholder="请输入邮箱"
        :error="errors.email"
      />

      <AppInput
        v-model="form.password"
        icon="lock"
        :type="showPassword ? 'text' : 'password'"
        autocomplete="current-password"
        placeholder="请输入密码"
        :error="errors.password"
      >
        <template #suffix>
          <button
            type="button"
            class="rounded p-1 text-ink-400 transition hover:text-ink-600"
            :aria-label="showPassword ? '隐藏密码' : '显示密码'"
            @click="showPassword = !showPassword"
          >
            <AppIcon :name="showPassword ? 'eye-off' : 'eye'" :size="17" />
          </button>
        </template>
      </AppInput>

      <div class="flex items-center justify-between">
        <AppCheckbox v-model="form.remember" label="记住我" />
        <RouterLink
          :to="{ name: 'forgot-password' }"
          class="text-[13px] text-brand-600 transition hover:text-brand-700"
        >
          忘记密码？
        </RouterLink>
      </div>

      <AppButton type="submit" size="lg" block :loading="submitting">登录</AppButton>
    </form>

    <div class="my-6 flex items-center gap-3">
      <span class="h-px flex-1 bg-ink-100" />
      <span class="text-[12.5px] text-ink-400">或使用以下方式登录</span>
      <span class="h-px flex-1 bg-ink-100" />
    </div>

    <!-- 第三方登录在演示环境不跳转，点击只做说明，避免给出假的成功反馈 -->
    <div class="grid grid-cols-2 gap-3">
      <button
        v-for="channel in [
          { icon: 'wechat', label: '微信登录' },
          { icon: 'douyin', label: '抖音登录' },
        ]"
        :key="channel.label"
        type="button"
        class="inline-flex h-11 items-center justify-center gap-2 rounded-xl border border-ink-200 bg-ink-50 text-[13.5px] text-ink-600 transition hover:border-brand-300 hover:text-brand-700"
        @click="toast.info(`${channel.label}将在正式环境接入`)"
      >
        <AppIcon :name="channel.icon" :size="18" />
        {{ channel.label }}
      </button>
    </div>

    <p v-if="USE_MOCK" class="mt-5 rounded-lg bg-ink-50 px-3 py-2 text-center text-[12px] text-ink-500">
      演示环境：任意邮箱 + 8 位以上密码即可登录。
      <br />
      想进入后台请用 <span class="font-medium text-ink-700">zhangwei@example.com</span>（管理员）。
    </p>

    <template #footer>
      <p>
        登录即代表同意
        <RouterLink :to="{ name: 'terms' }" class="text-brand-600 transition hover:underline">
          《用户服务协议》
        </RouterLink>
        和
        <RouterLink :to="{ name: 'privacy' }" class="text-brand-600 transition hover:underline">
          《隐私政策》
        </RouterLink>
      </p>
      <p class="mt-3">
        还没有账号？
        <RouterLink
          :to="{ name: 'register' }"
          class="font-medium text-brand-600 transition hover:text-brand-700"
        >
          立即注册
        </RouterLink>
      </p>
    </template>
  </AuthCard>
</template>
