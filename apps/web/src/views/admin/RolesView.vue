<script setup>
import { computed, reactive, ref } from 'vue'

import { adminApi } from '@/api'
import AdminTable from '@/components/AdminTable.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppModal from '@/components/AppModal.vue'
import AppSelect from '@/components/AppSelect.vue'
import PageHeader from '@/components/PageHeader.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { useToastStore } from '@/stores/toast'
import { ROLE_TONE, roleLabel } from '@/utils/format'

/**
 * 角色权限 = 设计稿第 8 屏（角色卡片 + 授权策略表）。
 *
 * 两个接口、两份数据，刻意不合并：
 *   GET /admin/roles             → { items: RoleView[] }，角色元数据 + 该角色的策略条数
 *   GET /admin/authz/policies    → { policies: [[role, obj, act]], stats: {policies, grouping_rules} }
 * 早先这里把两者当成一个响应的 items/policies/meta，结果角色卡片和策略表都取不到数据。
 *
 * 版面把「角色是什么」与「角色能做什么」分成上下两块：
 * 角色只是展示元数据，真正的授权事实只存在于下面的策略表里（Casbin p 规则）。
 * 因此改角色描述不会改变任何权限 —— 这一点必须让运营一眼看出来。
 */
const toast = useToastStore()

const columns = [
  { key: 'role', label: '角色', class: 'w-[130px]' },
  { key: 'obj', label: '访问对象' },
  { key: 'act', label: '方法', class: 'w-[110px]' },
  { key: 'actions', label: '操作', class: 'w-[90px]' },
]

/** 角色：name / display_name / description / is_system / level / permissions / user_count */
const { data: roleData, loading, error, ready, run: runRoles } = useAsync(() => adminApi.roles())

/** 策略：policies 是 [role, obj, act] 三元组数组，stats 是策略规模 */
const {
  data: policyData,
  error: policyError,
  run: runPolicies,
} = useAsync(() => adminApi.policies.list(), {
  immediate: false,
})

function load() {
  runRoles()
  runPolicies()
}

useAdminRefresh(load)

const roles = computed(() => roleData.value?.items ?? [])
const stats = computed(() => policyData.value?.stats ?? {})

/** 把 [role, obj, act] 三元组摊成表格行，顺手过滤掉长度不足的脏数据 */
const policies = computed(() =>
  (policyData.value?.policies ?? [])
    .filter((rule) => Array.isArray(rule) && rule.length >= 3)
    .map((rule) => ({ role: rule[0], obj: rule[1], act: rule[2] })),
)

/** 角色卡片上的图标：Casbin 里只有 admin / reviewer / user 三个稳定标识 */
const ROLE_ICON = { admin: 'shield-check', reviewer: 'clipboard', user: 'user' }
const ROLE_ICON_CLASS = {
  admin: 'bg-brand-50 text-brand-600',
  reviewer: 'bg-violet-50 text-violet-600',
  user: 'bg-ink-100 text-ink-500',
}

const roleOptions = computed(() =>
  roles.value.map((role) => ({ value: role.name, label: role.display_name || roleLabel(role.name) })),
)

/* —— 新增策略 —— */

const creating = ref(false)
const saving = ref(false)
const draft = reactive({ role: 'reviewer', obj: '', act: 'GET' })

function openCreate() {
  draft.role = roles.value.some((role) => role.name === 'reviewer') ? 'reviewer' : roles.value[0]?.name ?? 'user'
  draft.obj = ''
  draft.act = 'GET'
  creating.value = true
}

async function submitCreate() {
  if (!draft.obj.trim()) {
    toast.error('请填写访问对象，例如 /api/v1/admin/jobs')
    return
  }
  saving.value = true
  try {
    // 接口只接受 role / obj / act 三个字段（policyRequest），没有「说明」这一列
    await adminApi.policies.add({ role: draft.role, obj: draft.obj.trim(), act: draft.act })
    toast.success('策略已新增，最迟 60 秒内在所有实例生效')
    creating.value = false
    runPolicies()
  } catch (err) {
    toast.fromError(err, '新增策略失败')
  } finally {
    saving.value = false
  }
}

/* —— 删除策略 —— */

const removeTarget = ref(null)
const removing = ref(false)

async function submitRemove() {
  removing.value = true
  try {
    await adminApi.policies.remove({
      role: removeTarget.value.role,
      obj: removeTarget.value.obj,
      act: removeTarget.value.act,
    })
    toast.success('策略已删除')
    removeTarget.value = null
    runPolicies()
  } catch (err) {
    // 删掉最后一条入口策略会把自己锁在门外，后端可能直接拒绝
    toast.fromError(err, '删除策略失败')
  } finally {
    removing.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="角色权限" description="角色只承载展示信息，接口级授权由下面的策略表决定">
      <template #actions>
        <AppButton @click="openCreate">
          <AppIcon name="plus" :size="16" />
          新增策略
        </AppButton>
      </template>
    </PageHeader>

    <div v-if="loading && !ready" class="grid gap-4 lg:grid-cols-3">
      <div v-for="index in 3" :key="index" class="card h-[172px] animate-pulse" />
    </div>

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="权限数据加载失败"
      :description="error.message"
    >
      <AppButton @click="load()">重新加载</AppButton>
    </AppEmpty>

    <template v-else>
      <!-- 角色卡片：展示后端返回的角色元数据 -->
      <div class="grid gap-4 lg:grid-cols-3">
        <article v-for="role in roles" :key="role.name" class="card p-6">
          <span
            class="flex size-11 items-center justify-center rounded-xl"
            :class="ROLE_ICON_CLASS[role.name] ?? ROLE_ICON_CLASS.user"
          >
            <AppIcon :name="ROLE_ICON[role.name] ?? 'user'" :size="21" />
          </span>

          <div class="mt-4 flex flex-wrap items-center gap-2">
            <h2 class="text-[18px] font-semibold text-ink-900">
              {{ role.display_name || roleLabel(role.name) }}
            </h2>
            <AppBadge size="sm" tone="ink">{{ role.name }}</AppBadge>
            <AppBadge v-if="role.is_system" tone="brand" size="sm">内置</AppBadge>
          </div>

          <p class="mt-1.5 min-h-[40px] text-[13px] leading-relaxed text-ink-500">
            {{ role.description || '暂无说明' }}
          </p>

          <div class="mt-4 flex items-center justify-between border-t border-ink-100 pt-4">
            <span class="tabular text-[13.5px] text-ink-600">{{ role.user_count ?? 0 }} 位用户</span>
            <span class="tabular text-[12.5px] text-ink-400">
              {{ role.permissions?.length ?? 0 }} 条接口策略 · 级别 {{ role.level }}
            </span>
          </div>
        </article>

        <AppEmpty
          v-if="!roles.length"
          class="lg:col-span-3"
          title="暂无角色"
          description="角色表为空时，权限判断会拒绝所有后台请求，请先检查数据库初始化。"
        />
      </div>

      <!-- 授权策略表 -->
      <section class="mt-6">
        <div class="mb-3 flex flex-wrap items-baseline justify-between gap-2">
          <h2 class="text-[16px] font-semibold text-ink-900">权限策略</h2>
          <span class="text-[12.5px] text-ink-400">
            命中任意一条即放行 · 策略 {{ stats.policies ?? policies.length }} 条 · 角色分组规则
            {{ stats.grouping_rules ?? 0 }} 条 · 变更最迟 60 秒内生效
          </span>
        </div>

        <AppEmpty
          v-if="policyError"
          icon="alert"
          tone="danger"
          title="策略列表加载失败"
          :description="policyError.message"
        >
          <AppButton @click="runPolicies()">重新加载</AppButton>
        </AppEmpty>

        <AdminTable v-else :columns="columns" :empty="!policies.length" empty-text="暂无授权策略">
          <tr
            v-for="policy in policies"
            :key="`${policy.role}-${policy.act}-${policy.obj}`"
            class="transition hover:bg-ink-50/60"
          >
            <td class="td-cell">
              <AppBadge :tone="ROLE_TONE[policy.role] ?? 'ink'">
                <AppIcon :name="policy.role === 'admin' ? 'shield-check' : 'user'" :size="13" />
                {{ roleLabel(policy.role) }}
              </AppBadge>
            </td>

            <td class="td-cell">
              <code class="rounded bg-ink-50 px-2 py-1 font-mono text-[12.5px] text-ink-700">
                {{ policy.obj }}
              </code>
            </td>

            <td class="td-cell">
              <span class="font-mono text-[12.5px] font-medium text-brand-700">{{ policy.act }}</span>
            </td>

            <td class="td-cell">
              <button
                type="button"
                class="flex size-8 items-center justify-center rounded-lg text-ink-400 transition hover:bg-red-50 hover:text-red-600"
                :title="`删除策略 ${policy.role} → ${policy.act} ${policy.obj}`"
                @click="removeTarget = policy"
              >
                <AppIcon name="trash" :size="15" />
              </button>
            </td>
          </tr>

          <template #footer>
            <p class="text-[12.5px] text-ink-400">
              共 {{ stats.policies ?? policies.length }} 条策略 · 分组规则（角色继承 g 规则）
              {{ stats.grouping_rules ?? 0 }} 条
            </p>
          </template>
        </AdminTable>
      </section>
    </template>

    <!-- 新增策略 -->
    <AppModal
      v-model="creating"
      title="新增策略"
      description="权限策略是授权的唯一事实来源，新增后最迟 60 秒内在所有实例生效"
    >
      <div class="space-y-4">
        <AppSelect v-model="draft.role" label="角色" :options="roleOptions" />
        <AppInput
          v-model="draft.obj"
          label="访问对象"
          placeholder="例如 /api/v1/admin/jobs/:id/review"
          hint="支持 keyMatch2 路径参数，* 表示该前缀下的全部路径"
        />
        <AppSelect
          v-model="draft.act"
          label="请求方法"
          :options="[
            { value: 'GET', label: 'GET' },
            { value: 'POST', label: 'POST' },
            { value: 'PATCH', label: 'PATCH' },
            { value: 'PUT', label: 'PUT' },
            { value: 'DELETE', label: 'DELETE' },
            { value: '*', label: '*（全部方法）' },
          ]"
        />
        <p class="flex items-start gap-1.5 rounded-xl bg-ink-50 px-3.5 py-2.5 text-[12px] leading-relaxed text-ink-500">
          <AppIcon name="info" :size="14" class="mt-px shrink-0" />
          策略只存「角色 / 路径 / 方法」三个字段（后端没有备注列），说明请写进变更工单。
        </p>
      </div>

      <template #footer>
        <AppButton variant="secondary" @click="creating = false">取消</AppButton>
        <AppButton :loading="saving" @click="submitCreate">保存策略</AppButton>
      </template>
    </AppModal>

    <!-- 删除确认 -->
    <AppModal
      :model-value="Boolean(removeTarget)"
      title="删除策略"
      size="sm"
      @update:model-value="removeTarget = null"
    >
      <p class="text-[14px] leading-relaxed text-ink-600">
        删除后
        <span class="font-medium text-ink-900">{{ roleLabel(removeTarget?.role) }}</span>
        将立即失去
        <code class="rounded bg-ink-100 px-1.5 py-0.5 font-mono text-[12.5px]">
          {{ removeTarget?.act }} {{ removeTarget?.obj }}
        </code>
        的权限。若这是该角色唯一的入口策略，请确认不会把自己锁在外面。
      </p>

      <template #footer>
        <AppButton variant="secondary" @click="removeTarget = null">取消</AppButton>
        <AppButton variant="danger" :loading="removing" @click="submitRemove">确认删除</AppButton>
      </template>
    </AppModal>
  </div>
</template>
