<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { adminApi } from '@/api'
import AdminTable from '@/components/AdminTable.vue'
import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppCheckbox from '@/components/AppCheckbox.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppModal from '@/components/AppModal.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { useSessionStore } from '@/stores/session'
import { useToastStore } from '@/stores/toast'
import {
  ROLE_TONE,
  auditDetailText,
  formatDate,
  formatDateTime,
  relativeTime,
  roleLabel,
  toneFor,
  userInitial,
  userStatus,
} from '@/utils/format'

/**
 * 用户详情 = 账号的「权限与人机凭证」控制台。
 *
 * 三条安全边界在这里落地，而不是丢给后端报错：
 *   1. 不能禁用自己的账号 —— 否则会把自己锁在门外（后端 10001 也会拦）
 *   2. 不能移除最后一名管理员 —— 系统会失去唯一的管理入口
 *   3. 角色变更 / 重置密码都会让该用户令牌立即失效，必须提前说清楚，
 *      否则运营会以为是「改完没生效」
 *
 * 会话列表在后台只读：下线单个设备属于用户自己的自助操作，放在「我的」里，
 * 后台能做的只有「重置密码」这种一刀切动作。
 */
const route = useRoute()
const router = useRouter()
const session = useSessionStore()
const toast = useToastStore()

const userId = computed(() => route.params.id)

const { data, loading, error, ready, run } = useAsync(() => adminApi.users.detail(userId.value), {
  immediate: false,
})

// 角色说明来自后端 RoleView（字段是 name / display_name），
// 避免前端再维护一份「管理员/审核员/用户」的第二映射。
const { data: roleData, ready: rolesReady, run: runRoles } = useAsync(() => adminApi.roles(), {
  immediate: false,
})

/**
 * 该用户最近的审计记录。
 * UserView 里没有 sessions / recent_actions 这类数组字段，但审计接口支持 actor_id 精确过滤，
 * 因此「最近操作」直接查真实审计流水，而不是渲染一个不存在的 user.recent_actions。
 */
const { data: actionPage, run: runActions } = useAsync(
  () => adminApi.auditLogs({ actor_id: userId.value, page_size: 10 }),
  { immediate: false },
)

const user = computed(() => data.value ?? null)
const roleList = computed(() => roleData.value?.items ?? [])
const actions = computed(() => actionPage.value?.items ?? [])

/**
 * 系统机器身份（爬虫 / 邮件机器人）：
 * 可见、可审计，但任何后台写操作都必须隐藏入口（后端也会直接拒绝）。
 */
const isSystemIdentity = computed(() => user.value?.kind === 'system')

/** 展示顺序固定，勾选行的顺序才不会随后端返回顺序抖动 */
const ROLE_ORDER = ['admin', 'reviewer', 'user', 'system']

const roleRows = computed(() =>
  ROLE_ORDER.map((key) => {
    const row = roleList.value.find((item) => item.name === key)
    if (row) return row
    // roles 表还没播种时（全新库尚未执行 seed），系统身份详情页仍要能显示 system 这一行，
    // 否则看起来像「没有任何角色」。普通用户不补这一行，避免把系统角色当成可授予项。
    if (key === 'system' && isSystemIdentity.value) {
      return {
        name: 'system',
        display_name: roleLabel('system'),
        description: '系统内置机器身份（爬虫 / 邮件机器人）：不继承任何角色、不能登录、不能访问后台接口',
      }
    }
    return null
  }).filter(Boolean),
)

function load() {
  run()
  runRoles()
  runActions()
}

useAdminRefresh(load)

// 从列表点进另一个用户时组件会被复用，必须跟着 id 重新取数
watch(userId, () => {
  run()
  runActions()
})

/* -------------------------------------------------------------------------- */
/* 角色                                                                        */
/* -------------------------------------------------------------------------- */

const selectedRoles = ref([])
const savingRoles = ref(false)

watch(
  data,
  (value) => {
    selectedRoles.value = [...(value?.roles ?? [])]
  },
  { immediate: false },
)

/**
 * 角色中文名优先取后端 RoleView.display_name（运营改过展示名时以它为准），
 * 退回到 format.js 里的固定映射 —— UserView 只有 roles 字符串数组，
 * 没有与之平行的中文标签数组。
 */
function displayRole(key) {
  const fromRole = roleList.value.find((item) => item.name === key)?.display_name
  return fromRole || roleLabel(key)
}

const rolesDirty = computed(() => {
  const current = [...(user.value?.roles ?? [])].sort().join(',')
  return current !== [...selectedRoles.value].sort().join(',')
})

function toggleRole(key, checked) {
  selectedRoles.value = checked
    ? [...new Set([...selectedRoles.value, key])]
    : selectedRoles.value.filter((item) => item !== key)
}

async function saveRoles() {
  if (!selectedRoles.value.length) {
    toast.error('至少保留一个角色，否则该账号登录后没有任何权限')
    return
  }
  savingRoles.value = true
  try {
    await adminApi.users.setRoles(userId.value, selectedRoles.value)
    toast.success('角色已更新，该用户的登录令牌已失效')
    await run()
  } catch (err) {
    toast.fromError(err, '保存角色失败')
  } finally {
    savingRoles.value = false
  }
}

/* -------------------------------------------------------------------------- */
/* 启用 / 禁用 / 重置密码 / 删除                                                */
/* -------------------------------------------------------------------------- */

/**
 * 当前登录用户来自会话（/me 返回的 UserView 带 id），不做 `?? 1` 那种猜测：
 * 猜错会让防呆失效，而后端无论如何都会拒绝「修改自己的账号状态」。
 * 只有确实拿到自身 id 时才置灰按钮，拿不到就把判断交给后端。
 */
const isSelf = computed(() => {
  const currentId = session.user?.id
  // ID 是 ULID 字符串：统一按字符串比较，避免 Number('01J...') → NaN 让防呆失效。
  return currentId != null && String(user.value?.id ?? '') === String(currentId)
})

const statusLoading = ref(false)

function statusActionLabel() {
  const status = user.value?.status
  if (status === 'active') return '禁用账号'
  if (status === 'locked') return '解锁并启用账号'
  return '启用账号'
}

async function toggleStatus() {
  const next = user.value?.status === 'active' ? 'disabled' : 'active'
  statusLoading.value = true
  try {
    await adminApi.users.setStatus(userId.value, next)
    toast.success(next === 'active' ? '账号已启用' : '账号已禁用，该用户将无法登录')
    await run()
  } catch (err) {
    toast.fromError(err, '更新账号状态失败')
  } finally {
    statusLoading.value = false
  }
}

/* —— 重置密码：新密码要能一眼看到并复制走，否则等于没重置 —— */
const resetOpen = ref(false)
const resetting = ref(false)
const resetForm = reactive({ password: '' })

function suggestPassword() {
  return `Jobhub${Math.floor(1000 + Math.random() * 9000)}`
}

function openReset() {
  resetForm.password = suggestPassword()
  resetOpen.value = true
}

async function copyPassword() {
  try {
    await navigator.clipboard?.writeText(resetForm.password)
    toast.success('新密码已复制')
  } catch {
    toast.info('浏览器未授权剪贴板，请手动选中复制')
  }
}

async function submitReset() {
  if (String(resetForm.password).length < 8) {
    toast.error('密码至少 8 位，建议包含字母与数字')
    return
  }
  resetting.value = true
  try {
    const result = await adminApi.users.resetPassword(userId.value, resetForm.password)
    toast.success(result?.message || '密码已重置，该用户所有会话已失效')
    resetOpen.value = false
    await run()
  } catch (err) {
    toast.fromError(err, '重置密码失败')
  } finally {
    resetting.value = false
  }
}

/* —— 删除：不可逆 —— */
const removeOpen = ref(false)
const removing = ref(false)

async function confirmRemove() {
  removing.value = true
  try {
    await adminApi.users.remove(userId.value)
    toast.success('用户已删除')
    removeOpen.value = false
    router.push({ name: 'users' })
  } catch (err) {
    toast.fromError(err, '删除用户失败')
  } finally {
    removing.value = false
  }
}

/* -------------------------------------------------------------------------- */
/* 表格列                                                                      */
/* -------------------------------------------------------------------------- */

/**
 * 后台没有「查看他人会话」的接口（/me/sessions 只能看自己），
 * 因此原来的「活跃会话」表改成了账号档案 + 真实审计流水。
 */
const actionColumns = [
  { key: 'created', label: '时间' },
  { key: 'action', label: '动作' },
  { key: 'resource', label: '资源' },
  { key: 'result', label: '结果' },
  { key: 'detail', label: '详情' },
]
</script>

<template>
  <div>
    <RouterLink
      :to="{ name: 'users' }"
      class="mb-4 inline-flex items-center gap-1.5 text-[13px] text-ink-500 transition hover:text-brand-600"
    >
      <AppIcon name="arrow-left" :size="15" />
      返回用户管理
    </RouterLink>

    <!-- 首次加载骨架 -->
    <div v-if="loading && !ready" class="space-y-2.5">
      <div class="card h-[180px] animate-pulse" />
      <div class="card h-[260px] animate-pulse" />
    </div>

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="用户信息加载失败"
      :description="error.message"
    >
      <div class="flex gap-3">
        <AppButton variant="secondary" :to="{ name: 'users' }">返回列表</AppButton>
        <AppButton @click="load()">重新加载</AppButton>
      </div>
    </AppEmpty>

    <template v-else-if="user">
      <!-- 头部：身份 + 高风险操作 -->
      <section class="card p-6">
        <div class="flex flex-wrap items-start justify-between gap-5">
          <div class="flex min-w-0 items-start gap-4">
            <AppAvatar
              :name="user.nickname || user.email"
              :initial="userInitial(user)"
              :tone="toneFor(user.email ?? user.id)"
              size="xl"
            />

            <div class="min-w-0">
              <h1 class="text-[22px] font-semibold text-ink-900">{{ user.nickname || '未命名用户' }}</h1>
              <p class="mt-1 flex items-center gap-1.5 text-[13px] text-ink-500">
                <AppIcon name="mail" :size="14" />
                {{ user.email || '系统身份（无邮箱）' }}
              </p>

              <div class="mt-3 flex flex-wrap items-center gap-2">
                <!-- 系统机器身份标记：与角色徽标并列，一眼区分真人/机器 -->
                <AppBadge v-if="isSystemIdentity" tone="teal">系统身份</AppBadge>
                <AppBadge
                  v-for="role in user.roles ?? []"
                  :key="role"
                  :tone="ROLE_TONE[role] ?? 'ink'"
                >
                  {{ displayRole(role) }}
                </AppBadge>
                <AppBadge :tone="userStatus(user.status).tone" dot>
                  {{ userStatus(user.status).label }}
                </AppBadge>
              </div>

              <dl class="mt-4 flex flex-wrap gap-x-8 gap-y-2 text-[12.5px]">
                <div class="flex items-center gap-1.5">
                  <dt class="text-ink-500">注册时间</dt>
                  <dd class="tabular text-ink-800">{{ formatDate(user.created_at) }}</dd>
                </div>
                <div class="flex items-center gap-1.5">
                  <dt class="text-ink-500">最近登录</dt>
                  <dd class="text-ink-800">{{ relativeTime(user.last_login_at) }}</dd>
                </div>
              </dl>
            </div>
          </div>

          <div class="flex flex-col items-start gap-2.5">
            <AppButton
              v-if="!isSystemIdentity"
              variant="secondary"
              :loading="statusLoading"
              :disabled="isSelf"
              @click="toggleStatus"
            >
              {{ statusActionLabel() }}
            </AppButton>
            <AppButton v-if="!isSystemIdentity" variant="secondary" @click="openReset">
              <AppIcon name="key" :size="16" />
              重置密码
            </AppButton>
            <AppButton
              v-if="!isSystemIdentity"
              variant="danger"
              :disabled="isSelf"
              @click="removeOpen = true"
            >
              <AppIcon name="trash" :size="16" />
              删除用户
            </AppButton>

            <p v-if="isSystemIdentity" class="max-w-[240px] text-[12px] leading-relaxed text-ink-400">
              系统机器身份由系统自动维护：不能禁用、删除、改角色或重置密码（它本来也没有密码与邮箱）。
            </p>
            <p v-else-if="isSelf" class="max-w-[220px] text-[12px] leading-relaxed text-ink-400">
              这是当前登录账号，不能禁用或删除自己（否则会立刻失去后台入口）；系统也会拒绝移除最后一名管理员。
            </p>
          </div>
        </div>
      </section>

      <!-- 角色设置 -->
      <section class="card mt-2.5 p-6">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h2 class="text-[16px] font-semibold text-ink-900">角色设置</h2>
            <p class="mt-1 text-[12.5px] text-ink-500">
              角色变更会立即让该用户的令牌失效，避免权限已被收回却仍能操作。
            </p>
          </div>
          <AppButton
            :loading="savingRoles"
            :disabled="!rolesDirty || isSystemIdentity"
            @click="saveRoles"
          >
            保存角色
          </AppButton>
        </div>

        <p v-if="isSystemIdentity" class="mt-3 rounded-xl bg-page px-3.5 py-2.5 text-[12.5px] leading-relaxed text-ink-500">
          系统身份的「系统」角色是固定的：它不继承任何角色、没有后台路径权限，仅用于审计归属，因此不可修改。
        </p>

        <div v-if="roleRows.length" class="mt-5 divide-y divide-ink-100">
          <div v-for="row in roleRows" :key="row.name" class="flex items-start gap-3 py-3.5">
            <AppCheckbox
              :model-value="selectedRoles.includes(row.name)"
              :disabled="isSystemIdentity"
              @update:model-value="toggleRole(row.name, $event)"
            />
            <div class="min-w-0">
              <p class="text-[13.5px] font-medium text-ink-800">
                {{ row.display_name || displayRole(row.name) }}
              </p>
              <p class="mt-0.5 text-[12.5px] text-ink-500">{{ row.description || '暂无说明' }}</p>
            </div>
          </div>
        </div>
        <div v-else-if="!rolesReady" class="mt-5 space-y-2">
          <div v-for="n in 3" :key="n" class="h-10 animate-pulse rounded-lg bg-ink-100" />
        </div>
        <!-- 角色字典取不到时不能假装在加载：否则这一块会永远停在骨架屏上 -->
        <p v-else class="mt-5 text-[13px] text-ink-500">
          角色说明加载失败，可点击顶栏「刷新」重试。
        </p>
      </section>

      <!-- 账号档案：UserView 里真实存在、且原来没被展示的字段 -->
      <section class="card mt-6 p-6">
        <h2 class="text-[16px] font-semibold text-ink-900">账号档案</h2>
        <p class="mt-1 text-[12.5px] text-ink-500">
          后台没有「查看他人登录会话」的接口（/me/sessions 只能看自己），
          因此这里只展示账号自身的字段；要让某人的设备全部下线，请用「重置密码」。
        </p>
        <dl class="mt-4 grid gap-x-8 gap-y-4 text-[13px] sm:grid-cols-2 xl:grid-cols-4">
          <div>
            <dt class="text-ink-400">邮箱验证</dt>
            <dd class="mt-1">
              <AppBadge :tone="user.email_verified ? 'accent' : 'amber'" size="sm">
                {{ user.email_verified ? '已验证' : '未验证' }}
              </AppBadge>
            </dd>
          </div>
          <div>
            <dt class="text-ink-400">手机号</dt>
            <dd class="mt-1 text-ink-800">{{ user.phone || '未填写' }}</dd>
          </div>
          <div>
            <dt class="text-ink-400">最后登录</dt>
            <dd class="mt-1 text-ink-800">
              {{ user.last_login_at ? formatDateTime(user.last_login_at) : '从未登录' }}
            </dd>
          </div>
          <div>
            <dt class="text-ink-400">资料更新</dt>
            <dd class="tabular mt-1 text-ink-800">{{ formatDateTime(user.updated_at) }}</dd>
          </div>
        </dl>
        <p
          v-if="user.remark"
          class="mt-4 rounded-xl bg-page px-3.5 py-2.5 text-[12.5px] leading-relaxed text-ink-500"
        >
          备注：{{ user.remark }}
        </p>
      </section>

      <!-- 最近操作：查真实审计流水（按 actor_id 精确过滤） -->
      <section class="mt-3">
        <div class="mb-3 flex flex-wrap items-baseline justify-between gap-2">
          <h2 class="text-[16px] font-semibold text-ink-900">最近操作</h2>
          <span class="text-[12.5px] text-ink-400">
            取自审计日志，共 {{ actionPage?.total ?? actions.length }} 条，展示最近 10 条
          </span>
        </div>
        <AdminTable
          :columns="actionColumns"
          :empty="!actions.length"
          empty-text="该用户暂无操作记录"
          min-width="880px"
        >
          <tr v-for="row in actions" :key="row.id">
            <td class="td-cell tabular whitespace-nowrap text-[12.5px] text-ink-600">
              {{ formatDateTime(row.created_at) }}
            </td>
            <td class="td-cell whitespace-nowrap">
              <code class="rounded bg-ink-100 px-1.5 py-0.5 text-[12.5px] text-ink-700">
                {{ row.action }}
              </code>
            </td>
            <td class="td-cell whitespace-nowrap text-[12.5px] text-ink-600">
              {{ row.resource || '—' }}<span v-if="row.resource_id">#{{ row.resource_id }}</span>
            </td>
            <td class="td-cell">
              <AppBadge :tone="row.success ? 'accent' : 'danger'">
                {{ row.success ? '成功' : '失败' }} · {{ row.status_code }}
              </AppBadge>
            </td>
            <td class="td-cell">
              <!-- detail 是结构化对象（map[string]any），必须过 auditDetailText 才不会显示 [object Object] -->
              <span
                class="block max-w-[320px] truncate text-[12.5px] text-ink-500"
                :title="auditDetailText(row.detail)"
              >
                {{ auditDetailText(row.detail) }}
              </span>
            </td>
          </tr>
        </AdminTable>
      </section>
    </template>

    <!-- 重置密码确认 -->
    <AppModal
      v-model="resetOpen"
      title="重置密码"
      description="重置会让该用户所有会话立即失效，需要重新登录。"
    >
      <div class="space-y-4">
        <AppInput
          v-model="resetForm.password"
          label="新密码"
          hint="已生成一个建议密码，可以直接使用或改成自己的。"
        >
          <template #suffix>
            <AppButton variant="subtle" size="sm" @click="copyPassword">复制</AppButton>
          </template>
        </AppInput>

        <div class="rounded-xl bg-brand-50 px-4 py-3">
          <p class="text-[12.5px] leading-relaxed text-brand-700">
            建议密码：<code class="tabular font-medium">{{ resetForm.password }}</code>
            <br />
            请通过安全渠道发送给该用户，不要留在公开的聊天记录里。
          </p>
        </div>
      </div>

      <template #footer>
        <AppButton variant="secondary" @click="resetOpen = false">取消</AppButton>
        <AppButton :loading="resetting" @click="submitReset">确认重置</AppButton>
      </template>
    </AppModal>

    <!-- 删除确认 -->
    <AppModal v-model="removeOpen" title="删除用户" size="sm">
      <p class="text-[13.5px] leading-relaxed text-ink-600">
        确定要删除「<span class="font-medium text-ink-900">{{ user?.nickname || user?.email }}</span
        >」吗？删除后该账号无法登录，且无法恢复。
      </p>
      <p class="mt-3 rounded-lg bg-danger-50 px-3.5 py-2.5 text-[12.5px] leading-relaxed text-red-600">
        该操作不可撤销。若只是需要临时收回权限，请改用「禁用账号」。
      </p>

      <template #footer>
        <AppButton variant="secondary" @click="removeOpen = false">取消</AppButton>
        <AppButton variant="danger" :loading="removing" @click="confirmRemove">确认删除</AppButton>
      </template>
    </AppModal>
  </div>
</template>
