<script setup>
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'

import { meApi } from '@/api'
import { loadTokens } from '@/api/http'
import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import { useAsync } from '@/composables/useAsync'
import { useSessionStore } from '@/stores/session'
import { useToastStore } from '@/stores/toast'
import { ROLE_TONE, formatDateTime, relativeTime, roleLabel, toneFor, userInitial } from '@/utils/format'

/**
 * 我的。
 *
 * 路由守卫已经保证这里一定是登录态，所以页面不再做「未登录」分支 ——
 * 多余的分支只会让人误以为存在一个能看到的未登录版本。
 *
 * 三块内容按「风险从低到高」排列：资料 → 密码 → 设备。
 * 设备列表放在右侧独立成栏，因为它是唯一会随时间变化、需要反复查看的区域。
 */
const session = useSessionStore()
const toast = useToastStore()
const router = useRouter()

const PHONE_RE = /^(1[3-9]\d{9}|0\d{2,3}-?\d{7,8})$/

/** 去掉本文件里重复的 role→中文 映射：format.js 的 ROLE_LABEL 才是单一事实来源 */
const roleItems = computed(() =>
  (session.roles ?? []).map((role) => ({
    key: role,
    label: roleLabel(role),
    // ROLE_TONE 里没有的新角色一律退回中性灰，而不是随便挑一个颜色
    tone: ROLE_TONE[role] ?? 'ink',
  })),
)

/** 用户头像：UserView 没有 tone / initial，两者都按 email 稳定派生（改昵称不会变颜色） */
const userAvatar = computed(() => ({
  initial: userInitial(session.user),
  tone: toneFor(session.user?.email ?? session.user?.id),
}))

/* -------------------------------------------------------------------------- */
/* 资料设置                                                                    */
/* -------------------------------------------------------------------------- */

const profile = reactive({
  nickname: session.user?.nickname ?? '',
  phone: session.user?.phone ?? '',
})

const profileErrors = reactive({ nickname: '', phone: '' })
const savingProfile = ref(false)

function validateProfile() {
  profileErrors.nickname = ''
  profileErrors.phone = ''

  const nickname = profile.nickname.trim()
  const phone = profile.phone.trim()

  if (!nickname) profileErrors.nickname = '请填写昵称'
  else if (nickname.length > 20) profileErrors.nickname = '昵称不超过 20 个字符'

  // 手机号是选填项：留空代表不公开，填了就必须能打通
  if (phone && !PHONE_RE.test(phone)) profileErrors.phone = '请填写有效的手机号或座机号'

  return !profileErrors.nickname && !profileErrors.phone
}

async function saveProfile() {
  if (!validateProfile()) return
  savingProfile.value = true
  try {
    await session.updateProfile({ nickname: profile.nickname.trim(), phone: profile.phone.trim() })
    toast.success('资料已保存')
  } catch (err) {
    toast.fromError(err, '资料保存失败，请稍后重试')
  } finally {
    savingProfile.value = false
  }
}

/* -------------------------------------------------------------------------- */
/* 修改密码                                                                    */
/* -------------------------------------------------------------------------- */

const PASSWORD_FIELDS = [
  {
    key: 'oldPassword',
    label: '原密码',
    placeholder: '请输入当前密码',
    autocomplete: 'current-password',
    hint: '',
  },
  {
    key: 'newPassword',
    label: '新密码',
    placeholder: '至少 8 位，包含字母和数字',
    autocomplete: 'new-password',
    hint: '至少 8 位，需同时包含字母和数字，且不能与原密码相同',
  },
  {
    key: 'confirmPassword',
    label: '确认新密码',
    placeholder: '请再次输入新密码',
    autocomplete: 'new-password',
    hint: '',
  },
]

const password = reactive({ oldPassword: '', newPassword: '', confirmPassword: '' })
const passwordErrors = reactive({ oldPassword: '', newPassword: '', confirmPassword: '' })
const passwordVisible = reactive({ oldPassword: false, newPassword: false, confirmPassword: false })
const changingPassword = ref(false)

/**
 * 前端只做「能立刻判断」的校验，规则与后端保持一致；
 * 原密码是否正确只能由服务端回答，不在本地假装知道。
 */
function validatePassword() {
  passwordErrors.oldPassword = ''
  passwordErrors.newPassword = ''
  passwordErrors.confirmPassword = ''

  if (!password.oldPassword) passwordErrors.oldPassword = '请输入原密码'

  if (!password.newPassword) {
    passwordErrors.newPassword = '请输入新密码'
  } else if (password.newPassword.length < 8) {
    passwordErrors.newPassword = '新密码至少 8 位'
  } else if (!/[A-Za-z]/.test(password.newPassword) || !/\d/.test(password.newPassword)) {
    passwordErrors.newPassword = '新密码需同时包含字母和数字'
  } else if (password.newPassword === password.oldPassword) {
    passwordErrors.newPassword = '新密码不能与原密码相同'
  }

  if (!password.confirmPassword) passwordErrors.confirmPassword = '请再次输入新密码'
  else if (password.confirmPassword !== password.newPassword) {
    passwordErrors.confirmPassword = '两次输入的新密码不一致'
  }

  return !passwordErrors.oldPassword && !passwordErrors.newPassword && !passwordErrors.confirmPassword
}

async function changePassword() {
  if (!validatePassword()) return
  changingPassword.value = true
  try {
    // 服务端会作废其它会话并可能下发新令牌；令牌轮换由 http 层接管，这里只关心结果
    await meApi.changePassword({
      old_password: password.oldPassword,
      new_password: password.newPassword,
    })
    toast.success('密码已修改，其他设备已退出登录')
    password.oldPassword = ''
    password.newPassword = ''
    password.confirmPassword = ''
    loadSessions()
  } catch (err) {
    toast.fromError(err, '密码修改失败，请检查原密码是否正确')
  } finally {
    changingPassword.value = false
  }
}

/* -------------------------------------------------------------------------- */
/* 活跃设备                                                                    */
/* -------------------------------------------------------------------------- */

const {
  data: sessions,
  loading: sessionsLoading,
  error: sessionsError,
  ready: sessionsReady,
  run: loadSessions,
} = useAsync(() => meApi.sessions())

/**
 * 后端 SessionView 只有 { id, ip, user_agent, created_at, expires_at }。
 *
 * 两个字段刻意**不存在**，必须由前端补：
 *   - 设备名：后端有意不存 device 标签。UA 原文既是事实来源，
 *     也避免了「后端加字段→旧数据全空」的迁移问题，所以这里现解析。
 *   - 当前设备：会话列表不含 current 标记，唯一可靠的线索是登录时下发的
 *     token.session_id（TokenPair.session_id），它与 SessionView.id 同源。
 *     万一 localStorage 里没有（例如手工清过），就不显示该徽章——
 *     宁可不标，也不能把「当前设备」标错，那会让「下线」按钮变成自我踢出。
 */
const currentSessionId = ref(loadTokens()?.session_id ?? '')

/** 从 UA 推断设备与系统；识别不了就原样显示 UA 前缀，绝不编造一个好看的名字 */
function deviceLabel(userAgent) {
  const ua = String(userAgent ?? '')
  if (!ua) return '未知设备'

  // 顺序要紧：Edge / Opera 的 UA 里同样含 "Chrome"，必须先匹配更具体的
  const browsers = [
    [/Edg(?:e|A|iOS)?\/([\d.]+)/, 'Edge'],
    [/OPR\/([\d.]+)|Opera\/([\d.]+)/, 'Opera'],
    [/MicroMessenger\/([\d.]+)/, '微信'],
    [/(?:Chrome|CriOS)\/([\d.]+)/, 'Chrome'],
    [/(?:Firefox|FxiOS)\/([\d.]+)/, 'Firefox'],
    // Safari 的 UA 带 Version/ 且同样含 "Safari"，所以放在 Chrome/Edge 之后
    [/Version\/([\d.]+).*Safari/, 'Safari'],
    [/MSIE ([\d.]+)|Trident\/.*rv:([\d.]+)/, 'IE'],
  ]

  let browser = ''
  for (const [pattern, name] of browsers) {
    const matched = ua.match(pattern)
    if (matched) {
      const version = matched.slice(1).find(Boolean)
      browser = version ? `${name} ${version.split('.')[0]}` : name
      break
    }
  }

  const systems = [
    [/Windows NT 10\.0/, 'Windows'],
    [/Windows NT 6\.3/, 'Windows 8.1'],
    [/Windows NT 6\.1/, 'Windows 7'],
    [/Android[ /]([\d.]+)/, 'Android'],
    [/iPhone|iPad|iPod/, 'iOS'],
    [/Mac OS X/, 'macOS'],
    [/CrOS/, 'ChromeOS'],
    [/Linux/, 'Linux'],
  ]

  let system = ''
  for (const [pattern, name] of systems) {
    const matched = ua.match(pattern)
    if (matched) {
      system = matched[1] ? `${name} ${matched[1]}` : name
      break
    }
  }

  // 两边都认不出来时退回 UA 原文（截断），保证用户至少能看到真实依据
  if (!browser && !system) return ua.length > 42 ? `${ua.slice(0, 42)}…` : ua
  return [browser, system].filter(Boolean).join(' · ')
}

function isCurrent(item) {
  return Boolean(currentSessionId.value) && item.id === currentSessionId.value
}

/**
 * 派生出的展示字段用 deviceName / isCurrent 命名，刻意不叫 device / current：
 * 后两个名字在 mock 里存在过，而真实 SessionView 里没有，
 * 换个名字可以保证「谁要是再去读 item.device 就会读到 undefined」这种情况一眼可见。
 */
const sessionItems = computed(() =>
  (sessions.value?.items ?? []).map((item) => ({
    ...item,
    deviceName: deviceLabel(item.user_agent),
    isCurrent: isCurrent(item),
  })),
)

/** 剩余有效期：让用户知道「这个设备还能挂多久」，比只显示创建时间有用 */
function expiresText(value) {
  const expires = new Date(value).getTime()
  if (Number.isNaN(expires)) return '—'
  const days = Math.ceil((expires - Date.now()) / (24 * 60 * 60 * 1000))
  if (days <= 0) return '已过期'
  return `剩余 ${days} 天`
}

const revokingSid = ref('')

async function revoke(item) {
  revokingSid.value = item.id
  try {
    await meApi.revokeSession(item.id)
    toast.success('已下线该设备')
    await loadSessions()
  } catch (err) {
    toast.fromError(err, '下线失败，请稍后重试')
  } finally {
    revokingSid.value = ''
  }
}

async function handleLogout() {
  // logout 内部已清空本地登录态，即使服务端登出失败也不会把用户困在页面里
  await session.logout()
  router.push({ name: 'home' })
}
</script>

<template>
  <div class="container-page py-8">
    <div class="mb-6">
      <h1 class="text-[24px] font-semibold text-ink-900">我的</h1>
      <p class="mt-1.5 text-[13.5px] text-ink-500">管理账号资料、登录密码与活跃设备。</p>
    </div>

    <section class="card p-6">
      <div class="flex flex-wrap items-start justify-between gap-5">
        <div class="flex items-start gap-4">
          <AppAvatar
            :name="session.displayName"
            :initial="userAvatar.initial"
            :tone="userAvatar.tone"
            size="xl"
          />

          <div class="min-w-0">
            <p class="text-[21px] font-semibold text-ink-900">{{ session.displayName }}</p>
            <p class="mt-1.5 flex items-center gap-1.5 text-[13.5px] text-ink-500">
              <AppIcon name="mail" :size="15" class="text-ink-400" />
              {{ session.user?.email }}
            </p>

            <div class="mt-3 flex flex-wrap items-center gap-2">
              <AppBadge v-for="item in roleItems" :key="item.key" :tone="item.tone">
                {{ item.label }}
              </AppBadge>
            </div>
          </div>
        </div>

        <div class="flex flex-wrap items-center gap-2.5">
          <AppButton v-if="session.isStaff" :to="{ name: 'dashboard' }">
            <AppIcon name="gauge" :size="17" />
            进入后台管理
          </AppButton>
          <AppButton variant="secondary" @click="handleLogout">
            <AppIcon name="logout" :size="17" />
            退出登录
          </AppButton>
        </div>
      </div>
    </section>

    <div class="mt-6 grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_400px]">
      <div class="space-y-6">
        <section class="card p-6">
          <h2 class="text-[16px] font-semibold text-ink-900">资料设置</h2>
          <p class="mt-1 text-[13px] text-ink-500">
            昵称会出现在你的账号中；手机号是选填项，留空即不公开。
          </p>

          <div class="mt-5 space-y-4">
            <AppInput
              v-model="profile.nickname"
              label="昵称"
              placeholder="请输入昵称"
              maxlength="20"
              :error="profileErrors.nickname"
            />
            <AppInput
              v-model="profile.phone"
              label="手机号"
              placeholder="选填"
              inputmode="tel"
              :error="profileErrors.phone"
            />
          </div>

          <div class="mt-6 flex flex-wrap items-center gap-3 border-t border-ink-100 pt-5">
            <AppButton :loading="savingProfile" @click="saveProfile">保存资料</AppButton>
            <span class="text-[12.5px] text-ink-400">邮箱同时是登录账号，暂不支持修改。</span>
          </div>
        </section>

        <section class="card p-6">
          <h2 class="text-[16px] font-semibold text-ink-900">修改密码</h2>
          <p class="mt-1 text-[13px] text-ink-500">修改成功后，其它设备上的登录状态会被退出。</p>

          <div class="mt-5 space-y-4">
            <AppInput
              v-for="field in PASSWORD_FIELDS"
              :key="field.key"
              v-model="password[field.key]"
              :type="passwordVisible[field.key] ? 'text' : 'password'"
              :label="field.label"
              :placeholder="field.placeholder"
              :autocomplete="field.autocomplete"
              :hint="field.hint"
              :error="passwordErrors[field.key]"
            >
              <template #suffix>
                <button
                  type="button"
                  class="flex size-7 items-center justify-center rounded-md text-ink-400 transition hover:text-ink-600"
                  :aria-label="`${passwordVisible[field.key] ? '隐藏' : '显示'}${field.label}`"
                  :aria-pressed="passwordVisible[field.key]"
                  @click="passwordVisible[field.key] = !passwordVisible[field.key]"
                >
                  <AppIcon :name="passwordVisible[field.key] ? 'eye-off' : 'eye'" :size="17" />
                </button>
              </template>
            </AppInput>
          </div>

          <div class="mt-6 border-t border-ink-100 pt-5">
            <AppButton :loading="changingPassword" @click="changePassword">修改密码</AppButton>
          </div>
        </section>
      </div>

      <section class="card p-6 lg:sticky lg:top-24">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 class="text-[16px] font-semibold text-ink-900">活跃设备</h2>
            <p class="mt-1 text-[13px] text-ink-500">这些设备保持登录状态，可随时下线非本人的设备。</p>
          </div>
          <AppButton
            variant="ghost"
            size="sm"
            :loading="sessionsLoading"
            aria-label="刷新活跃设备列表"
            @click="loadSessions()"
          >
            <AppIcon name="refresh" :size="15" />
          </AppButton>
        </div>

        <div v-if="sessionsLoading && !sessionsReady" class="mt-5 space-y-3">
          <div v-for="index in 2" :key="index" class="h-[62px] animate-pulse rounded-xl bg-ink-100" />
        </div>

        <AppEmpty
          v-else-if="sessionsError"
          icon="alert"
          tone="danger"
          title="设备列表加载失败"
          :description="sessionsError.message"
        >
          <AppButton @click="loadSessions()">重新加载</AppButton>
        </AppEmpty>

        <AppEmpty
          v-else-if="!sessionItems.length"
          icon="lock-open"
          title="暂无其它登录设备"
          description="当前只有本设备在使用该账号。"
        />

        <ul v-else class="mt-5 divide-y divide-ink-100">
          <li
            v-for="item in sessionItems"
            :key="item.id"
            class="flex flex-wrap items-center gap-3 py-3.5 first:pt-0 last:pb-0"
          >
            <span class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-ink-100 text-ink-500">
              <AppIcon name="user" :size="17" />
            </span>

            <div class="min-w-0 flex-1">
              <p class="flex flex-wrap items-center gap-2 text-[13.5px] font-medium text-ink-800">
                <span class="truncate">{{ item.deviceName }}</span>
                <!-- 「当前设备」用 brand 而不是 accent：绿色只留给「官网直招」 -->
                <AppBadge v-if="item.isCurrent" tone="brand" size="sm">当前设备</AppBadge>
              </p>
              <p class="mt-1 text-[12.5px] text-ink-400">
                IP {{ item.ip || '未知' }} · 登录于 {{ relativeTime(item.created_at) }} ·
                {{ expiresText(item.expires_at) }}（{{ formatDateTime(item.expires_at) }} 到期）
              </p>
            </div>

            <!-- 当前设备不提供「下线」：那等于给自己一个把本机踢下线的按钮 -->
            <AppButton
              v-if="!item.isCurrent"
              variant="secondary"
              size="sm"
              :loading="revokingSid === item.id"
              :aria-label="`下线设备 ${item.deviceName}`"
              @click="revoke(item)"
            >
              下线
            </AppButton>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>
