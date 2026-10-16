<script setup>
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'

import { adminApi } from '@/api'
import AdminTable from '@/components/AdminTable.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppPagination from '@/components/AppPagination.vue'
import AppSelect from '@/components/AppSelect.vue'
import PageHeader from '@/components/PageHeader.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { auditDetailText, formatDateTime } from '@/utils/format'

/**
 * 审计日志 = 「谁在什么时候动了什么」的唯一事实来源。
 *
 * 页面刻意做成只读：没有编辑、没有删除，因为这些记录一旦可改就失去证据价值。
 * 审计写入失败不会回滚业务操作（后端保证），所以这里看到的是「尽力而为」的轨迹，
 * 排查问题时应当把它当作线索而不是账本。
 *
 * 筛选条件严格按 handler.AdminListAuditLogs 支持的参数来：
 * actor_id / action / resource / resource_id / success。
 * 后端**没有**关键词搜索（不能按操作者邮箱或备注模糊匹配），
 * 所以原来的「搜索操作者邮箱或备注」输入框改成了「动作前缀」筛选（action 是模糊匹配）。
 */
const PAGE_SIZE = 10

const query = reactive({ action: '', resource: '', success: '' })
const page = ref(1)

const columns = [
  { key: 'created', label: '时间' },
  { key: 'actor', label: '操作者' },
  { key: 'action', label: '动作' },
  { key: 'resource', label: '资源' },
  { key: 'method', label: '方法' },
  { key: 'code', label: '状态码', class: 'w-[90px]' },
  { key: 'result', label: '结果' },
  { key: 'ip', label: 'IP' },
  { key: 'detail', label: '详情' },
  { key: 'actions', label: '操作', class: 'text-right' },
]

/**
 * 资源取值来自后端埋点（service/*.go 里的 Audit.Record 第二个资源参数），
 * 是精确等值匹配的枚举：job / company / user / session / setting / policy / verification_code。
 * 这里用字面量而非字典接口：字典是给前台筛选用的，后台审计的资源枚举随埋点演进，
 * 两者生命周期不同，混用会让人误以为改字典就能改审计分类。
 */
const RESOURCE_OPTIONS = [
  { value: 'job', label: '岗位' },
  { value: 'company', label: '公司' },
  { value: 'user', label: '用户' },
  { value: 'session', label: '会话' },
  { value: 'setting', label: '配置' },
  { value: 'policy', label: '授权策略' },
  { value: 'verification_code', label: '验证码' },
]

const successOptions = [
  { value: '', label: '全部' },
  { value: 'true', label: '成功' },
  { value: 'false', label: '失败' },
]

/** 请求方法用颜色区分「读 / 写 / 改 / 删」，扫一眼就能找到改状态的记录 */
const METHOD_TONE = {
  GET: 'ink',
  POST: 'brand',
  PATCH: 'amber',
  PUT: 'amber',
  DELETE: 'danger',
}

function methodTone(method) {
  return METHOD_TONE[String(method || '').toUpperCase()] ?? 'ink'
}

const {
  data,
  loading,
  error,
  ready,
  run: runList,
} = useAsync(
  () => adminApi.auditLogs({ ...query, page: page.value, page_size: PAGE_SIZE }),
  { immediate: false },
)

function loadList() {
  return runList()
}

useAdminRefresh(loadList)

/* —— 动作前缀防抖：审计日志表通常很大，逐字查询代价高 —— */
let keywordTimer = null
const actionInput = ref('')

watch(actionInput, (value) => {
  clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => {
    const next = value.trim()
    if (query.action === next) return
    query.action = next
    page.value = 1
    loadList()
  }, 300)
})

function applyActionFilter() {
  clearTimeout(keywordTimer)
  const next = actionInput.value.trim()
  if (query.action === next) return
  query.action = next
  page.value = 1
  loadList()
}

function resetFilters() {
  clearTimeout(keywordTimer)
  actionInput.value = ''
  query.action = ''
  query.resource = ''
  query.success = ''
  page.value = 1
  loadList()
}

const filtersActive = computed(() => Boolean(query.action || query.resource || query.success))

watch([() => query.resource, () => query.success], () => {
  page.value = 1
  loadList()
})

watch(page, () => loadList())

onBeforeUnmount(() => clearTimeout(keywordTimer))
</script>

<template>
  <div>
    <PageHeader
      title="审计日志"
      description="所有会改变系统状态的操作都会留痕；审计写入失败不会导致业务失败，因此它记录的是线索而非账本。"
    />

    <div class="card mb-5 p-5">
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <AppInput
          v-model="actionInput"
          icon="search"
          placeholder="按动作筛选，如 company."
          @enter="applyActionFilter"
        />
        <AppSelect v-model="query.resource" :options="RESOURCE_OPTIONS" placeholder="全部资源" />
        <AppSelect v-model="query.success" :options="successOptions" placeholder="全部结果" />
        <div class="flex items-center">
          <AppButton variant="secondary" :disabled="!filtersActive" @click="resetFilters">
            <AppIcon name="refresh" :size="16" />
            重置
          </AppButton>
        </div>
      </div>
      <p class="mt-3 flex items-start gap-1.5 text-[12px] leading-relaxed text-ink-400">
        <AppIcon name="info" :size="14" class="mt-px shrink-0" />
        动作是前缀模糊匹配（例如 company. 能筛出公司相关的全部操作）；
        接口不支持按操作者邮箱或备注全文检索，需要定位某个人时请到用户详情页看他的操作记录。
      </p>
    </div>

    <!-- 首次加载骨架 -->
    <div v-if="loading && !ready" class="card space-y-3 p-6">
      <div v-for="n in 8" :key="n" class="h-10 animate-pulse rounded-lg bg-ink-100" />
    </div>

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="审计日志加载失败"
      :description="error.message"
    >
      <AppButton @click="loadList()">重新加载</AppButton>
    </AppEmpty>

    <AdminTable
      v-else
      :columns="columns"
      :empty="!data?.items?.length"
      empty-text="没有符合条件的审计日志"
      min-width="1320px"
    >
      <tr v-for="row in data?.items ?? []" :key="row.id">
        <td class="td-cell tabular whitespace-nowrap text-[12.5px] text-ink-600">
          {{ formatDateTime(row.created_at) }}
        </td>

        <td class="td-cell whitespace-nowrap">
          <!-- 操作者只有 actor_email（可空）：系统任务没有 actor_id，邮箱也为空 -->
          <AppBadge v-if="!row.actor_email" tone="ink">系统 / 未知来源</AppBadge>
          <span v-else class="text-[12.5px] text-ink-700">{{ row.actor_email }}</span>
        </td>

        <td class="td-cell whitespace-nowrap">
          <code class="rounded bg-ink-100 px-1.5 py-0.5 text-[12.5px] text-brand-700">
            {{ row.action }}
          </code>
        </td>

        <td class="td-cell whitespace-nowrap text-[12.5px] text-ink-600">
          {{ row.resource || '—' }}<span v-if="row.resource_id">#{{ row.resource_id }}</span>
        </td>

        <td class="td-cell">
          <AppBadge :tone="methodTone(row.method)" size="sm">{{ row.method || '—' }}</AppBadge>
        </td>

        <td class="td-cell tabular text-[12.5px] text-ink-600">{{ row.status_code }}</td>

        <td class="td-cell">
          <AppBadge :tone="row.success ? 'accent' : 'danger'">
            {{ row.success ? '成功' : '失败' }}
          </AppBadge>
        </td>

        <td class="td-cell tabular whitespace-nowrap text-[12.5px] text-ink-600">
          {{ row.ip || '—' }}
        </td>

        <td class="td-cell">
          <!-- detail 是结构化对象，必须过 auditDetailText，否则渲染成 [object Object] -->
          <span
            class="block max-w-[280px] truncate text-[12.5px] text-ink-500"
            :title="auditDetailText(row.detail)"
          >
            {{ auditDetailText(row.detail) }}
          </span>
        </td>

        <td class="td-cell text-right">
          <AppButton variant="ghost" size="sm" :to="{ name: 'log', params: { id: row.id } }">
            详情
          </AppButton>
        </td>
      </tr>

      <template #footer>
        <div class="flex items-center justify-between gap-4">
          <AppPagination
            :page="data?.page ?? 1"
            :pages="data?.pages ?? 1"
            :total="data?.total ?? 0"
            :page-size="PAGE_SIZE"
            @update:page="page = $event"
          />
          <span v-if="(data?.pages ?? 1) <= 1" class="tabular text-[13px] text-ink-400">
            共 {{ data?.total ?? 0 }} 条
          </span>
        </div>
      </template>
    </AdminTable>
  </div>
</template>
