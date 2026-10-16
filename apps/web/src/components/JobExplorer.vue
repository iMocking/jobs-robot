<script setup>
import { computed, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'

import { jobApi } from '@/api'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppPagination from '@/components/AppPagination.vue'
import JobCard from '@/components/JobCard.vue'
import JobFilterSidebar from '@/components/JobFilterSidebar.vue'
import { useAsync } from '@/composables/useAsync'
import { EXPERIENCE_OPTIONS, SALARY_RANGES, salaryRange, useDictionaryStore } from '@/stores/dictionaries'
import { formatNumber } from '@/utils/format'

/**
 * 职位浏览器：左侧筛选 + 右侧卡片流 + 分页。
 *
 * **筛选条件的唯一事实来源是地址栏**（route.query）而不是组件内部状态。
 * 这样筛选结果可以直接分享、刷新不丢、浏览器后退能回到上一次筛选 ——
 * 与路由的设计思路一致：能被链接表达的状态就放进路由。
 *
 * 地址栏里的参数名刻意与 GET /jobs 的查询参数完全一致（见 internal/server/handler/job.go
 * 的 listJobFilter），只有 salary 是例外：地址栏存区间 key，上送时才换算成
 * salary_min / salary_max 两个「元/月」数字，因为后端只认数字、不认区间名。
 */
const PAGE_SIZE = 10

const route = useRoute()
const router = useRouter()

const dictionary = useDictionaryStore()
const { data: dictionaries } = storeToRefs(dictionary)
dictionary.load()

/** 非 '' 的筛选项与查询参数同名，读地址栏时直接取即可 */
const model = computed(() => ({
  keyword: String(route.query.keyword ?? ''),
  city: String(route.query.city ?? ''),
  category: String(route.query.category ?? ''),
  industry: String(route.query.industry ?? ''),
  employment_type: String(route.query.employment_type ?? ''),
  salary: String(route.query.salary ?? 'all'),
  experience: String(route.query.experience ?? ''),
  direct: route.query.direct === '1',
}))

const page = computed(() => Math.max(1, Number(route.query.page) || 1))

/** 侧栏只需要 key/label，min/max 留在 buildQuery 里换算，不让「区间名」进入请求 */
const salaryOptions = SALARY_RANGES.map((item) => ({ value: item.key, label: item.label }))

function buildQuery() {
  const current = model.value
  const range = salaryRange(current.salary)
  return {
    keyword: current.keyword || undefined,
    city: current.city || undefined,
    category: current.category || undefined,
    industry: current.industry || undefined,
    employment_type: current.employment_type || undefined,
    experience: current.experience || undefined,
    salary_min: range.min ?? undefined,
    salary_max: range.max ?? undefined,
    is_direct_hire: current.direct ? 'true' : undefined,
    // 列表排序固定按「最后更新」：这是本产品最想让用户看到的信息新鲜度
    order_by: 'last_updated_at',
    page: page.value,
    page_size: PAGE_SIZE,
  }
}

const { data, loading, error, ready, run } = useAsync(() => jobApi.list(buildQuery()))

watch(() => route.query, () => run(), { deep: true })

/** 只保留有值的参数，地址栏才不会被一堆空串污染 */
function pushQuery(query) {
  const cleaned = {}
  Object.entries(query).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') cleaned[key] = value
  })
  router.push({ name: route.name, query: cleaned })
}

function updateFilter(patch) {
  // 改筛选一定要回到第 1 页，否则会停在「新条件下的第 3 页」看到空列表
  pushQuery({ ...route.query, ...patch, page: undefined })
}

function resetFilters() {
  pushQuery(model.value.keyword ? { keyword: model.value.keyword } : {})
}

function changePage(next) {
  pushQuery({ ...route.query, page: next > 1 ? String(next) : undefined })
}
</script>

<template>
  <div class="grid items-start gap-6 lg:grid-cols-[260px_minmax(0,1fr)]">
    <JobFilterSidebar
      :model="model"
      :dictionaries="dictionaries"
      :experience-options="EXPERIENCE_OPTIONS"
      :salary-options="salaryOptions"
      @update="updateFilter"
      @reset="resetFilters"
    />

    <div class="min-w-0">
      <!-- 首次加载：骨架屏，避免空列表闪一下再出现内容 -->
      <div v-if="loading && !ready" class="space-y-4">
        <div v-for="index in 3" :key="index" class="card h-[184px] animate-pulse" />
      </div>

      <AppEmpty
        v-else-if="error"
        icon="alert"
        tone="danger"
        title="职位加载失败"
        :description="error.message"
      >
        <AppButton @click="run()">重新加载</AppButton>
      </AppEmpty>

      <AppEmpty
        v-else-if="!data?.items?.length"
        title="没有匹配的职位"
        description="试试放宽筛选条件，或者换一个关键词。我们只收录企业官网《加入我们》页面上的岗位。"
      >
        <AppButton variant="secondary" @click="resetFilters">重置筛选</AppButton>
      </AppEmpty>

      <template v-else>
        <p class="mb-4 text-[13px] text-ink-500">
          共找到
          <!-- 数字统一走 formatNumber，避免各页各写一遍千分位 -->
          <span class="tabular font-medium text-ink-800">{{ formatNumber(data.total) }}</span>
          个官网直招职位
        </p>

        <div class="space-y-4" :class="loading ? 'opacity-60 transition' : ''">
          <JobCard v-for="job in data.items" :key="job.id" :job="job" />
        </div>

        <div class="mt-7">
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
  </div>
</template>
