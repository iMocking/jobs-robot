<script setup>
import { computed, ref, watch } from 'vue'

import { adminApi } from '@/api'
import AppButton from '@/components/AppButton.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppTagInput from '@/components/AppTagInput.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { useDictionaryStore } from '@/stores/dictionaries'
import { useToastStore } from '@/stores/toast'
import { formatDateTime } from '@/utils/format'

/**
 * 爬虫收集范围（crawler.scope）。
 *
 * 两层配置模型：configs/config.yaml 的 crawler.scope 是基线，数据库里的
 * crawler.scope 逐字段覆盖它；接口返回的永远是「最终生效」的结果。
 *
 * 白名单语义必须在界面上说清楚，否则运营会把「留空」理解成「什么都不收」：
 * 留空 = 不限该维度；行业无法判断时服务端放行，交给人工审核。
 * 显式空数组表示取消该维度限制，因此这里不做「非空才提交」的裁剪。
 */
const toast = useToastStore()
const dictionary = useDictionaryStore()
dictionary.load()

// 收集范围接口只返回生效值，不带 updated_at/updated_by，
// 因此额外读一次 settings 行来回答「谁在什么时候改的」——出问题时这是第一个要问的。
const { data, loading, error, ready, run } = useAsync(
  async () => {
    const [scope, row] = await Promise.all([
      adminApi.settings.scope(),
      adminApi.settings.get('crawler.scope').catch(() => null),
    ])
    return { scope, row }
  },
  { immediate: false },
)
useAdminRefresh(run)

const BLANK = {
  job_types: [],
  industries: [],
  employment_types: [],
  locations: [],
  keywords: [],
  exclude_keywords: [],
  min_quality_score: 0,
}

const form = ref({ ...BLANK })
const baseline = ref('')
const saving = ref(false)

function hydrate(scope) {
  form.value = {
    job_types: [...(scope?.job_types ?? [])],
    industries: [...(scope?.industries ?? [])],
    employment_types: [...(scope?.employment_types ?? [])],
    locations: [...(scope?.locations ?? [])],
    keywords: [...(scope?.keywords ?? [])],
    exclude_keywords: [...(scope?.exclude_keywords ?? [])],
    min_quality_score: Number(scope?.min_quality_score ?? 0),
  }
  baseline.value = JSON.stringify(form.value)
}

watch(data, (value) => hydrate(value?.scope), { immediate: true })

const dirty = computed(() => baseline.value !== '' && JSON.stringify(form.value) !== baseline.value)

async function save() {
  saving.value = true
  try {
    await adminApi.settings.saveScope({
      job_types: form.value.job_types,
      industries: form.value.industries,
      employment_types: form.value.employment_types,
      locations: form.value.locations,
      keywords: form.value.keywords,
      exclude_keywords: form.value.exclude_keywords,
      min_quality_score: Number(form.value.min_quality_score) || 0,
    })
    toast.success('收集范围已保存，下一次抓取立即生效')
    await run()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <div v-if="loading && !ready" class="h-72 animate-pulse rounded-card bg-ink-100" />

    <div v-else-if="error" class="rounded-card border border-red-200 bg-red-50 p-5">
      <p class="flex items-center gap-2 text-[14px] font-medium text-red-700">
        <AppIcon name="alert" :size="17" />
        收集范围加载失败
      </p>
      <p class="mt-1.5 text-[13px] text-red-600">{{ error.message }}</p>
      <AppButton variant="secondary" size="sm" class="mt-3" @click="run()">重新加载</AppButton>
    </div>

    <template v-else>
      <section class="card p-6">
        <h2 class="text-[16px] font-semibold text-ink-900">收集范围</h2>
        <p class="mt-1 mb-4 text-[12.5px] leading-relaxed text-ink-400">
          白名单语义：不在其中的岗位不采集。<strong class="font-medium text-ink-500">留空表示不限该维度</strong>；
          行业无法判断时自动放行，交由人工审核，避免误杀大量未标注行业的合规岗位。
        </p>

        <div class="divide-y divide-ink-100">
          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">工作类型</label>
            <AppTagInput
              v-model="form.job_types"
              :options="dictionary.data.job_types"
              placeholder="添加类型"
            />
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">行业</label>
            <AppTagInput
              v-model="form.industries"
              :options="dictionary.data.industries"
              placeholder="添加行业"
            />
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">用工性质</label>
            <AppTagInput
              v-model="form.employment_types"
              :options="dictionary.data.employment_types"
              placeholder="添加性质"
            />
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">地点</label>
            <div>
              <AppTagInput
                v-model="form.locations"
                :options="dictionary.data.locations"
                placeholder="添加地点"
              />
              <p class="mt-2 text-[12px] text-ink-400">留空表示不限地区。</p>
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">包含关键词</label>
            <div>
              <AppTagInput v-model="form.keywords" placeholder="添加关键词" />
              <p class="mt-2 text-[12px] text-ink-400">
                非空时，岗位必须命中至少一个关键词才会被采集；留空表示不按关键词过滤。
              </p>
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">排除关键词</label>
            <div>
              <AppTagInput
                v-model="form.exclude_keywords"
                :options="['外包', '劳务派遣', '中介', '代招']"
                placeholder="添加关键词"
              />
              <p class="mt-2 text-[12px] text-ink-400">
                命中任一即丢弃，优先级最高。被过滤的岗位仍会在抓取日志里留下原因。
              </p>
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">最低信息完整度评分（分）</label>
            <div class="max-w-[200px]">
              <AppInput v-model.number="form.min_quality_score" type="number" />
              <p class="mt-2 text-[12px] text-ink-400">低于该分值的岗位不进入审核队列。</p>
            </div>
          </div>
        </div>

        <div class="mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-ink-100 pt-5">
          <p class="text-[12.5px] text-ink-400">
            <template v-if="data?.row?.updated_at">
              最后保存：{{ formatDateTime(data.row.updated_at) }}
              <span v-if="data.row.updated_by">· {{ data.row.updated_by }}</span>
            </template>
            <template v-else>尚未保存过收集范围，当前为 configs/config.yaml 的默认值</template>
            <span v-if="dirty" class="ml-2 font-medium text-amber-600">· 有未保存的修改</span>
          </p>
          <AppButton :loading="saving" :disabled="!dirty" @click="save">保存收集范围</AppButton>
        </div>
      </section>
    </template>
  </div>
</template>
