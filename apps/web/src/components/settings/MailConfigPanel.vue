<script setup>
import { computed, reactive, ref, watch } from 'vue'

import { adminApi } from '@/api'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppModal from '@/components/AppModal.vue'
import { useAsync } from '@/composables/useAsync'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useSessionStore } from '@/stores/session'
import { useToastStore } from '@/stores/toast'
import { formatDateTime } from '@/utils/format'

/**
 * 邮件（SMTP）服务配置。
 *
 * 对应 internal/service/mail.go：配置以 configs/config.yaml 为基线、被数据库
 * mail.smtp 覆盖，保存后热替换投递器（无需重启），并立即对验证码/通知邮件生效。
 *
 * 三条刻意的设计约束：
 * 1. **密码永不回显**。后端只返回 has_password 与占位符，因此这里也绝不把密码放进
 *    placeholder 或 value；「留空 = 不修改」是接口约定，不是偷懒。
 * 2. **校验规则与服务端逐条对齐**（config.Mail.Validate），错误文案也用同一套中文，
 *    否则会出现「前端放行、后端 10001」的割裂体验。前端校验只是省一次往返。
 * 3. **测试邮件用的是「已保存」的配置**，不是表单里未保存的改动。必须写在按钮旁边，
 *    否则管理员会误以为「改了没生效」。
 */
const props = defineProps({
  /** 配置保存成功后通知父级，便于刷新其它面板与审计信息 */
  onSaved: { type: Function, default: null },
})

const toast = useToastStore()
const session = useSessionStore()

// immediate: false + useAdminRefresh：挂载时正好加载一次，顶栏「刷新」也能重取，
// 不会出现「挂载即请求 + onMounted 再请求一次」的重复调用。
const { data, loading, error, ready, run } = useAsync(() => adminApi.mail.config(), {
  immediate: false,
})
useAdminRefresh(run)

/** 表单副本；密码单独处理，见下面的 passwordDirty */
const form = reactive({
  driver: 'console',
  from_name: '',
  from_address: '',
  host: '',
  port: 465,
  username: '',
  password: '',
  tls: true,
  clear_password: false,
})

const errors = reactive({
  driver: '',
  from_address: '',
  host: '',
  port: '',
  username: '',
  password: '',
})

const baseline = ref('')
const changingPassword = ref(false)
const saving = ref(false)

/**
 * 把后端视图灌进表单，并同步「当前生效配置」的展示数据。
 *
 * 必须同时更新 data.value：状态卡上的来源徽标、投递器、最后保存时间都读它。
 * 早期版本只更新表单，导致保存成功后来源徽标仍是旧值（明明已写入数据库却还写着
 * 「来源：配置文件」），连「回落到配置文件」按钮都不会出现。
 *
 * 从 watch 回调进来时传入的就是 data 当前持有的同一个对象引用，
 * shallowRef 的 Object.is 比较会判定「无变化」，因此不会自触发死循环。
 */
function hydrate(view) {
  if (!view) return
  data.value = view
  form.driver = view.driver || 'console'
  form.from_name = view.from_name || ''
  form.from_address = view.from_address || ''
  form.host = view.host || ''
  form.port = view.port > 0 ? view.port : 465
  form.username = view.username || ''
  form.password = ''
  form.tls = Boolean(view.tls)
  form.clear_password = false
  changingPassword.value = false
  Object.keys(errors).forEach((key) => {
    errors[key] = ''
  })
  // 用结构化快照做脏检查：只有真正改过的字段才值得写一遍
  baseline.value = JSON.stringify({
    driver: form.driver,
    from_name: form.from_name,
    from_address: form.from_address,
    host: form.host,
    port: form.port,
    username: form.username,
    tls: form.tls,
    clear_password: false,
  })
}

watch(data, (value) => hydrate(value), { immediate: true })

const snapshot = computed(() =>
  JSON.stringify({
    driver: form.driver,
    from_name: form.from_name,
    from_address: form.from_address,
    host: form.host,
    port: Number(form.port) || 0,
    username: form.username,
    tls: form.tls,
    clear_password: form.clear_password,
  }),
)

const passwordTouched = computed(() => form.password !== '' || form.clear_password)
const dirty = computed(() => baseline.value !== '' && snapshot.value !== baseline.value)

const isSmtp = computed(() => form.driver === 'smtp')

const sourceLabel = computed(() => (data.value?.source === 'database' ? '后台设置' : '配置文件'))
const sourceHint = computed(() =>
  data.value?.source === 'database'
    ? '当前生效的是后台保存的配置，已覆盖 configs/config.yaml'
    : '当前生效的是 configs/config.yaml 中的配置，尚未在后台改过',
)

/**
 * 是否最终会有密码：想清空就是没有；填了新密码就是有；否则沿用已保存的状态。
 * 用户名与密码「要么都填、要么都空」是服务端的硬规则，这里据此提示。
 */
const willHavePassword = computed(() => {
  if (form.clear_password) return false
  if (form.password) return true
  return Boolean(data.value?.has_password)
})

const authMismatch = computed(() => Boolean(form.username.trim()) !== willHavePassword.value)

function validate() {
  errors.driver = ['console', 'smtp'].includes(form.driver) ? '' : '邮件驱动只支持 console/smtp'
  errors.host = ''
  errors.port = ''
  errors.username = ''
  errors.from_address = ''

  if (form.driver === 'smtp') {
    if (!form.host.trim()) errors.host = 'smtp 主机不能为空'

    const port = Number(form.port)
    if (!Number.isInteger(port) || port <= 0 || port > 65535) {
      errors.port = 'smtp 端口必须在 1~65535 之间'
    }

    if (!form.from_address.trim()) errors.from_address = '发件人邮箱不能为空'
    else if (!form.from_address.includes('@')) errors.from_address = '发件人邮箱格式不正确'

    if (authMismatch.value) {
      errors.username = 'SMTP 用户名与密码需要同时填写（若服务器无需认证请都留空）'
    }
  }

  return Object.values(errors).every((message) => !message)
}

async function save() {
  if (!validate()) {
    toast.error('请先修正表单中标红的问题')
    return
  }

  saving.value = true
  try {
    const payload = {
      driver: form.driver,
      from_name: form.from_name.trim(),
      from_address: form.from_address.trim(),
      host: form.host.trim(),
      port: Number(form.port) || 0,
      username: form.username.trim(),
      tls: form.tls,
      clear_password: form.clear_password,
      // 空字符串即「不修改已保存的密码」；这是接口约定而不是省略字段
      password: form.clear_password ? '' : form.password,
    }
    const view = await adminApi.mail.saveConfig(payload)
    hydrate(view)
    toast.success('邮件配置已保存并立即生效')
    props.onSaved?.()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

/* —— 发送测试邮件 —— */
const testOpen = ref(false)
const testTo = ref('')
const testing = ref(false)
/**
 * 试发失败的完整说明。
 *
 * 后端的失败文案是多段/多行文本（结论 + 排查步骤 1) 2) 3) + 服务器原文），
 * 只弹一个 4 秒的 toast 根本读不完，因此这里在弹窗内**原地**渲染，
 * 且弹窗保持打开，运维可以边看提示边改参数。
 */
const testError = ref('')

function openTest() {
  testTo.value = form.from_address || session.user?.email || ''
  testError.value = ''
  testOpen.value = true
}

async function submitTest() {
  const to = testTo.value.trim()
  if (!to.includes('@')) {
    toast.error('收件邮箱格式不正确')
    return
  }
  testing.value = true
  testError.value = ''
  try {
    const result = await adminApi.mail.sendTest(to)
    toast.success(result?.message ?? '测试邮件已发送')
    testOpen.value = false
  } catch (err) {
    // 同时给出短提示与可细读的原地说明：toast 告知「失败了」，弹窗里给出「怎么修」
    testError.value = err.message
    toast.error('测试邮件发送失败，详情见弹窗内的排查步骤')
  } finally {
    testing.value = false
  }
}

/* —— 回落到配置文件 —— */
const revertOpen = ref(false)
const reverting = ref(false)

async function submitRevert() {
  reverting.value = true
  try {
    const view = await adminApi.mail.revertConfig()
    hydrate(view)
    toast.success('已删除后台设置，回落到 configs/config.yaml')
    revertOpen.value = false
    props.onSaved?.()
  } catch (err) {
    toast.error(err.message)
  } finally {
    reverting.value = false
  }
}

const DRIVERS = [
  {
    value: 'console',
    label: '开发模式（console）',
    description: '不真实发信，把验证码等邮件内容打印到服务端日志，便于本地联调。',
    icon: 'document',
  },
  {
    value: 'smtp',
    label: 'SMTP 真实发信',
    description: '通过邮件服务商投递。保存后立即热生效，用于注册验证码与密码重置通知。',
    icon: 'mail',
  },
]
</script>

<template>
  <div>
    <div v-if="loading && !ready" class="space-y-4">
      <div class="h-24 animate-pulse rounded-card bg-ink-100" />
      <div class="h-64 animate-pulse rounded-card bg-ink-100" />
    </div>

    <div v-else-if="error" class="rounded-card border border-red-200 bg-red-50 p-5">
      <p class="flex items-center gap-2 text-[14px] font-medium text-red-700">
        <AppIcon name="alert" :size="17" />
        邮件配置加载失败
      </p>
      <p class="mt-1.5 text-[13px] text-red-600">{{ error.message }}</p>
      <AppButton variant="secondary" size="sm" class="mt-3" @click="run()">重新加载</AppButton>
    </div>

    <template v-else>
      <!-- 当前生效状态：先回答「现在到底是什么在发信」 -->
      <section class="card mb-5 p-5">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-[16px] font-semibold text-ink-900">当前生效配置</h2>
              <AppBadge :tone="data?.source === 'database' ? 'brand' : 'ink'" size="sm">
                来源：{{ sourceLabel }}
              </AppBadge>
              <AppBadge :tone="data?.ready ? 'accent' : 'amber'" size="sm" dot>
                投递器：{{ data?.active_driver || '未知' }}{{ data?.ready ? '（可用）' : '（不可用）' }}
              </AppBadge>
            </div>

            <p class="mt-2 text-[12.5px] text-ink-500">{{ sourceHint }}</p>

            <p v-if="data?.updated_at" class="mt-1 text-[12.5px] text-ink-400">
              最后保存：{{ formatDateTime(data.updated_at) }}
              <span v-if="data?.updated_by">· {{ data.updated_by }}</span>
            </p>
          </div>

          <AppButton
            v-if="data?.source === 'database'"
            variant="secondary"
            size="sm"
            @click="revertOpen = true"
          >
            <AppIcon name="refresh" :size="15" />
            回落到配置文件
          </AppButton>
        </div>
      </section>

      <!-- 驱动选择 -->
      <section class="card mb-5 p-6">
        <h2 class="text-[16px] font-semibold text-ink-900">投递方式</h2>
        <p class="mt-1 mb-4 text-[12.5px] text-ink-400">
          验证码、密码重置通知都走这里。开发阶段用 console 即可，上线前切到 SMTP。
        </p>

        <div class="grid gap-3 sm:grid-cols-2">
          <button
            v-for="item in DRIVERS"
            :key="item.value"
            type="button"
            class="rounded-xl border p-4 text-left transition"
            :class="
              form.driver === item.value
                ? 'border-brand-500 bg-brand-50/60 ring-1 ring-brand-200'
                : 'border-ink-200 bg-white hover:border-brand-300'
            "
            :aria-pressed="form.driver === item.value"
            @click="form.driver = item.value"
          >
            <span class="flex items-center gap-2">
              <AppIcon
                :name="item.icon"
                :size="18"
                :class="form.driver === item.value ? 'text-brand-600' : 'text-ink-400'"
              />
              <span class="text-[14px] font-medium text-ink-900">{{ item.label }}</span>
              <AppIcon
                v-if="form.driver === item.value"
                name="check-circle"
                :size="16"
                class="ml-auto text-brand-600"
              />
            </span>
            <span class="mt-2 block text-[12.5px] leading-relaxed text-ink-500">
              {{ item.description }}
            </span>
          </button>
        </div>

        <p v-if="errors.driver" class="mt-2 text-[12.5px] text-red-600">{{ errors.driver }}</p>
      </section>

      <!-- SMTP 参数 -->
      <section class="card mb-5 p-6">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 class="text-[16px] font-semibold text-ink-900">发件与 SMTP 参数</h2>
            <p class="mt-1 text-[12.5px] text-ink-400">
              465 端口请开启「隐式 TLS」，587 端口请关闭（自动走 STARTTLS 升级）。
            </p>
          </div>
          <AppBadge v-if="!isSmtp" tone="ink" size="sm">当前为开发模式，以下参数暂不生效</AppBadge>
        </div>

        <div class="divide-y divide-ink-100">
          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">发件人名称</label>
            <div class="max-w-[520px]">
              <AppInput v-model="form.from_name" placeholder="例如：JobHub 直招" />
              <p class="mt-1.5 text-[12px] text-ink-400">收件人看到的发件人显示名，可留空。</p>
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">
              发件人邮箱
              <span v-if="isSmtp" class="text-red-500">*</span>
            </label>
            <div class="max-w-[520px]">
              <AppInput
                v-model="form.from_address"
                icon="mail"
                type="email"
                placeholder="no-reply@your-domain.com"
                :error="errors.from_address"
              />
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">
              SMTP 主机
              <span v-if="isSmtp" class="text-red-500">*</span>
            </label>
            <div class="max-w-[520px]">
              <AppInput
                v-model="form.host"
                icon="globe"
                placeholder="smtp.example.com"
                :error="errors.host"
              />
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">SMTP 端口</label>
            <div class="max-w-[200px]">
              <AppInput v-model.number="form.port" type="number" :error="errors.port" />
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">SMTP 用户名</label>
            <div class="max-w-[520px]">
              <AppInput
                v-model="form.username"
                icon="user"
                placeholder="通常是完整邮箱地址"
                :error="errors.username"
                autocomplete="off"
              />
              <p class="mt-1.5 text-[12px] text-ink-400">
                若邮件服务器无需认证，用户名与密码请都留空。
              </p>
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">SMTP 密码 / 授权码</label>
            <div class="max-w-[520px]">
              <!-- 已保存且未在修改：只展示状态，绝不回显口令 -->
              <template v-if="data?.has_password && !changingPassword && !form.clear_password">
                <div
                  class="flex h-11 items-center gap-2 rounded-xl border border-ink-200 bg-ink-50 px-3.5"
                >
                  <AppIcon name="lock" :size="17" class="text-ink-400" />
                  <span class="flex-1 font-mono text-sm tracking-widest text-ink-500">
                    {{ data.password_mask }}
                  </span>
                  <AppBadge tone="accent" size="sm">已配置</AppBadge>
                  <button
                    type="button"
                    class="text-[12.5px] text-brand-600 transition hover:text-brand-700"
                    @click="changingPassword = true"
                  >
                    修改
                  </button>
                  <button
                    type="button"
                    class="text-[12.5px] text-ink-400 transition hover:text-red-600"
                    @click="form.clear_password = true"
                  >
                    清空
                  </button>
                </div>
                <p class="mt-1.5 text-[12px] text-ink-400">
                  出于安全考虑，已保存的密码不会回显。留空提交即表示保持原密码不变。
                </p>
              </template>

              <!-- 新填 / 修改密码 -->
              <template v-else>
                <AppInput
                  v-model="form.password"
                  icon="key"
                  type="password"
                  autocomplete="new-password"
                  placeholder="输入新的 SMTP 密码或授权码"
                  :disabled="form.clear_password"
                />
                <div class="mt-1.5 flex flex-wrap items-center gap-3">
                  <button
                    v-if="data?.has_password && changingPassword"
                    type="button"
                    class="text-[12.5px] text-ink-400 transition hover:text-ink-700"
                    @click="changingPassword = false; form.password = ''"
                  >
                    取消修改
                  </button>
                  <button
                    v-if="form.clear_password"
                    type="button"
                    class="text-[12.5px] text-brand-600 transition hover:text-brand-700"
                    @click="form.clear_password = false"
                  >
                    撤销「清空密码」
                  </button>
                </div>

                <p v-if="form.clear_password" class="mt-1.5 flex items-center gap-1.5 text-[12.5px] text-amber-600">
                  <AppIcon name="alert" :size="14" />
                  保存后将清空已存密码；若同时保留了用户名，服务端会拒绝保存。
                </p>
                <p v-else class="mt-1.5 text-[12px] text-ink-400">
                  请填写服务商生成的<strong class="font-medium text-ink-500">授权码</strong>（非登录密码）；
                  首尾空白会被自动忽略。密码以密文存储，接口永不回显明文。
                </p>
              </template>
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">隐式 TLS</label>
            <div class="max-w-[520px]">
              <AppSwitch v-model="form.tls" label="启用（465 端口通常需要）" />
              <p class="mt-1.5 text-[12px] text-ink-400">
                关闭时按 STARTTLS 升级，适用于 587 等端口。
              </p>
            </div>
          </div>
        </div>
      </section>

      <!-- 操作区 -->
      <div class="card flex flex-wrap items-center justify-between gap-3 p-5">
        <div class="text-[12.5px] text-ink-400">
          <p v-if="dirty" class="flex items-center gap-1.5 font-medium text-amber-600">
            <AppIcon name="alert" :size="14" />
            有未保存的修改
          </p>
          <p v-else>配置修改后立即生效，无需重启服务。</p>
          <p class="mt-1">
            测试邮件使用<strong class="font-medium text-ink-600">已保存</strong>的配置发送，不是表单里未保存的改动。
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <AppButton variant="secondary" @click="openTest">
            <AppIcon name="bolt" :size="16" />
            发送测试邮件
          </AppButton>
          <AppButton :loading="saving" :disabled="!dirty" @click="save">保存配置</AppButton>
        </div>
      </div>
    </template>

    <!-- 测试邮件 -->
    <AppModal v-model="testOpen" title="发送测试邮件" size="sm">
      <AppInput
        v-model="testTo"
        label="收件邮箱"
        icon="mail"
        type="email"
        placeholder="you@example.com"
        hint="会使用当前已保存的配置真实投递一封测试邮件。"
      />
      <p
        v-if="testError"
        class="mt-3 flex items-start gap-1.5 rounded-lg bg-red-50 px-3 py-2.5 text-[12.5px] leading-relaxed text-red-600"
      >
        <AppIcon name="alert" :size="14" class="mt-0.5 shrink-0" />
        <!-- whitespace-pre-line：后端文案含 1) 2) 3) 排查步骤与服务器原文，必须按行渲染 -->
        <span class="whitespace-pre-line">{{ testError }}</span>
      </p>
      <template #footer>
        <AppButton variant="secondary" @click="testOpen = false">取消</AppButton>
        <AppButton :loading="testing" @click="submitTest">立即发送</AppButton>
      </template>
    </AppModal>

    <!-- 回落到配置文件 -->
    <AppModal v-model="revertOpen" title="回落到配置文件" size="sm">
      <p class="text-[14px] leading-relaxed text-ink-600">
        将删除数据库中的 <code class="rounded bg-ink-100 px-1.5 py-0.5 font-mono text-[12.5px]">mail.smtp</code>
        配置，改用 <code class="rounded bg-ink-100 px-1.5 py-0.5 font-mono text-[12.5px]">configs/config.yaml</code>
        里的邮件配置，并立即热生效。已保存的 SMTP 密码会一并删除。
      </p>
      <template #footer>
        <AppButton variant="secondary" @click="revertOpen = false">取消</AppButton>
        <AppButton variant="danger" :loading="reverting" @click="submitRevert">确认回落</AppButton>
      </template>
    </AppModal>
  </div>
</template>
