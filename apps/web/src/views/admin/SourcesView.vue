<script setup>
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'

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
import AppSwitch from '@/components/AppSwitch.vue'
import PageHeader from '@/components/PageHeader.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { useDictionaryStore } from '@/stores/dictionaries'
import { useToastStore } from '@/stores/toast'
import { companyInitial, compactNumber, crawlStatus, toneFor } from '@/utils/format'

/**
 * 公司管理 = 「我们到底在抓谁」的清单。
 *
 * 这张表的核心矛盾是「招聘入口」：没有 careers_url 就没有抓取目标，
 * 所以未定位的公司必须给出醒目提示 + 一键自动定位，
 * 而不是让运营自己去猜哪个开关是坏的。
 *
 * 抓取开关与启用状态刻意分成两列：
 *   enabled      —— 这家公司是否进入前台公司库（对外可见性）
 *   crawl_enabled —— 是否继续定时抓取（对内作业开关）
 * 两者语义不同，合并成一个开关会让「临时停抓但不撤下已发布岗位」没法表达。
 */
const PAGE_SIZE = 10

const toast = useToastStore()
const dictionaries = useDictionaryStore()

const query = reactive({ keyword: '', industry: '', enabled: '', crawl_enabled: '' })
const page = ref(1)

const columns = [
  { key: 'company', label: '公司' },
  { key: 'industry', label: '行业' },
  { key: 'location', label: '地点' },
  { key: 'jobs', label: '在架岗位' },
  { key: 'careers', label: '招聘入口' },
  { key: 'crawl', label: '抓取' },
  { key: 'status', label: '状态' },
  { key: 'actions', label: '操作', class: 'text-right' },
]

const industryOptions = computed(() =>
  (dictionaries.data.industries ?? []).map((item) => ({ value: item, label: item })),
)

const enabledOptions = [
  { value: '', label: '全部' },
  { value: 'true', label: '已启用' },
  { value: 'false', label: '已停用' },
]

const crawlOptions = [
  { value: '', label: '全部' },
  { value: 'true', label: '已开启' },
  { value: 'false', label: '已关闭' },
]

const {
  data,
  loading,
  error,
  ready,
  run: runList,
} = useAsync(
  () => adminApi.companies.list({ ...query, page: page.value, page_size: PAGE_SIZE }),
  { immediate: false },
)

/**
 * 单次只允许一个写操作在飞。
 * 后台这些开关都作用于同一条记录，并发点击很容易写出「先开后关」的错序结果，
 * 用一把互斥锁 + 全局禁用比逐行维护 pending 状态更不容易漏。
 */
const busy = ref('')
const isBusy = (key) => busy.value === key

const runAllLoading = ref(false)

function loadList() {
  // 字典只在首次真正发请求，之后走 store 缓存
  dictionaries.load()
  return runList()
}

useAdminRefresh(loadList)

/* —— 关键词防抖：每敲一个字就请求会让列表持续重排，也浪费后端查询 —— */
let keywordTimer = null
const keywordInput = ref('')

watch(keywordInput, (value) => {
  clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => {
    const next = value.trim()
    if (query.keyword === next) return
    query.keyword = next
    page.value = 1
    loadList()
  }, 300)
})

function applyKeyword() {
  clearTimeout(keywordTimer)
  const next = keywordInput.value.trim()
  if (query.keyword === next) return
  query.keyword = next
  page.value = 1
  loadList()
}

watch([() => query.industry, () => query.enabled, () => query.crawl_enabled], () => {
  page.value = 1
  loadList()
})

watch(page, () => loadList())

onBeforeUnmount(() => clearTimeout(keywordTimer))

/* -------------------------------------------------------------------------- */
/* 顶部操作                                                                    */
/* -------------------------------------------------------------------------- */

async function runAll() {
  runAllLoading.value = true
  try {
    // 响应体是 { runs, count }：逐家公司的 CrawlRunView 列表，没有 message 字段
    const result = await adminApi.crawl.runAll()
    const count = result?.count ?? result?.runs?.length ?? 0
    toast.success(count ? `已提交 ${count} 家公司的抓取任务` : '当前没有可抓取的公司')
    await runList()
  } catch (err) {
    toast.fromError(err, '提交全量抓取失败')
  } finally {
    runAllLoading.value = false
  }
}

/* -------------------------------------------------------------------------- */
/* 行内操作                                                                    */
/* -------------------------------------------------------------------------- */

async function toggleCrawl(row, enabled) {
  if (busy.value) return
  busy.value = `crawl-${row.id}`
  try {
    await adminApi.companies.update(row.id, { crawl_enabled: enabled })
    toast.success(enabled ? `已开启「${row.name}」的抓取` : `已关闭「${row.name}」的抓取`)
    // 列表用 shallowRef 承载，改字段不会触发视图更新，改完统一重取一次
    await runList()
  } catch (err) {
    toast.fromError(err, '更新抓取开关失败')
  } finally {
    busy.value = ''
  }
}

async function toggleEnabled(row, enabled) {
  if (busy.value) return
  busy.value = `enabled-${row.id}`
  try {
    await adminApi.companies.setEnabled(row.id, enabled)
    toast.success(enabled ? `已启用「${row.name}」` : `已停用「${row.name}」`)
    await runList()
  } catch (err) {
    toast.fromError(err, '更新启用状态失败')
  } finally {
    busy.value = ''
  }
}

async function discover(row) {
  if (busy.value) return
  busy.value = `discover-${row.id}`
  try {
    // 响应是更新后的 CompanyView：用 discovery_url 是否被回填来判断结果，
    // 而不是依赖一个并不存在的 message 字段。
    const updated = await adminApi.companies.discover(row.id)
    toast.success(
      updated?.discovery_url || updated?.careers_url
        ? `已定位「${row.name}」的《加入我们》入口并回填`
        : `「${row.name}」未发现可用入口，请手工填写招聘页地址`,
    )
    await runList()
  } catch (err) {
    // 40006 是「定位不到」这一种可预期的业务结果，说清原因比一句「操作失败」有用得多
    if (err?.code === 40006) {
      toast.error(err.message || '未能定位到该公司的招聘页面入口')
    } else {
      toast.fromError(err, '自动定位失败')
    }
  } finally {
    busy.value = ''
  }
}

async function crawlNow(row) {
  if (busy.value) return
  busy.value = `run-${row.id}`
  try {
    // 返回的是本次 CrawlRunView：状态与计数都在里面，比一句通用提示更有信息量
    const run = await adminApi.companies.crawl(row.id)
    const status = crawlStatus(run?.status)
    toast.success(
      `「${row.name}」抓取${status.label}：发现 ${run?.jobs_found ?? 0} / 新建 ${run?.jobs_created ?? 0} / 过滤 ${run?.jobs_filtered ?? 0}`,
    )
    await runList()
  } catch (err) {
    // 40005「该公司的抓取任务正在执行中」需要原样透出，否则运营会反复点击
    toast.fromError(err, '触发抓取失败')
  } finally {
    busy.value = ''
  }
}

/* —— 删除：不可逆，必须走确认弹窗 —— */
const removeTarget = ref(null)
const removing = ref(false)

async function confirmRemove() {
  const row = removeTarget.value
  if (!row) return
  removing.value = true
  try {
    await adminApi.companies.remove(row.id)
    toast.success(`已删除「${row.name}」`)
    removeTarget.value = null
    await runList()
  } catch (err) {
    // 后端对「该公司下仍有岗位」返回的是 10001（参数不合法）+ details.job_count，
    // 属于可自行解决的约束：保留后端原文，避免被通用失败文案盖掉解决路径。
    if (err?.code === 10001 && err?.details?.job_count) {
      toast.error(err.message || `该公司下仍有 ${err.details.job_count} 个岗位，请先清理`)
    } else {
      toast.fromError(err, '删除公司失败')
    }
  } finally {
    removing.value = false
  }
}

/* -------------------------------------------------------------------------- */
/* 新增公司                                                                    */
/* -------------------------------------------------------------------------- */

const createOpen = ref(false)
const creating = ref(false)
const createForm = reactive({
  name: '',
  website: '',
  industry: '',
  location: '',
  careers_url: '',
  crawl_enabled: true,
})
const createErrors = reactive({ name: '', website: '' })

function openCreate() {
  createForm.name = ''
  createForm.website = ''
  createForm.industry = ''
  createForm.location = ''
  createForm.careers_url = ''
  createForm.crawl_enabled = true
  createErrors.name = ''
  createErrors.website = ''
  createOpen.value = true
}

function validateCreate() {
  createErrors.name = createForm.name.trim() ? '' : '请填写公司名称'
  const website = createForm.website.trim()
  createErrors.website = !website
    ? '请填写官网地址'
    : /^https?:\/\//i.test(website)
      ? ''
      : '官网地址需以 http:// 或 https:// 开头'
  return !createErrors.name && !createErrors.website
}

async function submitCreate() {
  if (!validateCreate()) return
  creating.value = true
  try {
    await adminApi.companies.create({
      name: createForm.name.trim(),
      website: createForm.website.trim(),
      industry: createForm.industry,
      location: createForm.location.trim(),
      careers_url: createForm.careers_url.trim(),
      crawl_enabled: createForm.crawl_enabled,
    })
    toast.success('公司已新增')
    createOpen.value = false
    page.value = 1
    await runList()
  } catch (err) {
    toast.fromError(err, '新增公司失败')
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader
      title="公司管理"
      description="这些公司就是岗位的来源官网，招聘入口地址决定了能抓到什么。"
    >
      <template #actions>
        <AppButton variant="secondary" :loading="runAllLoading" @click="runAll">
          <AppIcon name="refresh" :size="16" />
          全量抓取
        </AppButton>
        <AppButton @click="openCreate">
          <AppIcon name="plus" :size="16" />
          新增公司
        </AppButton>
      </template>
    </PageHeader>

    <!-- 筛选：只筛选已入库的数据，不触发抓取 -->
    <div class="card mb-5 p-5">
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <AppInput
          v-model="keywordInput"
          icon="search"
          placeholder="搜索公司名称"
          @enter="applyKeyword"
        />
        <AppSelect v-model="query.industry" :options="industryOptions" placeholder="全部行业" />
        <AppSelect v-model="query.enabled" :options="enabledOptions" placeholder="全部状态" />
        <AppSelect
          v-model="query.crawl_enabled"
          :options="crawlOptions"
          placeholder="全部抓取状态"
        />
      </div>
    </div>

    <!-- 首次加载骨架 -->
    <div v-if="loading && !ready" class="card space-y-3 p-6">
      <div v-for="n in 6" :key="n" class="h-11 animate-pulse rounded-lg bg-ink-100" />
    </div>

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="公司列表加载失败"
      :description="error.message"
    >
      <AppButton @click="loadList()">重新加载</AppButton>
    </AppEmpty>

    <AdminTable
      v-else
      :columns="columns"
      :empty="!data?.items?.length"
      empty-text="没有符合条件的公司"
      min-width="1240px"
    >
      <tr v-for="row in data?.items ?? []" :key="row.id">
        <td class="td-cell">
          <div class="flex items-center gap-3">
            <AppAvatar
              :name="row.name"
              :initial="companyInitial(row)"
              :tone="toneFor(row.id ?? row.name)"
              size="sm"
            />
            <div class="min-w-0">
              <p class="flex items-center gap-1.5">
                <span class="font-semibold text-ink-900">{{ row.name }}</span>
                <span
                  v-if="row.verified"
                  class="text-brand-500"
                  title="已核实企业官网《加入我们》页面"
                >
                  <AppIcon name="check-circle" :size="15" />
                </span>
              </p>
              <a
                v-if="row.website"
                :href="row.website"
                target="_blank"
                rel="noopener noreferrer"
                class="mt-0.5 inline-flex max-w-[220px] items-center gap-1 truncate text-[12px] text-ink-400 transition hover:text-brand-600"
              >
                <span class="truncate">{{ row.website }}</span>
                <AppIcon name="external-link" :size="12" />
              </a>
            </div>
          </div>
        </td>

        <td class="td-cell whitespace-nowrap text-[13px] text-ink-600">{{ row.industry || '—' }}</td>

        <td class="td-cell whitespace-nowrap text-[13px] text-ink-600">{{ row.location || '—' }}</td>

        <td class="td-cell tabular text-[13px] font-medium text-ink-800">
          {{ compactNumber(row.job_count ?? 0) }}
        </td>

        <td class="td-cell">
          <div v-if="row.careers_url" class="flex items-center gap-2">
            <span class="flex min-w-0 items-center gap-1 text-[12.5px] text-ink-600">
              <AppIcon name="globe" :size="14" class="text-ink-400" />
              <span class="max-w-[210px] truncate" :title="row.careers_url">
                {{ row.careers_url }}
              </span>
            </span>
            <AppButton
              variant="ghost"
              size="sm"
              :loading="isBusy(`discover-${row.id}`)"
              :disabled="Boolean(busy) && !isBusy(`discover-${row.id}`)"
              @click="discover(row)"
            >
              重新定位
            </AppButton>
          </div>

          <div v-else class="flex items-center gap-2">
            <AppBadge tone="amber">未定位</AppBadge>
            <AppButton
              variant="secondary"
              size="sm"
              :loading="isBusy(`discover-${row.id}`)"
              :disabled="Boolean(busy) && !isBusy(`discover-${row.id}`)"
              @click="discover(row)"
            >
              自动定位
            </AppButton>
          </div>
        </td>

        <td class="td-cell">
          <AppSwitch
            :model-value="Boolean(row.crawl_enabled)"
            :disabled="Boolean(busy)"
            @update:model-value="toggleCrawl(row, $event)"
          />
        </td>

        <td class="td-cell">
          <div class="flex items-center gap-2.5">
            <AppSwitch
              :model-value="Boolean(row.enabled)"
              :disabled="Boolean(busy)"
              @update:model-value="toggleEnabled(row, $event)"
            />
            <span class="text-[12.5px]" :class="row.enabled ? 'text-accent-700' : 'text-ink-400'">
              {{ row.enabled ? '启用' : '停用' }}
            </span>
          </div>
        </td>

        <td class="td-cell text-right">
          <div class="flex items-center justify-end gap-1.5 whitespace-nowrap">
            <AppButton variant="ghost" size="sm" :to="{ name: 'source', params: { id: row.id } }">
              详情
            </AppButton>
            <AppButton
              variant="secondary"
              size="sm"
              :loading="isBusy(`run-${row.id}`)"
              :disabled="Boolean(busy) && !isBusy(`run-${row.id}`)"
              @click="crawlNow(row)"
            >
              抓取
            </AppButton>
            <AppButton
              variant="danger"
              size="sm"
              :disabled="Boolean(busy)"
              @click="removeTarget = row"
            >
              删除
            </AppButton>
          </div>
        </td>
      </tr>

      <template #footer>
        <div class="flex items-center justify-between gap-4">
          <AppPagination
            :page="data?.page ?? 1"
            :pages="data?.pages ?? 1"
            :total="data?.total ?? 0"
            :page-size="PAGE_SIZE"
            @update:page="page = $event"
          />
          <span v-if="(data?.pages ?? 1) <= 1" class="tabular text-[13px] text-ink-400">
            共 {{ data?.total ?? 0 }} 条
          </span>
        </div>
      </template>
    </AdminTable>

    <!-- 删除确认：会连带影响该公司名下的岗位，因此把后果写在正文里 -->
    <AppModal
      :model-value="Boolean(removeTarget)"
      title="删除公司"
      size="sm"
      @update:model-value="removeTarget = null"
    >
      <p class="text-[13.5px] leading-relaxed text-ink-600">
        确定要从公司管理中删除「<span class="font-medium text-ink-900">{{ removeTarget?.name }}</span
        >」吗？删除后不再抓取该官网，已发布的岗位也不会自动下线。
      </p>
      <p class="mt-3 rounded-lg bg-warn-50 px-3.5 py-2.5 text-[12.5px] leading-relaxed text-amber-600">
        若该公司名下仍有岗位，接口会拒绝删除并提示先清理岗位。
      </p>

      <template #footer>
        <AppButton variant="secondary" @click="removeTarget = null">取消</AppButton>
        <AppButton variant="danger" :loading="removing" @click="confirmRemove">确认删除</AppButton>
      </template>
    </AppModal>

    <!-- 新增公司 -->
    <AppModal
      v-model="createOpen"
      title="新增公司"
      description="招聘入口是抓取的起点，可以留空，入库后用「自动定位」回填。"
      size="lg"
    >
      <div class="space-y-4">
        <AppInput v-model="createForm.name" label="公司名称" placeholder="例如：某某科技有限公司" :error="createErrors.name" />
        <AppInput
          v-model="createForm.website"
          label="官网地址"
          placeholder="https://www.example.com"
          :error="createErrors.website"
        />

        <div class="grid gap-4 sm:grid-cols-2">
          <AppSelect v-model="createForm.industry" label="行业" :options="industryOptions" placeholder="请选择行业" />
          <AppInput v-model="createForm.location" label="所在地" placeholder="例如：北京" />
        </div>

        <AppInput
          v-model="createForm.careers_url"
          label="招聘入口地址"
          placeholder="https://www.example.com/join-us"
          hint="留空可以之后用「自动定位」回填，定位失败时再手工填写完整地址。"
        />

        <div class="rounded-xl border border-ink-200 bg-ink-50/60 px-4 py-3.5">
          <AppSwitch v-model="createForm.crawl_enabled" label="开启抓取" />
          <p class="mt-1.5 text-[12.5px] text-ink-400">
            关闭后该公司仍保留在库中，但不会参与定时抓取；适合官网改版期间临时停抓。
          </p>
        </div>
      </div>

      <template #footer>
        <AppButton variant="secondary" @click="createOpen = false">取消</AppButton>
        <AppButton :loading="creating" @click="submitCreate">保存</AppButton>
      </template>
    </AppModal>
  </div>
</template>
