<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import { adminApi } from '@/api'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { auditDetailText, formatDateTime } from '@/utils/format'

/**
 * 日志详情。
 *
 * 后端没有 /admin/audit-logs/:id 这个端点 —— 审计表按保留策略轮转，
 * 单条记录不保证长期可寻址，所以只能在列表结果里按 id 命中。
 * 可用的过滤器只有 actor_id / action / resource / resource_id / success，
 * **没有按日志 id 过滤**的能力；因此这里取最近一页（page_size 取上限 100）后本地命中，
 * 命中不了时明确说明「可能已被轮转」，而不是谎称这条记录不存在。
 *
 * 页面没有任何编辑 / 删除入口：审计记录一旦可改就不再是证据。
 */
const route = useRoute()

// ID 现为 ULID 字符串，绝不能转成 Number：ULID 转数字会得到 NaN，
// 导致详情页永远匹配不到记录。
const logId = computed(() => String(route.params.id ?? ''))

const { data, loading, error, ready, run } = useAsync(
  // page_size 取上限 100：详情页只需要一次拉取，避免依赖分页拼接
  () => adminApi.auditLogs({ page_size: 100 }),
  { immediate: false },
)

const entry = computed(
  () => (data.value?.items ?? []).find((item) => String(item.id) === logId.value) ?? null,
)

function load() {
  return run()
}

useAdminRefresh(load)

// 从列表连点不同日志时组件会被复用，必须跟着 id 重新命中
watch(logId, () => load())

const copied = ref(false)

async function copyPath() {
  try {
    await navigator.clipboard?.writeText(entry.value?.path ?? '')
    copied.value = true
    setTimeout(() => (copied.value = false), 2000)
  } catch {
    copied.value = false
  }
}
</script>

<template>
  <div>
    <RouterLink
      :to="{ name: 'logs' }"
      class="mb-4 inline-flex items-center gap-1.5 text-[13px] text-ink-500 transition hover:text-brand-600"
    >
      <AppIcon name="arrow-left" :size="15" />
      返回审计日志
    </RouterLink>

    <!-- 首次加载骨架 -->
    <div v-if="loading && !ready" class="card h-[420px] max-w-[860px] animate-pulse" />

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="日志加载失败"
      :description="error.message"
    >
      <div class="flex gap-3">
        <AppButton variant="secondary" :to="{ name: 'logs' }">返回列表</AppButton>
        <AppButton @click="load()">重新加载</AppButton>
      </div>
    </AppEmpty>

    <!-- 命中不了大概率不是「这条不存在」，而是已经轮转出保留窗口 -->
    <AppEmpty
      v-else-if="!entry"
      icon="search"
      title="未找到该日志"
      description="日志编号在最近 100 条审计记录中不存在：该记录可能已被保留策略轮转清理。"
    >
      <AppButton variant="secondary" :to="{ name: 'logs' }">返回审计日志</AppButton>
    </AppEmpty>

    <section v-else class="card max-w-[860px] p-6">
      <header class="flex items-start justify-between gap-4 border-b border-ink-100 pb-4">
        <div>
          <h1 class="text-[20px] font-semibold text-ink-900">日志详情</h1>
          <p class="mt-1 text-[12.5px] text-ink-500">
            一次系统状态变更的完整留痕，包含操作者、来源与请求路径。
          </p>
        </div>
        <AppBadge :tone="entry.success ? 'accent' : 'danger'">
          {{ entry.success ? '成功' : '失败' }}
        </AppBadge>
      </header>

      <dl class="divide-y divide-ink-100">
        <div class="form-row">
          <dt class="text-[13px] text-ink-500">日志编号</dt>
          <dd class="tabular text-[13.5px] text-ink-900">#{{ entry.id }}</dd>
        </div>

        <div class="form-row">
          <dt class="text-[13px] text-ink-500">时间</dt>
          <dd class="tabular text-[13.5px] text-ink-900">{{ formatDateTime(entry.created_at) }}</dd>
        </div>

        <div class="form-row">
          <dt class="text-[13px] text-ink-500">操作者</dt>
          <dd class="text-[13.5px] text-ink-900">
            <!-- 审计只存 actor_email（可空）：系统任务的 actor_id 为 NULL、邮箱也为空 -->
            <template v-if="entry.actor_email">
              <span>{{ entry.actor_email }}</span>
              <span v-if="entry.actor_id" class="ml-1.5 text-[12px] text-ink-400">
                #{{ entry.actor_id }}
              </span>
            </template>
            <AppBadge v-else tone="ink">系统 / 未知来源</AppBadge>
          </dd>
        </div>

        <div class="form-row">
          <dt class="text-[13px] text-ink-500">结果</dt>
          <dd class="text-[13.5px] text-ink-900">
            <AppBadge :tone="entry.success ? 'accent' : 'danger'">
              {{ entry.success ? '成功' : '失败' }}
            </AppBadge>
            <span class="tabular ml-2 text-[12.5px] text-ink-500">
              HTTP {{ entry.status_code }}
            </span>
          </dd>
        </div>

        <div class="form-row">
          <dt class="text-[13px] text-ink-500">动作</dt>
          <dd class="text-[13.5px] text-ink-900">
            <code class="rounded bg-ink-100 px-1.5 py-0.5 text-[12.5px] text-brand-700">
              {{ entry.action }}
            </code>
          </dd>
        </div>

        <div class="form-row">
          <dt class="text-[13px] text-ink-500">资源</dt>
          <dd class="text-[13.5px] text-ink-900">{{ entry.resource || '—' }}</dd>
        </div>

        <div class="form-row">
          <dt class="text-[13px] text-ink-500">资源 ID</dt>
          <dd class="tabular text-[13.5px] text-ink-900">{{ entry.resource_id || '—' }}</dd>
        </div>

        <div class="form-row">
          <dt class="text-[13px] text-ink-500">请求方法</dt>
          <dd class="text-[13.5px] text-ink-900">{{ entry.method || '—' }}</dd>
        </div>

        <div class="form-row">
          <dt class="text-[13px] text-ink-500">请求路径</dt>
          <dd class="min-w-0">
            <div class="flex items-start gap-2">
              <code
                class="min-w-0 rounded bg-ink-100 px-1.5 py-0.5 font-mono text-[12.5px] break-all text-ink-700"
              >
                {{ entry.path || '—' }}
              </code>
              <AppButton v-if="entry.path" variant="ghost" size="sm" @click="copyPath">
                {{ copied ? '已复制' : '复制' }}
              </AppButton>
            </div>
          </dd>
        </div>

        <div class="form-row">
          <dt class="text-[13px] text-ink-500">来源 IP</dt>
          <dd class="tabular text-[13.5px] text-ink-900">{{ entry.ip || '—' }}</dd>
        </div>

        <div class="form-row">
          <dt class="text-[13px] text-ink-500">User-Agent</dt>
          <dd class="text-[13px] break-all text-ink-600">{{ entry.user_agent || '—' }}</dd>
        </div>

        <div class="form-row">
          <dt class="text-[13px] text-ink-500">详情</dt>
          <dd class="min-w-0">
            <!-- detail 是结构化对象（字段变更等），用 auditDetailText 压成一行可读文本 -->
            <p class="text-[13.5px] leading-relaxed text-ink-900">
              {{ auditDetailText(entry.detail) }}
            </p>
            <pre
              v-if="entry.detail && typeof entry.detail === 'object'"
              class="mt-2 overflow-auto rounded-xl bg-ink-50 px-3.5 py-3 font-mono text-[12px] leading-relaxed text-ink-700"
            >{{ JSON.stringify(entry.detail, null, 2) }}</pre>
          </dd>
        </div>
      </dl>

      <p class="mt-5 flex items-start gap-1.5 rounded-xl bg-page px-4 py-3 text-[12.5px] leading-relaxed text-ink-500">
        <AppIcon name="info" :size="14" class="mt-0.5" />
        审计记录不可修改、不可删除，仅按保留策略轮转，因此这里没有编辑或删除操作。
      </p>

      <div class="mt-5">
        <AppButton variant="secondary" :to="{ name: 'logs' }">返回列表</AppButton>
      </div>
    </section>
  </div>
</template>
