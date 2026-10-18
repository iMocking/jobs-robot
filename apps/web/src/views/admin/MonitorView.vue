<script setup>
import { computed, reactive, ref } from 'vue'

import { adminApi } from '@/api'
import AdminTable from '@/components/AdminTable.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppPagination from '@/components/AppPagination.vue'
import AppSelect from '@/components/AppSelect.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { useToastStore } from '@/stores/toast'
import { crawlStatus, formatDateTime, formatDuration, formatNumber, TRIGGER_LABEL } from '@/utils/format'

/**
 * 系统监控 = 设计稿第 10 屏。
 *
 * 上半屏回答「现在能不能干活」（依赖健康 + 爬虫开关 + 策略规模），
 * 下半屏回答「刚才干了什么」（运行记录与口径）。
 * 口径说明是刻意保留的：发现 / 新建 / 过滤 三个数字经常被误读成同一件事，
 * 不写清楚就会出现「发现了 12 个为什么只入库 8 个」的反复追问。
 *
 * 关于健康检查的两个契约事实：
 *  1. GET /healthz 返回的是裸 HealthView（status / postgres / redis / details），
 *     **不套** { code, message, data } 信封，所以不能走 http.get —— 它会把非 0 的 code 当错误抛掉。
 *     这个差异已经收敛在 http.js 的 requestProbe() 里，这里直接用 adminApi.health()。
 *  2. 依赖异常时后端返回 HTTP 503，但响应体仍是完整的 HealthView；
 *     探针的 HTTP 状态是给编排系统看的，页面必须照常渲染这份 payload。
 *     requestProbe 对非 2xx 同样返回 payload，因此这里不需要关心状态码。
 * 设计稿里的 version / uptime / api_prefix / environment / checks[] 后端都不返回，已全部移除。
 */
const PAGE_SIZE = 10
const toast = useToastStore()

const health = useAsync(() => adminApi.health())

// filters 必须先于 runs 声明：useAsync 默认 immediate=true，创建时就会调用取数函数
const filters = reactive({ status: '', page: 1 })
const running = ref(false)

const runs = useAsync(() =>
  adminApi.crawl.runs({
    status: filters.status || undefined,
    page: filters.page,
    page_size: PAGE_SIZE,
  }),
)

useAdminRefresh(() => {
  health.run()
  runs.run()
})

const columns = [
  { key: 'company', label: '公司' },
  { key: 'trigger', label: '触发方式', class: 'w-[110px]' },
  { key: 'status', label: '状态', class: 'w-[110px]' },
  { key: 'stats', label: '发现 / 新建 / 过滤', class: 'w-[190px]' },
  { key: 'duration', label: '耗时', class: 'w-[100px]' },
  { key: 'started', label: '开始时间', class: 'w-[180px]' },
]

/* —— 健康检查卡片：字段与 HealthView 一一对应 —— */

/** Tailwind 只扫描字面量类名，不能用 `bg-${tone}-500` 拼 */
const TONE_DOT = {
  brand: 'bg-brand-500',
  accent: 'bg-accent-500',
  amber: 'bg-amber-500',
  danger: 'bg-red-500',
  violet: 'bg-violet-500',
  ink: 'bg-ink-400',
}

/** 依赖状态取值来自后端（up / unknown / down…），未知一律按「不健康」保守处理 */
function dependencyTone(value) {
  if (value === 'up') return 'accent'
  if (value === 'unknown') return 'amber'
  return 'danger'
}

const DEPENDENCY_LABEL = { up: '可用', unknown: '状态未知', down: '不可用' }

const dimensionCards = computed(() => {
  const info = health.data.value
  if (!info) return []
  const details = info.details ?? {}
  const authz = details.authorization ?? {}
  const crawlerOn = Boolean(details.crawler)

  return [
    {
      key: 'status',
      label: '服务状态',
      value: info.status === 'ok' ? '正常' : '降级',
      hint: info.status === 'ok' ? '全部依赖可用' : '存在不可用依赖，见下方说明',
      tone: info.status === 'ok' ? 'accent' : 'danger',
    },
    {
      key: 'postgres',
      label: 'PostgreSQL',
      value: DEPENDENCY_LABEL[info.postgres] ?? info.postgres ?? '未知',
      hint: '岗位与用户数据的唯一存储',
      tone: dependencyTone(info.postgres),
    },
    {
      key: 'redis',
      label: 'Redis',
      value: DEPENDENCY_LABEL[info.redis] ?? info.redis ?? '未知',
      hint: '会话、验证码与限流依赖',
      tone: dependencyTone(info.redis),
    },
    {
      key: 'crawler',
      label: '爬虫开关',
      value: crawlerOn ? '已启用' : '已关闭',
      hint: crawlerOn ? '定时任务会按公司间隔抓取' : '关闭时手动触发也会被拒绝（40001）',
      tone: crawlerOn ? 'accent' : 'amber',
    },
    {
      key: 'authorization',
      label: '策略规模',
      value: authz.policies ?? 0,
      hint: `角色分组规则 ${authz.grouping_rules ?? 0} 条`,
      tone: 'violet',
    },
  ]
})

const healthError = computed(() => health.error.value?.message ?? '')
const degradationReason = computed(() => health.data.value?.details?.error ?? '')

const totalRuns = computed(() => runs.data.value?.total ?? 0)

async function runAll() {
  running.value = true
  try {
    // 响应体是 { runs, count }，没有 message 字段
    const result = await adminApi.crawl.runAll()
    const count = result?.count ?? result?.runs?.length ?? 0
    toast.success(count ? `已提交 ${count} 家公司的抓取任务` : '当前没有可抓取的公司')
    filters.page = 1
    runs.run()
  } catch (err) {
    toast.fromError(err, '提交全量抓取失败')
  } finally {
    running.value = false
  }
}

function changePage(next) {
  filters.page = next
  runs.run()
}
</script>

<template>
  <div>
    <!-- 健康检查 -->
    <div
      v-if="health.loading.value && !health.ready.value"
      class="grid gap-2 sm:grid-cols-2 xl:grid-cols-4"
    >
      <div v-for="index in 4" :key="index" class="card h-[132px] animate-pulse" />
    </div>

    <template v-else>
      <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
        <article v-for="card in dimensionCards" :key="card.key" class="card p-5">
          <div class="flex items-center gap-2.5">
            <span class="size-2.5 shrink-0 rounded-full" :class="TONE_DOT[card.tone]" />
            <h2 class="text-[14px] font-semibold text-ink-900">{{ card.label }}</h2>
          </div>

          <p class="tabular mt-3 text-[22px] leading-none font-semibold text-ink-900">
            {{ card.value }}
          </p>
          <p class="mt-2 text-[12px] leading-relaxed text-ink-400">{{ card.hint }}</p>
        </article>
      </div>

      <!-- 降级时后端会把具体错误放在 details.error；503 状态码本身只是给探针看的 -->
      <p
        v-if="degradationReason"
        class="mt-2 flex items-start gap-1.5 rounded-xl bg-red-50 px-4 py-3 text-[12.5px] leading-relaxed text-red-600"
      >
        <AppIcon name="alert" :size="15" class="mt-px shrink-0" />
        后端报告降级：{{ degradationReason }}（该响应 HTTP 状态为 503，页面按 payload 正常渲染）
      </p>

      <AppEmpty
        v-if="healthError"
        class="mt-2"
        icon="alert"
        tone="danger"
        title="健康检查不可用"
        :description="healthError"
      >
        <AppButton @click="health.run()">重新检查</AppButton>
      </AppEmpty>
    </template>

    <!-- 抓取运行记录 -->
    <section class="mt-3">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
        <h2 class="text-[16px] font-semibold text-ink-900">抓取运行记录</h2>
        <div class="flex flex-wrap items-center gap-3">
          <div class="w-[160px]">
            <AppSelect
              v-model="filters.status"
              compact
              placeholder="全部状态"
              :options="[
                { value: 'running', label: '进行中' },
                { value: 'success', label: '成功' },
                { value: 'partial', label: '部分成功' },
                { value: 'failed', label: '失败' },
                { value: 'skipped', label: '已跳过' },
              ]"
              @update:model-value="filters.page = 1; runs.run()"
            />
          </div>
          <AppButton size="sm" :loading="running" @click="runAll">
            <AppIcon name="bolt" :size="16" />
            全量抓取
          </AppButton>
        </div>
      </div>

      <AdminTable
        :columns="columns"
        :empty="!runs.loading.value && !runs.data.value?.items?.length"
        empty-text="暂无抓取记录"
      >
        <template v-if="runs.loading.value && !runs.ready.value">
          <tr v-for="index in 5" :key="index">
            <td :colspan="columns.length" class="td-cell">
              <div class="h-9 animate-pulse rounded-lg bg-ink-100" />
            </td>
          </tr>
        </template>

        <tr v-else-if="runs.error.value">
          <td :colspan="columns.length">
            <AppEmpty
              icon="alert"
              tone="danger"
              title="抓取记录加载失败"
              :description="runs.error.value.message"
            >
              <AppButton @click="runs.run()">重新加载</AppButton>
            </AppEmpty>
          </td>
        </tr>

        <tr
          v-for="item in runs.data.value?.items ?? []"
          :key="item.id"
          class="transition hover:bg-ink-50/60"
        >
          <td class="td-cell">
            <div class="flex items-center gap-2.5">
              <span class="font-medium text-ink-900">
                {{ item.company_name || `公司 #${item.company_id}` }}
              </span>
              <span
                v-if="item.used_headless"
                class="rounded bg-ink-100 px-1.5 py-0.5 text-[11px] text-ink-500"
                title="该次抓取回退到了无头渲染"
              >
                无头
              </span>
            </div>
            <p v-if="item.skip_reason" class="mt-1 text-[12px] text-ink-400">
              跳过原因：{{ item.skip_reason }}
            </p>
            <p v-if="item.error_message" class="mt-1 max-w-[320px] truncate text-[12px] text-red-500">
              {{ item.error_message }}
            </p>
          </td>

          <td class="td-cell">
            <AppBadge tone="ink" size="sm">{{ TRIGGER_LABEL[item.trigger] ?? item.trigger }}</AppBadge>
          </td>

          <td class="td-cell">
            <AppBadge :tone="crawlStatus(item.status).tone" size="sm">
              {{ crawlStatus(item.status).label }}
            </AppBadge>
          </td>

          <td class="td-cell tabular text-[13px] text-ink-600">
            {{ item.jobs_found }} / {{ item.jobs_created }} / {{ item.jobs_filtered }}
          </td>

          <td class="td-cell tabular text-[13px] text-ink-500">
            {{ formatDuration(item.duration_ms) }}
          </td>

          <td class="td-cell tabular text-[13px] text-ink-500">
            {{ formatDateTime(item.started_at) }}
          </td>
        </tr>

        <template #footer>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <p class="text-[12.5px] text-ink-400">
              共 {{ formatNumber(totalRuns) }} 条记录 · 口径：发现（页面链接数）/ 新建（落库）/ 过滤（被收集范围丢弃）
            </p>
            <AppPagination
              :page="runs.data.value?.page ?? 1"
              :pages="runs.data.value?.pages ?? 1"
              :total="totalRuns"
              :page-size="PAGE_SIZE"
              @update:page="changePage"
            />
          </div>
        </template>
      </AdminTable>
    </section>
  </div>
</template>
