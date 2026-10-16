<script setup>
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'

import { USE_MOCK } from '@/api/http'
import AppButton from '@/components/AppButton.vue'
import AppCheckbox from '@/components/AppCheckbox.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AuthCard from '@/components/AuthCard.vue'
import { useCountdown } from '@/composables/useCountdown'
import { useSessionStore } from '@/stores/session'
import { useToastStore } from '@/stores/toast'

/**
 * 邮箱注册 = 设计稿第 4 屏。
 *
 * 密码强度在提交前先本地校验一遍：服务端也会返回 20011 并带上 details.problems，
 * 但让用户先看到「哪里不合格」比等一次往返体验好得多。
 * 协议勾选是硬门槛 —— 未勾选不发请求，这与登录页的「登录即代表同意」不同：
 * 注册是建立合同关系，必须显式同意。
 */
const router = useRouter()
const session = useSessionStore()
const toast = useToastStore()
const countdown = useCountdown(60)

const form = reactive({
  email: '',
  code: '',
  password: '',
  confirm: '',
  agree: false,
})

const errors = reactive({ email: '', code: '', password: '', confirm: '', agree: '' })
const submitting = ref(false)
const showPassword = ref(false)
const codeHint = ref('')

const codeButtonLabel = computed(() => countdown.label())

function validate() {
  errors.email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email) ? '' : '请输入有效的邮箱地址'
  errors.code = /^\d{6}$/.test(form.code) ? '' : '请输入 6 位数字验证码'

  if (form.password.length < 8) errors.password = '密码至少 8 位'
  else if (!/[a-zA-Z]/.test(form.password) || !/\d/.test(form.password))
    errors.password = '密码需同时包含字母与数字'
  else errors.password = ''

  errors.confirm = form.confirm === form.password ? '' : '两次输入的密码不一致'
  errors.agree = form.agree ? '' : '请先阅读并同意用户服务协议与隐私政策'

  return !Object.values(errors).some(Boolean)
}

async function sendCode() {
  errors.email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email) ? '' : '请输入有效的邮箱地址'
  if (errors.email) return
  try {
    const result = await countdown.send(form.email.trim(), 'register')
    toast.success(result?.message ?? '验证码已发送')
    // 联调模式下服务端会回传验证码，直接显示出来省去查邮件
    if (result?.debug_code) codeHint.value = `演示验证码：${result.debug_code}`
  } catch (err) {
    toast.error(err.message)
  }
}

async function submit() {
  if (!validate()) return
  submitting.value = true
  try {
    await session.register({
      email: form.email.trim(),
      password: form.password,
      code: form.code,
      nickname: form.email.trim().split('@')[0],
    })
    toast.success('注册成功，欢迎加入 JobHub 直招')
    router.replace({ name: 'account' })
  } catch (err) {
    const problems = err.details?.problems
    errors.password = problems?.length ? problems.join('；') : err.message
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthCard title="创建账号" subtitle="注册 JobHub 直招，发现官网直招好工作">
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
        v-model="form.code"
        icon="shield-check"
        inputmode="numeric"
        maxlength="6"
        placeholder="请输入图形验证码"
        :error="errors.code"
        :hint="codeHint"
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
        placeholder="设置密码（至少8位）"
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
        placeholder="确认密码"
        :error="errors.confirm"
      />

      <div>
        <AppCheckbox v-model="form.agree">
          我已阅读并同意
          <RouterLink :to="{ name: 'terms' }" class="text-brand-600 hover:underline">
            《用户服务协议》
          </RouterLink>
          和
          <RouterLink :to="{ name: 'privacy' }" class="text-brand-600 hover:underline">
            《隐私政策》
          </RouterLink>
        </AppCheckbox>
        <p v-if="errors.agree" class="mt-1.5 flex items-center gap-1 text-[12.5px] text-red-600">
          <AppIcon name="alert" :size="14" />
          {{ errors.agree }}
        </p>
      </div>

      <AppButton type="submit" size="lg" block :loading="submitting">注册</AppButton>
    </form>

    <p v-if="USE_MOCK" class="mt-5 rounded-lg bg-ink-50 px-3 py-2 text-center text-[12px] text-ink-500">
      演示环境：先点「获取验证码」，再用 <span class="font-medium text-ink-700">135790</span> 完成注册。
    </p>

    <template #footer>
      <p>
        已有账号？
        <RouterLink
          :to="{ name: 'login' }"
          class="font-medium text-brand-600 transition hover:text-brand-700"
        >
          立即登录
        </RouterLink>
      </p>
    </template>
  </AuthCard>
</template>
