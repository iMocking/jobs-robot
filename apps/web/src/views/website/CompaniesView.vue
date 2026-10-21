<script setup>
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'

import { companyApi } from '@/api'
import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppPagination from '@/components/AppPagination.vue'
import AppSelect from '@/components/AppSelect.vue'
import { useAsync } from '@/composables/useAsync'
import { useDictionaryStore } from '@/stores/dictionaries'
import { companyInitial, formatNumber, toneFor } from '@/utils/format'

/**
 * 公司库。
 *
 * 这一页存在的理由不是「公司很多」，而是「每一家都核实过官网《加入我们》入口」，
 * 所以卡片的第一眼信息是认证标与来源，岗位数量放在其后作为次要信息。
 *
 * 后端契约（internal/server/handler/job.go 的 ListCompanies）：
 *   GET /companies?keyword&industry&page&page_size
 *   → response.Page<CompanyBriefView>，即 { items, total, page, page_size, pages }。
 * 注意前台拿到的是 **简要视图**，只有 id/name/short_name/logo_url/industry/location/verified；
 * 没有 job_count、没有 website/careers_url/jobs —— 那些只在管理端 CompanyView 里。
 * 因此卡片上不展示岗位数（前端凭空算不出来），改成一个按 company_id 过滤的入口。
 *
 * 筛选条件沿用 JobExplorer 的约定：**地址栏是唯一事实来源**（route.query）。
 * 这样从公司详情页返回、刷新、分享链接都能复原同一次筛选结果。
 */
const PAGE_SIZE = 12

const route = useRoute()
const router = useRouter()

const dictionary = useDictionaryStore()
const { data: dictionaries } = storeToRefs(dictionary)
dictionary.load()

/**
 * 关键词是本地受控态而不是直接读写 route.query：
 * 每敲一个字符就 push 一次会让浏览器历史被逐字填满，后退键变得不可用。
 * 因此只有回车（或点「搜索」）才写进地址栏，其余时刻只更新输入框。
 */
const keyword = ref(String(route.query.keyword ?? ''))

const industry = computed({
  get: () => String(route.query.industry ?? ''),
  set: (value) => pushQuery({ industry: value || undefined, page: undefined }),
})

/** 行业选项来自后端字典 MetaView.industries，与筛选栏同一份数据 */
const industryOptions = computed(() => dictionaries.value.industries ?? [])

const page = computed(() => Math.max(1, Number(route.query.page) || 1))

/** 接口不支持空串筛选（见 http.js 的 buildQuery），空值一律不上送 */
const { data, loading, error, ready, run } = useAsync(() =>
  companyApi.list({
    keyword: String(route.query.keyword ?? '') || undefined,
    industry: String(route.query.industry ?? '') || undefined,
    page: page.value,
    page_size: PAGE_SIZE,
  }),
)

const hasFilter = computed(() => Boolean(route.query.keyword || route.query.industry))

// 后退 / 前进 / 手工改地址栏都要重新取数，同时把输入框拉回与地址栏一致
watch(
  () => route.query,
  () => {
    keyword.value = String(route.query.keyword ?? '')
    run()
  },
  { deep: true },
)

/** 只保留有值的参数，地址栏不被空串污染 */
function pushQuery(patch) {
  const cleaned = {}
  Object.entries({ ...route.query, ...patch }).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') cleaned[key] = value
  })
  router.push({ name: 'companies', query: cleaned })
}

function applyKeyword() {
  pushQuery({ keyword: keyword.value.trim() || undefined, page: undefined })
}

function clearFilters() {
  // 显式写 undefined 而不是推空对象：pushQuery 会先合并现有 query 再清理
  pushQuery({ keyword: undefined, industry: undefined, page: undefined })
}

function changePage(next) {
  pushQuery({ page: next > 1 ? String(next) : undefined })
}
</script>

<template>
  <div class="container-page py-8">
    <div class="mb-6">
      <h1 class="text-[24px] font-semibold text-ink-900">公司库</h1>
      <p class="mt-1.5 text-[13.5px] text-ink-500">
        这里的每一家公司都已核实官网《加入我们》招聘入口，岗位信息可回到来源页面逐条核对。
      </p>
    </div>

    <div class="card mb-6 p-5">
      <div class="grid gap-4 sm:grid-cols-[minmax(0,1fr)_220px_auto] sm:items-end">
        <AppInput
          v-model="keyword"
          label="公司名称"
          icon="search"
          placeholder="输入公司名称后回车"
          @enter="applyKeyword"
        />

        <AppSelect v-model="industry" label="所属行业" placeholder="全部行业" :options="industryOptions" />

        <div class="flex items-center gap-2.5 pb-0.5">
          <AppButton @click="applyKeyword">
            <AppIcon name="search" :size="17" />
            搜索
          </AppButton>
          <AppButton v-if="hasFilter" variant="secondary" @click="clearFilters">重置</AppButton>
        </div>
      </div>
    </div>

    <!-- 首次加载：骨架屏，避免「空列表闪一下再有内容」 -->
    <div v-if="loading && !ready" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
      <div v-for="index in 6" :key="index" class="card h-[186px] animate-pulse" />
    </div>

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="公司列表加载失败"
      :description="error.message"
    >
      <AppButton @click="run()">重新加载</AppButton>
    </AppEmpty>

    <AppEmpty
      v-else-if="!data?.items?.length"
      title="没有匹配的公司"
      description="换个关键词或放宽行业筛选试试。我们只收录已核实官网《加入我们》入口的公司。"
    >
      <AppButton v-if="hasFilter" variant="secondary" @click="clearFilters">重置筛选</AppButton>
    </AppEmpty>

    <template v-else>
      <p class="mb-4 text-[13px] text-ink-500">
        共
        <span class="tabular font-medium text-ink-800">{{ formatNumber(data.total) }}</span>
        家公司
      </p>

      <div
        class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3"
        :class="loading ? 'opacity-60 transition' : ''"
      >
        <article v-for="company in data.items" :key="company.id" class="card card-hover flex flex-col p-5">
          <div class="flex items-start gap-3.5">
            <!-- 首字与配色都在前端派生：CompanyBriefView 没有 initial / tone / logo -->
            <AppAvatar
              :name="company.name"
              :initial="companyInitial(company)"
              :tone="toneFor(company.id ?? company.name)"
            />

            <div class="min-w-0 flex-1">
              <p class="flex items-center gap-1.5 text-[15.5px] font-semibold text-ink-900">
                <span class="truncate">{{ company.name }}</span>
                <span
                  v-if="company.verified"
                  class="text-brand-500"
                  title="已核实企业官网《加入我们》页面"
                >
                  <AppIcon name="check-circle" :size="16" />
                </span>
              </p>

              <!-- short_name 是后端的可选简称，有则显示，没有就不占位 -->
              <p v-if="company.short_name" class="mt-1 truncate text-[12.5px] text-ink-400">
                {{ company.short_name }}
              </p>

              <div class="mt-2.5 flex flex-wrap items-center gap-2">
                <AppBadge v-if="company.industry" tone="ink">{{ company.industry }}</AppBadge>
                <AppBadge v-if="company.location" tone="ink">{{ company.location }}</AppBadge>
              </div>
            </div>
          </div>

          <div class="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-ink-100 pt-3.5">
            <RouterLink
              :to="{ name: 'jobs', query: { company_id: company.id } }"
              class="inline-flex items-center gap-1.5 text-[12.5px] text-brand-600 transition hover:text-brand-700"
            >
              <AppIcon name="briefcase" :size="14" />
              查看该公司在架岗位
            </RouterLink>
            <AppButton size="sm" :to="{ name: 'company', params: { id: company.id } }">
              查看公司
            </AppButton>
          </div>
        </article>
      </div>

      <div class="mt-8">
        <AppPagination
          :page="data.page"
          :pages="data.pages"
          :total="data.total"
          :page-size="data.page_size"
          @update:page="changePage"
        />
      </div>
    </template>
  </div>
</template>
