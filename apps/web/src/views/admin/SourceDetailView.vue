<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import { adminApi } from '@/api'
import AdminTable from '@/components/AdminTable.vue'
import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppSwitch from '@/components/AppSwitch.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { useToastStore } from '@/stores/toast'
import {
  TRIGGER_LABEL,
  companyInitial,
  crawlStatus,
  formatDateTime,
  formatDuration,
  jobStatus,
  relativeTime,
  salaryText,
  toneFor,
} from '@/utils/format'

/**
 * 公司详情 = 单家来源官网的「作业台」。
 *
 * 三块信息对应三个问题：
 *   抓取设置 —— 怎么抓（间隔、页数上限、是否需要无头渲染、是否守 robots）
 *   该公司岗位 —— 抓到了什么（包含未发布的，方便核对审核链路）
 *   最近抓取记录 —— 抓得怎么样（发现 / 新建 / 被过滤，以及失败原因）
 *
 * 关键契约事实：GET /admin/companies/:id 只返回 CompanyView 本身，
 * **不含** jobs / runs 数组（原来的 company.jobs / company.runs 是 mock 里的假字段），
 * 因此这里分别用 /admin/jobs?company_id= 与 /admin/crawl-runs?company_id= 取数。
 */
const route = useRoute()
const toast = useToastStore()

const companyId = computed(() => route.params.id)
const PAGE_SIZE = 10

const { data, loading, error, ready, run } = useAsync(
  () => adminApi.companies.detail(companyId.value),
  { immediate: false },
)

const { data: jobPage, run: runJobs } = useAsync(
  () => adminApi.jobs.list({ company_id: companyId.value, page_size: PAGE_SIZE }),
  { immediate: false },
)

const { data: runPage, run: runRuns } = useAsync(
  () => adminApi.crawl.runs({ company_id: companyId.value, page_size: PAGE_SIZE }),
  { immediate: false },
)

const company = computed(() => data.value ?? null)
const jobs = computed(() => jobPage.value?.items ?? [])
const runs = computed(() => runPage.value?.items ?? [])

function load() {
  run()
  runJobs()
  runRuns()
}

useAdminRefresh(load)

// 从列表点进另一家公司时组件会被复用，必须跟着 id 重新取数
watch(companyId, () => load())

/* -------------------------------------------------------------------------- */
/* 抓取设置                                                                    */
/* -------------------------------------------------------------------------- */

const settings = reactive({
  crawl_interval_seconds: 21600,
  max_pages: 0,
  force_headless: false,
  respect_robots: true,
})

/** 回填时记住原始 max_pages：crawl_rules 是整份覆盖，没改动就不要回传这个 map */
let baselineRules = {}
let baselineMaxPages = 0

/**
 * 只读后端真实存在的字段。
 * crawl_rules 是站点定向覆盖（选择器等），这里只把 max_pages 暴露成可编辑项，
 * 其余键原样保留 —— 覆盖提交会把这些站点适配规则整体抹掉。
 */
function hydrate(value) {
  if (!value) return
  const rules = value.crawl_rules ?? {}
  baselineRules = { ...rules }
  baselineMaxPages = Number(rules.max_pages ?? 0)

  settings.crawl_interval_seconds = Number(value.crawl_interval_seconds ?? 21600)
  settings.max_pages = baselineMaxPages
  settings.force_headless = Boolean(value.force_headless)
  // respect_robots 默认 true —— 缺少字段时按「遵守」处理，不能默认放开
  settings.respect_robots = value.respect_robots === undefined ? true : Boolean(value.respect_robots)
}

watch(data, (value) => hydrate(value))

const saving = ref(false)

async function saveSettings() {
  saving.value = true
  try {
    const payload = {
      crawl_interval_seconds: Math.max(0, Number(settings.crawl_interval_seconds) || 0),
      force_headless: settings.force_headless,
      respect_robots: settings.respect_robots,
    }

    // 页数上限存在 crawl_rules.max_pages 里，而 PATCH 对该字段是整份替换：
    // 只有真的改动过才回传，并在此基础上合并原有键，避免丢掉站点选择器。
    const nextMaxPages = Math.max(0, Number(settings.max_pages) || 0)
    if (nextMaxPages !== baselineMaxPages) {
      const merged = { ...baselineRules }
      if (nextMaxPages > 0) merged.max_pages = nextMaxPages
      else delete merged.max_pages
      payload.crawl_rules = merged
    }

    await adminApi.companies.update(companyId.value, payload)
    toast.success('抓取设置已保存')
    await load()
  } catch (err) {
    toast.fromError(err, '保存抓取设置失败')
  } finally {
    saving.value = false
  }
}

/* -------------------------------------------------------------------------- */
/* 头部操作                                                                    */
/* -------------------------------------------------------------------------- */

const togglingEnabled = ref(false)
const togglingCrawl = ref(false)
const crawling = ref(false)

async function toggleEnabled(enabled) {
  togglingEnabled.value = true
  try {
    await adminApi.companies.setEnabled(companyId.value, enabled)
    toast.success(enabled ? '已启用该公司' : '已停用该公司')
    await load()
  } catch (err) {
    toast.fromError(err, '更新启用状态失败')
  } finally {
    togglingEnabled.value = false
  }
}

async function toggleCrawl(enabled) {
  togglingCrawl.value = true
  try {
    await adminApi.companies.update(companyId.value, { crawl_enabled: enabled })
    toast.success(enabled ? '已开启抓取' : '已关闭抓取')
    await load()
  } catch (err) {
    toast.fromError(err, '更新抓取开关失败')
  } finally {
    togglingCrawl.value = false
  }
}

async function crawlNow() {
  crawling.value = true
  try {
    const result = await adminApi.companies.crawl(companyId.value)
    const status = crawlStatus(result?.status)
    toast.success(
      `抓取${status.label}：发现 ${result?.jobs_found ?? 0} / 新建 ${result?.jobs_created ?? 0} / 过滤 ${result?.jobs_filtered ?? 0}`,
    )
    await load()
  } catch (err) {
    // 40005 表示已有任务在跑，透出原文让运营知道「不是没生效」
    toast.fromError(err, '触发抓取失败')
  } finally {
    crawling.value = false
  }
}

async function discover() {
  try {
    const updated = await adminApi.companies.discover(companyId.value)
    toast.success(
      updated?.discovery_url || updated?.careers_url
        ? '已定位到《加入我们》入口并回填'
        : '未发现可用的招聘入口，请手工填写招聘页地址',
    )
    await load()
  } catch (err) {
    if (err?.code === 40006) {
      toast.error(err.message || '未能定位到该公司的招聘页面入口')
    } else {
      toast.fromError(err, '自动定位失败')
    }
  }
}

/* -------------------------------------------------------------------------- */
/* 只读展示                                                                    */
/* -------------------------------------------------------------------------- */

const jobColumns = [
  { key: 'title', label: '岗位' },
  { key: 'status', label: '状态' },
  { key: 'salary', label: '薪资' },
  { key: 'updated', label: '最后更新' },
  { key: 'actions', label: '操作', class: 'text-right' },
]

const runColumns = [
  { key: 'trigger', label: '触发方式' },
  { key: 'status', label: '状态' },
  { key: 'result', label: '发现 / 新建 / 过滤' },
  { key: 'duration', label: '耗时' },
  { key: 'started', label: '开始时间' },
]

/** 后端 source 只有 manual（后台录入）/ discovered（爬虫发现）两种取值 */
const SOURCE_LABEL = { manual: '后台录入', discovered: '爬虫发现' }

const lastCrawl = computed(() => company.value?.last_crawl_status ?? '')

const rulesText = computed(() => {
  const rules = company.value?.crawl_rules ?? {}
  return Object.keys(rules).length ? JSON.stringify(rules, null, 2) : ''
})
</script>

<template>
  <div>
    <RouterLink
      :to="{ name: 'sources' }"
      class="mb-4 inline-flex items-center gap-1.5 text-[13px] text-ink-500 transition hover:text-brand-600"
    >
      <AppIcon name="arrow-left" :size="15" />
      返回公司管理
    </RouterLink>

    <!-- 首次加载骨架 -->
    <div v-if="loading && !ready" class="space-y-2.5">
      <div class="card h-[190px] animate-pulse" />
      <div class="card h-[300px] animate-pulse" />
    </div>

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="公司信息加载失败"
      :description="error.message"
    >
      <div class="flex gap-3">
        <AppButton variant="secondary" :to="{ name: 'sources' }">返回列表</AppButton>
        <AppButton @click="load()">重新加载</AppButton>
      </div>
    </AppEmpty>

    <template v-else-if="company">
      <!-- 头部：来源身份 + 两个作业开关 -->
      <section class="card p-6">
        <div class="flex flex-wrap items-start justify-between gap-5">
          <div class="flex min-w-0 items-start gap-4">
            <AppAvatar
              :name="company.name"
              :initial="companyInitial(company)"
              :tone="toneFor(company.id ?? company.name)"
              size="xl"
            />

            <div class="min-w-0">
              <h1 class="flex items-center gap-2 text-[22px] font-semibold text-ink-900">
                {{ company.name }}
                <span
                  v-if="company.verified"
                  class="text-brand-500"
                  title="已核实企业官网《加入我们》页面"
                >
                  <AppIcon name="check-circle" :size="18" />
                </span>
              </h1>

              <div class="mt-2.5 flex flex-wrap items-center gap-2">
                <AppBadge tone="ink">{{ company.industry || '行业未填' }}</AppBadge>
                <AppBadge tone="ink">{{ company.location || '地点未填' }}</AppBadge>
                <AppBadge :tone="company.enabled ? 'accent' : 'ink'">
                  {{ company.enabled ? '已启用' : '已停用' }}
                </AppBadge>
                <AppBadge :tone="company.verified ? 'brand' : 'amber'">
                  {{ company.verified ? '已核实来源' : '待核实来源' }}
                </AppBadge>
                <AppBadge tone="ink">{{ SOURCE_LABEL[company.source] ?? company.source }}</AppBadge>
                <a
                  v-if="company.website"
                  :href="company.website"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="inline-flex items-center gap-1 text-[12.5px] text-ink-500 transition hover:text-brand-600"
                >
                  {{ company.website }}
                  <AppIcon name="external-link" :size="13" />
                </a>
              </div>

              <!-- 招聘入口决定「能抓到什么」，所以单独一行 -->
              <div class="mt-4 flex flex-wrap items-center gap-2 rounded-xl bg-page px-3.5 py-2.5">
                <AppIcon name="globe" :size="15" class="text-ink-400" />
                <span class="text-[12.5px] text-ink-500">招聘入口</span>
                <template v-if="company.careers_url">
                  <span class="max-w-[360px] truncate text-[12.5px] text-ink-700">
                    {{ company.careers_url }}
                  </span>
                  <AppButton variant="secondary" size="sm" :href="company.careers_url">
                    打开招聘页
                  </AppButton>
                </template>
                <template v-else>
                  <AppBadge tone="amber">未定位</AppBadge>
                  <AppButton variant="secondary" size="sm" @click="discover">自动定位</AppButton>
                </template>
              </div>

              <p v-if="company.discovery_url" class="mt-2 text-[12px] text-ink-400">
                入口由爬虫从该页面发现：
                <a
                  :href="company.discovery_url"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="break-all text-ink-500 transition hover:text-brand-600"
                >
                  {{ company.discovery_url }}
                </a>
              </p>
            </div>
          </div>

          <div class="flex flex-col items-start gap-3">
            <AppSwitch
              :model-value="Boolean(company.enabled)"
              :disabled="togglingEnabled"
              label="启用"
              @update:model-value="toggleEnabled"
            />
            <AppSwitch
              :model-value="Boolean(company.crawl_enabled)"
              :disabled="togglingCrawl"
              label="抓取"
              @update:model-value="toggleCrawl"
            />
            <AppButton :loading="crawling" class="mt-1" @click="crawlNow">
              <AppIcon name="bolt" :size="16" />
              立即抓取
            </AppButton>
          </div>
        </div>
      </section>

      <!-- 抓取健康：全部来自后端已有的 last_crawl_* / robots_* 字段 -->
      <section class="mt-2.5 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
        <article class="card p-5">
          <div class="flex items-center gap-2.5">
            <span
              class="size-2.5 shrink-0 rounded-full"
              :class="
                lastCrawl === 'success'
                  ? 'bg-accent-500'
                  : lastCrawl === 'failed'
                    ? 'bg-red-500'
                    : 'bg-ink-300'
              "
            />
            <h2 class="text-[14px] font-semibold text-ink-900">最近一次抓取</h2>
          </div>
          <p class="mt-2 text-[13px] text-ink-600">
            {{ lastCrawl ? crawlStatus(lastCrawl).label : '暂无记录' }}
          </p>
          <p class="mt-1 text-[12px] text-ink-400">
            {{ company.last_crawled_at ? formatDateTime(company.last_crawled_at) : '尚未抓取过' }}
          </p>
        </article>

        <article class="card p-5">
          <div class="flex items-center gap-2.5">
            <span
              class="size-2.5 shrink-0 rounded-full"
              :class="company.robots_allowed ? 'bg-accent-500' : 'bg-red-500'"
            />
            <h2 class="text-[14px] font-semibold text-ink-900">robots.txt</h2>
          </div>
          <p class="mt-2 text-[13px] text-ink-600">
            {{ company.robots_allowed ? '允许抓取' : '站点声明不允许抓取' }}
          </p>
          <p class="mt-1 text-[12px] text-ink-400">
            {{
              company.robots_checked_at
                ? `最后检查 ${formatDateTime(company.robots_checked_at)}`
                : '尚未检查'
            }}
            · {{ company.respect_robots ? '遵守 robots' : '未遵守 robots' }}
          </p>
        </article>

        <article class="card p-5">
          <div class="flex items-center gap-2.5">
            <span class="size-2.5 shrink-0 rounded-full bg-brand-500" />
            <h2 class="text-[14px] font-semibold text-ink-900">在架已发布岗位</h2>
          </div>
          <p class="tabular mt-2 text-[20px] leading-none font-semibold text-ink-900">
            {{ company.job_count ?? 0 }}
          </p>
          <p class="mt-1 text-[12px] text-ink-400">本页共列出 {{ jobs.length }} 条（含未发布）</p>
        </article>

        <article class="card p-5">
          <div class="flex items-center gap-2.5">
            <span class="size-2.5 shrink-0 rounded-full bg-violet-500" />
            <h2 class="text-[14px] font-semibold text-ink-900">抓取优先级</h2>
          </div>
          <p class="tabular mt-2 text-[20px] leading-none font-semibold text-ink-900">
            {{ company.priority ?? 0 }}
          </p>
          <p class="mt-1 text-[12px] text-ink-400">数值越大越先被抓取</p>
        </article>
      </section>

      <p
        v-if="company.last_crawl_error"
        class="mt-4 flex items-start gap-1.5 rounded-xl bg-red-50 px-4 py-3 text-[12.5px] leading-relaxed text-red-600"
      >
        <AppIcon name="alert" :size="15" class="mt-px shrink-0" />
        最近一次抓取失败原因：{{ company.last_crawl_error }}
      </p>

      <!-- 抓取设置 -->
      <section class="card mt-5 p-6">
        <div class="flex items-center justify-between gap-4">
          <div>
            <h2 class="text-[16px] font-semibold text-ink-900">抓取设置</h2>
            <p class="mt-1 text-[12.5px] text-ink-500">仅影响该公司，保存后从下一次抓取开始生效。</p>
          </div>
          <AppButton :loading="saving" @click="saveSettings">保存设置</AppButton>
        </div>

        <div class="mt-3 divide-y divide-ink-100">
          <div class="form-row">
            <span class="text-[13px] text-ink-600">抓取间隔（秒）</span>
            <div>
              <AppInput v-model="settings.crawl_interval_seconds" type="number" inputmode="numeric" />
              <p class="mt-1.5 text-[12.5px] text-ink-400">
                该公司的独立抓取周期，默认 21600 秒（6 小时）；定时任务按此间隔决定是否跳过。
              </p>
            </div>
          </div>

          <div class="form-row">
            <span class="text-[13px] text-ink-600">单次最大页数</span>
            <div>
              <AppInput v-model="settings.max_pages" type="number" inputmode="numeric" placeholder="0 = 使用全局默认" />
              <p class="mt-1.5 text-[12.5px] text-ink-400">
                写入 crawl_rules.max_pages；留 0 表示沿用配置文件的全局上限。保存时会合并
                原有的站点选择器，不会覆盖其它 crawl_rules 键。
              </p>
            </div>
          </div>

          <div class="form-row">
            <span class="text-[13px] text-ink-600">强制无头渲染</span>
            <div>
              <AppSwitch v-model="settings.force_headless" />
              <p class="mt-1.5 text-[12.5px] text-ink-400">
                列表由前端脚本渲染的官网需要打开它，代价是抓取速度明显变慢。
              </p>
            </div>
          </div>

          <div class="form-row">
            <span class="text-[13px] text-ink-600">遵守 robots.txt</span>
            <div>
              <AppSwitch v-model="settings.respect_robots" />
              <p class="mt-1.5 text-[12.5px] text-ink-400">
                默认开启。关闭会让抓取合规性失去保障，仅在确有把握时使用。
              </p>
            </div>
          </div>
        </div>

        <div v-if="rulesText" class="mt-5">
          <p class="mb-2 text-[12.5px] text-ink-500">
            站点定向规则（crawl_rules，只读展示：选择器需要熟悉目标站点结构后再改）
          </p>
          <pre
            class="max-h-[220px] overflow-auto rounded-xl bg-ink-50 px-3.5 py-3 font-mono text-[12px] leading-relaxed text-ink-700"
          >{{ rulesText }}</pre>
        </div>
      </section>

      <!-- 公司档案（CompanyView 的真实字段） -->
      <section class="card mt-5 p-6">
        <h2 class="text-[16px] font-semibold text-ink-900">公司档案</h2>
        <dl class="mt-4 grid gap-x-8 gap-y-4 text-[13px] sm:grid-cols-2 xl:grid-cols-3">
          <div>
            <dt class="text-ink-400">简称</dt>
            <dd class="mt-1 text-ink-800">{{ company.short_name || '—' }}</dd>
          </div>
          <div>
            <dt class="text-ink-400">二级行业</dt>
            <dd class="mt-1 text-ink-800">{{ company.sub_industry || '—' }}</dd>
          </div>
          <div>
            <dt class="text-ink-400">规模</dt>
            <dd class="mt-1 text-ink-800">{{ company.scale || '—' }}</dd>
          </div>
          <div>
            <dt class="text-ink-400">企业性质</dt>
            <dd class="mt-1 text-ink-800">{{ company.nature || '—' }}</dd>
          </div>
          <div>
            <dt class="text-ink-400">联系邮箱</dt>
            <dd class="mt-1 break-all text-ink-800">{{ company.contact_email || '—' }}</dd>
          </div>
          <div>
            <dt class="text-ink-400">联系电话</dt>
            <dd class="mt-1 text-ink-800">{{ company.contact_phone || '—' }}</dd>
          </div>
          <div>
            <dt class="text-ink-400">收录时间</dt>
            <dd class="tabular mt-1 text-ink-800">{{ formatDateTime(company.created_at) }}</dd>
          </div>
          <div>
            <dt class="text-ink-400">最后更新</dt>
            <dd class="tabular mt-1 text-ink-800">{{ formatDateTime(company.updated_at) }}</dd>
          </div>
        </dl>

        <p v-if="company.description" class="mt-4 text-[13px] leading-relaxed text-ink-600">
          {{ company.description }}
        </p>
        <p v-if="company.remark" class="mt-3 rounded-xl bg-page px-3.5 py-2.5 text-[12.5px] leading-relaxed text-ink-500">
          备注：{{ company.remark }}
        </p>
      </section>

      <!-- 该公司岗位（含未发布，便于核对审核链路） -->
      <section class="mt-3">
        <h2 class="mb-3 text-[16px] font-semibold text-ink-900">该公司岗位</h2>
        <AdminTable
          :columns="jobColumns"
          :empty="!jobs.length"
          empty-text="该公司暂无岗位"
          min-width="880px"
        >
          <tr v-for="row in jobs" :key="row.id">
            <td class="td-cell">
              <p class="font-medium text-ink-900">{{ row.title }}</p>
              <p class="mt-0.5 text-[12.5px] text-ink-400">{{ row.city || '地点未填' }}</p>
            </td>
            <td class="td-cell">
              <AppBadge :tone="jobStatus(row.status).tone">{{ jobStatus(row.status).label }}</AppBadge>
            </td>
            <td class="td-cell tabular whitespace-nowrap text-[13px] text-brand-700">
              {{ salaryText(row) }}
            </td>
            <td class="td-cell whitespace-nowrap text-[13px] text-ink-600">
              {{ relativeTime(row.last_updated_at) }}
            </td>
            <td class="td-cell text-right">
              <AppButton variant="ghost" size="sm" :to="{ name: 'posting', params: { id: row.id } }">
                详情
              </AppButton>
            </td>
          </tr>

          <template #footer>
            <p class="text-[12.5px] text-ink-400">
              共 {{ jobPage?.total ?? jobs.length }} 条，按最后更新时间倒序展示前 {{ PAGE_SIZE }} 条
            </p>
          </template>
        </AdminTable>
      </section>

      <!-- 抓取记录 -->
      <section class="mt-3">
        <h2 class="mb-3 text-[16px] font-semibold text-ink-900">最近抓取记录</h2>
        <AdminTable
          :columns="runColumns"
          :empty="!runs.length"
          empty-text="暂无抓取记录，可点击右上角“立即抓取”"
          min-width="820px"
        >
          <tr v-for="row in runs" :key="row.id">
            <td class="td-cell whitespace-nowrap text-[13px] text-ink-700">
              {{ TRIGGER_LABEL[row.trigger] ?? row.trigger ?? '—' }}
            </td>
            <td class="td-cell">
              <AppBadge :tone="crawlStatus(row.status).tone">{{ crawlStatus(row.status).label }}</AppBadge>
              <p v-if="row.skip_reason" class="mt-1 text-[12px] text-ink-400">
                跳过原因：{{ row.skip_reason }}
              </p>
              <p v-if="row.error_message" class="mt-1 max-w-[280px] truncate text-[12px] text-red-500">
                {{ row.error_message }}
              </p>
            </td>
            <td class="td-cell tabular whitespace-nowrap text-[13px] text-ink-700">
              {{ row.jobs_found ?? 0 }} / {{ row.jobs_created ?? 0 }} / {{ row.jobs_filtered ?? 0 }}
            </td>
            <td class="td-cell tabular whitespace-nowrap text-[13px] text-ink-600">
              {{ formatDuration(row.duration_ms) }}
            </td>
            <td class="td-cell tabular whitespace-nowrap text-[13px] text-ink-600">
              {{ formatDateTime(row.started_at) }}
            </td>
          </tr>

          <template #footer>
            <p class="text-[12.5px] text-ink-400">
              共 {{ runPage?.total ?? runs.length }} 条抓取运行记录，完整明细可前往「系统监控」查看
            </p>
          </template>
        </AdminTable>
      </section>
    </template>
  </div>
</template>
