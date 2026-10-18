<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute } from 'vue-router'

import { adminApi } from '@/api'
import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppModal from '@/components/AppModal.vue'
import AppSelect from '@/components/AppSelect.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { useDictionaryStore } from '@/stores/dictionaries'
import { useToastStore } from '@/stores/toast'
import {
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
 * 岗位详情：左边改内容，右边看来源。
 *
 * 后台的岗位不是「平台自己发布的」，而是从企业官网《加入我们》抓来的，
 * 所以这一页必须同时回答两个问题：怎么修（编辑区）和凭什么信（溯源区 + 变更历史）。
 * 两栏固定比例而不是响应式收缩，是因为编辑区字段多达 18 个，太窄会严重影响录入效率。
 */
const route = useRoute()
const toast = useToastStore()
const dictionary = useDictionaryStore()
const { data: dictionaries } = storeToRefs(dictionary)
dictionary.load()

const jobId = computed(() => route.params.id)

const {
  data: job,
  loading,
  error,
  ready,
  run: runJob,
} = useAsync(() => adminApi.jobs.detail(jobId.value), { immediate: false })

const statusOf = computed(() => jobStatus(job.value?.status))
const headerTags = computed(() =>
  [job.value?.employment_type, job.value?.experience, job.value?.education].filter(Boolean),
)
const revisions = computed(() => job.value?.revisions ?? [])

/* -------------------------------------------------------------------------- */
/* 编辑表单                                                                    */
/* -------------------------------------------------------------------------- */

const FORM_FIELDS = [
  'title',
  'category',
  'employment_type',
  'city',
  'district',
  'address',
  'salary_min',
  'salary_max',
  'salary_months',
  'experience',
  'education',
  'description',
  'responsibilities',
  'requirements',
  'benefits',
  'contact_email',
  'contact_phone',
  'apply_url',
]
const NUMERIC_FIELDS = ['salary_min', 'salary_max', 'salary_months']

const form = reactive({})
const baseline = ref({})

function text(value) {
  return value === null || value === undefined ? '' : String(value)
}

/** 任职要求在接口里是数组，编辑区按「一行一条」呈现 —— 比顿号串好改，也不容易改丢分号 */
function requirementsToText(source) {
  if (source.requirements_list?.length) return source.requirements_list.join('\n')
  return String(source.requirements || '')
    .split(/[；;\n]/)
    .map((line) => line.trim())
    .filter(Boolean)
    .join('\n')
}

function formFromJob(source) {
  const next = {}
  FORM_FIELDS.forEach((field) => {
    next[field] = field === 'requirements' ? requirementsToText(source) : text(source[field])
  })
  return next
}

/** 表单值与接口值都转成字符串再存一份基线：输入框里的 '25000' 与接口的 25000 视为未改动 */
function syncForm(source) {
  const next = formFromJob(source)
  Object.assign(form, next)
  baseline.value = next
}

async function load() {
  const result = await runJob()
  if (result) syncForm(result)
}

useAdminRefresh(load)
watch(jobId, () => load())

/**
 * 只提交改动过的字段。
 * 后端把「实质性变更」当作退回待审核的依据，全量提交会让没改过的字段也被记为一次变更，
 * 已发布的岗位就会被莫名其妙地打回审核。
 *
 * 数值字段留空时**跳过而不是发 null**：PATCH 的入参是指针，JSON null 绑定成 nil 等于
 * 「不修改该字段」，发 null 只会让运营以为自己清掉了薪资，实际什么也没发生。
 */
function buildPatch() {
  const patch = {}
  FORM_FIELDS.forEach((field) => {
    const value = form[field]
    if (value === baseline.value[field]) return
    if (NUMERIC_FIELDS.includes(field)) {
      if (value === '') return
      patch[field] = Number(value)
      return
    }
    patch[field] = String(value).trim()
  })

  if ('requirements' in patch) {
    const list = patch.requirements
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
    patch.requirements_list = list
    // requirements 是前台仍在读的顿号串字段，保存时同步写回，避免两处口径不一致
    patch.requirements = list.join('；')
  }

  return patch
}

const dirty = computed(() => Object.keys(buildPatch()).length > 0)
const saving = ref(false)
const saveError = ref('')

async function save() {
  saveError.value = ''
  if (!form.title.trim()) {
    saveError.value = '岗位名称不能为空'
    return
  }
  const min = form.salary_min === '' ? null : Number(form.salary_min)
  const max = form.salary_max === '' ? null : Number(form.salary_max)
  if (min !== null && max !== null && min > max) {
    saveError.value = '月薪下限不能高于月薪上限'
    return
  }

  const patch = buildPatch()
  if (!Object.keys(patch).length) {
    toast.info('没有检测到改动')
    return
  }

  saving.value = true
  try {
    await adminApi.jobs.update(jobId.value, patch)
    toast.success('岗位信息已保存')
    // 保存会刷新 last_updated_at 并新增一条版本记录，重新取一次才看得到
    await load()
  } catch (err) {
    saveError.value = err?.message ?? '保存失败，请稍后重试'
    toast.fromError(err)
  } finally {
    saving.value = false
  }
}

/** 字典里没有的历史值也要留在选项里，否则下拉显示为空，一保存就把原值悄悄改掉了 */
function withCurrent(options, current) {
  const list = options ?? []
  if (!current || list.includes(current)) return list
  return [current, ...list]
}

/* -------------------------------------------------------------------------- */
/* 审核与上下线                                                                */
/* -------------------------------------------------------------------------- */

const canReview = computed(
  () => job.value?.status === 'pending_review' || job.value?.status === 'rejected',
)
const canPublish = computed(
  () => job.value?.status === 'offline' || job.value?.status === 'expired',
)

const approving = ref(false)

async function approve() {
  approving.value = true
  try {
    await adminApi.jobs.review(jobId.value, { decision: 'approve' })
    toast.success('审核通过，岗位已发布')
    await load()
  } catch (err) {
    toast.fromError(err)
  } finally {
    approving.value = false
  }
}

const rejectOpen = ref(false)
const rejectReason = ref('')
const rejectError = ref('')
const rejecting = ref(false)

function openReject() {
  rejectReason.value = job.value?.reject_reason ?? ''
  rejectError.value = ''
  rejectOpen.value = true
}

async function submitReject() {
  const reason = rejectReason.value.trim()
  // 后端对空原因返回 10001「驳回必须填写原因」；先在前端拦一次，省掉一次必然失败的请求
  if (!reason) {
    rejectError.value = '驳回必须填写原因，理由会写入审计日志'
    return
  }
  rejecting.value = true
  try {
    await adminApi.jobs.review(jobId.value, { decision: 'reject', reject_reason: reason })
    rejectOpen.value = false
    toast.success('已驳回，原因已写入审计日志')
    await load()
  } catch (err) {
    rejectError.value = err?.message ?? '驳回失败，请稍后重试'
  } finally {
    rejecting.value = false
  }
}

const offlineOpen = ref(false)
const working = ref(false)

const STATUS_DONE = {
  approved: '已上线，前台可以搜索到该岗位',
  offline: '已下线，前台不再展示该岗位',
  expired: '已标记过期，前台不再展示该岗位',
}

async function applyStatus(next) {
  working.value = true
  try {
    await adminApi.jobs.setStatus(jobId.value, next)
    toast.success(STATUS_DONE[next] ?? '状态已更新')
    offlineOpen.value = false
    await load()
  } catch (err) {
    toast.fromError(err)
  } finally {
    working.value = false
  }
}

/* -------------------------------------------------------------------------- */
/* 变更历史展示                                                                */
/* -------------------------------------------------------------------------- */

/**
 * TRIGGER_LABEL（定时 / 手动）是抓取记录的口径，变更历史的 trigger 是 crawl / manual，
 * 两者含义不同，复用会让时间线上出现「定时」这种对不上的文案，因此单独定义。
 *
 * 注意：RevisionView 里**没有** actor 字段（后端只存 trigger / changed_fields / snapshot），
 * 所以时间线不显示「谁改的」；要追操作人去「审计日志」按 job.update + 资源 ID 过滤。
 */
const REVISION_TRIGGER = { crawl: '抓取', manual: '人工' }

const FIELD_LABELS = {
  title: '岗位名称',
  category: '职位类别',
  employment_type: '用工性质',
  city: '城市',
  district: '区县',
  address: '工作地址',
  salary_min: '月薪下限',
  salary_max: '月薪上限',
  salary_months: '薪资月数',
  salary_text: '薪资文案',
  experience: '经验要求',
  education: '学历要求',
  description: '岗位介绍',
  responsibilities: '岗位职责',
  requirements: '任职要求',
  benefits: '福利待遇',
  contact_email: '联系邮箱',
  contact_phone: '联系电话',
  apply_url: '官网投递链接',
  source_url: '来源链接',
  quality_score: '质量分',
  __created__: '首次收录',
}

const QUALITY_BAR = { accent: 'bg-accent-500', amber: 'bg-amber-500', danger: 'bg-red-500' }

function qualityTone(score) {
  const value = Number(score) || 0
  if (value >= 80) return 'accent'
  if (value >= 60) return 'amber'
  return 'danger'
}

const TEXTAREA_CLASS =
  'w-full rounded-xl border border-ink-200 bg-white px-3.5 py-2.5 text-sm leading-relaxed text-ink-900 transition placeholder:text-ink-400 focus:border-brand-400 focus:ring-2 focus:ring-brand-500/12 focus:outline-none'

/** 文本域行：与 .form-row 共用同一套栅格，但标签顶端对齐（多行输入时垂直居中会显得很别扭） */
const TEXTAREA_ROW = 'grid gap-4 py-3.5 sm:grid-cols-[160px_1fr]'
</script>

<template>
  <div>
    <RouterLink
      :to="{ name: 'postings' }"
      class="mb-4 inline-flex items-center gap-1.5 text-[13px] text-ink-500 transition hover:text-brand-700"
    >
      <AppIcon name="arrow-left" :size="15" />
      返回岗位管理
    </RouterLink>

    <!-- 首次加载骨架 -->
    <div v-if="loading && !ready" class="space-y-2.5">
      <div class="card h-[190px] animate-pulse" />
      <div class="grid gap-2.5 xl:grid-cols-[minmax(0,1fr)_340px]">
        <div class="card h-[640px] animate-pulse" />
        <div class="card h-[380px] animate-pulse" />
      </div>
    </div>

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="岗位不存在或已被删除"
      :description="error.message"
    >
      <div class="flex gap-3">
        <AppButton variant="secondary" :to="{ name: 'postings' }">返回岗位管理</AppButton>
        <AppButton @click="load()">重新加载</AppButton>
      </div>
    </AppEmpty>

    <template v-else-if="job">
      <!-- 头部：身份 + 状态 + 动作，一屏之内决定这条岗位接下来怎么处理 -->
      <section class="card mb-2.5 p-6">
        <div class="flex flex-wrap items-start gap-5">
          <AppAvatar
            size="lg"
            :name="job.company?.name"
            :initial="companyInitial(job.company)"
            :tone="toneFor(job.company?.id ?? job.company?.name)"
          />

          <div class="min-w-0 flex-1">
            <h1 class="text-[22px] leading-snug font-semibold text-ink-900">{{ job.title }}</h1>

            <p class="mt-1.5 flex flex-wrap items-center gap-1.5 text-[13.5px] text-ink-600">
              <span>{{ job.company?.name }}</span>
              <span
                v-if="job.company?.verified"
                class="text-brand-500"
                title="已核实企业官网《加入我们》页面"
              >
                <AppIcon name="check-circle" :size="15" />
              </span>
              <span class="text-ink-300">·</span>
              <span>{{ locationText(job) || job.address || '地点待定' }}</span>
            </p>

            <div class="mt-3 flex flex-wrap items-center gap-2">
              <AppBadge :tone="statusOf.tone">{{ statusOf.label }}</AppBadge>
              <AppBadge v-if="job.is_direct_hire" tone="accent">官网直招</AppBadge>
              <AppBadge v-for="tag in headerTags" :key="tag" tone="ink">{{ tag }}</AppBadge>
            </div>
          </div>

          <div class="shrink-0 text-right">
            <p class="tabular text-[20px] font-semibold text-brand-700">{{ salaryText(job) }}</p>
            <p class="mt-1.5 text-[12.5px] text-ink-400">
              {{ formatDateTime(job.published_at) }} 发布
            </p>
          </div>
        </div>

        <div class="mt-5 flex flex-wrap items-center gap-2.5 border-t border-ink-100 pt-5">
          <AppButton v-if="canReview" :loading="approving" @click="approve">
            <AppIcon name="check" :size="16" />
            审核通过
          </AppButton>
          <AppButton v-if="canReview" variant="danger" @click="openReject">
            <AppIcon name="x" :size="16" />
            驳回
          </AppButton>
          <AppButton v-if="canPublish" variant="secondary" @click="applyStatus('approved')">
            <AppIcon name="check-circle" :size="16" />
            上线
          </AppButton>
          <AppButton v-if="job.status === 'approved'" variant="secondary" @click="offlineOpen = true">
            <AppIcon name="lock" :size="16" />
            下线
          </AppButton>
          <AppButton
            v-if="job.status === 'approved'"
            variant="ghost"
            @click="applyStatus('expired')"
          >
            <AppIcon name="clock" :size="16" />
            标记过期
          </AppButton>

          <!-- 已发布的岗位没有「审核」按钮：后端会以 30004 拒绝重复审核，摆一个必然失败的按钮只会误导操作员 -->
          <p
            v-if="job.status === 'approved'"
            class="flex flex-1 items-start gap-1.5 text-[12.5px] leading-relaxed text-ink-500"
          >
            <AppIcon name="info" :size="15" class="mt-px shrink-0 text-ink-400" />
            已发布的岗位不能重复审核（会被后端以 30004 拒绝）。如需改动内容，请先「下线」，
            编辑保存后会自动退回待审核，再走一次人工审核。
          </p>
          <p
            v-else-if="job.status === 'offline' || job.status === 'expired'"
            class="flex flex-1 items-start gap-1.5 text-[12.5px] leading-relaxed text-ink-500"
          >
            <AppIcon name="info" :size="15" class="mt-px shrink-0 text-ink-400" />
            该岗位当前不在前台展示，点「上线」可立即恢复发布。
          </p>
        </div>

        <p
          v-if="job.reject_reason"
          class="mt-4 flex items-start gap-1.5 rounded-xl bg-red-50 px-3.5 py-2.5 text-[12.5px] leading-relaxed text-red-600"
        >
          <AppIcon name="alert" :size="15" class="mt-px shrink-0" />
          驳回原因：{{ job.reject_reason }}
        </p>
      </section>

      <div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
        <!-- 左：编辑区 -->
        <section class="card p-6">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 class="text-[16px] font-semibold text-ink-900">岗位信息编辑</h2>
              <p class="mt-1 text-[12.5px] text-ink-400">
                只提交改动过的字段，未改动的内容不会产生新的版本记录。
              </p>
            </div>
            <AppButton :loading="saving" :disabled="!dirty" @click="save">保存修改</AppButton>
          </div>

          <p
            class="mt-4 flex items-start gap-1.5 rounded-xl bg-warn-50 px-3.5 py-2.5 text-[12.5px] leading-relaxed text-warn-600"
          >
            <AppIcon name="info" :size="15" class="mt-px shrink-0" />
            实质性变更（标题 / 任职要求 / 职责 / 薪资 / 用工性质 / 城市 / 地址）会让已发布的岗位自动退回待审核：
            与官网原文不一致的改动必须重新过一遍人工，这是「官网直招」可信度的底线。
          </p>

          <p v-if="saveError" class="mt-3 flex items-center gap-1.5 text-[12.5px] text-red-600">
            <AppIcon name="alert" :size="14" />
            {{ saveError }}
          </p>

          <div class="mt-3 divide-y divide-ink-100">
            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">岗位名称</label>
              <AppInput v-model="form.title" placeholder="岗位名称" />
            </div>

            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">职位类别</label>
              <AppSelect
                v-model="form.category"
                placeholder="请选择职位类别"
                :options="withCurrent(dictionaries.job_types, form.category)"
              />
            </div>

            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">用工性质</label>
              <AppSelect
                v-model="form.employment_type"
                placeholder="请选择用工性质"
                :options="withCurrent(dictionaries.employment_types, form.employment_type)"
              />
            </div>

            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">城市</label>
              <AppInput v-model="form.city" placeholder="如：北京" />
            </div>

            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">区县</label>
              <AppInput v-model="form.district" placeholder="如：海淀区" />
            </div>

            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">工作地址</label>
              <AppInput v-model="form.address" placeholder="与官网《加入我们》页面保持一致" />
            </div>

            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">月薪（元 / 月）</label>
              <div class="grid gap-3 sm:grid-cols-3">
                <AppInput
                  v-model="form.salary_min"
                  type="number"
                  inputmode="numeric"
                  placeholder="下限"
                />
                <AppInput
                  v-model="form.salary_max"
                  type="number"
                  inputmode="numeric"
                  placeholder="上限"
                />
                <AppInput
                  v-model="form.salary_months"
                  type="number"
                  inputmode="numeric"
                  placeholder="薪资月数"
                />
              </div>
              <p class="mt-1.5 text-[12px] text-ink-400">
                留空的原值是清不掉的：PATCH 里 null 的语义是「不修改该字段」，
                所以留空的数值字段不会提交，保存后仍保持原值。
              </p>
            </div>

            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">经验要求</label>
              <AppInput v-model="form.experience" placeholder="如：3-5年 / 不限" />
            </div>

            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">学历要求</label>
              <AppInput v-model="form.education" placeholder="如：本科及以上" />
            </div>

            <div :class="TEXTAREA_ROW">
              <label class="text-[13px] font-medium text-ink-700">岗位介绍</label>
              <textarea v-model="form.description" rows="4" :class="TEXTAREA_CLASS" />
            </div>

            <div :class="TEXTAREA_ROW">
              <label class="text-[13px] font-medium text-ink-700">岗位职责</label>
              <textarea v-model="form.responsibilities" rows="5" :class="TEXTAREA_CLASS" />
            </div>

            <div :class="TEXTAREA_ROW">
              <label class="text-[13px] font-medium text-ink-700">任职要求</label>
              <div>
                <textarea v-model="form.requirements" rows="6" :class="TEXTAREA_CLASS" />
                <p class="mt-1.5 text-[12.5px] text-ink-400">
                  一行一条；保存时会写回 requirements_list 数组，并同步顿号串字段。
                </p>
              </div>
            </div>

            <div :class="TEXTAREA_ROW">
              <label class="text-[13px] font-medium text-ink-700">福利待遇</label>
              <textarea v-model="form.benefits" rows="3" :class="TEXTAREA_CLASS" />
            </div>

            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">联系邮箱</label>
              <AppInput v-model="form.contact_email" placeholder="hr@example.com" />
            </div>

            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">联系电话</label>
              <AppInput v-model="form.contact_phone" placeholder="010-8888-8888" />
            </div>

            <div class="form-row">
              <label class="text-[13px] font-medium text-ink-700">官网投递链接</label>
              <AppInput v-model="form.apply_url" placeholder="https://…" />
            </div>
          </div>
        </section>

        <!-- 右：溯源 + 变更历史 -->
        <aside class="space-y-2.5">
          <section class="card p-5">
            <h2 class="text-[15px] font-semibold text-ink-900">溯源信息</h2>
            <p class="mt-1 text-[12.5px] leading-relaxed text-ink-400">
              这一屏回答「这条岗位从哪来、可不可信」。
            </p>

            <dl class="mt-4 space-y-4 text-[13px]">
              <div>
                <dt class="text-ink-500">来源站点</dt>
                <dd class="mt-1 font-medium text-ink-800">{{ job.source_site || '—' }}</dd>
              </div>

              <div>
                <dt class="text-ink-500">来源页面</dt>
                <dd class="mt-1">
                  <a
                    v-if="job.source_url"
                    :href="job.source_url"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="inline-flex items-start gap-1 break-all text-accent-700 transition hover:underline"
                  >
                    {{ job.source_url }}
                    <AppIcon name="external-link" :size="13" class="mt-0.5 shrink-0" />
                  </a>
                  <span v-else class="text-ink-400">未记录</span>
                </dd>
              </div>

              <div>
                <dt class="text-ink-500">直招标识</dt>
                <dd class="mt-1.5">
                  <AppBadge :tone="job.is_direct_hire ? 'accent' : 'ink'">
                    {{ job.is_direct_hire ? '官网直招' : '来源待核实' }}
                  </AppBadge>
                </dd>
              </div>

              <div>
                <dt class="text-ink-500">质量分</dt>
                <dd class="mt-1.5 flex items-center gap-2.5">
                  <span class="tabular text-[13.5px] font-semibold text-ink-800">
                    {{ job.quality_score }}
                  </span>
                  <span class="h-1.5 w-24 overflow-hidden rounded-full bg-ink-100">
                    <span
                      class="block h-full rounded-full"
                      :class="QUALITY_BAR[qualityTone(job.quality_score)]"
                      :style="{ width: `${Math.min(100, Number(job.quality_score) || 0)}%` }"
                    />
                  </span>
                </dd>
              </div>

              <div v-if="job.parse_confidence !== undefined && job.parse_confidence !== null">
                <dt class="text-ink-500">解析置信度</dt>
                <dd class="tabular mt-1 text-ink-800">
                  {{ Math.round(Number(job.parse_confidence) * 100) }}%
                </dd>
              </div>
            </dl>

            <div class="mt-5 space-y-2.5 border-t border-ink-100 pt-4 text-[12.5px]">
              <p class="flex items-center justify-between gap-2">
                <span class="text-ink-500">发布时间</span>
                <span class="text-ink-800">{{ formatDateTime(job.published_at) }}</span>
              </p>
              <p class="flex items-center justify-between gap-2">
                <span class="text-ink-500">最后更新</span>
                <span class="text-ink-800">{{ formatDateTime(job.last_updated_at) }}</span>
              </p>
              <p class="flex items-center justify-between gap-2">
                <span class="text-ink-500">最后核查</span>
                <span class="text-ink-800">
                  {{ relativeTime(job.last_seen_at) }}
                  <span class="text-ink-400">（{{ formatDateTime(job.last_seen_at) }}）</span>
                </span>
              </p>
            </div>

            <p class="mt-3 rounded-xl bg-ink-50 px-3.5 py-2.5 text-[12px] leading-relaxed text-ink-500">
              三个时间口径不同：发布 = 官网挂出该岗位的时间；最后更新 = 抓取到内容变化的最近一次；
              最后核查 = 最近一次确认官网上仍然挂着它（用于判断是否已经撤下）。
            </p>

            <p class="mt-3 text-[12.5px] leading-relaxed text-ink-500">
              来源：{{ sourceLabel(job.company) }}
            </p>
          </section>

          <section class="card p-5">
            <h2 class="text-[15px] font-semibold text-ink-900">变更历史</h2>
            <p class="mt-1 text-[12.5px] text-ink-400">共 {{ revisions.length }} 条版本记录。</p>

            <ol v-if="revisions.length" class="mt-4">
              <li
                v-for="(item, index) in revisions"
                :key="`${item.revision}-${item.created_at}`"
                class="relative pb-5 pl-5 last:pb-0"
              >
                <span
                  class="absolute top-1.5 left-0 size-2.5 rounded-full ring-1 ring-brand-200"
                  :class="index === 0 ? 'bg-brand-600' : 'bg-ink-300'"
                />
                <span
                  v-if="index !== revisions.length - 1"
                  class="absolute top-4 bottom-0 left-[4.5px] w-px bg-ink-200"
                />

                <div class="flex flex-wrap items-center gap-2">
                  <AppBadge size="sm" :tone="item.trigger === 'manual' ? 'brand' : 'ink'">
                    {{ REVISION_TRIGGER[item.trigger] ?? item.trigger }}
                  </AppBadge>
                  <span class="tabular text-[12px] text-ink-500">v{{ item.revision }}</span>
                  <span class="text-[12px] text-ink-400">{{ formatDateTime(item.created_at) }}</span>
                </div>

                <div class="mt-2 flex flex-wrap gap-1.5">
                  <AppBadge
                    v-for="field in item.changed_fields ?? []"
                    :key="field"
                    size="sm"
                    :tone="field === '__created__' ? 'accent' : 'ink'"
                  >
                    {{ FIELD_LABELS[field] ?? field }}
                  </AppBadge>
                  <span v-if="!item.changed_fields?.length" class="text-[12px] text-ink-400">
                    未记录变更字段
                  </span>
                </div>
              </li>
            </ol>

            <p v-else class="mt-4 text-[13px] text-ink-400">暂无变更记录。</p>
          </section>
        </aside>
      </div>
    </template>

    <!-- 驳回：原因必填，后端缺原因会返回 10001 -->
    <AppModal
      v-model="rejectOpen"
      title="驳回岗位"
      :description="job ? `《${job.title}》` : ''"
      size="md"
    >
      <div>
        <label class="mb-1.5 block text-[13px] font-medium text-ink-700">
          驳回原因 <span class="text-red-500">*</span>
        </label>
        <textarea
          v-model="rejectReason"
          rows="4"
          :class="TEXTAREA_CLASS"
          placeholder="例如：岗位信息与官网《加入我们》页面不一致，缺少任职要求"
          @input="rejectError = ''"
        />
        <p v-if="rejectError" class="mt-1.5 flex items-center gap-1 text-[12.5px] text-red-600">
          <AppIcon name="alert" :size="14" />
          {{ rejectError }}
        </p>
        <p v-else class="mt-1.5 text-[12.5px] text-ink-400">
          原因会写入审计日志，并随岗位状态一起保留，供后续复核与来源方沟通使用。
        </p>
      </div>

      <template #footer>
        <AppButton variant="secondary" @click="rejectOpen = false">取消</AppButton>
        <AppButton variant="danger" :loading="rejecting" @click="submitReject">确认驳回</AppButton>
      </template>
    </AppModal>

    <!-- 下线：会让岗位从前台消失，属于不可逆感知较强的动作，先确认 -->
    <AppModal
      v-model="offlineOpen"
      title="下线岗位"
      description="下线后该岗位会从公开列表中移除，求职者将搜不到它；可在本页重新上线。"
      size="sm"
    >
      <p class="text-[13px] leading-relaxed text-ink-500">
        该操作会写入审计日志。如果只是因为内容有误，建议直接修改并保存，让岗位退回待审核。
      </p>
      <template #footer>
        <AppButton variant="secondary" @click="offlineOpen = false">取消</AppButton>
        <AppButton :loading="working" @click="applyStatus('offline')">确认下线</AppButton>
      </template>
    </AppModal>
  </div>
</template>
