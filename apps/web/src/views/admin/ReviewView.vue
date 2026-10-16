<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { adminApi } from '@/api'
import AdminTable from '@/components/AdminTable.vue'
import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppModal from '@/components/AppModal.vue'
import AppPagination from '@/components/AppPagination.vue'
import PageHeader from '@/components/PageHeader.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { useToastStore } from '@/stores/toast'
import {
  companyInitial,
  formatDateTime,
  jobStatus,
  locationText,
  relativeTime,
  sourceLabel,
  toneFor,
} from '@/utils/format'

/**
 * 审核队列：后台唯一「必须有人做判断」的页面。
 *
 * 两个决定：
 * 1. 只收待审核 + 已驳回两种状态，按 last_updated_at 排序 —— 队列要的是
 *    「最先变化的排在最前面」，而不是「谁发布得晚」，否则积压的旧岗位会被一直压到底部。
 * 2. 顶部两个计数用独立的 page_size=1 请求取 total，而不是拿当前页长度相加：
 *    分页之后只有接口给的 total 才代表真实积压量。
 */
const PAGE_SIZE = 10
const QUEUE_STATUS = 'pending_review,rejected'

const route = useRoute()
const router = useRouter()
const toast = useToastStore()

const page = computed(() => Math.max(1, Number(route.query.page) || 1))

const columns = [
  { key: 'job', label: '岗位' },
  { key: 'source', label: '来源' },
  { key: 'quality', label: '信息完整度' },
  { key: 'status', label: '状态' },
  { key: 'actions', label: '操作', class: 'text-right' },
]

const {
  data,
  loading,
  error,
  ready,
  run: runList,
} = useAsync(
  () =>
    adminApi.jobs.list({
      status: QUEUE_STATUS,
      order_by: 'last_updated_at',
      page: page.value,
      page_size: PAGE_SIZE,
    }),
  { immediate: false },
)

/* -------------------------------------------------------------------------- */
/* 计数                                                                        */
/* -------------------------------------------------------------------------- */

const counts = reactive({ pending: 0, rejected: 0 })

async function loadCounts() {
  const [pending, rejected] = await Promise.all([
    adminApi.jobs.list({ status: 'pending_review', page_size: 1 }).catch(() => null),
    adminApi.jobs.list({ status: 'rejected', page_size: 1 }).catch(() => null),
  ])
  counts.pending = pending?.total ?? 0
  counts.rejected = rejected?.total ?? 0
}

function reload() {
  runList()
  loadCounts()
}

// 挂载 + 顶栏「刷新」都走这里；列表用 immediate:false，避免进来就取两次
useAdminRefresh(reload)

watch(page, () => runList())

function changePage(next) {
  router.push({ name: route.name, query: next > 1 ? { page: next } : {} })
}

/* -------------------------------------------------------------------------- */
/* 审核动作                                                                    */
/* -------------------------------------------------------------------------- */

/** 质量分分级：>=80 可信、60-79 需留意、<60 建议逐字比对官网原文 */
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

const reviewingId = ref(null)

async function approve(job) {
  reviewingId.value = job.id
  try {
    await adminApi.jobs.review(job.id, { decision: 'approve' })
    toast.success(`已通过《${job.title}》，岗位已发布`)
    reload()
  } catch (err) {
    toast.fromError(err)
    // 30004：这条岗位已经被别人审过或状态已变。列表与计数都会过期，
    // 必须重新拉一次，否则操作员会对着一条早就审完的记录反复点击。
    if (err?.code === 30004) reload()
  } finally {
    reviewingId.value = null
  }
}

const rejectOpen = ref(false)
const rejectTarget = ref(null)
const rejectReason = ref('')
const rejectError = ref('')
const rejecting = ref(false)

/** 常见原因一键填入：审核是重复劳动，统一话术也方便来源方照着改 */
const REJECT_SUGGESTIONS = [
  '岗位信息与官网不一致',
  '缺少任职要求',
  '薪资与实际不符',
  '疑似外包/中介岗位',
]

function openReject(job) {
  rejectTarget.value = job
  rejectReason.value = ''
  rejectError.value = ''
  rejectOpen.value = true
}

function fillReason(text) {
  rejectReason.value = rejectReason.value ? `${rejectReason.value}；${text}` : text
  rejectError.value = ''
}

async function submitReject() {
  const reason = rejectReason.value.trim()
  // 后端对空原因返回 10001「驳回必须填写原因」；先在前端拦一次，省掉一次必然失败的请求
  if (!reason) {
    rejectError.value = '请填写驳回原因，理由会写入审计日志'
    return
  }
  rejecting.value = true
  try {
    await adminApi.jobs.review(rejectTarget.value.id, {
      decision: 'reject',
      reject_reason: reason,
    })
    toast.success(`已驳回《${rejectTarget.value.title}》`)
    rejectOpen.value = false
    reload()
  } catch (err) {
    if (err?.code === 30004) {
      // 状态已变，弹窗里的信息已经过期，关掉并刷新比让操作员继续改理由更合理
      rejectOpen.value = false
      toast.fromError(err)
      reload()
    } else {
      rejectError.value = err?.message ?? '驳回失败，请稍后重试'
    }
  } finally {
    rejecting.value = false
  }
}

const TEXTAREA_CLASS =
  'w-full rounded-xl border border-ink-200 bg-white px-3.5 py-2.5 text-sm leading-relaxed text-ink-900 transition placeholder:text-ink-400 focus:border-brand-400 focus:ring-2 focus:ring-brand-500/12 focus:outline-none'
</script>

<template>
  <div>
    <PageHeader
      title="审核队列"
      description="只显示待审核与已驳回的岗位：通过或驳回都会写入审计日志，可回溯到具体操作人与时间。"
    />

    <!-- 队列规模：分页不影响这两个数字 -->
    <div class="mb-4 grid gap-4 sm:grid-cols-2">
      <div class="card flex items-center gap-3.5 p-4">
        <span
          class="flex size-10 shrink-0 items-center justify-center rounded-xl bg-amber-50 text-amber-600"
        >
          <AppIcon name="clipboard" :size="19" />
        </span>
        <div>
          <p class="text-[12.5px] text-ink-500">待审核</p>
          <p class="tabular text-[20px] leading-tight font-semibold text-ink-900">
            {{ counts.pending }}
          </p>
        </div>
        <span class="ml-auto text-[12px] text-ink-400">等待人工判断</span>
      </div>

      <div class="card flex items-center gap-3.5 p-4">
        <span class="flex size-10 shrink-0 items-center justify-center rounded-xl bg-red-50 text-red-600">
          <AppIcon name="alert" :size="19" />
        </span>
        <div>
          <p class="text-[12.5px] text-ink-500">已驳回</p>
          <p class="tabular text-[20px] leading-tight font-semibold text-ink-900">
            {{ counts.rejected }}
          </p>
        </div>
        <span class="ml-auto text-[12px] text-ink-400">等待来源方修正</span>
      </div>
    </div>

    <div v-if="loading && !ready" class="card h-[480px] animate-pulse" />

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="审核队列加载失败"
      :description="error.message"
    >
      <AppButton @click="reload()">重新加载</AppButton>
    </AppEmpty>

    <!--
      空态没有交给 AdminTable 的 empty 插槽：那里的空态只接受一句标题，
      而队列清空恰恰需要一句解释和一个「去岗位管理」的出口。
    -->
    <AppEmpty
      v-else-if="!data?.items?.length"
      icon="check-circle"
      title="审核队列已清空"
      description="当前没有待审核或已驳回的岗位。抓取任务发现新岗位后会自动出现在这里；如果只是想调整某条已发布岗位的状态，请前往岗位管理。"
    >
      <AppButton variant="secondary" :to="{ name: 'postings' }">前往岗位管理</AppButton>
    </AppEmpty>

    <AdminTable v-else :columns="columns" min-width="1020px">
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
                class="block max-w-[280px] truncate font-semibold text-ink-900 transition hover:text-brand-700"
              >
                {{ job.title }}
              </RouterLink>
              <p class="mt-0.5 text-[12.5px] text-ink-500">
                {{ job.company?.name }}
                <span class="text-ink-300">·</span>
                {{ locationText(job) || '地点待定' }}
              </p>
            </div>
          </div>
        </td>

        <td class="td-cell">
          <p class="flex items-center gap-1.5 text-[13px] text-ink-600">
            <AppIcon name="globe" :size="14" class="shrink-0 text-ink-400" />
            <span class="max-w-[190px] truncate">{{ sourceLabel(job.company) }}</span>
          </p>
          <p class="mt-1 text-[12px] text-ink-400" :title="formatDateTime(job.published_at)">
            官网发布 {{ relativeTime(job.published_at) }}
          </p>
        </td>

        <td class="td-cell">
          <div class="flex items-center gap-2.5">
            <span
              class="tabular text-[13px] font-semibold"
              :class="QUALITY_TEXT[qualityTone(job.quality_score)]"
            >
              {{ job.quality_score }}
            </span>
            <span class="h-1.5 w-20 overflow-hidden rounded-full bg-ink-100">
              <span
                class="block h-full rounded-full"
                :class="QUALITY_BAR[qualityTone(job.quality_score)]"
                :style="{ width: `${barWidth(job.quality_score)}%` }"
              />
            </span>
          </div>
          <p
            v-if="qualityTone(job.quality_score) === 'danger'"
            class="mt-1 text-[12px] text-red-600"
          >
            低分岗位需重点核对
          </p>
        </td>

        <td class="td-cell">
          <AppBadge :tone="statusOf(job).tone">{{ statusOf(job).label }}</AppBadge>
          <!--
            列表用的是 JobListView，它没有 reject_reason（只有详情视图带这个字段）。
            这里不编一个假原因，而是把「去哪里看原因」说清楚。
          -->
          <p v-if="job.status === 'rejected'" class="mt-1 text-[12px] text-red-500">
            上一轮驳回原因见详情页
          </p>
        </td>

        <td class="td-cell">
          <div class="flex items-center justify-end gap-2">
            <AppButton size="sm" :loading="reviewingId === job.id" @click="approve(job)">
              通过
            </AppButton>
            <AppButton size="sm" variant="danger" @click="openReject(job)">驳回</AppButton>
            <AppButton
              size="sm"
              variant="secondary"
              :to="{ name: 'posting', params: { id: job.id } }"
            >
              详情
            </AppButton>
          </div>
        </td>
      </tr>

      <template #footer>
        <AppPagination
          :page="data?.page ?? 1"
          :pages="data?.pages ?? 1"
          :total="data?.total ?? 0"
          :page-size="data?.page_size ?? PAGE_SIZE"
          @update:page="changePage"
        />
      </template>
    </AdminTable>

    <!-- 驳回：原因必填，后端缺原因会返回 10001 -->
    <AppModal
      v-model="rejectOpen"
      title="驳回岗位"
      :description="rejectTarget ? `《${rejectTarget.title}》` : ''"
      size="md"
    >
      <div class="space-y-4">
        <div>
          <label class="mb-1.5 block text-[13px] font-medium text-ink-700">
            驳回原因 <span class="text-red-500">*</span>
          </label>
          <textarea
            v-model="rejectReason"
            rows="3"
            :class="TEXTAREA_CLASS"
            placeholder="例如：岗位信息与官网《加入我们》页面不一致"
            @input="rejectError = ''"
          />
          <p v-if="rejectError" class="mt-1.5 flex items-center gap-1 text-[12.5px] text-red-600">
            <AppIcon name="alert" :size="14" />
            {{ rejectError }}
          </p>
        </div>

        <div>
          <p class="mb-2 text-[12.5px] text-ink-500">常见原因（点击填入，可继续补充）</p>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="item in REJECT_SUGGESTIONS"
              :key="item"
              type="button"
              class="rounded-full border border-ink-200 bg-white px-3 py-1.5 text-[12.5px] text-ink-600 transition hover:border-brand-300 hover:text-brand-700"
              @click="fillReason(item)"
            >
              {{ item }}
            </button>
          </div>
        </div>

        <p
          class="flex items-start gap-1.5 rounded-xl bg-ink-50 px-3.5 py-2.5 text-[12px] leading-relaxed text-ink-500"
        >
          <AppIcon name="info" :size="14" class="mt-px shrink-0" />
          原因必填：它会写入审计日志，并随岗位状态一起保留，供后续复核与来源方修正时对照。
        </p>
      </div>

      <template #footer>
        <AppButton variant="secondary" @click="rejectOpen = false">取消</AppButton>
        <AppButton variant="danger" :loading="rejecting" @click="submitReject">确认驳回</AppButton>
      </template>
    </AppModal>
  </div>
</template>
