<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import { jobApi } from '@/api'
import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import JobCard from '@/components/JobCard.vue'
import { useAsync } from '@/composables/useAsync'
import {
  companyInitial,
  formatDateTime,
  jobTags,
  locationText,
  relativeTime,
  salaryText,
  sourceLabel,
  toneFor,
} from '@/utils/format'

/**
 * 职位详情 = 设计稿第 2 屏。
 *
 * 消费者点名的 6 类信息一个都不能少：
 *   岗位介绍 / 任职要求（主栏）、工作时间线 / 联系方式 / 工作地址（信息卡）、
 *   以及「来源：XX官网《加入我们》」+ 官网投递入口（绿色卡）。
 * 绿卡单独成块是刻意的：它是这个产品与普通招聘聚合站的区别所在。
 *
 * 后端契约要点（见 internal/service/view.go 的 JobDetailView）：
 *   - company 是 CompanyBriefView，没有 tone/logo/initial，头像靠前端派生；
 *   - apply_url 可空（omitempty），许多官网来源页并不提供独立投递链接；
 *   - reject_reason 虽然存在于 DTO，但公开接口只返回 approved 岗位，
 *     它是管理端的审核痕迹，不应出现在求职者页面。
 */
const route = useRoute()
const jobId = computed(() => route.params.id)

const { data: job, loading, error, ready, run: loadJob } = useAsync(() => jobApi.detail(jobId.value))
const { data: related, run: loadRelated } = useAsync(() => jobApi.related(jobId.value, 3))

// 从「相关职位」点进另一个岗位时组件会被复用，必须跟着 id 重新取数
watch(jobId, () => {
  loadJob()
  loadRelated()
})

const saved = ref(false)

/** 公司头像：首字与配色都由前端按 id 稳定派生 */
const companyAvatar = computed(() => ({
  initial: companyInitial(job.value?.company),
  tone: toneFor(job.value?.company?.id ?? job.value?.company?.name),
}))

/** 标签组（用工性质 + 经验 + 学历）走 format.js，与列表卡片的写法保持一致 */
const tags = computed(() => {
  const education = job.value?.education
  return education ? [...jobTags(job.value), education] : jobTags(job.value)
})

/** 信息卡里的工作地址：后端可能给了完整地址，退回到「城市 · 区」 */
const addressText = computed(() => {
  const current = job.value
  if (!current) return '未提供'
  return current.address || current.location_text || locationText(current) || '未提供'
})

/**
 * 投递入口的降级链：apply_url → contact_email(mailto) → source_url。
 *
 * 为什么必须降级：apply_url 是 omitempty 的可空字段，来源页不提供独立投递链接时它就是空，
 * 此时若照旧渲染「申请该职位」会得到一个 href 为空的死按钮——
 * 对求职者来说比没有按钮更糟。而企业官网招聘页与 HR 邮箱都是真实可用的投递通道。
 */
const applyHref = computed(() => {
  const current = job.value
  if (!current) return ''
  if (current.apply_url) return current.apply_url
  if (current.contact_email) return `mailto:${current.contact_email}`
  return current.source_url || ''
})

/** 降级到邮箱/来源页时按钮文案要如实说明去向，不能一律写「申请该职位」 */
const applyLabel = computed(() => {
  const current = job.value
  if (!current) return '申请该职位'
  if (current.apply_url) return '申请该职位'
  if (current.contact_email) return '发送简历至 HR 邮箱'
  if (current.source_url) return '前往官方来源页投递'
  return ''
})

/**
 * 可溯源信息：本品类的核心卖点就是「这个岗位是从哪抓来的、解析得多准、改过几版」。
 * parse_confidence 后端是 0~1 的小数，这里转成百分比；extractor / revision 为空则不渲染。
 */
const provenance = computed(() => {
  const current = job.value
  if (!current) return []
  const items = []
  if (current.extractor) items.push({ label: '解析方式', value: current.extractor })
  if (current.parse_confidence > 0) {
    items.push({ label: '解析置信度', value: `${Math.round(current.parse_confidence * 100)}%` })
  }
  if (current.revision > 0) items.push({ label: '内容版本', value: `第 ${current.revision} 版` })
  if (current.crawled_at) {
    items.push({ label: '入库时间', value: formatDateTime(current.crawled_at) })
  }
  if (current.source_updated_at) {
    items.push({ label: '来源页更新', value: formatDateTime(current.source_updated_at) })
  }
  return items
})

/** 截止日期只在该日期还没过去时提示，过期岗位本身已经不会出现在公开列表里 */
const deadlineText = computed(() => (job.value?.deadline_at ? formatDateTime(job.value.deadline_at) : ''))
</script>

<template>
  <div class="container-page py-6">
    <nav class="mb-5 flex items-center gap-2 text-[13px] text-ink-400">
      <RouterLink :to="{ name: 'home' }" class="transition hover:text-brand-600">首页</RouterLink>
      <span>/</span>
      <RouterLink :to="{ name: 'jobs' }" class="transition hover:text-brand-600">职位</RouterLink>
      <span>/</span>
      <span class="max-w-[420px] truncate text-ink-600">{{ job?.title ?? '职位详情' }}</span>
    </nav>

    <!-- 首次加载骨架 -->
    <div v-if="loading && !ready" class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_340px]">
      <div class="card h-[420px] animate-pulse" />
      <div class="card h-[300px] animate-pulse" />
    </div>

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="职位不存在或已下架"
      :description="error.message"
    >
      <div class="flex gap-3">
        <AppButton variant="secondary" :to="{ name: 'jobs' }">浏览其它职位</AppButton>
        <AppButton @click="loadJob()">重新加载</AppButton>
      </div>
    </AppEmpty>

    <template v-else-if="job">
      <div class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_340px]">
        <div class="min-w-0 space-y-5">
          <!-- 职位头部 -->
          <section class="card p-6">
            <div class="flex items-start gap-4">
              <AppAvatar
                :name="job.company?.name"
                :initial="companyAvatar.initial"
                :tone="companyAvatar.tone"
                size="lg"
              />

              <div class="min-w-0 flex-1">
                <p class="flex items-center gap-1.5 text-[14px] text-ink-600">
                  <RouterLink
                    :to="{ name: 'company', params: { id: job.company?.id } }"
                    class="transition hover:text-brand-700"
                  >
                    {{ job.company?.name }}
                  </RouterLink>
                  <span
                    v-if="job.company?.verified"
                    class="text-brand-500"
                    title="已核实企业官网《加入我们》页面"
                  >
                    <AppIcon name="check-circle" :size="16" />
                  </span>
                </p>

                <h1 class="mt-2 text-[28px] leading-snug font-bold text-ink-900 lg:text-[30px]">
                  {{ job.title }}
                </h1>

                <div class="mt-3 flex flex-wrap items-center gap-2">
                  <AppBadge v-if="job.is_direct_hire" tone="accent">官网直招</AppBadge>
                  <AppBadge v-for="tag in tags" :key="tag" tone="ink">{{ tag }}</AppBadge>
                  <span v-if="locationText(job)" class="text-[12.5px] text-ink-400">
                    {{ locationText(job) }}
                  </span>
                </div>
              </div>

              <div class="shrink-0 text-right">
                <p class="tabular text-[22px] font-semibold text-brand-700">{{ salaryText(job) }}</p>
                <!-- published_at 是可空指针，relativeTime 对 null 返回占位符而不是 Invalid Date -->
                <p class="mt-1.5 text-[12.5px] text-ink-400">
                  {{ relativeTime(job.published_at) }}发布
                </p>
              </div>
            </div>
          </section>

          <!-- 岗位介绍 -->
          <section class="card p-6">
            <h2 class="text-[18px] font-semibold text-ink-900">岗位介绍</h2>
            <div class="mt-3.5 space-y-3 text-[14px] leading-[1.95] text-ink-600">
              <p v-if="job.description">{{ job.description }}</p>
              <p v-if="job.responsibilities">{{ job.responsibilities }}</p>
              <p v-if="!job.description && !job.responsibilities" class="text-ink-400">
                该岗位暂未提供详细介绍，可前往企业官网查看完整信息。
              </p>
            </div>

            <div v-if="job.benefits" class="mt-5 border-t border-ink-100 pt-4">
              <h3 class="text-[14px] font-medium text-ink-800">福利待遇</h3>
              <p class="mt-2 text-[14px] leading-[1.95] text-ink-600">{{ job.benefits }}</p>
            </div>
          </section>

          <!-- 任职要求 -->
          <section class="card p-6">
            <h2 class="text-[18px] font-semibold text-ink-900">任职要求</h2>
            <ul v-if="job.requirements_list?.length" class="mt-3.5 space-y-2.5">
              <li
                v-for="item in job.requirements_list"
                :key="item"
                class="flex gap-2.5 text-[14px] leading-relaxed text-ink-600"
              >
                <span class="mt-[9px] size-1.5 shrink-0 rounded-full bg-brand-500" />
                <span>{{ item }}</span>
              </li>
            </ul>
            <p v-else class="mt-3.5 text-[14px] text-ink-500">{{ job.requirements || '暂无' }}</p>

            <!-- 行动区：投递出去才是这个页面的终点 -->
            <div class="mt-7 flex flex-wrap items-center gap-3 border-t border-ink-100 pt-5">
              <!-- applyHref 已把 apply_url 为空的情况降级到邮箱 / 来源页，不会出现空 href -->
              <AppButton v-if="applyHref" :href="applyHref" size="lg" class="px-9">
                {{ applyLabel }}
              </AppButton>
              <AppButton v-else size="lg" class="px-9" @click="saved = !saved">
                留下求职意向
              </AppButton>

              <AppButton
                variant="secondary"
                size="lg"
                class="ml-auto px-6"
                :aria-pressed="saved"
                @click="saved = !saved"
              >
                <AppIcon name="bookmark" :size="17" :class="saved ? 'text-brand-600' : ''" />
                {{ saved ? '已收藏' : '收藏' }}
              </AppButton>
            </div>
          </section>
        </div>

        <!-- 右侧信息卡 -->
        <aside class="space-y-5 lg:sticky lg:top-24">
          <section class="card p-6">
            <h2 class="mb-5 text-[16px] font-semibold text-ink-900">职位信息</h2>

            <dl class="space-y-5">
              <div>
                <dt class="flex items-center gap-2 text-[13px] text-ink-500">
                  <AppIcon name="map-pin" :size="15" class="text-ink-400" />
                  工作地址
                </dt>
                <dd class="mt-1.5 pl-[23px] text-[13.5px] leading-relaxed text-ink-800">
                  {{ addressText }}
                </dd>
              </div>

              <div v-if="job.department || job.headcount || job.work_mode || job.job_code">
                <dt class="flex items-center gap-2 text-[13px] text-ink-500">
                  <AppIcon name="briefcase" :size="15" class="text-ink-400" />
                  职位属性
                </dt>
                <dd class="mt-1.5 space-y-1 pl-[23px] text-[13.5px] text-ink-800">
                  <p v-if="job.department">部门：{{ job.department }}</p>
                  <p v-if="job.headcount">招聘人数：{{ job.headcount }} 人</p>
                  <p v-if="job.work_mode">工作方式：{{ job.work_mode }}</p>
                  <p v-if="job.job_code">职位编号：{{ job.job_code }}</p>
                </dd>
              </div>

              <div>
                <dt class="flex items-center gap-2 text-[13px] text-ink-500">
                  <AppIcon name="mail" :size="15" class="text-ink-400" />
                  联系方式
                </dt>
                <dd class="mt-1.5 space-y-1 pl-[23px] text-[13.5px] text-ink-800">
                  <p v-if="job.contact_person">联系人：{{ job.contact_person }}</p>
                  <p v-if="job.contact_email">{{ job.contact_email }}</p>
                  <p v-if="job.contact_phone">{{ job.contact_phone }}</p>
                  <p v-if="job.contact_wechat">微信：{{ job.contact_wechat }}</p>
                  <p v-if="job.apply_method">投递方式：{{ job.apply_method }}</p>
                  <p
                    v-if="
                      !job.contact_person &&
                      !job.contact_email &&
                      !job.contact_phone &&
                      !job.contact_wechat
                    "
                    class="text-ink-400"
                  >
                    该企业未在来源页公开联系方式
                  </p>
                </dd>
              </div>

              <div>
                <dt class="flex items-center gap-2 text-[13px] text-ink-500">
                  <AppIcon name="clock" :size="15" class="text-ink-400" />
                  最后更新时间
                </dt>
                <dd class="mt-1.5 pl-[23px] text-[13.5px] text-ink-800">
                  {{ relativeTime(job.last_updated_at) }}
                  <span class="ml-1 text-[12px] text-ink-400">
                    （{{ formatDateTime(job.last_updated_at) }}）
                  </span>
                </dd>
              </div>

              <div v-if="deadlineText">
                <dt class="flex items-center gap-2 text-[13px] text-ink-500">
                  <AppIcon name="calendar" :size="15" class="text-ink-400" />
                  投递截止
                </dt>
                <dd class="mt-1.5 pl-[23px] text-[13.5px] text-ink-800">{{ deadlineText }}</dd>
              </div>
            </dl>
          </section>

          <!-- 溯源卡：绿色在本产品里只代表「官网直招」 -->
          <section class="rounded-card border border-accent-200 bg-accent-50 p-6">
            <AppBadge tone="accent">官网直招</AppBadge>

            <p class="mt-3.5 text-[13.5px] leading-relaxed text-ink-700">
              来源：{{ sourceLabel(job.company) }}
            </p>
            <p v-if="job.source_company_name && job.source_company_name !== job.company?.name" class="mt-1 text-[12.5px] text-ink-500">
              来源页署名：{{ job.source_company_name }}
            </p>

            <a
              v-if="job.source_url"
              :href="job.source_url"
              target="_blank"
              rel="noopener noreferrer"
              class="mt-1.5 inline-flex items-center gap-1 text-[12.5px] text-accent-700 transition hover:underline"
            >
              查看来源页面
              <AppIcon name="external-link" :size="13" />
            </a>

            <!-- 溯源明细：解析方式 / 置信度 / 内容版本 / 入库时间，全部来自后端 DTO -->
            <dl v-if="provenance.length" class="mt-4 space-y-1.5 border-t border-accent-200 pt-3.5">
              <div v-for="item in provenance" :key="item.label" class="flex justify-between gap-3 text-[12.5px]">
                <dt class="text-ink-500">{{ item.label }}</dt>
                <dd class="tabular text-right text-ink-700">{{ item.value }}</dd>
              </div>
            </dl>

            <AppButton v-if="applyHref" variant="success" block class="mt-4" :href="applyHref">
              {{ applyLabel }}
            </AppButton>
          </section>
        </aside>
      </div>

      <!-- 相关职位 -->
      <section v-if="related?.items?.length" class="mt-10">
        <h2 class="mb-4 text-[18px] font-semibold text-ink-900">相关职位</h2>
        <div class="grid gap-4 xl:grid-cols-2">
          <JobCard v-for="item in related.items" :key="item.id" :job="item" />
        </div>
      </section>
    </template>
  </div>
</template>
