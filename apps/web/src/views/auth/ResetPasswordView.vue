<script setup>
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'

import { authApi } from '@/api'
import { USE_MOCK } from '@/api/http'
import AppButton from '@/components/AppButton.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AuthCard from '@/components/AuthCard.vue'
import AuthSteps from '@/components/AuthSteps.vue'
import { useCountdown } from '@/composables/useCountdown'
import { usePasswordResetStore } from '@/stores/passwordReset'
import { useSessionStore } from '@/stores/session'
import { useToastStore } from '@/stores/toast'

/**
 * 找回密码 · 第 2 步「重置密码」+ 第 3 步「完成」。
 *
 * 这条路由刻意不加 guestOnly：用户可能已经登录，却点开了邮件里的重置链接，
 * 此时把他弹回「我的」会让重置永远做不完。真正的安全边界是「必须持有邮箱验证码」，
 * 而不是「必须未登录」。
 *
 * 重置成功后服务端会吊销该账号的全部会话，因此必须清掉本地登录态 ——
 * 否则浏览器里会留下一个已经失效的令牌，下一次请求才报错，体验很莫名其妙。
 */
const router = useRouter()
const toast = useToastStore()
const session = useSessionStore()
const reset = usePasswordResetStore()
const countdown = useCountdown(60)

const form = reactive({
  email: reset.email,
  code: reset.code,
  password: '',
  confirm: '',
})

const errors = reactive({ email: '', code: '', password: '', confirm: '' })
const submitting = ref(false)
const showPassword = ref(false)
const done = ref(false)

const step = computed(() => (done.value ? 3 : 2))
const codeButtonLabel = computed(() => countdown.label())

async function sendCode() {
  errors.email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email) ? '' : '请输入注册时使用的邮箱'
  if (errors.email) return
  try {
    const result = await countdown.send(form.email.trim(), 'reset_password')
    toast.success(result?.message ?? '验证码已发送，请查收邮件')
  } catch (err) {
    toast.error(err.message)
  }
}

function validate() {
  errors.email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email) ? '' : '请输入注册时使用的邮箱'
  errors.code = /^\d{6}$/.test(form.code) ? '' : '请输入 6 位数字验证码'

  if (form.password.length < 8) errors.password = '新密码至少 8 位'
  else if (!/[a-zA-Z]/.test(form.password) || !/\d/.test(form.password))
    errors.password = '新密码需同时包含字母与数字'
  else errors.password = ''

  errors.confirm = form.confirm === form.password ? '' : '两次输入的密码不一致'

  return !Object.values(errors).some(Boolean)
}

async function submit() {
  if (!validate()) return
  submitting.value = true
  try {
    await authApi.resetPassword({
      email: form.email.trim(),
      code: form.code,
      new_password: form.password,
    })

    // 重置会吊销全部会话：本地登录态必须一起清掉，否则留下一个必然失效的令牌
    session.clear()
    reset.clear()
    done.value = true
    toast.success('密码重置成功')
  } catch (err) {
    errors.code = err.message
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthCard
    :title="done ? '重置完成' : '重置密码'"
    :subtitle="done ? '现在可以用新密码登录了' : '设置一个新的登录密码'"
  >
    <template #header>
      <AuthSteps :current="step" />
    </template>

    <!-- 第 3 步：完成 -->
    <div v-if="done" class="text-center">
      <span class="mx-auto flex size-16 items-center justify-center rounded-full bg-accent-50">
        <AppIcon name="check-circle" :size="32" class="text-accent-600" />
      </span>

      <p class="mt-5 text-[14px] leading-relaxed text-ink-600">
        密码已更新，你在其它设备上的登录已全部退出。
        <br />
        为了账号安全，请使用新密码重新登录。
      </p>

      <AppButton size="lg" block class="mt-6" @click="router.replace({ name: 'login' })">
        去登录
      </AppButton>
    </div>

    <!-- 第 2 步：设置新密码 -->
    <form v-else class="space-y-4" novalidate @submit.prevent="submit">
      <AppInput
        v-model="form.email"
        icon="mail"
        type="email"
        autocomplete="email"
        placeholder="请输入注册邮箱"
        :error="errors.email"
      />

      <AppInput
        v-model="form.code"
        icon="bolt"
        inputmode="numeric"
        maxlength="6"
        placeholder="请输入邮箱验证码"
        :error="errors.code"
      >
        <template #suffix>
          <button
            type="button"
            class="shrink-0 rounded-lg bg-brand-50 px-3 py-1.5 text-[12.5px] font-medium text-brand-600 transition hover:bg-brand-100 disabled:cursor-not-allowed disabled:bg-ink-100 disabled:text-ink-400"
            :disabled="countdown.remaining.value > 0 || countdown.sending.value"
            @click="sendCode"
          >
            {{ codeButtonLabel }}
          </button>
        </template>
      </AppInput>

      <AppInput
        v-model="form.password"
        icon="lock"
        :type="showPassword ? 'text' : 'password'"
        autocomplete="new-password"
        placeholder="设置新密码（至少8位）"
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

      <AppInput
        v-model="form.confirm"
        icon="lock"
        :type="showPassword ? 'text' : 'password'"
        autocomplete="new-password"
        placeholder="确认新密码"
        :error="errors.confirm"
      />

      <AppButton type="submit" size="lg" block :loading="submitting">确认重置</AppButton>
    </form>

    <p
      v-if="USE_MOCK && !done"
      class="mt-5 rounded-lg bg-ink-50 px-3 py-2 text-center text-[12px] text-ink-500"
    >
      演示环境验证码：<span class="font-medium text-ink-700">135790</span>
    </p>

    <template v-if="!done" #footer>
      <RouterLink
        :to="{ name: 'login' }"
        class="font-medium text-brand-600 transition hover:text-brand-700"
      >
        返回登录
      </RouterLink>
    </template>
  </AuthCard>
</template>
