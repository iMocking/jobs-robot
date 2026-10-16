import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { jobApi } from '@/api'

/**
 * 字典（工作地点 / 职位类型 / 行业 / 用工性质 / 各类状态枚举）。
 *
 * 契约来源：internal/service/meta.go 的 MetaView，键名原样搬过来，不做二次命名。
 * 早期版本自己发明了 categories / cities / experiences / salary_ranges 四个键，
 * 而真实接口返回的是 job_types / locations，且**不含**经验与薪资区间——
 * 结果页面上「职位类型」永远是空的，选了城市也永远搜不到。
 * 现在本文件只做三件事：透传 MetaView、给一个键名一致的离线兜底、缓存一次请求。
 */

/**
 * 离线兜底：仅在后端不可用时使用，键名必须与 MetaView 完全一致，
 * 否则「兜底能跑、联调就崩」，兜底本身反而成了缺陷来源。
 *
 * 注意 locations 在真实后端默认是空数组（crawler.scope.locations 未配置即不限制城市），
 * 这里保持空数组而不是塞一批城市：城市筛选在前端另有「常用城市」便利列表，
 * 与字典是两回事（见 components/JobFilterSidebar.vue 的说明）。
 */
const FALLBACK = {
  job_types: ['技术', '产品', '设计', '运营', '市场', '职能'],
  industries: ['互联网', '人工智能', '金融科技', '智能制造', '新能源'],
  employment_types: ['全职', '实习', '校招', '社招'],
  locations: [],
  job_statuses: [
    { value: 'pending_review', label: '待审核' },
    { value: 'approved', label: '已发布' },
    { value: 'rejected', label: '已驳回' },
    { value: 'offline', label: '已下线' },
    { value: 'expired', label: '已过期' },
  ],
  user_statuses: [
    { value: 'pending', label: '待验证' },
    { value: 'active', label: '正常' },
    { value: 'disabled', label: '已禁用' },
    { value: 'locked', label: '已锁定' },
  ],
  roles: [
    { value: 'admin', label: '管理员' },
    { value: 'reviewer', label: '审核员' },
    { value: 'user', label: '用户' },
  ],
  crawl_statuses: [
    { value: 'running', label: '执行中' },
    { value: 'success', label: '成功' },
    { value: 'partial', label: '部分成功' },
    { value: 'failed', label: '失败' },
    { value: 'skipped', label: '已跳过' },
  ],
  crawl_triggers: [
    { value: 'schedule', label: '定时任务' },
    { value: 'manual', label: '手动触发' },
    { value: 'retry', label: '失败重试' },
  ],
  code_purposes: [
    { value: 'register', label: '注册' },
    { value: 'reset_password', label: '找回密码' },
    { value: 'change_email', label: '更换邮箱' },
  ],
}

/**
 * 经验要求：**不是后端枚举**。
 *
 * GET /jobs 的 experience 是自由文本、按「包含」匹配（internal/service/job.go 用的
 * ExperienceContainsFold），所以这里只是帮用户把常见写法填进输入框的便利列表，
 * 用户完全可以自己传「3年以上」这类值。放在本文件是因为筛选栏与职位流需要共用同一份。
 */
export const EXPERIENCE_OPTIONS = [
  { value: '', label: '不限' },
  { value: '1-3年', label: '1-3年' },
  { value: '3-5年', label: '3-5年' },
  { value: '5-10年', label: '5-10年' },
  { value: '10年以上', label: '10年以上' },
]

/**
 * 薪资区间：同样**不是后端枚举**。
 * 后端只接受 salary_min / salary_max 两个以「元/月」为单位的数字，语义为区间相交；
 * 这里的 key 只存在于地址栏，请求前必须换算成 min/max 两个数字。
 */
export const SALARY_RANGES = [
  { key: 'all', label: '不限', min: null, max: null },
  { key: '10-20', label: '10-20K', min: 10000, max: 20000 },
  { key: '20-30', label: '20-30K', min: 20000, max: 30000 },
  { key: '30-40', label: '30-40K', min: 30000, max: 40000 },
  { key: '40+', label: '40K以上', min: 40000, max: null },
]

/** 地址栏里的薪资 key → 后端要的两个数字；未知 key 一律视为「不限」 */
export function salaryRange(key) {
  return SALARY_RANGES.find((item) => item.key === key) ?? SALARY_RANGES[0]
}

/** 字典里的 value/label 选项 → 可直接配置到 AppSelect 的结构 */
export function optionList(items) {
  return (Array.isArray(items) ? items : []).map((item) => ({
    value: item?.value ?? item,
    label: item?.label ?? item?.value ?? item,
  }))
}

export const useDictionaryStore = defineStore('dictionaries', () => {
  const data = ref({ ...FALLBACK })
  const loaded = ref(false)
  let pending = null

  /** 后端字段缺省时回落到兜底数组，页面不用到处写 `?? []` */
  const jobTypes = computed(() => data.value.job_types ?? [])
  const industries = computed(() => data.value.industries ?? [])
  const employmentTypes = computed(() => data.value.employment_types ?? [])
  const locations = computed(() => data.value.locations ?? [])

  function load() {
    if (loaded.value) return Promise.resolve(data.value)
    pending ??= jobApi
      .dictionaries()
      .then((result) => {
        // 后端返回空数组是「没有配置」而不是「加载失败」，因此不能用 || 覆盖成兜底值：
        // 覆盖会让运营在后台清空某类目后，前台仍然显示一个永远搜不到的选项。
        data.value = { ...FALLBACK, ...(result ?? {}) }
        loaded.value = true
        return data.value
      })
      .catch(() => {
        loaded.value = true
        return data.value
      })
      .finally(() => {
        pending = null
      })
    return pending
  }

  return { data, loaded, jobTypes, industries, employmentTypes, locations, load }
})
