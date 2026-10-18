<script setup>
import { computed } from 'vue'

import { adminApi } from '@/api'
import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import BarChart from '@/components/BarChart.vue'
import DonutChart from '@/components/DonutChart.vue'
import StatCard from '@/components/StatCard.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import {
  JOB_STATUS,
  TRIGGER_LABEL,
  companyInitial,
  crawlStatus,
  formatDateTime,
  formatDuration,
  jobStatus,
  locationText,
  relativeTime,
  salaryText,
  toneFor,
} from '@/utils/format'

/**
 * 仪表盘 = 设计稿第 6 屏。
 *
 * 信息顺序刻意是「规模 → 分布 → 待办」：
 * 运营每天打开后台真正要动手的是下面的待审核队列，因此它必须靠前，
 * 而不是被两张图表挤到折叠线以下。
 *
 * 与后端契约相关的两个关键取舍（原设计稿在这一页假设了不存在的字段）：
 * 1. 指标一律取自 GET /admin/stats 的嵌套 StatsView（users/jobs/companies/crawl），
 *    后端不返回扁平的岗位总数与状态分布字段。
 * 2. 「近 7 日趋势」后端没有聚合接口，因此由客户端把抓取运行记录按本地日期分桶，
 *    并把口径写在图下方 —— 宁可标注样本量，也不编一个看起来精确的假图表。
 */
const TREND_DAYS = 7
/** 一次最多取 100 条（接口上限）：趋势图是「最近 100 次运行」的真实汇总，不是全量 */
const TREND_SAMPLE = 100

const { data: stats, loading, error, ready, run: runStats } = useAsync(() => adminApi.stats())
const { data: trendRuns, run: runTrend } = useAsync(
  () => adminApi.crawl.runs({ page_size: TREND_SAMPLE }),
  { immediate: false },
)
const { data: pendingQueue, run: runQueue } = useAsync(
  () => adminApi.jobs.list({ status: 'pending_review', page_size: 5 }),
  { immediate: false },
)
const { data: recentRuns, run: runRecent } = useAsync(
  () => adminApi.crawl.runs({ page_size: 5 }),
  { immediate: false },
)

function load() {
  runStats()
  runTrend()
  runQueue()
  runRecent()
}

useAdminRefresh(load)

/* -------------------------------------------------------------------------- */
/* 统计卡                                                                      */
/* -------------------------------------------------------------------------- */

const cards = computed(() => {
  const jobs = stats.value?.jobs ?? {}
  const companies = stats.value?.companies ?? {}
  const users = stats.value?.users ?? {}
  const crawl = stats.value?.crawl ?? {}

  return [
    {
      key: 'jobs',
      label: '岗位总数',
      value: jobs.total ?? 0,
      hint: `已发布 ${jobs.approved ?? 0}`,
      icon: 'briefcase',
    },
    {
      key: 'pending',
      label: '待审核岗位',
      value: jobs.pending_review ?? 0,
      hint: `全部岗位 ${jobs.total ?? 0}`,
      icon: 'clock',
      alert: (jobs.pending_review ?? 0) > 0,
    },
    {
      key: 'companies',
      label: '收录公司',
      value: companies.total ?? 0,
      hint: `启用 ${companies.enabled ?? 0}`,
      icon: 'building',
    },
    {
      key: 'users',
      label: '注册用户',
      value: users.total ?? 0,
      hint: `正常 ${users.active ?? 0}`,
      icon: 'users',
    },
    {
      key: 'crawl',
      label: '今日抓取运行',
      value: crawl.runs_today ?? 0,
      hint: `累计 ${crawl.runs_total ?? 0}`,
      icon: 'bolt',
    },
  ]
})

/* -------------------------------------------------------------------------- */
/* 岗位状态分布（环形图）                                                       */
/* -------------------------------------------------------------------------- */

/**
 * 环形图的五个分段由 jobs 的五个状态计数在客户端拼装。
 * 顺序按「待处理 → 已通过 → 已驳回 → 已下线 → 已过期」，与运营的关注顺序一致。
 */
const STATUS_ORDER = ['pending_review', 'approved', 'rejected', 'offline', 'expired']

const distribution = computed(() =>
  STATUS_ORDER.map((key) => ({
    key,
    label: JOB_STATUS[key].label,
    tone: JOB_STATUS[key].tone,
    count: stats.value?.jobs?.[key] ?? 0,
  })),
)

const distributionTotal = computed(() =>
  distribution.value.reduce((sum, item) => sum + item.count, 0),
)

/* -------------------------------------------------------------------------- */
/* 近 7 日抓取趋势（客户端分桶）                                                 */
/* -------------------------------------------------------------------------- */

function dayKey(value) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (n) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

/**
 * 按「抓取运行记录」的本地日期分桶汇总 jobs_created。
 *
 * 为什么不画 jobs_found：found 是招聘页上发现的链接数，会把同一岗位的重复链接也计入，
 * 用它当「趋势」会系统性放大波动；created 才是真正入库的新岗位，口径更硬。
 */
const trend = computed(() => {
  const buckets = []
  const today = new Date()
  for (let offset = TREND_DAYS - 1; offset >= 0; offset -= 1) {
    const day = new Date(today.getFullYear(), today.getMonth(), today.getDate() - offset)
    buckets.push({
      key: dayKey(day),
      label: `${day.getMonth() + 1}/${day.getDate()}`,
      value: 0,
    })
  }

  const byDay = new Map(buckets.map((bucket) => [bucket.key, bucket]))
  for (const run of trendRuns.value?.items ?? []) {
    const bucket = byDay.get(dayKey(run.started_at))
    if (bucket) bucket.value += Number(run.jobs_created) || 0
  }
  return buckets
})

const trendSample = computed(() => trendRuns.value?.items?.length ?? 0)
const trendSum = computed(() => trend.value.reduce((sum, item) => sum + item.value, 0))
</script>

<template>
  <div>
    <!-- 加载骨架：保持与真实内容一致的栅格，避免加载完成时页面跳动 -->
    <div v-if="loading && !ready" class="grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
      <div v-for="index in 5" :key="index" class="card h-[116px] animate-pulse" />
    </div>

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="统计数据加载失败"
      :description="error.message"
    >
      <AppButton @click="load()">重新加载</AppButton>
    </AppEmpty>

    <template v-else>
      <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
        <StatCard
          v-for="card in cards"
          :key="card.key"
          :label="card.label"
          :value="card.value"
          :hint="card.hint"
          :icon="card.icon"
          :alert="card.alert"
        />
      </div>

      <div class="mt-2.5 grid gap-2.5 xl:grid-cols-2">
        <section class="card p-6">
          <header class="mb-6 flex items-center justify-between">
            <h2 class="text-[16px] font-semibold text-ink-900">岗位状态分布</h2>
            <span class="text-[12.5px] text-ink-400">全部岗位 {{ distributionTotal }} 条</span>
          </header>
          <DonutChart :items="distribution" />
        </section>

        <section class="card p-6">
          <header class="mb-6 flex items-center justify-between">
            <h2 class="text-[16px] font-semibold text-ink-900">近 7 日新增岗位</h2>
            <span class="text-[12.5px] text-ink-400">单位：条</span>
          </header>
          <BarChart :items="trend" :height="196" />

          <!--
            诚实标注口径：后端没有「抓取趋势」聚合接口，这张图是客户端按
            抓取运行记录的 started_at 做本地日期分桶后汇总 jobs_created 得到的。
          -->
          <p class="mt-4 flex items-start gap-1.5 text-[12px] leading-relaxed text-ink-400">
            <AppIcon name="info" :size="14" class="mt-px shrink-0" />
            口径：由抓取运行记录在客户端按 started_at 的本地日期分桶，汇总每天的
            jobs_created（新建入库岗位数），合计 {{ trendSum }} 条，样本为最近
            {{ trendSample }} 次运行；后端暂无聚合接口，因此运行记录超过
            {{ TREND_SAMPLE }} 次时该图不覆盖更早的数据。
          </p>
        </section>
      </div>

      <!-- 待审核队列：这一屏里唯一需要立刻动手的地方 -->
      <section class="card mt-2.5 p-6">
        <header class="mb-4 flex items-center justify-between">
          <h2 class="text-[16px] font-semibold text-ink-900">
            待审核队列
            <span
              v-if="stats?.jobs?.pending_review"
              class="ml-1.5 text-[13px] font-normal text-amber-600"
            >
              共 {{ stats.jobs.pending_review }} 条待处理
            </span>
          </h2>
          <RouterLink
            :to="{ name: 'review' }"
            class="inline-flex items-center gap-1 text-[13px] text-brand-600 transition hover:text-brand-700"
          >
            查看全部
            <AppIcon name="chevron-right" :size="14" />
          </RouterLink>
        </header>

        <ul v-if="pendingQueue?.items?.length" class="divide-y divide-ink-100">
          <li v-for="job in pendingQueue.items" :key="job.id" class="flex items-center gap-4 py-3.5">
            <AppAvatar
              size="sm"
              :name="job.company?.name"
              :initial="companyInitial(job.company)"
              :tone="toneFor(job.company?.id ?? job.company?.name)"
            />
            <div class="min-w-0 flex-1">
              <p class="truncate text-[14px] font-medium text-ink-800">{{ job.title }}</p>
              <p class="mt-0.5 truncate text-[12.5px] text-ink-400">
                {{ job.company?.name }} · {{ locationText(job) || '地点待定' }} ·
                {{ salaryText(job) }} · {{ relativeTime(job.last_updated_at) }}更新
              </p>
            </div>
            <AppBadge :tone="jobStatus(job.status).tone" size="sm">
              {{ jobStatus(job.status).label }}
            </AppBadge>
            <AppButton size="sm" :to="{ name: 'posting', params: { id: job.id } }">审核</AppButton>
          </li>
        </ul>

        <AppEmpty
          v-else
          icon="check-circle"
          title="审核队列已清空"
          description="当前没有等待审核的岗位，新的抓取结果会自动进入这里。"
        />
      </section>

      <!-- 最新抓取运行：回答「数据是怎么来的」 -->
      <section class="card mt-2.5 p-6">
        <header class="mb-4 flex items-center justify-between">
          <h2 class="text-[16px] font-semibold text-ink-900">最新抓取运行</h2>
          <RouterLink
            :to="{ name: 'monitor' }"
            class="inline-flex items-center gap-1 text-[13px] text-brand-600 transition hover:text-brand-700"
          >
            查看监控
            <AppIcon name="chevron-right" :size="14" />
          </RouterLink>
        </header>

        <ul v-if="recentRuns?.items?.length" class="divide-y divide-ink-100">
          <li
            v-for="item in recentRuns.items"
            :key="item.id"
            class="flex flex-wrap items-center gap-x-4 gap-y-2 py-3.5 text-[13px]"
          >
            <span class="min-w-[120px] font-medium text-ink-800">
              {{ item.company_name || `公司 #${item.company_id}` }}
            </span>
            <AppBadge tone="ink" size="sm">{{ TRIGGER_LABEL[item.trigger] ?? item.trigger }}</AppBadge>
            <AppBadge :tone="crawlStatus(item.status).tone" size="sm">
              {{ crawlStatus(item.status).label }}
            </AppBadge>
            <span class="tabular text-ink-500">
              发现 {{ item.jobs_found }} / 新建 {{ item.jobs_created }} / 过滤 {{ item.jobs_filtered }}
            </span>
            <span class="tabular ml-auto text-ink-400">
              耗时 {{ formatDuration(item.duration_ms) }} · {{ formatDateTime(item.started_at) }}
            </span>
          </li>
        </ul>

        <AppEmpty v-else title="暂无抓取记录" description="触发一次抓取后，运行结果会出现在这里。" />
      </section>
    </template>
  </div>
</template>
