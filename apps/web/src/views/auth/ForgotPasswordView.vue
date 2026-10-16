<script setup>
import { computed, reactive } from 'vue'
import { useRouter } from 'vue-router'

import AppButton from '@/components/AppButton.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AuthCard from '@/components/AuthCard.vue'
import AuthSteps from '@/components/AuthSteps.vue'
import { useCountdown } from '@/composables/useCountdown'
import { usePasswordResetStore } from '@/stores/passwordReset'
import { useToastStore } from '@/stores/toast'

/**
 * 找回密码 · 第 1 步「邮箱验证」= 设计稿第 5 屏。
 *
 * 说明：接口没有独立的「校验验证码」端点，验证码真正的校验发生在
 * /auth/password/reset。因此这一步只做「邮箱 + 已发码」的把关，
 * 把邮箱与验证码交给第 2 步，避免为了一个假的校验再发一次请求。
 */
const router = useRouter()
const toast = useToastStore()
const reset = usePasswordResetStore()
const countdown = useCountdown(60)

const form = reactive({
  email: reset.email,
  code: reset.code,
})

const errors = reactive({ email: '', code: '' })

const codeButtonLabel = computed(() => countdown.label())

async function sendCode() {
  errors.email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email) ? '' : '请输入注册时使用的邮箱'
  if (errors.email) return
  try {
    // 该接口对未注册邮箱也返回同一话术，前端据此提示即可，不做存在性判断
    const result = await countdown.send(form.email.trim(), 'reset_password')
    toast.success(result?.message ?? '验证码已发送，请查收邮件')
  } catch (err) {
    toast.error(err.message)
  }
}

function next() {
  errors.email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email) ? '' : '请输入注册时使用的邮箱'
  errors.code = /^\d{6}$/.test(form.code) ? '' : '请输入 6 位数字验证码'
  if (errors.email || errors.code) return

  reset.set({ email: form.email.trim(), code: form.code })
  router.push({ name: 'reset-password' })
}
</script>

<template>
  <AuthCard title="找回密码" subtitle="我们会向你的注册邮箱发送验证码">
    <template #header>
      <AuthSteps :current="1" />
    </template>

    <form class="space-y-4" novalidate @submit.prevent="next">
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
        hint="验证码 10 分钟内有效"
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

      <AppButton type="submit" size="lg" block>下一步</AppButton>
    </form>

    <p class="mt-5 flex items-start gap-1.5 text-[12.5px] leading-relaxed text-ink-400">
      <AppIcon name="info" :size="14" class="mt-0.5" />
      出于安全考虑，无论该邮箱是否已注册，我们都会显示同样的提示。
    </p>

    <template #footer>
      <RouterLink
        :to="{ name: 'login' }"
        class="font-medium text-brand-600 transition hover:text-brand-700"
      >
        返回登录
      </RouterLink>
    </template>
  </AuthCard>
</template>
