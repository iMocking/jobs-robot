/**
 * 展示层格式化与语义色映射 —— 也是**后端 DTO 到界面的唯一适配层**。
 *
 * 为什么要单独强调这一点：
 * 后端（internal/service/view.go）只返回业务字段，不返回任何「用哪个颜色、取哪个字做头像」。
 * 早期版本把这些当成接口字段（company.tone / user.initial / registered_at），
 * 结果只有 mock 里有这些字段，一接真后端整页错位。
 * 现在所有派生展示都在这里算，页面只消费这里导出的函数。
 *
 * 颜色语义（来自设计稿）：
 *   brand  蓝色 —— 主操作、进行中
 *   accent 绿色 —— **只服务**「官网直招」与「正常/成功」
 *   amber  黄色 —— 待处理
 *   danger 红色 —— 驳回、失败
 *   ink    灰色 —— 已下线、已禁用等中性终态
 *
 * 注意：Tailwind 只扫描源码里的字面量类名，因此这里必须写完整的类名字符串，
 * 不能用模板拼接（`bg-${tone}-50` 这类写法不会生成样式）。
 */

export const TONE_RING = {
  brand: 'bg-brand-50 text-brand-700 ring-1 ring-brand-100',
  accent: 'bg-accent-50 text-accent-700 ring-1 ring-accent-100',
  amber: 'bg-amber-50 text-amber-600 ring-1 ring-amber-100',
  danger: 'bg-red-50 text-red-600 ring-1 ring-red-100',
  ink: 'bg-ink-100 text-ink-600 ring-1 ring-ink-200',
  violet: 'bg-violet-50 text-violet-700 ring-1 ring-violet-100',
  indigo: 'bg-indigo-50 text-indigo-700 ring-1 ring-indigo-100',
  teal: 'bg-teal-50 text-teal-700 ring-1 ring-teal-100',
}

export const TONE_SOLID = {
  brand: 'bg-brand-600 text-white',
  accent: 'bg-accent-600 text-white',
  amber: 'bg-amber-500 text-white',
  danger: 'bg-red-500 text-white',
  ink: 'bg-ink-500 text-white',
  violet: 'bg-violet-600 text-white',
  indigo: 'bg-indigo-600 text-white',
  teal: 'bg-teal-600 text-white',
}

/**
 * 公司/用户头像的纯色底，用首字代替真实 Logo。
 * 刻意不含 accent：绿色是这个产品的「官网直招」专用语义，
 * 拿来染头像会让唯一的信任信号贬值。
 */
export const AVATAR_TONE = {
  brand: 'bg-brand-600',
  violet: 'bg-violet-600',
  amber: 'bg-amber-500',
  danger: 'bg-rose-500',
  indigo: 'bg-indigo-600',
  teal: 'bg-teal-600',
  sky: 'bg-sky-600',
}

const AVATAR_TONE_KEYS = Object.keys(AVATAR_TONE)

/**
 * 由任意种子（公司 id、邮箱、名称）稳定地推出一个头像配色。
 *
 * 为什么不用随机：同一条数据在列表和详情页必须同色，
 * 而且每次刷新都要一致，否则列表会「闪色」。
 */
export function toneFor(seed) {
  const text = String(seed ?? '')
  let hash = 0
  for (let index = 0; index < text.length; index += 1) {
    hash = (hash * 31 + text.charCodeAt(index)) % 99991
  }
  return AVATAR_TONE_KEYS[hash % AVATAR_TONE_KEYS.length]
}

/** 岗位状态 —— 取值与 internal/ent/schema/job_posting.go 的状态常量一致 */
export const JOB_STATUS = {
  pending_review: { label: '待审核', tone: 'amber' },
  approved: { label: '已发布', tone: 'accent' },
  rejected: { label: '已驳回', tone: 'danger' },
  offline: { label: '已下线', tone: 'ink' },
  expired: { label: '已过期', tone: 'ink' },
}

/** 用户状态 —— 后端有 pending/locked 两态，界面必须能显示，否则会显示成「未知」 */
export const USER_STATUS = {
  pending: { label: '待验证', tone: 'amber' },
  active: { label: '正常', tone: 'accent' },
  disabled: { label: '已禁用', tone: 'ink' },
  locked: { label: '已锁定', tone: 'danger' },
}

export const CRAWL_STATUS = {
  running: { label: '执行中', tone: 'amber' },
  success: { label: '成功', tone: 'accent' },
  partial: { label: '部分成功', tone: 'amber' },
  failed: { label: '失败', tone: 'danger' },
  skipped: { label: '已跳过', tone: 'ink' },
}

export const TRIGGER_LABEL = {
  schedule: '定时任务',
  manual: '手动触发',
  retry: '失败重试',
}

export const ROLE_TONE = {
  admin: 'brand',
  reviewer: 'violet',
  user: 'ink',
  system: 'teal',
}

/** 角色 key → 中文名。admin/reviewer/user/system 是 Casbin 里的稳定标识 */
export const ROLE_LABEL = {
  admin: '管理员',
  reviewer: '审核员',
  user: '用户',
  system: '系统',
}

export function roleLabel(key) {
  return ROLE_LABEL[key] ?? key
}

export function jobStatus(key) {
  return JOB_STATUS[key] ?? { label: key || '未知', tone: 'ink' }
}

export function userStatus(key) {
  return USER_STATUS[key] ?? { label: key || '未知', tone: 'ink' }
}

export function crawlStatus(key) {
  return CRAWL_STATUS[key] ?? { label: key || '未知', tone: 'ink' }
}

/* -------------------------------------------------------------------------- */
/* 时间                                                                        */
/* -------------------------------------------------------------------------- */

const MINUTE = 60 * 1000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

function toDate(value) {
  if (!value) return null
  const date = value instanceof Date ? value : new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

/**
 * 「今天 / 昨天 / 3 天前 / 2 小时前」，用于「最后更新」这类相对时间。
 *
 * 后端的 published_at / last_login_at 都是可空指针（*time.Time），
 * 序列化后可能是 null 或直接缺字段，因此这里统一返回占位符而不是抛错或显示 Invalid Date。
 */
export function relativeTime(value) {
  const date = toDate(value)
  if (!date) return '—'
  const diff = Date.now() - date.getTime()
  if (diff < 0) return '刚刚'
  if (diff < HOUR) return `${Math.max(1, Math.floor(diff / MINUTE))} 分钟前`
  if (diff < DAY) return `${Math.floor(diff / HOUR)} 小时前`
  const days = Math.floor(diff / DAY)
  if (days === 1) return '昨天'
  if (days < 30) return `${days} 天前`
  if (days < 365) return `${Math.floor(days / 30)} 个月前`
  return `${Math.floor(days / 365)} 年前`
}

export function formatDate(value) {
  const date = toDate(value)
  if (!date) return '—'
  const pad = (n) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

export function formatDateTime(value) {
  const date = toDate(value)
  if (!date) return '—'
  const pad = (n) => String(n).padStart(2, '0')
  return `${formatDate(date)} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

/** 抓取耗时：2140ms → 2.1s */
export function formatDuration(ms) {
  if (!ms) return '—'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

/* -------------------------------------------------------------------------- */
/* 业务文案                                                                    */
/* -------------------------------------------------------------------------- */

/** 「来源：字节跳动官网《加入我们》」里的来源主体 */
export function sourceLabel(company) {
  const name = typeof company === 'string' ? company : company?.name
  return name ? `${name}官网《加入我们》` : '企业官网《加入我们》'
}

/** 列表卡片的地点文案：北京 · 海淀 */
export function locationText(job) {
  if (!job) return ''
  return [job.city, job.district].filter(Boolean).join(' · ')
}

/** 月薪区间转成「25K-40K·14薪」；后端给了 salary_text 就优先用它 */
export function salaryText(job) {
  if (!job) return '面议'
  if (job.salary_text) return job.salary_text
  if (!job.salary_min && !job.salary_max) return '面议'
  const min = job.salary_min ? `${Math.round(job.salary_min / 1000)}K` : '面议'
  const max = job.salary_max ? `${Math.round(job.salary_max / 1000)}K` : ''
  const months = job.salary_months && job.salary_months !== 12 ? `·${job.salary_months}薪` : ''
  return `${min}${max ? `-${max}` : ''}${months}`
}

/** 列表卡片的标签组：用工性质 + 经验（后端各自可空） */
export function jobTags(job) {
  if (!job) return []
  return [job.employment_type, job.experience].filter(Boolean)
}

/** 公司头像首字：后端有 short_name 就用它，否则取名称首字 */
export function companyInitial(company) {
  if (!company) return '?'
  return (company.short_name || company.name || '?').trim().slice(0, 1)
}

/** 用户头像首字：昵称为空时退回邮箱首字母 */
export function userInitial(user) {
  if (!user) return '?'
  const text = (user.nickname || user.email || '?').trim()
  return text.slice(0, 1).toUpperCase()
}

export function initialsOf(name) {
  const text = String(name || '?').trim()
  return text.slice(0, 1).toUpperCase()
}

export function compactNumber(value) {
  if (value === null || value === undefined) return '—'
  if (value >= 10000) return `${(value / 10000).toFixed(value % 10000 === 0 ? 0 : 1)} 万`
  return String(value)
}

/** 千分位：统计卡的 1,286 / 1,024 用等宽数字 + 分组更易扫读 */
export function formatNumber(value) {
  if (value === null || value === undefined) return '—'
  return Number(value).toLocaleString('en-US')
}

/**
 * 审计日志的 detail 在后端是结构化对象（map[string]any），不是一句话。
 * 把它压成可读的一行，同时保留键名，方便排查「改了哪个字段」。
 */
export function auditDetailText(detail) {
  if (!detail) return '—'
  if (typeof detail === 'string') return detail
  const parts = Object.entries(detail).map(([key, value]) => {
    if (value === null || value === undefined) return `${key}=—`
    if (typeof value === 'object') return `${key}=${JSON.stringify(value)}`
    return `${key}=${value}`
  })
  return parts.length ? parts.join(' · ') : '—'
}
