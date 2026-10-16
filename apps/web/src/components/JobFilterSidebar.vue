<script setup>
import { computed } from 'vue'

import AppCheckbox from '@/components/AppCheckbox.vue'

/**
 * 职位筛选栏。
 *
 * 两个关键约束：
 *
 * 1) 选项值必须与 GET /jobs 的查询参数同名（city / category / industry /
 *    employment_type / experience / salary_min / salary_max），
 *    这样 JobExplorer 才能把 model 原样映射成请求参数，不需要中间翻译层。
 *
 * 2) **筛选组的来源分三类，不能混为一谈**：
 *    - 后端字典（MetaView）：locations → city、job_types → category、
 *      industries → industry、employment_types → employment_type。
 *      运营在后台调整收集范围后这些选项会跟着变，前端不许写死。
 *    - 自由文本：experience。后端用「包含」匹配（ExperienceContainsFold），
 *      接受「3-5年」「3年以上」等任意写法，因此下面的选项只是**本地便利列表**，
 *      不是服务端枚举，用户传别的值同样能搜到。
 *    - 数值区间：salary_min / salary_max 单位是「元/月」，语义是区间相交。
 *      地址栏里存的是区间 key，请求前由 JobExplorer 换算成两个数字。
 *    换言之 experience 与薪资都是客户端的顺手功能，不是后端能力边界。
 *
 * 交互取舍：设计稿把这几组画成多选复选框，但公开岗位接口对它们只接受单值，
 * 做成多选会给出「选了却搜不到」的假能力，所以统一为单选胶囊——
 * 视觉重量与设计稿一致，交互与接口能力一致。
 * 唯一的布尔筛选项「仅看官网直招」保留复选框形态。
 */
const props = defineProps({
  model: { type: Object, required: true },
  dictionaries: { type: Object, required: true },
  /** 经验选项由字典 store 提供，保证筛选栏与职位流用的是同一份 */
  experienceOptions: { type: Array, default: () => [] },
  /** 薪资区间选项同理：key 存在地址栏，请求前换算成 salary_min / salary_max */
  salaryOptions: { type: Array, default: () => [] },
})

const emit = defineEmits(['update', 'reset'])

/** 字典项可能是 undefined（store 首次加载前后），统一收敛成数组 */
function toOptions(list) {
  return (Array.isArray(list) ? list : []).map((item) => ({ value: item, label: item }))
}

/**
 * 后端 locations 默认是空数组（未配置城市范围 = 不限制城市）。
 * 空着会让「工作地点」只剩「全部」一个按钮，看起来像坏了；
 * 而 city 参数本身就是自由文本包含匹配，所以这里补一组常用城市作为便利入口，
 * 与字典选项合并去重——运营配置了城市时以配置为准，也不丢下常用城市。
 */
const COMMON_LOCATIONS = ['北京', '上海', '深圳', '杭州', '广州', '成都']

const locationOptions = computed(() => {
  const configured = toOptions(props.dictionaries.locations)
  const values = new Set(configured.map((item) => item.value))
  const extra = COMMON_LOCATIONS.filter((city) => !values.has(city)).map((city) => ({
    value: city,
    label: city,
  }))
  return [{ value: '', label: '全部' }, ...configured, ...extra]
})

const GROUPS = computed(() => [
  { key: 'city', label: '工作地点', options: locationOptions.value },
  {
    key: 'category',
    label: '职位类型',
    options: [{ value: '', label: '全部' }, ...toOptions(props.dictionaries.job_types)],
  },
  {
    key: 'industry',
    label: '行业',
    options: [{ value: '', label: '全部' }, ...toOptions(props.dictionaries.industries)],
  },
  {
    key: 'employment_type',
    label: '用工性质',
    options: [{ value: '', label: '全部' }, ...toOptions(props.dictionaries.employment_types)],
  },
  {
    key: 'salary',
    label: '薪资范围',
    options: props.salaryOptions,
  },
  {
    key: 'experience',
    label: '经验要求',
    options: props.experienceOptions,
  },
])

/** 除「全部 / 不限」以外有值的组才算一项，计数才与用户的直觉一致 */
const activeCount = computed(
  () =>
    ['city', 'category', 'industry', 'employment_type', 'experience'].filter(
      (key) => props.model[key],
    ).length +
    (props.model.salary && props.model.salary !== 'all' ? 1 : 0) +
    (props.model.direct ? 1 : 0),
)
</script>

<template>
  <aside class="card h-fit p-5 lg:sticky lg:top-24">
    <div class="mb-5 flex items-center justify-between">
      <h2 class="text-[15px] font-semibold text-ink-900">
        筛选条件
        <span v-if="activeCount" class="ml-1 text-[12.5px] font-normal text-brand-600">
          {{ activeCount }} 项
        </span>
      </h2>
      <button
        type="button"
        class="text-[13px] text-brand-600 transition hover:text-brand-700 disabled:text-ink-300"
        :disabled="!activeCount"
        @click="emit('reset')"
      >
        重置
      </button>
    </div>

    <div class="space-y-5">
      <section v-for="group in GROUPS" :key="group.key">
        <h3 class="mb-2.5 text-[13px] font-medium text-ink-600">{{ group.label }}</h3>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="option in group.options"
            :key="`${group.key}-${option.value}`"
            type="button"
            class="rounded-lg border px-3 py-1.5 text-[12.5px] transition"
            :class="
              model[group.key] === option.value
                ? 'border-brand-600 bg-brand-600 font-medium text-white'
                : 'border-ink-200 bg-white text-ink-600 hover:border-brand-300 hover:text-brand-700'
            "
            @click="emit('update', { [group.key]: option.value })"
          >
            {{ option.label }}
          </button>
        </div>
      </section>

      <section class="border-t border-ink-100 pt-5">
        <h3 class="mb-2.5 text-[13px] font-medium text-ink-600">直招来源</h3>
        <!-- 绑到 is_direct_hire：只有官网直招岗位才带这个标记 -->
        <AppCheckbox
          :model-value="model.direct"
          label="仅看企业官网《加入我们》直招"
          @update:model-value="emit('update', { direct: $event })"
        />
      </section>
    </div>
  </aside>
</template>
