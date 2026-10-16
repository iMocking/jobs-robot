<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import { systemApi } from '@/api'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AuthCard from '@/components/AuthCard.vue'
import AuthSteps from '@/components/AuthSteps.vue'
import { useSystemStore } from '@/stores/system'
import { useToastStore } from '@/stores/toast'

/**
 * 首次安装向导（/setup）。
 *
 * 与后端的配合关系：
 *   GET  /system/status            → 本页据此判断「展示向导 / 展示已关闭说明」（守卫已先行判断）
 *   POST /system/setup             → 一次性初始化：建管理员 + 可选落邮件配置
 *   POST /system/setup/mail/test   → 用**表单里正在填的**参数试发测试邮件，不落库
 *
 * 四条刻意的设计取舍：
 * 1. **测试邮件不落库**。这正是「先验证再保存」的意义：若先写库再试发，一个填错的
 *    host/授权码就已经热替换了投递器，系统立刻处于「验证码发不出去」的状态——
 *    而这恰恰是向导要避免的。所以测试按钮只把当前表单值发过去。
 * 2. **加密方式是一个三选一，而不是一个裸的 tls 开关**。映射严格依据
 *    internal/mail/smtp.go：tls=true 表示 tls.Dial 的**隐式 TLS**（465），
 *    tls=false 表示先明文连接、再按服务端 EXTENSION 自动升级 STARTTLS（587/25）。
 *    让运维直接选「465 / 587 / 25」比让他猜「隐式 TLS 是什么意思」可靠得多。
 * 3. **密码强度在前端先校验一遍**，规则与服务端 auth.ValidatePasswordStrength 对齐
 *    （≥8 位且同时含字母与数字）。前端校验只是省一次往返，服务端仍是权威。
 * 4. **提交成功后必须 refresh 系统状态**：否则路由守卫仍认为「未初始化」，
 *    用户点「去登录」会被立刻弹回本页，形成死循环。
 */
const router = useRouter()
const system = useSystemStore()
const toast = useToastStore()

const STEP_LABELS = ['管理员账号', '邮件服务', '确认完成']

const step = ref(1)
const submitting = ref(false)
const done = ref(false)
const result = ref(null)

/* —— 第 1 步：管理员账号 —— */

const admin = reactive({ email: '', nickname: '', password: '', confirm: '' })
const adminErrors = reactive({ email: '', password: '', confirm: '' })
const showAdminPassword = ref(false)

/** 与服务端 ValidatePasswordStrength 对齐；服务端阈值来自 auth.password_min_length（默认 8） */
const passwordChecks = computed(() => ({
  length: admin.password.length >= 8,
  letter: /[a-zA-Z]/.test(admin.password),
  digit: /\d/.test(admin.password),
}))

const passwordStrong = computed(
  () => passwordChecks.value.length && passwordChecks.value.letter && passwordChecks.value.digit,
)

function validateAdmin() {
  adminErrors.email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(admin.email.trim())
    ? ''
    : '请输入有效的邮箱地址'

  if (!passwordChecks.value.length) adminErrors.password = '密码至少 8 位'
  else if (!passwordChecks.value.letter || !passwordChecks.value.digit)
    adminErrors.password = '密码需同时包含字母与数字'
  else adminErrors.password = ''

  adminErrors.confirm = admin.confirm === admin.password ? '' : '两次输入的密码不一致'

  return !adminErrors.email && !adminErrors.password && !adminErrors.confirm
}

/* —— 第 2 步：邮件服务 —— */

// smtp：真实发信（推荐）；console：只打印到服务端日志（开发/演示）；skip：稍后到后台补配
const mailMode = ref('smtp')

const mail = reactive({
  from_name: 'JobHub 直招',
  from_address: '',
  host: '',
  port: 465,
  username: '',
  password: '',
  tls: true,
})

const mailErrors = reactive({ from_address: '', host: '', port: '', username: '', password: '' })
const showMailPassword = ref(false)

/**
 * 授权码首尾是否带空白（复制粘贴时几乎总会带上一个空格或换行）。
 *
 * 只用于提示，不阻断提交：后端统一 trimMailSecret 会自动去除并记一条不含口令的 WARN。
 */
const passwordHasPadding = computed(
  () => Boolean(mail.password) && mail.password !== mail.password.trim(),
)

const needsSmtpFields = computed(() => mailMode.value === 'smtp')

/**
 * 加密方式：三选一，直接对应 internal/mail/smtp.go 的两个分支。
 *
 * 映射依据（不要凭猜测改）：
 *   - 465 / SSL：cfg.SMTP.TLS=true  → dial() 走 tls.Dial，整条连接都是 TLS（隐式 TLS）；
 *   - 587 / STARTTLS：TLS=false     → 先 smtp.Dial 明文，若服务端通告 STARTTLS 则升级；
 *   - 25 / 不加密：TLS=false        → 同上但服务端通常不通告 STARTTLS，于是全程明文。
 *
 * 注意 587 与 25 在后端是**同一个分支**（都只传 TLS=false），差别在于服务端是否
 * 通告 STARTTLS。这里仍然拆成两个选项，因为运维需要知道「我选的是希望加密的」——
 * 而 25 端口在多数服务商那里会因明文投递被拒绝或降级，选它就该看到风险提示。
 */
const ENCRYPTIONS = [
  {
    value: 'ssl',
    label: 'SSL/TLS 直连',
    port: 465,
    tls: true,
    hint: '整条连接即 TLS（隐式 TLS），465 端口的标准做法，腾讯企业邮 / QQ / 163 都用它。',
  },
  {
    value: 'starttls',
    label: 'STARTTLS',
    port: 587,
    tls: false,
    hint: '先明文连接、再升级为 TLS（587 端口标准做法）。服务端不通告 STARTTLS 时后端会静默继续，因此选它更稳妥。',
  },
  {
    value: 'plain',
    label: '不加密',
    port: 25,
    tls: false,
    hint: '全程明文，仅用于内网邮件中继。公网服务商基本已拒绝 25 端口发信，凭据也会明文传输。',
  },
]

/** 只保存运维选的「加密方式」，端口可另行手改，因此这里不直接存 port */
const encryption = ref('ssl')

const encryptionHint = computed(
  () => ENCRYPTIONS.find((item) => item.value === encryption.value)?.hint ?? '',
)

/**
 * 选择加密方式时联动端口与 tls。
 *
 * 端口只在「当前端口仍等于上一档的默认端口」时才跟着改：运维把 465 手动改成
 * 2465（部分服务商如此）之后再切加密方式，不应该把他的自定义端口冲掉。
 */
function chooseEncryption(next) {
  const previous = ENCRYPTIONS.find((item) => item.value === encryption.value)
  const target = ENCRYPTIONS.find((item) => item.value === next)
  if (!target) return
  if (!previous || Number(mail.port) === previous.port || !Number(mail.port)) {
    mail.port = target.port
  }
  mail.tls = target.tls
  encryption.value = next
  invalidateTest()
}

/** 手动改端口时反向推断加密方式（465→SSL、587→STARTTLS、25→不加密），其余保持不动 */
function onPortChange() {
  const matched = ENCRYPTIONS.find((item) => item.port === Number(mail.port))
  if (matched) {
    encryption.value = matched.value
    mail.tls = matched.tls
  }
  invalidateTest()
}

/**
 * 常见服务商参考：点一下就把 host / 加密方式 / 端口填好。
 *
 * 做成一键填入而不是让运维去翻文档，是因为「端口 + 加密方式」这一对最容易填错，
 * 而填错的表现是「连接超时」或「认证失败」，排查成本极高。
 */
const PROVIDERS = [
  { name: '腾讯企业邮', host: 'smtp.exmail.qq.com', encryption: 'ssl' },
  { name: 'QQ 邮箱', host: 'smtp.qq.com', encryption: 'ssl' },
  { name: '163 邮箱', host: 'smtp.163.com', encryption: 'ssl' },
  { name: 'Gmail', host: 'smtp.gmail.com', encryption: 'starttls' },
  { name: '阿里云企业邮', host: 'smtp.qiye.aliyun.com', encryption: 'ssl' },
  { name: 'Outlook', host: 'smtp.office365.com', encryption: 'starttls' },
]

function applyProvider(provider) {
  mail.host = provider.host
  chooseEncryption(provider.encryption)
  // 选完加密方式后再强制一次端口：chooseEncryption 只在端口等于上一档默认值时才改，
  // 而这里运维的意图是「完全按这个服务商来」，因此以服务商推荐值为准。
  const target = ENCRYPTIONS.find((item) => item.value === provider.encryption)
  if (target) mail.port = target.port
  mail.tls = target?.tls ?? mail.tls
  invalidateTest()
}

/** 发件人地址与 SMTP 用户名默认跟随管理员邮箱（多数服务商要求两者一致） */
watch(
  () => admin.email,
  (value) => {
    const email = value.trim()
    if (!mail.from_address) mail.from_address = email
    if (!mail.username) mail.username = email
  },
)

function validateMail() {
  mailErrors.from_address = ''
  mailErrors.host = ''
  mailErrors.port = ''
  mailErrors.username = ''
  mailErrors.password = ''

  if (!needsSmtpFields.value) return true

  if (!mail.from_address.trim()) mailErrors.from_address = '发件人邮箱不能为空'
  else if (!mail.from_address.includes('@')) mailErrors.from_address = '发件人邮箱格式不正确'

  if (!mail.host.trim()) mailErrors.host = 'smtp 主机不能为空'

  const port = Number(mail.port)
  if (!Number.isInteger(port) || port <= 0 || port > 65535) {
    mailErrors.port = 'smtp 端口必须在 1~65535 之间'
  }

  // 服务端规则：用户名与密码要么都填、要么都空。只填一个几乎总是配置失误。
  const hasUser = Boolean(mail.username.trim())
  const hasPass = Boolean(mail.password)
  if (hasUser !== hasPass) {
    mailErrors.username = 'SMTP 用户名与密码需要同时填写（若服务器无需认证请都留空）'
  }

  // 授权码首尾空白不再报错：后端会自动 trim（复制授权码时几乎总会带一个空格），
  // 前端只做非阻断提示，避免「明明能自动修好却不让提交」。
  return Object.values(mailErrors).every((message) => !message)
}

/** 组装后端要的 mail 对象；null 表示「本次不配置邮件服务」 */
function mailPayload() {
  if (mailMode.value === 'skip') return null
  if (mailMode.value === 'console') {
    return {
      driver: 'console',
      from_name: mail.from_name.trim(),
      from_address: mail.from_address.trim(),
    }
  }
  return {
    driver: 'smtp',
    from_name: mail.from_name.trim(),
    from_address: mail.from_address.trim(),
    host: mail.host.trim(),
    port: Number(mail.port) || 0,
    username: mail.username.trim(),
    password: mail.password,
    tls: mail.tls,
  }
}

/* —— 测试邮件 —— */

const testTo = ref('')
const testing = ref(false)
const testPassed = ref(false)
const testError = ref('')

async function sendTest() {
  if (!validateMail()) {
    toast.error('请先修正邮件配置中标红的问题')
    return
  }
  // 收件人默认取管理员邮箱：向导阶段最可能收到的就是运维自己刚填的地址
  const to = testTo.value.trim() || admin.email.trim() || mail.from_address.trim()
  if (!to.includes('@')) {
    toast.error('请填写用于接收测试邮件的地址')
    return
  }

  testing.value = true
  testError.value = ''
  try {
    const response = await systemApi.testSetupMail({ mail: mailPayload(), to })
    testPassed.value = true
    toast.success(response?.message ?? '测试邮件已发送')
  } catch (err) {
    testPassed.value = false
    // 后台返回的是可读原因（连接失败 / STARTTLS 握手失败 / 认证失败 / RCPT 被拒），
    // 原样展示在按钮下方：只弹一句「失败」的话，运维不知道该改哪一项。
    testError.value = err.message
  } finally {
    testing.value = false
  }
}

/** 表单改动后先前的「测试通过」就不再可信 */
function invalidateTest() {
  testPassed.value = false
  testError.value = ''
}

/* —— 步骤流转 —— */

function next() {
  if (step.value === 1) {
    if (!validateAdmin()) return
    step.value = 2
    return
  }
  if (step.value === 2) {
    if (mailMode.value !== 'skip' && !validateMail()) return
    step.value = 3
  }
}

function back() {
  if (step.value > 1) step.value -= 1
}

/* —— 提交 —— */

const canSubmit = computed(() => system.canSetup && !submitting.value)

async function submit() {
  if (!validateAdmin()) {
    step.value = 1
    return
  }
  submitting.value = true
  try {
    const payload = {
      admin_email: admin.email.trim(),
      admin_password: admin.password,
      nickname: admin.nickname.trim() || admin.email.trim().split('@')[0],
      mail: mailPayload(),
    }
    const response = await systemApi.setup(payload)
    result.value = response
    // 关键：让守卫立刻知道系统已初始化，否则「去登录」会被弹回本页
    await system.refresh()
    done.value = true
  } catch (err) {
    toast.error(err.message)
  } finally {
    submitting.value = false
  }
}

function goLogin() {
  router.replace({ name: 'login' })
}

/* —— 向导被关闭时的说明页 —— */

const reloading = ref(false)

async function recheck() {
  reloading.value = true
  await system.refresh()
  reloading.value = false
  if (!system.canSetup) toast.info('仍未检测到可用的安装向导')
}
</script>

<template>
  <!-- 完成 -->
  <AuthCard
    v-if="done && result"
    title="初始化完成"
    subtitle="管理员账号已创建，安装向导已永久关闭"
  >
    <div class="text-center">
      <span class="mx-auto flex size-16 items-center justify-center rounded-full bg-accent-50">
        <AppIcon name="check-circle" :size="32" class="text-accent-600" />
      </span>
    </div>

    <dl class="mt-6 space-y-3 rounded-xl bg-ink-50 p-4 text-[13px]">
      <div class="flex items-center justify-between gap-3">
        <dt class="text-ink-500">管理员邮箱</dt>
        <dd class="font-medium text-ink-900">{{ result.admin?.email }}</dd>
      </div>
      <div class="flex items-center justify-between gap-3">
        <dt class="text-ink-500">角色</dt>
        <dd>
          <AppBadge tone="brand" size="sm">管理员</AppBadge>
        </dd>
      </div>
      <div class="flex items-center justify-between gap-3">
        <dt class="text-ink-500">邮件通道</dt>
        <dd class="flex items-center gap-1.5">
          <AppBadge :tone="result.mail?.ready ? 'accent' : 'ink'" size="sm">
            {{ result.mail?.driver || '—' }}{{ result.mail?.ready ? '（可用）' : '' }}
          </AppBadge>
          <span v-if="!result.mail_configured" class="text-[12px] text-ink-400">本次跳过</span>
        </dd>
      </div>
      <div class="flex items-center justify-between gap-3">
        <dt class="text-ink-500">注册通道</dt>
        <dd>
          <AppBadge :tone="result.registration_open ? 'accent' : 'amber'" size="sm">
            {{ result.registration_open ? '可用' : '不可用（缺 SMTP）' }}
          </AppBadge>
        </dd>
      </div>
    </dl>

    <p
      v-if="!result.registration_open"
      class="mt-4 flex items-start gap-1.5 rounded-lg bg-amber-50 px-3 py-2.5 text-[12.5px] leading-relaxed text-amber-700"
    >
      <AppIcon name="alert" :size="15" class="mt-0.5 shrink-0" />
      未配置邮件服务：用户注册与找回密码暂不可用（验证码发不出去），可稍后在后台「系统配置 → 邮件服务」补配并发送测试邮件。
    </p>

    <p
      v-if="system.bootstrapCredentialFileExists"
      class="mt-3 flex items-start gap-1.5 rounded-lg bg-amber-50 px-3 py-2.5 text-[12.5px] leading-relaxed text-amber-700"
    >
      <AppIcon name="alert" :size="15" class="mt-0.5 shrink-0" />
      初始凭据文件仍然存在，登录后请及时删除它（其中包含启动自举生成的明文口令）。
    </p>

    <AppButton size="lg" block class="mt-6" @click="goLogin">进入后台</AppButton>

    <template #footer>
      <p>安装向导已关闭：系统中已存在可用管理员。</p>
    </template>
  </AuthCard>

  <!-- 向导被配置关闭（且系统尚无管理员）：给出可执行的替代步骤 -->
  <AuthCard
    v-else-if="!system.canSetup"
    title="安装向导已关闭"
    subtitle="系统尚未初始化，但 auth.setup.enabled 为 false"
  >
    <div class="rounded-xl bg-amber-50 p-4 text-[13px] leading-relaxed text-amber-800">
      <p class="flex items-center gap-1.5 font-medium">
        <AppIcon name="alert" :size="16" />
        无法通过界面完成初始化
      </p>
      <p class="mt-2">
        安装向导已在配置中关闭（<code class="rounded bg-white/70 px-1 py-0.5 font-mono text-[12px]">auth.setup.enabled=false</code>），
        通常意味着这个部署由外部系统预置账号。请改用命令行自举管理员：
      </p>
    </div>

    <pre
      class="mt-4 overflow-x-auto rounded-xl bg-ink-900 p-4 font-mono text-[12px] leading-relaxed text-ink-100"
    >go run ./cmd/seed \
  -admin-email you@example.com \
  -admin-password 'YourPassw0rd'</pre>

    <p class="mt-3 text-[12.5px] leading-relaxed text-ink-500">
      该命令是幂等的：已存在的管理员不会被改动，也不会覆盖配置。执行完成后本页会自动失效。
    </p>

    <AppButton variant="secondary" block class="mt-5" :loading="reloading" @click="recheck">
      重新检测
    </AppButton>
  </AuthCard>

  <!-- 向导主流程 -->
  <AuthCard
    v-else
    :title="
      step === 1 ? '初始化 JobHub 直招' : step === 2 ? '配置邮件服务' : '确认初始化'
    "
    :subtitle="
      step === 1
        ? '创建第一个管理员账号，完成系统初始化'
        : step === 2
          ? '验证码与通知邮件都走这里，也可以稍后到后台补配'
          : '提交后安装向导将永久关闭，无法再次进入'
    "
  >
    <template #header>
      <AuthSteps :current="step" :steps="STEP_LABELS" />
    </template>

    <!-- 第 1 步 -->
    <form v-if="step === 1" class="space-y-4" novalidate @submit.prevent="next">
      <AppInput
        v-model="admin.email"
        icon="mail"
        type="email"
        autocomplete="email"
        placeholder="管理员邮箱"
        :error="adminErrors.email"
      />

      <AppInput
        v-model="admin.nickname"
        icon="user"
        placeholder="昵称（留空则取邮箱前缀）"
      />

      <div>
        <AppInput
          v-model="admin.password"
          icon="lock"
          :type="showAdminPassword ? 'text' : 'password'"
          autocomplete="new-password"
          placeholder="设置密码（至少 8 位，含字母与数字）"
          :error="adminErrors.password"
        >
          <template #suffix>
            <button
              type="button"
              class="rounded p-1 text-ink-400 transition hover:text-ink-600"
              :aria-label="showAdminPassword ? '隐藏密码' : '显示密码'"
              @click="showAdminPassword = !showAdminPassword"
            >
              <AppIcon :name="showAdminPassword ? 'eye-off' : 'eye'" :size="17" />
            </button>
          </template>
        </AppInput>
        <ul class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[12px]">
          <li
            v-for="item in [
              { ok: passwordChecks.length, label: '至少 8 位' },
              { ok: passwordChecks.letter, label: '包含字母' },
              { ok: passwordChecks.digit, label: '包含数字' },
            ]"
            :key="item.label"
            class="flex items-center gap-1"
            :class="item.ok ? 'text-accent-600' : 'text-ink-400'"
          >
            <AppIcon :name="item.ok ? 'check-circle' : 'info'" :size="13" />
            {{ item.label }}
          </li>
        </ul>
      </div>

      <AppInput
        v-model="admin.confirm"
        icon="lock"
        :type="showAdminPassword ? 'text' : 'password'"
        autocomplete="new-password"
        placeholder="确认密码"
        :error="adminErrors.confirm"
      />

      <p class="rounded-lg bg-ink-50 px-3 py-2.5 text-[12.5px] leading-relaxed text-ink-500">
        该账号将获得<strong class="font-medium text-ink-700">全部管理权限</strong>：
        管理公司、审核岗位、修改系统配置与授权策略。请使用真实可达的邮箱，找回密码依赖它。
      </p>

      <AppButton type="submit" size="lg" block>下一步</AppButton>
    </form>

    <!-- 第 2 步 -->
    <form v-else-if="step === 2" class="space-y-4" novalidate @submit.prevent="next">
      <div class="space-y-2.5">
        <button
          v-for="option in [
            {
              value: 'smtp',
              icon: 'mail',
              label: '配置 SMTP 真实发信',
              hint: '注册验证码、密码重置通知都依赖它；建议现在配好并测试。',
            },
            {
              value: 'console',
              icon: 'document',
              label: '开发模式（console）',
              hint: '不真实发信，验证码等邮件内容只打印到服务端日志，适合本地联调。',
            },
            {
              value: 'skip',
              icon: 'clock',
              label: '稍后到后台补配',
              hint: '只创建管理员。若注册要求邮箱验证，此时注册与找回密码都不可用。',
            },
          ]"
          :key="option.value"
          type="button"
          class="w-full rounded-xl border p-3.5 text-left transition"
          :class="
            mailMode === option.value
              ? 'border-brand-500 bg-brand-50/60 ring-1 ring-brand-200'
              : 'border-ink-200 bg-white hover:border-brand-300'
          "
          :aria-pressed="mailMode === option.value"
          @click="mailMode = option.value; invalidateTest()"
        >
          <span class="flex items-center gap-2">
            <AppIcon
              :name="option.icon"
              :size="17"
              :class="mailMode === option.value ? 'text-brand-600' : 'text-ink-400'"
            />
            <span class="text-[13.5px] font-medium text-ink-900">{{ option.label }}</span>
            <AppIcon
              v-if="mailMode === option.value"
              name="check-circle"
              :size="15"
              class="ml-auto text-brand-600"
            />
          </span>
          <span class="mt-1.5 block text-[12px] leading-relaxed text-ink-500">{{ option.hint }}</span>
        </button>
      </div>

      <p
        v-if="mailMode === 'skip' && !system.registrationOpen"
        class="flex items-start gap-1.5 rounded-lg bg-amber-50 px-3 py-2.5 text-[12.5px] leading-relaxed text-amber-700"
      >
        <AppIcon name="alert" :size="15" class="mt-0.5 shrink-0" />
        未配置邮件服务：用户注册与找回密码暂不可用（验证码发不出去），可稍后在后台「系统配置 → 邮件服务」补配。
      </p>

      <template v-if="needsSmtpFields">
        <!-- 常见服务商：一键把 host + 加密方式 + 端口填对 -->
        <div class="rounded-xl border border-ink-200 bg-white p-3.5">
          <p class="flex items-center gap-1.5 text-[12.5px] font-medium text-ink-700">
            <AppIcon name="bolt" :size="14" class="text-brand-600" />
            常见服务商参考（点击自动填入主机与端口）
          </p>
          <div class="mt-2.5 flex flex-wrap gap-2">
            <button
              v-for="provider in PROVIDERS"
              :key="provider.name"
              type="button"
              class="rounded-lg border px-2.5 py-1.5 text-left text-[12px] transition"
              :class="
                mail.host === provider.host
                  ? 'border-brand-400 bg-brand-50 text-brand-700'
                  : 'border-ink-200 text-ink-600 hover:border-brand-300 hover:text-brand-700'
              "
              @click="applyProvider(provider)"
            >
              <span class="font-medium">{{ provider.name }}</span>
              <span class="ml-1.5 font-mono text-[11.5px] text-ink-400">
                {{ provider.host }}:{{
                  ENCRYPTIONS.find((item) => item.value === provider.encryption)?.port
                }}
                {{ provider.encryption === 'ssl' ? 'SSL' : 'STARTTLS' }}
              </span>
            </button>
          </div>
          <p class="mt-2 text-[11.5px] leading-relaxed text-ink-400">
            多数服务商要求这里的密码填<strong class="font-medium text-ink-500">授权码</strong>而非登录密码；
            未列出的服务商请查阅其「客户端 SMTP 设置」文档。
          </p>
        </div>

        <!-- 加密方式：三选一，联动 tls 字段与端口 -->
        <div>
          <p class="mb-2 text-[12.5px] font-medium text-ink-700">加密方式</p>
          <div class="grid gap-2.5 sm:grid-cols-3">
            <button
              v-for="item in ENCRYPTIONS"
              :key="item.value"
              type="button"
              class="rounded-xl border p-3 text-left transition"
              :class="
                encryption === item.value
                  ? 'border-brand-500 bg-brand-50/60 ring-1 ring-brand-200'
                  : 'border-ink-200 bg-white hover:border-brand-300'
              "
              :aria-pressed="encryption === item.value"
              @click="chooseEncryption(item.value)"
            >
              <span class="flex items-center gap-1.5">
                <span class="text-[13px] font-medium text-ink-900">{{ item.label }}</span>
                <AppIcon
                  v-if="encryption === item.value"
                  name="check-circle"
                  :size="14"
                  class="ml-auto text-brand-600"
                />
              </span>
              <span class="mt-1 block font-mono text-[11.5px] text-ink-400">
                {{ item.port }} · tls={{ item.tls }}
              </span>
            </button>
          </div>
          <p class="mt-2 text-[12px] leading-relaxed text-ink-400">{{ encryptionHint }}</p>
        </div>

        <div class="grid gap-4 sm:grid-cols-[minmax(0,1fr)_140px]">
          <AppInput
            v-model="mail.host"
            icon="globe"
            placeholder="smtp 主机，如 smtp.exmail.qq.com"
            :error="mailErrors.host"
            @update:model-value="invalidateTest"
          />
          <AppInput
            v-model.number="mail.port"
            type="number"
            placeholder="端口"
            :error="mailErrors.port"
            hint="可手改"
            @update:model-value="onPortChange"
          />
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <AppInput
            v-model="mail.username"
            icon="user"
            placeholder="SMTP 账号（通常是完整邮箱）"
            :error="mailErrors.username"
            autocomplete="off"
            @update:model-value="invalidateTest"
          />
          <AppInput
            v-model="mail.from_address"
            icon="mail"
            type="email"
            placeholder="发件人邮箱，如 ops@example.com"
            :error="mailErrors.from_address"
            @update:model-value="invalidateTest"
          />
        </div>

        <AppInput
          v-model="mail.password"
          icon="key"
          :type="showMailPassword ? 'text' : 'password'"
          placeholder="SMTP 密码 / 授权码"
          autocomplete="new-password"
          :error="mailErrors.password"
          hint="请填写服务商生成的授权码（非登录密码）；首尾空白会被自动忽略。"
          @update:model-value="invalidateTest"
        >
          <template #suffix>
            <button
              type="button"
              class="rounded p-1 text-ink-400 transition hover:text-ink-600"
              :aria-label="showMailPassword ? '隐藏密码' : '显示密码'"
              @click="showMailPassword = !showMailPassword"
            >
              <AppIcon :name="showMailPassword ? 'eye-off' : 'eye'" :size="17" />
            </button>
          </template>
        </AppInput>
        <p v-if="passwordHasPadding" class="flex items-center gap-1.5 text-[12px] text-amber-600">
          <AppIcon name="alert" :size="13" />
          检测到授权码首尾有空白字符，提交与发信时会自动忽略，无需手动删。
        </p>

        <AppInput
          v-model="mail.from_name"
          icon="user"
          placeholder="发件人名称，如 直招好工作（可留空）"
          @update:model-value="invalidateTest"
        />

        <!-- 测试发信：用当前表单值，不落库 -->
        <div class="rounded-xl border border-ink-200 bg-ink-50/70 p-3.5">
          <p class="flex items-center gap-1.5 text-[12.5px] font-medium text-ink-700">
            <AppIcon name="bolt" :size="14" class="text-brand-600" />
            发送测试邮件
          </p>
          <p class="mt-1 text-[12px] leading-relaxed text-ink-400">
            使用上面填写但<strong class="font-medium text-ink-500">尚未保存</strong>的参数试发一封，
            用来确认主机 / 端口 / 加密方式 / 授权码是否正确。测试不会写入任何配置。
          </p>
          <div class="mt-2.5 flex flex-wrap items-start gap-2">
            <div class="min-w-[200px] flex-1">
              <AppInput
                v-model="testTo"
                icon="mail"
                type="email"
                placeholder="接收测试邮件的地址（默认管理员邮箱）"
              />
            </div>
            <AppButton variant="secondary" :loading="testing" @click="sendTest">
              发送测试邮件
            </AppButton>
          </div>

          <p
            v-if="testPassed"
            class="mt-2 flex items-start gap-1.5 rounded-lg bg-accent-50 px-2.5 py-2 text-[12.5px] text-accent-700"
          >
            <AppIcon name="check-circle" :size="14" class="mt-0.5 shrink-0" />
            测试邮件已发出，说明这组参数可用。
          </p>
          <p
            v-else-if="testError"
            class="mt-2 flex items-start gap-1.5 rounded-lg bg-red-50 px-2.5 py-2 text-[12.5px] leading-relaxed text-red-600"
          >
            <AppIcon name="alert" :size="14" class="mt-0.5 shrink-0" />
            <span>
              <!-- whitespace-pre-line：后端的排查步骤是多行文本（1) 2) 3)…），
                   必须按行渲染，否则会被折叠成一整段而失去可读性 -->
              <span class="whitespace-pre-line">{{ testError }}</span>
              <br />
              <span class="text-red-500">
                可以先修正上面的参数再试，或直接提交——提交后仍可在后台「邮件服务」重新配置。
              </span>
            </span>
          </p>
        </div>
      </template>

      <div class="flex items-center gap-3">
        <AppButton variant="secondary" @click="back">上一步</AppButton>
        <AppButton type="submit" class="flex-1">下一步</AppButton>
      </div>
    </form>

    <!-- 第 3 步 -->
    <div v-else class="space-y-4">
      <dl class="space-y-3 rounded-xl bg-ink-50 p-4 text-[13px]">
        <div class="flex items-center justify-between gap-3">
          <dt class="text-ink-500">管理员邮箱</dt>
          <dd class="font-medium text-ink-900">{{ admin.email }}</dd>
        </div>
        <div class="flex items-center justify-between gap-3">
          <dt class="text-ink-500">昵称</dt>
          <dd class="text-ink-800">{{ admin.nickname || admin.email.split('@')[0] }}</dd>
        </div>
        <div class="flex items-center justify-between gap-3">
          <dt class="text-ink-500">邮件服务</dt>
          <dd>
            <AppBadge
              :tone="mailMode === 'smtp' ? 'accent' : mailMode === 'console' ? 'ink' : 'amber'"
              size="sm"
            >
              {{ mailMode === 'smtp' ? 'SMTP 真实发信' : mailMode === 'console' ? '开发模式' : '稍后补配' }}
            </AppBadge>
          </dd>
        </div>
        <div v-if="mailMode === 'smtp'" class="flex items-center justify-between gap-3">
          <dt class="text-ink-500">SMTP</dt>
          <dd class="font-mono text-[12.5px] text-ink-800">
            {{ mail.host }}:{{ mail.port }} · {{ mail.tls ? 'SSL/TLS 直连' : encryption === 'plain' ? '不加密' : 'STARTTLS' }}
          </dd>
        </div>
        <div v-if="mailMode === 'smtp'" class="flex items-center justify-between gap-3">
          <dt class="text-ink-500">测试邮件</dt>
          <dd>
            <AppBadge :tone="testPassed ? 'accent' : 'amber'" size="sm">
              {{ testPassed ? '已验证通过' : '未验证（仍可提交）' }}
            </AppBadge>
          </dd>
        </div>
      </dl>

      <p
        v-if="mailMode === 'skip' || mailMode === 'console'"
        class="flex items-start gap-1.5 rounded-lg bg-amber-50 px-3 py-2.5 text-[12.5px] leading-relaxed text-amber-700"
      >
        <AppIcon name="alert" :size="15" class="mt-0.5 shrink-0" />
        未配置邮件服务：用户注册与找回密码暂不可用，可稍后在后台「系统配置 → 邮件服务」补配。
      </p>

      <p
        v-else-if="!testPassed"
        class="flex items-start gap-1.5 rounded-lg bg-amber-50 px-3 py-2.5 text-[12.5px] leading-relaxed text-amber-700"
      >
        <AppIcon name="alert" :size="15" class="mt-0.5 shrink-0" />
        这组 SMTP 参数尚未通过测试邮件验证。仍可提交，但请准备好登录后台重新调整。
      </p>

      <p class="flex items-start gap-1.5 rounded-lg bg-ink-50 px-3 py-2.5 text-[12.5px] leading-relaxed text-ink-500">
        <AppIcon name="info" :size="15" class="mt-0.5 shrink-0" />
        提交后系统中将存在可用管理员，安装向导<strong class="font-medium text-ink-700">永久关闭</strong>，
        无法再次通过本页创建账号。后续新增管理员请登录后台操作。
      </p>

      <div class="flex items-center gap-3">
        <AppButton variant="secondary" :disabled="submitting" @click="back">上一步</AppButton>
        <AppButton class="flex-1" :loading="submitting" :disabled="!canSubmit" @click="submit">
          完成初始化
        </AppButton>
      </div>
    </div>

    <template #footer>
      <p class="text-[12px] leading-relaxed text-ink-400">
        本次初始化共 {{ system.userCount }} 个账号、{{ system.adminCount }} 名管理员。
      </p>
    </template>
  </AuthCard>
</template>
