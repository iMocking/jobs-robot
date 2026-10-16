<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'

import { adminApi } from '@/api'
import AdminTable from '@/components/AdminTable.vue'
import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppModal from '@/components/AppModal.vue'
import AppPagination from '@/components/AppPagination.vue'
import AppSelect from '@/components/AppSelect.vue'
import PageHeader from '@/components/PageHeader.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { useDictionaryStore } from '@/stores/dictionaries'
import { useToastStore } from '@/stores/toast'
import {
  JOB_STATUS,
  TONE_SOLID,
  companyInitial,
  formatDateTime,
  jobStatus,
  locationText,
  relativeTime,
  salaryText,
  sourceLabel,
  toneFor,
} from '@/utils/format'

/**
 * 岗位管理（后台岗位池的 CRUD 入口）。
 *
 * 两个刻意的设计决定：
 * 1. 筛选条件放在地址栏而不是组件内部状态 —— 运营排查「某公司的待审核岗位」时，
 *    经常要把链接直接丢给同事；地址栏是唯一能承载这份上下文的介质。
 * 2. 状态筛选做成一行多选 chip 而不是下拉 —— 状态是这一页最高频的筛选维度，
 *    摊开成一行省掉一次点击，且能一眼看到各状态还有多少条。
 */
const PAGE_SIZE = 10

const route = useRoute()
const router = useRouter()
const toast = useToastStore()
const dictionary = useDictionaryStore()
const { data: dictionaries } = storeToRefs(dictionary)
dictionary.load()

/* -------------------------------------------------------------------------- */
/* 筛选条件                                                                    */
/* -------------------------------------------------------------------------- */

const STATUS_KEYS = ['pending_review', 'approved', 'rejected', 'offline']
const STATUS_TABS = [
  { key: '', label: '全部', tone: 'brand' },
  ...STATUS_KEYS.map((key) => ({ key, ...JOB_STATUS[key] })),
]

/** 状态圆点：Tailwind 只扫描源码里的字面量类名，`bg-${tone}-500` 不会生成任何样式 */
const TONE_DOT = {
  brand: 'bg-brand-500',
  accent: 'bg-accent-500',
  amber: 'bg-amber-500',
  danger: 'bg-red-500',
  ink: 'bg-ink-400',
}

const statuses = computed(() => String(route.query.status ?? '').split(',').filter(Boolean))
const category = computed(() => String(route.query.category ?? ''))
const companyId = computed(() => String(route.query.company_id ?? ''))
const page = computed(() => Math.max(1, Number(route.query.page) || 1))

/** 关键词单独放本地 ref：输入框要即时反馈，地址栏只接收防抖后的结果 */
const keywordInput = ref(String(route.query.keyword ?? ''))

/** 当前展开的操作菜单所属岗位 id；声明在这里是为了让下面的路由 watch 能安全引用它 */
const menuFor = ref(null)

function buildQuery() {
  return {
    keyword: String(route.query.keyword ?? '') || undefined,
    status: statuses.value,
    category: category.value || undefined,
    company_id: companyId.value || undefined,
    order_by: 'last_updated_at',
    page: page.value,
    page_size: PAGE_SIZE,
  }
}

function pushQuery(query) {
  const cleaned = {}
  Object.entries(query).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') cleaned[key] = value
  })
  router.push({ name: route.name, query: cleaned })
}

/** 改筛选一律回到第 1 页，否则会停在一个「筛选后没有数据」的空页上 */
function patchQuery(patch) {
  pushQuery({ ...route.query, ...patch, page: undefined })
}

function toggleStatus(key) {
  if (!key) {
    patchQuery({ status: undefined })
    return
  }
  const next = statuses.value.includes(key)
    ? statuses.value.filter((item) => item !== key)
    : [...statuses.value, key]
  patchQuery({ status: next.join(',') || undefined })
}

function isActive(key) {
  return key ? statuses.value.includes(key) : statuses.value.length === 0
}

function resetFilters() {
  keywordInput.value = ''
  pushQuery({})
}

// 翻页不能复用 patchQuery：那个函数会把 page 一并清掉（改筛选时才需要回到第 1 页）
function changePage(next) {
  pushQuery({ ...route.query, page: next > 1 ? next : undefined })
}

let keywordTimer = null

function applyKeywordNow() {
  clearTimeout(keywordTimer)
  pushQuery({ keyword: keywordInput.value.trim() || undefined })
}

// 每敲一个字都发请求会打满后台的审计与慢查询；300ms 防抖 + 回车立即生效
watch(keywordInput, (value) => {
  clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => {
    if (value.trim() === String(route.query.keyword ?? '')) return
    pushQuery({ keyword: value.trim() || undefined })
  }, 300)
})

// 浏览器后退 / 别人发来的链接：把地址栏里的关键词同步回输入框
watch(
  () => route.query.keyword,
  (value) => {
    const next = String(value ?? '')
    if (next !== keywordInput.value) keywordInput.value = next
  },
)

/* -------------------------------------------------------------------------- */
/* 取数                                                                        */
/* -------------------------------------------------------------------------- */

const {
  data,
  loading,
  error,
  ready,
  run: runList,
} = useAsync(() => adminApi.jobs.list(buildQuery()), { immediate: false })

const { data: companyData, run: runCompanies } = useAsync(
  () => adminApi.companies.list({ page_size: 100 }),
  { immediate: false },
)

const companyOptions = computed(() =>
  (companyData.value?.items ?? []).map((company) => ({
    value: String(company.id),
    label: company.name,
  })),
)

/**
 * 字典字段名与后端 MetaView 对齐：
 * 职位类别 = job_types，城市 = locations（后端没有 categories / cities 这两个键）。
 */
const categoryOptions = computed(() => dictionaries.value.job_types ?? [])
const cityOptions = computed(() => (dictionaries.value.locations ?? []).filter((city) => city !== '全部'))

/**
 * 各状态的条数。
 * 用 page_size=1 只借接口返回的 total：这样数字与表格分页口径完全一致，
 * 而且不受当前筛选影响；单个状态取数失败只让那一个 chip 不显示数字，不影响整排筛选。
 */
const counts = ref({})
let countSeq = 0

async function loadCounts() {
  const seq = (countSeq += 1)
  const entries = await Promise.all(
    STATUS_KEYS.map(async (key) => {
      try {
        const result = await adminApi.jobs.list({ status: key, page_size: 1 })
        return [key, result.total]
      } catch {
        return [key, null]
      }
    }),
  )
  if (seq === countSeq) counts.value = Object.fromEntries(entries)
}

function load() {
  runList()
  loadCounts()
  runCompanies()
}

// 挂载与顶栏「刷新」都走这里；useAsync 用 immediate:false 避免进来就取两次
useAdminRefresh(load)

watch(
  () => route.query,
  () => {
    menuFor.value = null
    runList()
  },
  { deep: true },
)

/* -------------------------------------------------------------------------- */
/* 质量分                                                                      */
/* -------------------------------------------------------------------------- */

/** 质量分是「这条要不要人工重点看」的第一信号：>=80 可信、60-79 需留意、<60 建议复核 */
const QUALITY_BAR = { accent: 'bg-accent-500', amber: 'bg-amber-500', danger: 'bg-red-500' }
const QUALITY_TEXT = { accent: 'text-accent-700', amber: 'text-amber-600', danger: 'text-red-600' }

function qualityTone(score) {
  const value = Number(score) || 0
  if (value >= 80) return 'accent'
  if (value >= 60) return 'amber'
  return 'danger'
}

function barWidth(score) {
  return Math.max(4, Math.min(100, Number(score) || 0))
}

function statusOf(job) {
  return jobStatus(job.status)
}

function canReview(job) {
  return job.status === 'pending_review' || job.status === 'rejected'
}

/** 只有已下线的岗位能直接上线；待审核 / 已驳回必须走审核页，否则等于绕过审核 */
function canPublish(job) {
  return job.status === 'offline' || job.status === 'expired'
}

function canOffline(job) {
  return job.status === 'approved'
}

/* -------------------------------------------------------------------------- */
/* 行内操作                                                                    */
/* -------------------------------------------------------------------------- */

function toggleMenu(id) {
  menuFor.value = menuFor.value === id ? null : id
}

// 点击空白处收起操作菜单；菜单内的点击各自 stop，避免「刚点开就被关掉」
function closeMenu() {
  menuFor.value = null
}

onMounted(() => document.addEventListener('click', closeMenu))
onBeforeUnmount(() => document.removeEventListener('click', closeMenu))

const pending = ref(null) // { job, kind: 'offline' | 'remove' }
const confirmOpen = ref(false)
const working = ref(false)

const confirmCopy = computed(() => {
  const job = pending.value?.job
  if (pending.value?.kind === 'remove') {
    return {
      title: '删除岗位',
      description: `《${job?.title ?? ''}》及其变更历史会被永久删除，无法恢复。`,
      confirmText: '确认删除',
    }
  }
  return {
    title: '下线岗位',
    description: `《${job?.title ?? ''}》会从公开列表中移除，求职者将搜不到它。之后可以在岗位详情页重新上线。`,
    confirmText: '确认下线',
  }
})

function askOffline(job) {
  menuFor.value = null
  pending.value = { job, kind: 'offline' }
  confirmOpen.value = true
}

function askRemove(job) {
  menuFor.value = null
  pending.value = { job, kind: 'remove' }
  confirmOpen.value = true
}

async function submitConfirm() {
  const target = pending.value
  if (!target) return
  working.value = true
  try {
    if (target.kind === 'remove') {
      await adminApi.jobs.remove(target.job.id)
      toast.success(`已删除《${target.job.title}》`)
    } else {
      await adminApi.jobs.setStatus(target.job.id, 'offline')
      toast.success(`已下线《${target.job.title}》`)
    }
    confirmOpen.value = false
    pending.value = null

    // 删除的是当前页最后一条时要往前翻一页，否则会停在一个空列表上
    const isLastRow = (data.value?.items?.length ?? 0) === 1 && page.value > 1
    if (target.kind === 'remove' && isLastRow) changePage(page.value - 1)
    else runList()
    loadCounts()
  } catch (err) {
    toast.fromError(err)
  } finally {
    working.value = false
  }
}

async function publishNow(job) {
  menuFor.value = null
  try {
    await adminApi.jobs.setStatus(job.id, 'approved')
    toast.success(`《${job.title}》已上线`)
    runList()
    loadCounts()
  } catch (err) {
    toast.fromError(err)
  }
}

/* -------------------------------------------------------------------------- */
/* 全量抓取                                                                    */
/* -------------------------------------------------------------------------- */

const crawling = ref(false)

async function runAll() {
  crawling.value = true
  try {
    // 响应体是 { runs, count }，没有 message 字段；直接用 count 报出实际提交了几家
    const result = await adminApi.crawl.runAll()
    const count = result?.count ?? result?.runs?.length ?? 0
    toast.success(count ? `已提交 ${count} 家公司的抓取任务` : '全量抓取已触发，当前没有可抓取的公司')
    runList()
  } catch (err) {
    toast.fromError(err, '全量抓取提交失败')
  } finally {
    crawling.value = false
  }
}

/* -------------------------------------------------------------------------- */
/* 新增岗位                                                                    */
/* -------------------------------------------------------------------------- */

const createOpen = ref(false)
const creating = ref(false)
const createError = ref('')

const createForm = reactive({
  title: '',
  company_id: '',
  source_url: '',
  category: '',
  city: '',
  salary_min: '',
  salary_max: '',
  description: '',
})

function openCreate() {
  createError.value = ''
  createOpen.value = true
}

function resetCreateForm() {
  createForm.title = ''
  createForm.company_id = ''
  createForm.source_url = ''
  createForm.category = ''
  createForm.city = ''
  createForm.salary_min = ''
  createForm.salary_max = ''
  createForm.description = ''
}

async function submitCreate() {
  createError.value = ''
  if (!createForm.title.trim()) {
    createError.value = '请填写岗位名称'
    return
  }
  if (!createForm.company_id) {
    createError.value = '请选择岗位所属公司（岗位必须挂在某个官网来源下）'
    return
  }
  // source_url 是后端必填且唯一：它既是溯源证据，也是「同一岗位不重复入库」的判重键，
  // 缺了它接口会直接返回 10001，所以这里先拦一次。
  const sourceUrl = createForm.source_url.trim()
  if (!sourceUrl) {
    createError.value = '请填写岗位来源地址（官网《加入我们》页面上的原始链接）'
    return
  }
  if (!/^https?:\/\//i.test(sourceUrl)) {
    createError.value = '来源地址需以 http:// 或 https:// 开头'
    return
  }
  const min = createForm.salary_min === '' ? null : Number(createForm.salary_min)
  const max = createForm.salary_max === '' ? null : Number(createForm.salary_max)
  if (min !== null && max !== null && min > max) {
    createError.value = '月薪下限不能高于月薪上限'
    return
  }

  creating.value = true
  try {
    const created = await adminApi.jobs.create({
      title: createForm.title.trim(),
      // company_id 是 ULID 字符串，直接透传：转 Number 会得到 NaN，
      // 后端会以「必须指定所属公司」拒绝，导致手工建岗位必然失败。
      company_id: String(createForm.company_id),
      source_url: sourceUrl,
      category: createForm.category || undefined,
      city: createForm.city || undefined,
      salary_min: min ?? undefined,
      salary_max: max ?? undefined,
      description: createForm.description.trim() || undefined,
    })
    createOpen.value = false
    const title = created?.title ?? createForm.title.trim()
    resetCreateForm()
    toast.success(`《${title}》已提交，等待审核`)
    // 新岗位一定落在待审核：当前筛选里看不到它时给一句解释，避免管理员以为没提交成功
    if (statuses.value.length && !statuses.value.includes('pending_review')) {
      toast.info('当前筛选不含「待审核」状态，切换后即可看到这条新岗位')
    }
    runList()
    loadCounts()
  } catch (err) {
    // 30006 表示同一来源链接已存在：这不是「操作失败」，而是可以直接跳去改那条记录
    if (err?.code === 30006) {
      createError.value = '该来源地址已存在同名岗位（岗位以来源链接判重），请到列表中直接编辑'
    } else {
      createError.value = err?.message ?? '新增失败，请稍后重试'
    }
    toast.fromError(err)
  } finally {
    creating.value = false
  }
}

/* -------------------------------------------------------------------------- */
/* 模板                                                                        */
/* -------------------------------------------------------------------------- */

const columns = [
  { key: 'job', label: '岗位' },
  { key: 'status', label: '状态' },
  { key: 'salary', label: '薪资' },
  { key: 'source', label: '来源' },
  { key: 'published', label: '发布时间' },
  { key: 'quality', label: '质量分' },
  { key: 'actions', label: '操作', class: 'text-right' },
]

/** 文本域没有公共组件，这里统一一份样式，避免三个页面各写一遍 */
const TEXTAREA_CLASS =
  'w-full rounded-xl border border-ink-200 bg-white px-3.5 py-2.5 text-sm leading-relaxed text-ink-900 transition placeholder:text-ink-400 focus:border-brand-400 focus:ring-2 focus:ring-brand-500/12 focus:outline-none'
</script>

<template>
  <div>
    <PageHeader
      title="岗位管理"
      description="不选状态即返回全部状态，便于一次看清整个岗位池；筛选条件会写进地址栏，可直接把链接发给同事复现同一份结果。"
    >
      <template #actions>
        <AppButton variant="secondary" :loading="crawling" @click="runAll">
          <AppIcon name="bolt" :size="16" />
          全量抓取
        </AppButton>
        <AppButton @click="openCreate">
          <AppIcon name="plus" :size="16" />
          新增岗位
        </AppButton>
      </template>
    </PageHeader>

    <!-- 筛选：状态一行摊开，关键词 / 类别 / 公司收在下面一行 -->
    <section class="card mb-4 p-4">
      <div class="flex flex-wrap items-center gap-2">
        <button
          v-for="tab in STATUS_TABS"
          :key="tab.key || 'all'"
          type="button"
          class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[13px] font-medium transition"
          :class="
            isActive(tab.key)
              ? `border-transparent ${TONE_SOLID[tab.tone]}`
              : 'border-ink-200 bg-white text-ink-600 hover:border-brand-300 hover:text-brand-700'
          "
          :aria-pressed="isActive(tab.key)"
          @click="toggleStatus(tab.key)"
        >
          <span
            class="size-1.5 rounded-full"
            :class="isActive(tab.key) ? 'bg-white/75' : TONE_DOT[tab.tone]"
          />
          {{ tab.label }}
          <span
            v-if="tab.key && typeof counts[tab.key] === 'number'"
            class="tabular text-[12px] opacity-80"
          >
            {{ counts[tab.key] }}
          </span>
        </button>
      </div>

      <div class="mt-4 grid gap-3 border-t border-ink-100 pt-4 lg:grid-cols-[minmax(0,1fr)_200px_220px]">
        <AppInput
          v-model="keywordInput"
          icon="search"
          placeholder="搜索岗位名称 / 公司"
          @enter="applyKeywordNow"
        />
        <AppSelect
          :model-value="category"
          placeholder="全部职位类别"
          :options="categoryOptions"
          @update:model-value="(value) => patchQuery({ category: value })"
        />
        <AppSelect
          :model-value="companyId"
          placeholder="全部公司"
          :options="companyOptions"
          @update:model-value="(value) => patchQuery({ company_id: value })"
        />
      </div>
    </section>

    <!-- 首次加载：骨架屏，避免「空表格闪一下再出数据」 -->
    <div v-if="loading && !ready" class="card h-[520px] animate-pulse" />

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="岗位列表加载失败"
      :description="error.message"
    >
      <div class="flex gap-3">
        <AppButton @click="load()">重新加载</AppButton>
        <AppButton variant="secondary" @click="resetFilters">清空筛选</AppButton>
      </div>
    </AppEmpty>

    <AdminTable
      v-else
      :columns="columns"
      :empty="!data?.items?.length"
      empty-text="没有匹配的岗位"
      min-width="1080px"
    >
      <tr v-for="job in data?.items ?? []" :key="job.id" class="transition hover:bg-ink-50/60">
        <td class="td-cell">
          <div class="flex items-center gap-3">
            <AppAvatar
              size="sm"
              :name="job.company?.name"
              :initial="companyInitial(job.company)"
              :tone="toneFor(job.company?.id ?? job.company?.name)"
            />
            <div class="min-w-0">
              <RouterLink
                :to="{ name: 'posting', params: { id: job.id } }"
                class="block max-w-[300px] truncate font-semibold text-ink-900 transition hover:text-brand-700"
              >
                {{ job.title }}
              </RouterLink>
              <p class="mt-0.5 flex items-center gap-1.5 text-[12.5px] text-ink-500">
                <span class="max-w-[140px] truncate">{{ job.company?.name }}</span>
                <span class="text-ink-300">·</span>
                <span class="max-w-[180px] truncate">{{ locationText(job) || '地点待定' }}</span>
              </p>
            </div>
          </div>
        </td>

        <td class="td-cell">
          <AppBadge :tone="statusOf(job).tone">{{ statusOf(job).label }}</AppBadge>
        </td>

        <td class="td-cell">
          <span class="tabular font-medium text-brand-700">{{ salaryText(job) }}</span>
        </td>

        <td class="td-cell">
          <p class="flex items-center gap-1.5 text-[13px] text-ink-600">
            <AppIcon name="globe" :size="14" class="shrink-0 text-ink-400" />
            <span class="max-w-[180px] truncate">{{ sourceLabel(job.company) }}</span>
          </p>
        </td>

        <td class="td-cell">
          <span class="text-[13px] text-ink-600" :title="formatDateTime(job.published_at)">
            {{ relativeTime(job.published_at) }}
          </span>
        </td>

        <td class="td-cell">
          <div class="flex items-center gap-2">
            <span
              class="tabular text-[13px] font-semibold"
              :class="QUALITY_TEXT[qualityTone(job.quality_score)]"
            >
              {{ job.quality_score }}
            </span>
            <span class="h-1.5 w-14 overflow-hidden rounded-full bg-ink-100">
              <span
                class="block h-full rounded-full"
                :class="QUALITY_BAR[qualityTone(job.quality_score)]"
                :style="{ width: `${barWidth(job.quality_score)}%` }"
              />
            </span>
          </div>
        </td>

        <td class="td-cell">
          <div class="flex items-center justify-end gap-2">
            <AppButton
              size="sm"
              variant="secondary"
              :to="{ name: 'posting', params: { id: job.id } }"
            >
              详情
            </AppButton>
            <AppButton v-if="canReview(job)" size="sm" :to="{ name: 'review' }">审核</AppButton>

            <div class="relative">
              <button
                type="button"
                class="flex size-8 items-center justify-center rounded-lg border border-ink-200 bg-white text-ink-500 transition hover:border-brand-300 hover:text-brand-600"
                aria-label="更多操作"
                :aria-expanded="menuFor === job.id"
                @click.stop="toggleMenu(job.id)"
              >
                <AppIcon name="dots" :size="16" />
              </button>

              <div
                v-if="menuFor === job.id"
                class="absolute right-0 z-30 mt-1 w-28 overflow-hidden rounded-xl border border-ink-200 bg-white py-1 shadow-panel"
                @click.stop
              >
                <button
                  v-if="canPublish(job)"
                  type="button"
                  class="flex w-full items-center px-3 py-2 text-left text-[13px] text-ink-700 transition hover:bg-ink-50"
                  @click="publishNow(job)"
                >
                  上线
                </button>
                <button
                  v-if="canOffline(job)"
                  type="button"
                  class="flex w-full items-center px-3 py-2 text-left text-[13px] text-ink-700 transition hover:bg-ink-50"
                  @click="askOffline(job)"
                >
                  下线
                </button>
                <button
                  type="button"
                  class="flex w-full items-center px-3 py-2 text-left text-[13px] text-red-600 transition hover:bg-red-50"
                  @click="askRemove(job)"
                >
                  删除
                </button>
              </div>
            </div>
          </div>
        </td>
      </tr>

      <template #footer>
        <AppPagination
          :page="data?.page ?? 1"
          :pages="data?.pages ?? 1"
          :total="data?.total ?? 0"
          :page-size="data?.page_size ?? 10"
          @update:page="changePage"
        />
      </template>
    </AdminTable>

    <!-- 下线 / 删除确认：不可逆的动作一律先过一层弹窗 -->
    <AppModal
      v-model="confirmOpen"
      :title="confirmCopy.title"
      :description="confirmCopy.description"
      size="sm"
    >
      <p class="text-[13px] leading-relaxed text-ink-500">
        该操作会写入审计日志，可在「审计日志」中按岗位 ID 追溯操作人与时间。
      </p>
      <template #footer>
        <AppButton variant="secondary" @click="confirmOpen = false">取消</AppButton>
        <AppButton
          :variant="pending?.kind === 'remove' ? 'danger' : 'primary'"
          :loading="working"
          @click="submitConfirm"
        >
          {{ confirmCopy.confirmText }}
        </AppButton>
      </template>
    </AppModal>

    <!-- 新增岗位 -->
    <AppModal
      v-model="createOpen"
      title="新增岗位"
      description="手工补录官网《加入我们》页面上的岗位；提交后与抓取路径走同一条审核链路。"
      size="lg"
    >
      <div class="space-y-4">
        <AppInput v-model="createForm.title" label="岗位名称" placeholder="如：高级前端开发工程师（React）" />

        <AppInput
          v-model="createForm.source_url"
          label="来源地址"
          placeholder="https://www.example.com/jobs/backend.html"
          hint="必填：官网《加入我们》页面上的原始链接，既是溯源证据，也是岗位判重键。"
        />

        <div class="grid gap-4 sm:grid-cols-2">
          <AppSelect
            v-model="createForm.company_id"
            label="公司"
            placeholder="请选择来源公司"
            :options="companyOptions"
          />
          <AppSelect
            v-model="createForm.category"
            label="职位类别"
            placeholder="请选择职位类别"
            :options="categoryOptions"
          />
        </div>

        <div class="grid gap-4 sm:grid-cols-3">
          <AppSelect
            v-model="createForm.city"
            label="城市"
            placeholder="请选择城市"
            :options="cityOptions"
          />
          <AppInput
            v-model="createForm.salary_min"
            label="月薪下限（元）"
            type="number"
            inputmode="numeric"
            placeholder="25000"
          />
          <AppInput
            v-model="createForm.salary_max"
            label="月薪上限（元）"
            type="number"
            inputmode="numeric"
            placeholder="40000"
          />
        </div>

        <div>
          <label class="mb-1.5 block text-[13px] font-medium text-ink-700">岗位介绍</label>
          <textarea
            v-model="createForm.description"
            rows="4"
            :class="TEXTAREA_CLASS"
            placeholder="建议直接摘录官网原文，方便审核时逐字比对"
          />
        </div>

        <p
          class="flex items-start gap-1.5 rounded-xl bg-brand-50 px-3.5 py-2.5 text-[12.5px] leading-relaxed text-brand-700"
        >
          <AppIcon name="info" :size="15" class="mt-px shrink-0" />
          手工新增的岗位默认进入待审核，与抓取路径口径一致：只有审核通过后才会出现在前台列表，
          因此提交后不会立刻在「已发布」里看到它，需要在「审核队列」里再点一次通过。
        </p>

        <p v-if="createError" class="flex items-center gap-1.5 text-[12.5px] text-red-600">
          <AppIcon name="alert" :size="14" />
          {{ createError }}
        </p>
      </div>

      <template #footer>
        <AppButton variant="secondary" @click="createOpen = false">取消</AppButton>
        <AppButton :loading="creating" @click="submitCreate">提交并进入待审核</AppButton>
      </template>
    </AppModal>
  </div>
</template>
