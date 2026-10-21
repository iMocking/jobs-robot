<script setup>
import { computed, reactive, ref, watch } from 'vue'

import { adminApi, authApi } from '@/api'
import AdminTable from '@/components/AdminTable.vue'
import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppCheckbox from '@/components/AppCheckbox.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppModal from '@/components/AppModal.vue'
import AppPagination from '@/components/AppPagination.vue'
import AppSelect from '@/components/AppSelect.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { useSessionStore } from '@/stores/session'
import { useToastStore } from '@/stores/toast'
import {
  ROLE_LABEL,
  ROLE_TONE,
  USER_STATUS,
  formatDate,
  formatNumber,
  relativeTime,
  roleLabel,
  toneFor,
  userInitial,
  userStatus,
} from '@/utils/format'

/**
 * 用户管理 = 设计稿第 7 屏。
 *
 * 四个行内操作（编辑 / 禁用 / 重置密码 / 删除）沿用设计稿的图标按钮形态，
 * 但每个都会先经过确认或表单弹窗 —— 这些操作都会立刻让目标用户的令牌失效，
 * 一次误点就是一次线上事故。
 */
const PAGE_SIZE = 10

const toast = useToastStore()
const session = useSessionStore()

const filters = reactive({
  keyword: '',
  status: '',
  role: '',
  page: 1,
})

const columns = [
  { key: 'user', label: '用户' },
  { key: 'role', label: '角色', class: 'w-[120px]' },
  { key: 'status', label: '状态', class: 'w-[120px]' },
  { key: 'registered', label: '注册时间', class: 'w-[140px]' },
  { key: 'login', label: '最近登录', class: 'w-[130px]' },
  { key: 'actions', label: '操作', class: 'w-[190px]' },
]

const { data, loading, error, ready, run } = useAsync(() =>
  adminApi.users.list({
    keyword: filters.keyword || undefined,
    status: filters.status || undefined,
    role: filters.role || undefined,
    page: filters.page,
    page_size: PAGE_SIZE,
  }),
)

useAdminRefresh(run)

// 关键词防抖：每敲一个字就发一次请求，在 1,024 个用户上会明显拖慢输入
let debounce = null
watch(
  () => filters.keyword,
  () => {
    clearTimeout(debounce)
    debounce = setTimeout(() => {
      filters.page = 1
      run()
    }, 300)
  },
)

watch([() => filters.status, () => filters.role], () => {
  filters.page = 1
  run()
})

function changePage(next) {
  filters.page = next
  run()
}

function resetFilters() {
  filters.keyword = ''
  filters.status = ''
  filters.role = ''
  filters.page = 1
  run()
}

const hasFilter = computed(() => Boolean(filters.keyword || filters.status || filters.role))

/**
 * 状态筛选项直接由 USER_STATUS 生成：
 * 后端有 pending（待验证）与 locked（多次登录失败被锁定）两态，
 * 只列 active/disabled 会让这两类用户「存在但搜不到」。
 */
const statusOptions = Object.entries(USER_STATUS).map(([value, meta]) => ({
  value,
  label: meta.label,
}))

/* —— 行内操作 —— */

const busyId = ref(null)

/** 目标状态：只有 active 是「已启用」，其余（待验证 / 已禁用 / 已锁定）都点向 active */
function nextStatus(user) {
  return user.status === 'active' ? 'disabled' : 'active'
}

function statusActionLabel(user) {
  if (user.status === 'active') return '禁用账号'
  if (user.status === 'locked') return '解锁并启用'
  return '启用账号'
}

async function toggleStatus(user) {
  busyId.value = user.id
  try {
    const next = nextStatus(user)
    await adminApi.users.setStatus(user.id, next)
    toast.success(next === 'active' ? `已启用 ${user.email}` : `已禁用 ${user.email}`)
    run()
  } catch (err) {
    // 后端会拒绝「修改自己的账号状态」并说明原因，原样透出比通用文案有用
    toast.fromError(err)
  } finally {
    busyId.value = null
  }
}

/* —— 身份变更（管理员本人邮箱验证码二次确认 + 双向邮件通知） —— */

/** 可授予的真人身份：不含 system（系统机器身份不可变更） */
const roleOptions = [
  { value: 'admin', label: ROLE_LABEL.admin },
  { value: 'reviewer', label: ROLE_LABEL.reviewer },
  { value: 'user', label: ROLE_LABEL.user },
]

const roleTarget = ref(null)
const roleForm = reactive({ roles: [], code: '' })
const changingRoles = ref(false)
const sendingCode = ref(false)
const codeSeconds = ref(0)
let codeCountdown = null

function openRoles(user) {
  roleTarget.value = user
  roleForm.roles = [...user.roles]
  roleForm.code = ''
  codeSeconds.value = 0
  clearInterval(codeCountdown)
}

function toggleRole(value) {
  const idx = roleForm.roles.indexOf(value)
  if (idx >= 0) roleForm.roles.splice(idx, 1)
  else roleForm.roles.push(value)
}

async function sendRoleCode() {
  const email = session.user?.email
  if (!email) {
    toast.error('无法获取当前登录邮箱，请刷新后重试')
    return
  }
  sendingCode.value = true
  try {
    await authApi.sendCode({ email, purpose: 'role_change' })
    codeSeconds.value = 60
    clearInterval(codeCountdown)
    codeCountdown = setInterval(() => {
      codeSeconds.value -= 1
      if (codeSeconds.value <= 0) clearInterval(codeCountdown)
    }, 1000)
    toast.success(`验证码已发送至 ${email}`)
  } catch (err) {
    toast.fromError(err, '发送验证码失败')
  } finally {
    sendingCode.value = false
  }
}

async function submitRoles() {
  if (roleForm.roles.length === 0) {
    toast.error('请至少选择一个身份')
    return
  }
  if (!roleForm.code.trim()) {
    toast.error('请输入邮箱验证码')
    return
  }
  changingRoles.value = true
  try {
    await adminApi.users.setRoles(roleTarget.value.id, roleForm.roles, roleForm.code.trim())
    const labels = roleForm.roles.map((r) => ROLE_LABEL[r] ?? r).join('、')
    toast.success(`已将 ${roleTarget.value.email} 的身份变更为「${labels}」，已邮件通知双方`)
    roleTarget.value = null
    clearInterval(codeCountdown)
    run()
  } catch (err) {
    toast.fromError(err, '身份变更失败')
  } finally {
    changingRoles.value = false
  }
}

const resetTarget = ref(null)
const resetForm = reactive({ password: '' })
const resetting = ref(false)

function openReset(user) {
  resetTarget.value = user
  // 给一个符合强度要求的默认值，管理员直接确认即可，也可以改成自己指定的
  resetForm.password = `Jobhub${Math.floor(1000 + Math.random() * 9000)}`
}

async function submitReset() {
  if (resetForm.password.length < 8) {
    toast.error('新密码至少 8 位')
    return
  }
  resetting.value = true
  try {
    const result = await adminApi.users.resetPassword(resetTarget.value.id, resetForm.password)
    toast.success(result?.message ?? '密码已重置')
    resetTarget.value = null
    run()
  } catch (err) {
    toast.fromError(err, '重置密码失败')
  } finally {
    resetting.value = false
  }
}

const deleteTarget = ref(null)
const deleting = ref(false)

async function submitDelete() {
  deleting.value = true
  try {
    await adminApi.users.remove(deleteTarget.value.id)
    toast.success('用户已删除')
    deleteTarget.value = null
    run()
  } catch (err) {
    // 不能删除自己 / 不能移除最后一名管理员，后端会返回可读原因
    toast.fromError(err, '删除用户失败')
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div>
    <!-- 筛选 -->
    <div class="card mb-2.5 flex flex-wrap items-center gap-3 p-4">
      <div class="min-w-[240px] flex-1">
        <AppInput v-model="filters.keyword" icon="search" placeholder="搜索邮箱 / 昵称" />
      </div>

      <div class="w-[150px]">
        <AppSelect
          v-model="filters.status"
          compact
          placeholder="全部状态"
          :options="statusOptions"
        />
      </div>

      <div class="w-[150px]">
        <AppSelect
          v-model="filters.role"
          compact
          placeholder="全部角色"
          :options="[
            { value: 'admin', label: '管理员' },
            { value: 'reviewer', label: '审核员' },
            { value: 'user', label: '用户' },
            { value: 'system', label: '系统' },
          ]"
        />
      </div>

      <AppButton v-if="hasFilter" variant="ghost" size="sm" @click="resetFilters">重置</AppButton>
    </div>

    <div class="mb-3 flex items-center justify-between">
      <h2 class="text-[16px] font-semibold text-ink-900">用户列表</h2>
      <AppBadge v-if="data" tone="brand">
        共 {{ formatNumber(data.total) }} 位用户
      </AppBadge>
    </div>

    <AdminTable
      :columns="columns"
      :empty="!loading && !data?.items?.length"
      :empty-text="hasFilter ? '没有匹配的用户' : '暂无用户'"
    >
      <template v-if="loading && !ready">
        <tr v-for="index in 5" :key="index">
          <td :colspan="columns.length" class="td-cell">
            <div class="h-10 animate-pulse rounded-lg bg-ink-100" />
          </td>
        </tr>
      </template>

      <tr v-else-if="error">
        <td :colspan="columns.length">
          <AppEmpty
            icon="alert"
            tone="danger"
            title="用户列表加载失败"
            :description="error.message"
          >
            <AppButton @click="run()">重新加载</AppButton>
          </AppEmpty>
        </td>
      </tr>

      <tr v-for="user in data?.items ?? []" :key="user.id" class="transition hover:bg-ink-50/60">
        <td class="td-cell">
          <div class="flex items-center gap-3">
            <AppAvatar
              :name="user.nickname || user.email"
              :initial="userInitial(user)"
              :tone="toneFor(user.email ?? user.id)"
              size="sm"
            />
            <div class="min-w-0">
              <p class="truncate text-[13.5px] font-medium text-ink-900">
                {{ user.email || '系统身份（无邮箱）' }}
              </p>
              <p class="mt-0.5 flex items-center gap-1.5 truncate text-[12.5px] text-ink-400">
                <span class="truncate">{{ user.nickname }}</span>
                <!-- 系统机器身份：可见但不可操作（后端也会拒绝任何写操作） -->
                <AppBadge v-if="user.kind === 'system'" tone="teal" size="sm">系统</AppBadge>
              </p>
            </div>
          </div>
        </td>

        <td class="td-cell">
          <div class="flex flex-wrap gap-1.5">
            <AppBadge
              v-for="role in user.roles"
              :key="role"
              :tone="ROLE_TONE[role] ?? 'ink'"
              size="sm"
            >
              {{ roleLabel(role) }}
            </AppBadge>
          </div>
        </td>

        <td class="td-cell">
          <AppBadge :tone="userStatus(user.status).tone" size="sm" dot>
            {{ userStatus(user.status).label }}
          </AppBadge>
        </td>

        <td class="td-cell tabular text-[13px] text-ink-500">
          {{ formatDate(user.created_at) }}
        </td>

        <td class="td-cell text-[13px] text-ink-500">
          {{ relativeTime(user.last_login_at) }}
        </td>

        <td class="td-cell">
          <div class="flex items-center gap-1.5">
            <button
              v-if="user.kind !== 'system'"
              type="button"
              class="flex size-8 items-center justify-center rounded-lg border border-ink-200 text-ink-500 transition hover:border-brand-300 hover:text-brand-600"
              title="变更身份"
              @click="openRoles(user)"
            >
              <AppIcon name="shield-check" :size="15" />
            </button>

            <RouterLink
              :to="{ name: 'user', params: { id: user.id } }"
              class="flex size-8 items-center justify-center rounded-lg border border-ink-200 text-ink-500 transition hover:border-brand-300 hover:text-brand-600"
              title="编辑用户"
            >
              <AppIcon name="pencil" :size="15" />
            </RouterLink>

            <button
              v-if="user.kind !== 'system'"
              type="button"
              class="flex size-8 items-center justify-center rounded-lg border border-ink-200 text-ink-500 transition hover:border-brand-300 hover:text-brand-600 disabled:opacity-50"
              :disabled="busyId === user.id"
              :title="statusActionLabel(user)"
              @click="toggleStatus(user)"
            >
              <AppIcon :name="user.status === 'active' ? 'lock' : 'lock-open'" :size="15" />
            </button>

            <button
              v-if="user.kind !== 'system'"
              type="button"
              class="flex size-8 items-center justify-center rounded-lg border border-ink-200 text-ink-500 transition hover:border-brand-300 hover:text-brand-600"
              title="重置密码"
              @click="openReset(user)"
            >
              <AppIcon name="key" :size="15" />
            </button>

            <button
              v-if="user.kind !== 'system'"
              type="button"
              class="flex size-8 items-center justify-center rounded-lg border border-ink-200 text-ink-500 transition hover:border-red-300 hover:text-red-600"
              title="删除用户"
              @click="deleteTarget = user"
            >
              <AppIcon name="trash" :size="15" />
            </button>

            <span v-if="user.kind === 'system'" class="text-[11.5px] text-ink-400">不可修改</span>
          </div>
        </td>
      </tr>

      <template #footer>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <p class="text-[12.5px] text-ink-400">
            注册、角色变更、禁用与重置密码都会写入审计日志。
          </p>
          <AppPagination
            :page="data?.page ?? 1"
            :pages="data?.pages ?? 1"
            :total="data?.total ?? 0"
            :page-size="PAGE_SIZE"
            @update:page="changePage"
          />
        </div>
      </template>
    </AdminTable>

    <!-- 变更身份（邮箱验证码二次确认） -->
    <AppModal
      :model-value="Boolean(roleTarget)"
      title="变更身份"
      :description="roleTarget ? `目标账号：${roleTarget.nickname}（${roleTarget.email}）` : ''"
      size="sm"
      @update:model-value="roleTarget = null"
    >
      <p class="mb-2 text-[12.5px] font-medium text-ink-500">新身份（可多选）</p>
      <div class="flex flex-col gap-2">
        <label
          v-for="opt in roleOptions"
          :key="opt.value"
          class="flex cursor-pointer items-center gap-2.5 rounded-lg border border-ink-200 px-3 py-2.5 transition hover:border-brand-300"
          :class="roleForm.roles.includes(opt.value) ? 'border-brand-300 bg-brand-50/50' : ''"
        >
          <AppCheckbox
            :model-value="roleForm.roles.includes(opt.value)"
            @update:model-value="toggleRole(opt.value)"
          />
          <span class="text-[13.5px] text-ink-900">{{ opt.label }}</span>
        </label>
      </div>

      <div class="mt-4 border-t border-ink-100 pt-4">
        <div class="mb-1.5 flex items-center justify-between">
          <p class="text-[12.5px] font-medium text-ink-500">邮箱验证</p>
          <button
            type="button"
            class="text-[12.5px] font-medium text-brand-600 transition hover:text-brand-700 disabled:opacity-50"
            :disabled="sendingCode || codeSeconds > 0"
            @click="sendRoleCode"
          >
            {{ codeSeconds > 0 ? `${codeSeconds}s 后重发` : '发送验证码' }}
          </button>
        </div>
        <p class="mb-2.5 text-[12px] leading-relaxed text-ink-400">
          验证码将发送至你的管理员邮箱
          <span class="font-medium text-ink-600">{{ session.user?.email }}</span
          >，用于确认本次身份变更由你本人操作。
        </p>
        <AppInput v-model="roleForm.code" label="验证码" icon="lock" placeholder="6 位数字验证码" />
      </div>

      <template #footer>
        <AppButton variant="secondary" @click="roleTarget = null">取消</AppButton>
        <AppButton :loading="changingRoles" @click="submitRoles">确认变更</AppButton>
      </template>
    </AppModal>

    <!-- 重置密码 -->
    <AppModal
      :model-value="Boolean(resetTarget)"
      title="重置密码"
      :description="resetTarget ? `将为 ${resetTarget.email} 设置新密码` : ''"
      size="sm"
      @update:model-value="resetTarget = null"
    >
      <AppInput v-model="resetForm.password" label="新密码" icon="key" placeholder="至少 8 位" />
      <p class="mt-3 flex items-start gap-1.5 text-[12.5px] leading-relaxed text-ink-400">
        <AppIcon name="info" :size="14" class="mt-0.5" />
        重置后该用户的所有会话立即失效，需要用新密码重新登录。
      </p>

      <template #footer>
        <AppButton variant="secondary" @click="resetTarget = null">取消</AppButton>
        <AppButton :loading="resetting" @click="submitReset">确认重置</AppButton>
      </template>
    </AppModal>

    <!-- 删除确认 -->
    <AppModal
      :model-value="Boolean(deleteTarget)"
      title="删除用户"
      size="sm"
      @update:model-value="deleteTarget = null"
    >
      <p class="text-[14px] leading-relaxed text-ink-600">
        确定要删除
        <span class="font-medium text-ink-900">{{ deleteTarget?.email }}</span>
        吗？该操作不可恢复，且不能删除自己的账号。
      </p>

      <template #footer>
        <AppButton variant="secondary" @click="deleteTarget = null">取消</AppButton>
        <AppButton variant="danger" :loading="deleting" @click="submitDelete">确认删除</AppButton>
      </template>
    </AppModal>
  </div>
</template>
