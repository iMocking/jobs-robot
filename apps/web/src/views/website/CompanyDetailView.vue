<script setup>
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'

import { companyApi, jobApi } from '@/api'
import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import JobCard from '@/components/JobCard.vue'
import TrustBadge from '@/components/TrustBadge.vue'
import { useAsync } from '@/composables/useAsync'
import { companyInitial, formatNumber, sourceLabel, toneFor } from '@/utils/format'

/**
 * 公司详情。
 *
 * 页面顺序刻意与「信任」的建立顺序一致：
 *   先证明这家公司的官网《加入我们》入口是真的（头部卡 + 绿色徽章），
 *   再展示由它派生出来的岗位 —— 而不是反过来把岗位数当卖点。
 *
 * 后端契约（internal/server/handler/job.go 的 CompanyDetail）：
 *   GET /companies/:id → CompanyBriefView，**只有**
 *   { id, name, short_name?, logo_url?, industry?, location?, verified }。
 * 没有 website / careers_url / job_count / jobs —— 这些字段只存在于管理端 CompanyView，
 * 早期版本照 mock 渲染它们，接真后端后整块链接与岗位数都会是空的。
 *
 * 在架岗位因此改为按 company_id 查公开岗位接口：
 *   GET /jobs?company_id=:id —— 这也是唯一能拿到该公司岗位的公开入口。
 */
const route = useRoute()
const companyId = computed(() => route.params.id)

const { data: company, loading, error, ready, run: loadCompany } = useAsync(() =>
  companyApi.detail(companyId.value),
)

const {
  data: jobs,
  loading: jobsLoading,
  error: jobsError,
  run: loadJobs,
} = useAsync(() => jobApi.list({ company_id: companyId.value, page_size: 6 }))

// 从「在架岗位」跳到另一家公司，或从相关链接进入时组件会被复用，必须跟着 id 重新取数
watch(companyId, () => {
  loadCompany()
  loadJobs()
})

/** 首字与配色在前端派生：CompanyBriefView 既没有 initial 也没有 tone */
const avatar = computed(() => ({
  initial: companyInitial(company.value),
  tone: toneFor(company.value?.id ?? company.value?.name),
}))
</script>

<template>
  <div class="container-page py-8">
    <nav class="mb-5 flex items-center gap-2 text-[13px] text-ink-400">
      <RouterLink :to="{ name: 'home' }" class="transition hover:text-brand-600">首页</RouterLink>
      <span>/</span>
      <RouterLink :to="{ name: 'companies' }" class="transition hover:text-brand-600">公司库</RouterLink>
      <span>/</span>
      <span class="max-w-[420px] truncate text-ink-600">{{ company?.name ?? '公司详情' }}</span>
    </nav>

    <!-- 首次加载骨架 -->
    <div v-if="loading && !ready" class="space-y-5">
      <div class="card h-[196px] animate-pulse" />
      <div class="grid gap-4 xl:grid-cols-2">
        <div class="card h-[184px] animate-pulse" />
        <div class="card h-[184px] animate-pulse" />
      </div>
    </div>

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="公司不存在或已下架"
      :description="error.message"
    >
      <div class="flex gap-3">
        <AppButton variant="secondary" :to="{ name: 'companies' }">返回公司库</AppButton>
        <AppButton @click="loadCompany()">重新加载</AppButton>
      </div>
    </AppEmpty>

    <template v-else-if="company">
      <section class="card p-6 sm:p-7">
        <div class="flex flex-wrap items-start gap-5">
          <AppAvatar
            :name="company.name"
            :initial="avatar.initial"
            :tone="avatar.tone"
            size="xl"
          />

          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2.5">
              <h1 class="text-[24px] font-semibold text-ink-900">{{ company.name }}</h1>
              <span
                v-if="company.verified"
                class="text-brand-500"
                title="已核实企业官网《加入我们》页面"
              >
                <AppIcon name="check-circle" :size="19" />
              </span>
              <TrustBadge v-if="company.verified" variant="green" />
            </div>

            <p v-if="company.short_name" class="mt-1.5 text-[13.5px] text-ink-500">
              {{ company.short_name }}
            </p>

            <div class="mt-3.5 flex flex-wrap items-center gap-2">
              <AppBadge v-if="company.industry" tone="ink">{{ company.industry }}</AppBadge>
              <AppBadge v-if="company.location" tone="ink">{{ company.location }}</AppBadge>
              <!-- 岗位数来自公开岗位接口的 total，不是公司 DTO 字段 -->
              <span v-if="jobs" class="inline-flex items-center gap-1.5 text-[13px] text-ink-500">
                <AppIcon name="briefcase" :size="14" class="text-ink-400" />
                <span class="tabular font-medium text-ink-800">{{ formatNumber(jobs.total) }}</span>
                个在架岗位
              </span>
            </div>
          </div>
        </div>

        <!-- 溯源说明：绿色只出现在「官网直招」这一类语义上，所以这里用中性底 + 绿色前缀 -->
        <p class="mt-6 flex flex-wrap items-center gap-2 rounded-xl bg-ink-50 px-4 py-3 text-[13px] text-ink-600">
          <AppIcon name="shield-check" :size="16" class="text-accent-600" />
          岗位来源：{{ sourceLabel(company) }}页面
          <RouterLink
            :to="{ name: 'jobs', query: { company_id: company.id } }"
            class="inline-flex items-center gap-1 text-accent-700 transition hover:underline"
          >
            核对在架岗位
            <AppIcon name="arrow-right" :size="12" />
          </RouterLink>
        </p>
      </section>

      <section class="mt-8">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
          <h2 class="text-[18px] font-semibold text-ink-900">在架岗位</h2>
          <RouterLink
            v-if="jobs?.total"
            :to="{ name: 'jobs', query: { company_id: company.id } }"
            class="text-[13px] text-brand-600 transition hover:text-brand-700"
          >
            查看全部 {{ formatNumber(jobs.total) }} 个职位
          </RouterLink>
        </div>

        <div v-if="jobsLoading && !jobs" class="grid gap-4 xl:grid-cols-2">
          <div v-for="index in 2" :key="index" class="card h-[184px] animate-pulse" />
        </div>

        <div v-else-if="jobsError" class="card">
          <AppEmpty
            icon="alert"
            tone="danger"
            title="在架岗位加载失败"
            :description="jobsError.message"
          >
            <AppButton @click="loadJobs()">重新加载</AppButton>
          </AppEmpty>
        </div>

        <div v-else-if="jobs?.items?.length" class="grid gap-4 xl:grid-cols-2">
          <JobCard v-for="job in jobs.items" :key="job.id" :job="job" />
        </div>

        <!-- 与「加载失败」分开：取数成功但没有岗位才是真的没有 -->
        <div v-else class="card">
          <AppEmpty
            icon="briefcase"
            title="该公司暂无在架岗位"
            description="企业官网《加入我们》页面当前没有已发布的岗位，新岗位通过审核后会自动出现在这里。"
          >
            <AppButton variant="secondary" :to="{ name: 'jobs' }">先看看其它职位</AppButton>
          </AppEmpty>
        </div>
      </section>
    </template>
  </div>
</template>
