/**
 * 演示数据适配层（mock adapter）。
 *
 * 只用一份内存表模拟 Go 后端的接口行为：分页、筛选、鉴权、写操作与错误码，
 * 让界面在没有后端时依然「点得动」。
 * 关闭方式：VITE_USE_MOCK=false（默认就是关闭，避免在演示数据上积累假契约）。
 *
 * 本文件的唯一目标是「**字节级对齐真实契约**」：
 * 每个响应体都由下面的 view 组装函数逐字段挑选，字段名严格等于
 * `internal/service/view.go` 的 json tag，并显式处理 Go 的 `omitempty`
 * （空字符串 / nil 指针 / 空切片 / 空 map 在响应里应当**整个键消失**）。
 * 因此这里刻意不做 `{ ...record }` 这种透传：透传会把内部字段（company_id、
 * revision_history、smtp.password）泄漏成响应字段，正是「演示能跑、联调全崩」的来源。
 *
 * 路由表覆盖的端点（与 internal/server/router.go 一一对应）：
 *   POST   /auth/codes|register|login|refresh|logout|password/forgot|password/reset
 *   GET    /healthz|readyz|meta/dictionaries
 *   GET    /system/status   POST /system/setup   POST /system/setup/mail/test
 *   GET    /jobs | /jobs/:id | /jobs/:id/related
 *   GET    /companies | /companies/:id
 *   GET    /me   PATCH /me   POST /me/password
 *   GET    /me/sessions   DELETE /me/sessions/:sid
 *   GET    /admin/stats
 *   GET|POST /admin/jobs   GET|PATCH|DELETE /admin/jobs/:id
 *   POST   /admin/jobs/:id/review|status   GET /admin/jobs/:id/revisions
 *   GET|POST /admin/companies   GET|PATCH|DELETE /admin/companies/:id
 *   POST   /admin/companies/:id/enabled|discover|crawl
 *   GET    /admin/crawl-runs | /admin/crawl-runs/:id   POST /admin/crawl/run-all
 *   GET|DELETE /admin/users/:id  POST /admin/users/:id/status|roles|reset-password
 *   GET    /admin/users   GET /admin/roles
 *   GET    /admin/settings | /admin/settings/:key   PUT|DELETE /admin/settings/:key
 *   GET|PUT /admin/crawl-scope
 *   GET|PUT|DELETE /admin/mail   POST /admin/mail/test
 *   GET    /admin/audit-logs
 *   GET|POST|DELETE /admin/authz/policies
 */
import { ApiError } from '@/api/http'
import {
  MAIL_PASSWORD_MASK,
  META_ENUMS,
  auditLogs,
  buildCrawlLogs,
  companies,
  contentSnapshot,
  crawlRuns,
  demoAccount,
  demoHash,
  groupingRules,
  isUlid,
  mailConfigBaseline,
  mailConfigSeed,
  makeCompany,
  makeJob,
  mockAuthConfig,
  nextUlid,
  policies as seedPolicies,
  revisionsOf,
  roles as roleMeta,
  setIf,
  settings as seedSettings,
  users as seedUsers,
  jobs as allJobs,
} from '@/api/mock/data'

/** 演示操作者：真实身份来自令牌，mockRequest 只拿到 authed 布尔值，因此固定为第一个演示用户 */
const DEMO_UA =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36'

/**
 * 演示态：写操作直接改内存副本，刷新页面即还原。
 *
 * 单一事实来源约定：
 *   - 收集范围只存在 settings 里的 crawler.scope 一行（真实后端也是这样，Scope 是从 Setting 读出来的）
 *   - 权限只存在 policies（casbin 三元组），角色的 permissions/user_count 都从它现算
 *   - 公司的 job_count 不入库，按 approved 岗位实时统计
 * 这样界面上的数字永远不会和列表对不上。
 */
const db = {
  jobs: allJobs.map((job) => ({ ...job })),
  revisions: new Map(allJobs.map((job) => [job.id, revisionsOf(job)])),
  companies: companies.map((company) => ({ ...company })),
  users: seedUsers.map((user) => ({ ...user, roles: [...user.roles] })),
  policies: seedPolicies.map((row) => [...row]),
  groupingRules: groupingRules.map((row) => [...row]),
  settings: seedSettings.map((setting) => ({ ...setting, value: { ...setting.value } })),
  runs: crawlRuns.map((run) => ({ ...run })),
  logs: auditLogs.map((log) => ({ ...log })),
  // 邮件配置：override 为 null 表示「没有后台覆盖」，回落到配置文件基线
  mail: { override: cloneMail(mailConfigSeed) },
  // 会话 id 也是 ULID（真实后端由会话管理器生成，前端只原样回传）
  sessions: [
    {
      id: nextUlid(),
      ip: '10.0.0.12',
      user_agent: DEMO_UA,
      created_at: new Date(Date.now() - 40 * 60 * 1000).toISOString(),
      expires_at: new Date(Date.now() + 7 * 24 * 3600 * 1000).toISOString(),
    },
    {
      id: nextUlid(),
      ip: '10.0.0.44',
      user_agent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15',
      created_at: new Date(Date.now() - 32 * 3600 * 1000).toISOString(),
      expires_at: new Date(Date.now() + 6 * 24 * 3600 * 1000).toISOString(),
    },
  ],
  // 最近一次成功登录/注册的邮箱：/auth/refresh 用它换发新令牌（mock 无法校验签名）
  identityEmail: demoAccount.email,
}

db.logs = auditLogs.map((log) => ({ ...log }))

/**
 * 令牌里回传的会话 id：必须与 /me/sessions 返回的第一条会话一致（真实后端就是这样）。
 * 这里在加载期取一次固定值，避免删除会话后再签发令牌时读到空数组。
 */
const primarySessionId = db.sessions[0].id

/* -------------------------------------------------------------------------- */
/* 小工具                                                                      */
/* -------------------------------------------------------------------------- */

function cloneMail(config) {
  return { ...config, smtp: { ...config.smtp } }
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function nowIso() {
  return new Date().toISOString()
}

/**
 * ULID 排序：id 是 26 位 Crockford Base32，字典序即时间序（前 48 位是毫秒时间戳）。
 *
 * 为什么必须单独抽一个：过去写的是 `b.id - a.id`，在字符串 id 上会算成 NaN，
 * 而 `Array.prototype.sort` 拿到 NaN 时**不报错也不排序**——列表顺序会静默乱掉。
 * 也正因为它等价于「创建时间倒序」，原来所有「id 越大越新」的语义都落在这里。
 */
function byIdDesc(a, b) {
  if (a.id === b.id) return 0
  return a.id < b.id ? 1 : -1
}

function runeLength(text) {
  return Array.from(String(text ?? '')).length
}

/** UTF-8 字节长度：bcrypt 只处理前 72 字节，后端会按字节数拒绝超长密码 */
function byteLength(text) {
  let bytes = 0
  for (const ch of String(text ?? '')) {
    const code = ch.codePointAt(0)
    if (code <= 0x7f) bytes += 1
    else if (code <= 0x7ff) bytes += 2
    else if (code <= 0xffff) bytes += 3
    else bytes += 4
  }
  return bytes
}

function hostOf(rawUrl) {
  try {
    return new URL(String(rawUrl)).host
  } catch {
    return ''
  }
}

/** 与后端 looksLikeEmail 同一口径：含 @、域名有点、无空白 */
function looksLikeEmail(email) {
  const text = String(email ?? '').trim()
  if (!text || /\s/.test(text)) return false
  const at = text.lastIndexOf('@')
  if (at <= 0 || at === text.length - 1) return false
  const domain = text.slice(at + 1)
  return domain.includes('.') && !domain.startsWith('.') && !domain.endsWith('.')
}

function includesFold(haystack, needle) {
  return String(haystack ?? '')
    .toLowerCase()
    .includes(String(needle ?? '').toLowerCase())
}

/**
 * 单值参数归一化。
 * 真实后端的查询串里一个参数只可能是字符串，但前端有时会直接把数组交下来
 * （例如「状态」这种多选），因此这里两种形态都接受。
 */
function firstValue(value) {
  if (Array.isArray(value)) return value.length ? String(value[0]) : ''
  return value === undefined || value === null ? '' : String(value)
}

/**
 * 多值参数归一化：逗号分隔字符串或数组都接受。
 *
 * 关键语义：**空列表等于「不筛选」，而不是「谁都不要」**。
 * 真实接口收到空的 status 参数时会忽略它；早期实现把空数组当成「没有任何状态匹配」，
 * 结果一个筛选条件都没选时列表反而是空的。
 */
function listValue(value) {
  if (Array.isArray(value)) return value.filter((item) => item !== '' && item !== null).map(String)
  const text = value === undefined || value === null ? '' : String(value)
  return text ? text.split(',').filter(Boolean) : []
}

/** 对齐 handler.queryInt：非法输入回落默认值，而不是 NaN */
function queryInt(value, fallback) {
  const text = firstValue(value).trim()
  if (!/^[+-]?\d+$/.test(text)) return fallback
  const parsed = Number(text)
  return Number.isFinite(parsed) ? parsed : fallback
}

/**
 * 对齐 handler.queryBool：解析失败等同于「不过滤」。
 * 之所以不写 `value === 'true'`：Go 的 strconv.ParseBool 还接受 1/0/t/f 等写法，
 * 前端一旦传 "1" 就会出现「演示里不生效、真后端生效」的诡异差异。
 */
function queryBool(value) {
  const text = firstValue(value).trim()
  if (text === '') return null
  if (['1', 't', 'T', 'TRUE', 'true', 'True'].includes(text)) return true
  if (['0', 'f', 'F', 'FALSE', 'false', 'False'].includes(text)) return false
  return null
}

function badRequest(message, details) {
  return new ApiError({ code: 10001, message, details: details ?? null, status: 400 })
}

/**
 * 分页：口径抄自 service.paginate + response.NewPage。
 *
 * 两个容易抄错的点：
 *   - page_size 上限 100、缺省 20（不是每条路由各写一个默认值）
 *   - total=0 时 pages 是 **0**（`(0 + size - 1) / size`），不是 1
 */
function paginate(items, query = {}) {
  let page = queryInt(query.page, 1)
  if (page <= 0) page = 1
  let pageSize = queryInt(query.page_size, 20)
  if (pageSize <= 0) pageSize = 20
  if (pageSize > 100) pageSize = 100
  const start = (page - 1) * pageSize
  const total = items.length
  return {
    items: items.slice(start, start + pageSize),
    total,
    page,
    page_size: pageSize,
    pages: pageSize > 0 ? Math.ceil(total / pageSize) : 0,
  }
}

function pageOf(items, query, mapper) {
  const page = paginate(items, query)
  return {
    items: mapper ? page.items.map(mapper) : page.items,
    total: page.total,
    page: page.page,
    page_size: page.page_size,
    pages: page.pages,
  }
}

/* -------------------------------------------------------------------------- */
/* 视图组装（每个函数 = 一个 DTO）                                               */
/* -------------------------------------------------------------------------- */

/**
 * 按 id 找公司/岗位。
 *
 * id 是不透明字符串（ULID），因此一律用 `String(a) === String(b)` 比较：
 * 旧的 `company.id === Number(id)` 有两个静默陷阱 —— `Number('')` 是 0、
 * `Number('abc')` 是 NaN，后者与任何 id 都不相等，于是「查不到」和「参数非法」
 * 这两条完全不同的分支会被混成同一个 404。
 */
function findCompanyRecord(id) {
  const target = String(id ?? '')
  return db.companies.find((company) => String(company.id) === target) || null
}

function findJobRecord(id) {
  const target = String(id ?? '')
  return db.jobs.find((job) => String(job.id) === target) || null
}

function companyOf(job) {
  return findCompanyRecord(job.company_id) ?? { id: job.company_id, name: '', verified: false }
}

/** 已发布岗位数（真实后端由 CompanyService.RefreshJobCount 维护，只统计 approved） */
function jobCountOf(companyId) {
  const target = String(companyId ?? '')
  return db.jobs.filter((job) => String(job.company_id) === target && job.status === 'approved').length
}

/** CompanyBriefView */
function briefView(company) {
  const view = { id: company.id, name: company.name }
  if (company.short_name) view.short_name = company.short_name
  if (company.logo_url) view.logo_url = company.logo_url
  if (company.industry) view.industry = company.industry
  if (company.location) view.location = company.location
  view.verified = Boolean(company.verified)
  return view
}

/** CompanyView（管理端）；job_count 现算，绝不写死 */
function companyView(company) {
  const view = {
    id: company.id,
    name: company.name,
    short_name: company.short_name,
    website: company.website,
    careers_url: company.careers_url,
    discovery_url: company.discovery_url,
    logo_url: company.logo_url,
    industry: company.industry,
    sub_industry: company.sub_industry,
    scale: company.scale,
    nature: company.nature,
    location: company.location,
    description: company.description,
    contact_email: company.contact_email,
    contact_phone: company.contact_phone,
    enabled: company.enabled,
    verified: company.verified,
    priority: company.priority,
    crawl_enabled: company.crawl_enabled,
    crawl_interval_seconds: company.crawl_interval_seconds,
    force_headless: company.force_headless,
    respect_robots: company.respect_robots,
    job_count: jobCountOf(company.id),
    robots_allowed: company.robots_allowed,
    source: company.source,
    created_at: company.created_at,
    updated_at: company.updated_at,
  }
  // 以下字段带 omitempty：空值必须在响应里消失（蔚来汽车就靠这个区分「从未抓取」）
  if (Object.keys(company.crawl_rules ?? {}).length) view.crawl_rules = company.crawl_rules
  setIf(view, 'last_crawled_at', company.last_crawled_at)
  setIf(view, 'last_crawl_status', company.last_crawl_status)
  setIf(view, 'last_crawl_error', company.last_crawl_error)
  setIf(view, 'robots_checked_at', company.robots_checked_at)
  setIf(view, 'remark', company.remark)
  return view
}

/**
 * JobListView。
 *
 * salary_min / salary_max 是 `*int` + omitempty：**为 null 时整个键消失**。
 * 所以这里不能用 setIf（它会把合法值 0 也吞掉），必须显式判 null。
 * 注意 salary_months 不属于列表视图 —— 它只在详情里出现。
 */
function jobListView(job) {
  const view = {
    id: job.id,
    title: job.title,
    company: briefView(companyOf(job)),
  }
  setIf(view, 'category', job.category)
  setIf(view, 'industry', job.industry)
  setIf(view, 'employment_type', job.employment_type)
  setIf(view, 'city', job.city)
  setIf(view, 'district', job.district)
  setIf(view, 'salary_text', job.salary_text)
  if (job.salary_min !== null && job.salary_min !== undefined) view.salary_min = job.salary_min
  if (job.salary_max !== null && job.salary_max !== undefined) view.salary_max = job.salary_max
  setIf(view, 'experience', job.experience)
  setIf(view, 'education', job.education)
  setIf(view, 'tags', job.tags)
  setIf(view, 'summary', job.summary)
  setIf(view, 'published_at', job.published_at)
  view.last_updated_at = job.last_updated_at
  view.last_seen_at = job.last_seen_at
  view.is_direct_hire = Boolean(job.is_direct_hire)
  setIf(view, 'apply_url', job.apply_url)
  view.source_url = job.source_url
  setIf(view, 'source_site', job.source_site)
  view.status = job.status
  view.quality_score = job.quality_score
  view.view_count = job.view_count
  return view
}

/** RevisionView */
function revisionView(row) {
  const view = { id: row.id, revision: row.revision }
  setIf(view, 'changed_fields', row.changed_fields)
  setIf(view, 'content_hash', row.content_hash)
  setIf(view, 'source_url', row.source_url)
  view.trigger = row.trigger
  setIf(view, 'snapshot', row.snapshot)
  view.created_at = row.created_at
  return view
}

function revisionsFor(job) {
  if (!db.revisions.has(job.id)) db.revisions.set(job.id, revisionsOf(job))
  return db.revisions.get(job.id)
}

/** JobDetailView = JobListView + 详情字段 + revisions */
function jobDetailView(job) {
  const view = jobListView(job)
  setIf(view, 'job_code', job.job_code)
  setIf(view, 'department', job.department)
  if (job.headcount !== null && job.headcount !== undefined) view.headcount = job.headcount
  setIf(view, 'work_mode', job.work_mode)
  if (job.salary_months !== null && job.salary_months !== undefined) view.salary_months = job.salary_months
  setIf(view, 'country', job.country)
  setIf(view, 'province', job.province)
  setIf(view, 'address', job.address)
  setIf(view, 'location_text', job.location_text)
  setIf(view, 'description', job.description)
  setIf(view, 'responsibilities', job.responsibilities)
  setIf(view, 'requirements', job.requirements)
  setIf(view, 'requirements_list', job.requirements_list)
  setIf(view, 'benefits', job.benefits)
  setIf(view, 'contact_person', job.contact_person)
  setIf(view, 'contact_email', job.contact_email)
  setIf(view, 'contact_phone', job.contact_phone)
  setIf(view, 'contact_wechat', job.contact_wechat)
  setIf(view, 'apply_method', job.apply_method)
  setIf(view, 'deadline_at', job.deadline_at)
  setIf(view, 'source_updated_at', job.source_updated_at)
  view.crawled_at = job.crawled_at
  view.revision = job.revision
  setIf(view, 'extractor', job.extractor)
  view.parse_confidence = job.parse_confidence
  setIf(view, 'source_company_name', job.source_company_name)
  if (job.reviewer_id !== null && job.reviewer_id !== undefined) view.reviewer_id = job.reviewer_id
  setIf(view, 'reviewed_at', job.reviewed_at)
  setIf(view, 'reject_reason', job.reject_reason)
  setIf(view, 'expired_at', job.expired_at)
  view.created_at = job.created_at
  view.updated_at = job.updated_at
  const revisions = revisionsFor(job)
  if (revisions.length) view.revisions = revisions.map(revisionView)
  return view
}

/** UserView：没有 role_labels / tone / initial，头像是前端派生的 */
function userView(user) {
  const view = {
    id: user.id,
    email: user.email,
    nickname: user.nickname,
    avatar_url: user.avatar_url,
    status: user.status,
    email_verified: user.email_verified,
    roles: [...user.roles],
    created_at: user.created_at,
    updated_at: user.updated_at,
  }
  setIf(view, 'phone', user.phone)
  setIf(view, 'last_login_at', user.last_login_at)
  setIf(view, 'remark', user.remark)
  return view
}

/** SessionView：字段是 id/ip/user_agent/created_at/expires_at，没有 device/current */
function sessionView(session) {
  return {
    id: session.id,
    ip: session.ip,
    user_agent: session.user_agent,
    created_at: session.created_at,
    expires_at: session.expires_at,
  }
}

/** RoleView：permissions 是 casbin 三元组数组，user_count 现算 */
function roleViews() {
  return roleMeta
    .slice()
    .sort((a, b) => b.level - a.level)
    .map((role) => {
      const view = {
        name: role.name,
        display_name: role.display_name,
        description: role.description,
        is_system: role.is_system,
        level: role.level,
      }
      const permissions = db.policies.filter((row) => row[0] === role.name).map((row) => [...row])
      setIf(view, 'permissions', permissions)
      const userCount = db.users.filter((user) => user.roles.includes(role.name)).length
      if (userCount > 0) view.user_count = userCount
      return view
    })
}

/** SettingView：value 恒为 JSON 对象（标量配置在这个系统里不存在） */
function settingView(setting) {
  const view = {
    id: setting.id,
    key: setting.key,
    value: { ...setting.value },
    is_system: setting.is_system,
    updated_at: setting.updated_at,
  }
  setIf(view, 'description', setting.description)
  setIf(view, 'group', setting.group)
  setIf(view, 'updated_by', setting.updated_by)
  return view
}

/** AuditLogView：actor_email 而不是 actor，detail 是对象 */
function auditView(log) {
  const view = {
    id: log.id,
    action: log.action,
    status_code: log.status_code,
    success: log.success,
    created_at: log.created_at,
  }
  if (log.actor_id !== null && log.actor_id !== undefined) view.actor_id = log.actor_id
  setIf(view, 'actor_email', log.actor_email)
  setIf(view, 'resource', log.resource)
  setIf(view, 'resource_id', log.resource_id)
  setIf(view, 'method', log.method)
  setIf(view, 'path', log.path)
  setIf(view, 'ip', log.ip)
  setIf(view, 'user_agent', log.user_agent)
  setIf(view, 'detail', log.detail)
  return view
}

/** CrawlRunView */
function runView(run) {
  const view = {
    id: run.id,
    company_id: run.company_id,
    trigger: run.trigger,
    status: run.status,
    started_at: run.started_at,
    duration_ms: run.duration_ms,
    pages_fetched: run.pages_fetched,
    jobs_found: run.jobs_found,
    jobs_created: run.jobs_created,
    jobs_updated: run.jobs_updated,
    jobs_unchanged: run.jobs_unchanged,
    jobs_filtered: run.jobs_filtered,
    jobs_expired: run.jobs_expired,
    used_headless: run.used_headless,
  }
  setIf(view, 'company_name', run.company_name)
  setIf(view, 'skip_reason', run.skip_reason)
  setIf(view, 'careers_url', run.careers_url)
  setIf(view, 'finished_at', run.finished_at)
  setIf(view, 'error_message', run.error_message)
  if (run.detail && Object.keys(run.detail).length) view.detail = run.detail
  return view
}

/** CrawlLogView */
function crawlLogView(log) {
  const view = {
    id: log.id,
    run_id: log.run_id,
    level: log.level,
    message: log.message,
    created_at: log.created_at,
  }
  setIf(view, 'stage', log.stage)
  setIf(view, 'url', log.url)
  if (log.context && Object.keys(log.context).length) view.context = log.context
  return view
}

/**
 * StatsView：**嵌套结构**，不是一堆 jobs_total / fetch_trend 平铺字段。
 * 每个数字都从内存数组现算，保证与列表接口返回的 total 完全一致。
 */
function statsView() {
  const jobCount = (status) => db.jobs.filter((job) => job.status === status).length
  const userCount = (status) => db.users.filter((user) => user.status === status).length
  const startToday = new Date()
  startToday.setHours(0, 0, 0, 0)
  return {
    users: {
      total: db.users.length,
      active: userCount('active'),
      disabled: userCount('disabled'),
      locked: userCount('locked'),
    },
    jobs: {
      total: db.jobs.length,
      pending_review: jobCount('pending_review'),
      approved: jobCount('approved'),
      rejected: jobCount('rejected'),
      offline: jobCount('offline'),
      expired: jobCount('expired'),
    },
    companies: {
      total: db.companies.length,
      enabled: db.companies.filter((company) => company.enabled).length,
      crawl_enabled: db.companies.filter((company) => company.crawl_enabled).length,
      verified: db.companies.filter((company) => company.verified).length,
    },
    crawl: {
      runs_total: db.runs.length,
      runs_running: db.runs.filter((run) => run.status === 'running').length,
      runs_failed: db.runs.filter((run) => run.status === 'failed').length,
      runs_today: db.runs.filter((run) => new Date(run.started_at) >= startToday).length,
    },
    authorization: authzStats(),
  }
}

/** Authz.Stats() → 策略规模 */
function authzStats() {
  return { policies: db.policies.length, grouping_rules: db.groupingRules.length }
}

/**
 * HealthView：`{status, postgres, redis, details:{authorization, crawler}}`。
 * 早期 mock 的 version/uptime/checks[] 全是编的 —— 探针接口没有那些字段。
 */
function healthView() {
  return {
    status: 'ok',
    postgres: 'up',
    redis: 'up',
    details: {
      authorization: authzStats(),
      crawler: true,
    },
  }
}

/** MetaView：工作类型等来自收集范围（可被后台改），状态枚举是静态的 */
function metaView() {
  const scope = currentScope()
  return {
    job_types: [...scope.job_types],
    industries: [...scope.industries],
    employment_types: [...scope.employment_types],
    locations: [...scope.locations],
    ...META_ENUMS,
  }
}

function scopeSetting() {
  return db.settings.find((setting) => setting.key === 'crawler.scope') ?? null
}

function currentScope() {
  const setting = scopeSetting()
  const scope = setting?.value ?? {}
  return {
    job_types: scope.job_types ?? [],
    industries: scope.industries ?? [],
    employment_types: scope.employment_types ?? [],
    locations: scope.locations ?? [],
    keywords: scope.keywords ?? [],
    exclude_keywords: scope.exclude_keywords ?? [],
    min_quality_score: scope.min_quality_score ?? 0,
  }
}

/**
 * 当前**生效**的邮件配置：口径抄自 `service.ResolveMailConfig` + `decodeMailSetting`。
 *
 * 两处容易抄错的地方：
 *   - 没有数据库覆盖时回落到配置基线（`mailConfigBaseline`），source 因此是 config；
 *   - 有覆盖时**不是整体替换**：driver / from_name / from_address 只在非空时才覆盖基线
 *     （空值表示「沿用配置文件」），而 host / port / username / tls 是整段覆盖，
 *     允许把 host 清空表示「尚未配置 SMTP」。写成 `override ?? baseline` 整体替换，
 *     会让「后台把发件人邮箱留空」这种保存结果与真后端不一致。
 */
function effectiveMailConfig() {
  const override = db.mail.override
  if (!override) return { config: cloneMail(mailConfigBaseline), source: 'config' }
  const config = cloneMail(mailConfigBaseline)
  if (override.driver) config.driver = override.driver
  if (override.from_name) config.from_name = override.from_name
  if (override.from_address) config.from_address = override.from_address
  config.smtp.host = override.smtp.host ?? ''
  config.smtp.port = override.smtp.port ?? 0
  config.smtp.username = override.smtp.username ?? ''
  config.smtp.password = override.smtp.password ?? ''
  config.smtp.tls = Boolean(override.smtp.tls)
  return { config, source: 'database' }
}

/**
 * SystemMailStatus（`internal/service/meta.go` 的 systemMailStatus）。
 *
 * driver 归一化后为空串要回落到 console（而不是原样返回空串）；
 * smtp_configured 只在 driver=smtp 且 host / from_address 都非空时为真 ——
 * 这正是「注册通道是否可用」的一半依据（register_need_verify=true 时）。
 * ready 在演示环境恒为 true：mock 没有真实投递器，「配置残缺到建不出投递器」
 * 那条分支离线无法复现。
 */
function systemMailStatus() {
  const { config, source } = effectiveMailConfig()
  const driver = String(config.driver ?? '').trim().toLowerCase() || 'console'
  return {
    driver,
    source,
    ready: true,
    smtp_configured:
      driver === 'smtp' &&
      String(config.smtp.host ?? '').trim() !== '' &&
      String(config.from_address ?? '').trim() !== '',
  }
}

/**
 * MailConfigView：**扁平结构**，没有嵌套 smtp 对象。
 *
 * 密码永远不出现在响应里：只暴露 has_password 与固定的 password_mask。
 * source=database 时才带 updated_by / updated_at（Go 对这两个字段是 omitempty 的可空时间/字符串）。
 */
function mailView() {
  const fromDatabase = Boolean(db.mail.override)
  const { config } = effectiveMailConfig()
  const view = {
    driver: config.driver,
    from_name: config.from_name,
    from_address: config.from_address,
    host: config.smtp.host,
    port: config.smtp.port,
    username: config.smtp.username,
    tls: config.smtp.tls,
    has_password: Boolean(config.smtp.password),
    password_mask: MAIL_PASSWORD_MASK,
    source: fromDatabase ? 'database' : 'config',
    // 投递器名字：console/smtp；配置残缺到建不出投递器时真实后端会返回 none
    active_driver: config.driver === 'smtp' ? 'smtp' : 'console',
    ready: true,
  }
  if (fromDatabase) {
    // updated_by / updated_at 只存在于数据库覆盖层，因此从 override 读而不是从合并结果读
    setIf(view, 'updated_by', db.mail.override.updated_by)
    setIf(view, 'updated_at', db.mail.override.updated_at)
  }
  return view
}

/* -------------------------------------------------------------------------- */
/* 筛选                                                                        */
/* -------------------------------------------------------------------------- */

/**
 * 岗位筛选，语义逐条对齐 service.JobService.List。
 *
 * 几个必须踩准的点：
 *   - 关键词匹配 title / summary / requirements / **source_company_name**，
 *     不是 company.name（来源页上的公司名可以和库里的公司名不一致）；
 *   - 薪资是**区间相交**，而且只填了下限/上限的岗位要用另一端兜底比较；
 *   - order_by=published_at 时未标注发布时间的岗位排**最后**（Postgres 默认是 NULLS FIRST）。
 */
function jobFilter(query, { publicOnly }) {
  const statuses = listValue(query.status)
  const keyword = firstValue(query.keyword).trim()
  const category = firstValue(query.category).trim()
  const industry = firstValue(query.industry).trim()
  const employmentType = firstValue(query.employment_type).trim()
  const experience = firstValue(query.experience).trim()
  const city = firstValue(query.city).trim()
  // 标识类过滤器是字符串（handler.queryString）：空串表示不过滤，非空则精确匹配。
  // 绝不能用 queryInt：company_id 传 "abc" 时后端会去查这个不存在的 ULID（结果为空），
  // 而不是像旧的 mock 那样把 NaN 当成「不过滤」把全部岗位返回。
  const companyId = firstValue(query.company_id).trim()
  const directHire = queryBool(query.is_direct_hire)
  const hasQuality = firstValue(query.min_quality_score) !== ''
  const minQuality = queryInt(query.min_quality_score, 0)
  const hasSalaryMin = queryInt(query.salary_min, 0) > 0
  const salaryMin = queryInt(query.salary_min, 0)
  const hasSalaryMax = queryInt(query.salary_max, 0) > 0
  const salaryMax = queryInt(query.salary_max, 0)

  const list = db.jobs.filter((job) => {
    if (publicOnly) {
      if (job.status !== 'approved') return false
    } else if (statuses.length && !statuses.includes(job.status)) {
      return false
    }
    if (keyword) {
      const hit =
        includesFold(job.title, keyword) ||
        includesFold(job.summary, keyword) ||
        includesFold(job.requirements, keyword) ||
        includesFold(job.source_company_name, keyword)
      if (!hit) return false
    }
    if (category && job.category !== category) return false
    if (industry && job.industry !== industry) return false
    if (employmentType && job.employment_type !== employmentType) return false
    // 经验/城市都是包含匹配，兼容「3-5年」「北京市」这类写法差异
    if (experience && !includesFold(job.experience, experience)) return false
    if (city && !includesFold(job.city, city)) return false
    if (companyId && String(job.company_id) !== companyId) return false
    if (directHire !== null && Boolean(job.is_direct_hire) !== directHire) return false
    if (hasQuality && job.quality_score < minQuality) return false
    if (hasSalaryMin) {
      // 岗位薪资上限达到下限即命中；只填了下限的岗位用下限兜底比较
      const hit =
        (job.salary_max !== null && job.salary_max >= salaryMin) ||
        (job.salary_max === null && job.salary_min !== null && job.salary_min >= salaryMin)
      if (!hit) return false
    }
    if (hasSalaryMax) {
      const hit =
        (job.salary_min !== null && job.salary_min <= salaryMax) ||
        (job.salary_min === null && job.salary_max !== null && job.salary_max <= salaryMax)
      if (!hit) return false
    }
    return true
  })

  const orderBy = firstValue(query.order_by).trim()
  const timeOf = (value) => (value ? new Date(value).getTime() : null)
  return [...list].sort((a, b) => {
    if (orderBy === 'published_at') {
      const left = timeOf(a.published_at)
      const right = timeOf(b.published_at)
      if (left === null && right === null) return byIdDesc(a, b)
      if (left === null) return 1
      if (right === null) return -1
      return right - left || byIdDesc(a, b)
    }
    if (orderBy === 'quality_score') return b.quality_score - a.quality_score || byIdDesc(a, b)
    if (orderBy === 'created_at') return byIdDesc(a, b)
    return timeOf(b.last_updated_at) - timeOf(a.last_updated_at) || byIdDesc(a, b)
  })
}

/** 用户列表：keyword 匹配 email/nickname，status 精确，role 走角色表 */
function userFilter(query) {
  const keyword = firstValue(query.keyword).trim()
  const status = firstValue(query.status).trim()
  const role = firstValue(query.role).trim()
  let list = db.users.filter((user) => {
    if (keyword && !(includesFold(user.email, keyword) || includesFold(user.nickname, keyword))) return false
    if (status && user.status !== status) return false
    if (role && !user.roles.includes(role)) return false
    return true
  })
  const orderBy = firstValue(query.order_by).trim()
  if (orderBy === 'email') {
    list = [...list].sort((a, b) => a.email.localeCompare(b.email))
  } else if (orderBy === 'last_login_at') {
    // Postgres 的 DESC 默认是 NULLS FIRST：没登录过的用户反而排最前
    const timeOf = (value) => (value ? new Date(value).getTime() : null)
    list = [...list].sort((a, b) => {
      const left = timeOf(a.last_login_at)
      const right = timeOf(b.last_login_at)
      if (left === null && right === null) return byIdDesc(a, b)
      if (left === null) return -1
      if (right === null) return 1
      return right - left || byIdDesc(a, b)
    })
  } else {
    list = [...list].sort(byIdDesc)
  }
  return list
}

/** 公司列表：keyword 覆盖 name/short_name/website，排序字段非法时后端会报 10001 */
function companyFilter(query, { publicOnly }) {
  const keyword = firstValue(query.keyword).trim()
  const industry = firstValue(query.industry).trim()
  const enabled = queryBool(query.enabled)
  const crawlEnabled = queryBool(query.crawl_enabled)
  const verified = queryBool(query.verified)
  const orderBy = firstValue(query.order_by).trim().toLowerCase()

  let list = db.companies.filter((company) => {
    if (publicOnly && !company.enabled) return false
    if (keyword) {
      const hit = includesFold(company.name, keyword) || includesFold(company.short_name, keyword) || includesFold(company.website, keyword)
      if (!hit) return false
    }
    if (industry && company.industry !== industry) return false
    if (enabled !== null && company.enabled !== enabled) return false
    if (crawlEnabled !== null && company.crawl_enabled !== crawlEnabled) return false
    if (verified !== null && company.verified !== verified) return false
    return true
  })

  const byPriority = (a, b) => b.priority - a.priority || byIdDesc(a, b)
  switch (orderBy) {
    case '':
    case 'priority':
      list = [...list].sort(byPriority)
      break
    case 'created_at':
      list = [...list].sort((a, b) => new Date(b.created_at) - new Date(a.created_at) || byIdDesc(a, b))
      break
    case 'last_crawled_at':
      list = [...list].sort((a, b) => {
        const left = a.last_crawled_at ? new Date(a.last_crawled_at).getTime() : null
        const right = b.last_crawled_at ? new Date(b.last_crawled_at).getTime() : null
        if (left === null && right === null) return byIdDesc(a, b)
        if (left === null) return 1 // 从未抓取的排最后（NULLS LAST）
        if (right === null) return -1
        return right - left || byIdDesc(a, b)
      })
      break
    case 'job_count':
      list = [...list].sort((a, b) => jobCountOf(b.id) - jobCountOf(a.id) || byIdDesc(a, b))
      break
    default:
      throw badRequest('排序字段不支持，可选 priority/created_at/last_crawled_at/job_count', {
        field: 'order_by',
        value: firstValue(query.order_by),
      })
  }
  return list
}

/**
 * 路径参数校验：口径抄自 handler.pathID + ulidid.Valid。
 *
 * 所有实体主键都是 26 位 ULID 字符串，因此「必须是正整数」的老规则已经不存在了：
 * 格式非法（长度不对、含 I/L/O/U、首字符大于 '7' 导致 128 位溢出）一律在进 service
 * 之前返回 **10001（HTTP 400）**，文案与后端逐字一致；
 * 而**格式合法但查不到**是另一条分支，由各 requireXxx 抛 30003/30001/40007/20014 的 404。
 * 把两者混成一个错误码，正是「演示能跑、联调对不上」的典型来源。
 */
function requireUlid(raw, name = 'id') {
  const text = String(raw ?? '').trim()
  if (!isUlid(text)) {
    throw badRequest(`路径参数 ${name} 必须是合法的 ULID`)
  }
  return text
}

function requireJob(id) {
  const job = findJobRecord(requireUlid(id))
  if (!job) throw new ApiError({ code: 30003, message: '岗位不存在', status: 404 })
  return job
}

function requireCompany(id) {
  const company = findCompanyRecord(requireUlid(id))
  if (!company) throw new ApiError({ code: 30001, message: '公司不存在', status: 404 })
  return company
}

function requireUser(id) {
  const target = requireUlid(id)
  const user = db.users.find((item) => String(item.id) === target)
  if (!user) throw new ApiError({ code: 20014, message: '用户不存在', status: 404 })
  return user
}

/**
 * 官网/招聘页地址校验，口径抄自 service.validateHTTPURL：
 * 空串表示「暂未填写」而放行（公司允许先录官网、招聘入口稍后由发现流程回填），
 * 非空则必须是 http/https 绝对地址，否则返回 30007。
 */
function validateHttpUrl(field, raw) {
  const text = String(raw ?? '').trim()
  if (!text) return
  let parsed = null
  try {
    parsed = new URL(text)
  } catch {
    parsed = null
  }
  if (!parsed || !parsed.host || (parsed.protocol !== 'http:' && parsed.protocol !== 'https:')) {
    const message = field === 'careers_url' ? '招聘页地址无效，必须是 http/https 绝对地址' : '官网地址无效，必须是 http/https 绝对地址'
    throw new ApiError({ code: 30007, message, details: { field, value: text }, status: 400 })
  }
}

/** companyUpdateRequest 的字段集合：请求体里的其它键会被 gin 忽略 */
const COMPANY_UPDATE_FIELDS = [
  'name',
  'short_name',
  'website',
  'careers_url',
  'logo_url',
  'industry',
  'sub_industry',
  'scale',
  'nature',
  'location',
  'description',
  'contact_email',
  'contact_phone',
  'enabled',
  'verified',
  'priority',
  'crawl_enabled',
  'crawl_interval_seconds',
  'crawl_rules',
  'force_headless',
  'respect_robots',
  'remark',
]

/* -------------------------------------------------------------------------- */
/* 认证与账号                                                                  */
/* -------------------------------------------------------------------------- */

/** 演示操作者：固定为第一个演示用户（管理员张伟） */
function currentUser() {
  return db.users[0]
}

function makeToken(email) {
  const now = Date.now()
  return {
    access_token: `demo.${btoa(String(email))}.${now}`,
    refresh_token: `demo-refresh.${btoa(String(email))}.${now}`,
    token_type: 'Bearer',
    expires_in: 7200,
    expires_at: new Date(now + 7200 * 1000).toISOString(),
    refresh_expires_at: new Date(now + 7 * 24 * 3600 * 1000).toISOString(),
    // 会话 id 与 GET /me/sessions 返回的第一条会话一致（真实后端也是从会话里带出来的）
    session_id: primarySessionId,
  }
}

/**
 * 按邮箱推断登录身份：演示环境用邮箱前缀区分权限，方便逐屏验收后台。
 *
 * 取舍说明：真实后端对不存在的邮箱一定返回 20002（不泄露账号是否存在），
 * 这里为了「任意邮箱都能进演示」而合成一个 UserView —— 这是全局唯一一处
 * 刻意放宽的地方，形状仍然严格是 UserView（没有 role_labels / tone / initial）。
 *
 * 合成用户同样必须有 ULID 主键（原来固定写 999，既不是合法 id 也会让
 * 「同一个人两次登录拿到不同 id」）。这里按邮箱缓存：同一个邮箱在一次会话里始终是同一个 id。
 */
const synthesizedUserIds = new Map()

function synthesizedUserId(email) {
  if (!synthesizedUserIds.has(email)) synthesizedUserIds.set(email, nextUlid())
  return synthesizedUserIds.get(email)
}

function identityFor(email) {
  const normalized = String(email).trim().toLowerCase()
  const known = db.users.find((user) => user.email === normalized)
  if (known) return known
  const reviewerLike = /^wang|^chen|^sunqi|^wuyan/.test(normalized)
  const now = nowIso()
  return {
    id: synthesizedUserId(normalized),
    email: normalized,
    nickname: normalized.split('@')[0],
    avatar_url: '',
    phone: '',
    status: 'active',
    email_verified: true,
    roles: reviewerLike ? ['reviewer'] : ['user'],
    last_login_at: now,
    remark: '',
    created_at: now,
    updated_at: now,
  }
}

const WEAK_PASSWORDS = new Set([
  'password',
  '12345678',
  '123456789',
  '1234567890',
  'qwertyui',
  'qwerty123',
  'abc123456',
  'admin123',
  'iloveyou',
  '11111111',
  'password1',
  'passw0rd',
  'a1234567',
  '1qaz2wsx',
  'woaini1314',
])

/**
 * 密码强度校验，规则与文案抄自 `internal/auth/password.go`。
 * 顺序也必须一致：短信/前端会把 details.problems 直接拼成一句话展示。
 */
function validatePassword(plain, minLength = 8) {
  const text = String(plain ?? '')
  const problems = []
  if (runeLength(text) < minLength) problems.push(`长度至少 ${minLength} 位`)
  if (byteLength(text) > 72) problems.push('长度不得超过 72 字节')
  if (text !== text.trim()) problems.push('首尾不能包含空白字符')
  if (!/\p{L}/u.test(text)) problems.push('需包含至少一个字母')
  if (!/\d/.test(text)) problems.push('需包含至少一个数字')
  if (WEAK_PASSWORDS.has(text.toLowerCase())) problems.push('不能使用常见弱密码')
  if (problems.length) {
    throw new ApiError({ code: 20011, message: '密码强度不足', details: { problems }, status: 400 })
  }
}

/** 验证码固定为 135790：演示环境等价于打开 auth.expose_code_in_response 的联调模式 */
const DEMO_CODE = '135790'
const CODE_PURPOSES = ['register', 'reset_password', 'change_email', 'login']

function sendCodeResult(email, purpose) {
  const known = db.users.some((user) => user.email === email)
  if (purpose === 'register' && known) {
    throw new ApiError({ code: 20001, message: '该邮箱已被注册', status: 409 })
  }
  // 找回密码/登录发码不暴露邮箱是否注册：话术与 expires_in 都和「已注册」保持一致地返回
  if ((purpose === 'reset_password' || purpose === 'login') && !known) {
    return {
      email,
      purpose,
      expires_in: 0,
      message: '如果该邮箱已注册，验证码已发送，请查收邮件',
    }
  }
  return {
    email,
    purpose,
    expires_in: 600,
    // 真实配置里 expose_code_in_response=false（生产不回显）；演示环境刻意打开，
    // 否则注册页拿不到验证码，整条注册链路在演示里走不通。
    debug_code: DEMO_CODE,
    message: '验证码已发送，请查收邮件',
  }
}

/* -------------------------------------------------------------------------- */
/* 审计留痕                                                                    */
/* -------------------------------------------------------------------------- */

/**
 * 追加一条审计记录。
 *
 * detail 是**对象**（后端是 map[string]any），不是一句中文；
 * 系统任务（无 actor_id）必须让 actor_id 整个键消失，否则前端会把 0 当成一个用户 id。
 * 参数名用 operator 而不是 actor：响应里的操作人是 `actor_email`，
 * 曾经有一版把它写成 `actor`，保持命名区分可以避免这类串味。
 */
function recordAudit({ action, resource, resourceId, detail, method = 'POST', path = '', operator = currentUser(), statusCode = 200, success = true }) {
  db.logs.unshift({
    id: nextUlid(),
    actor_id: success && operator ? operator.id : null,
    actor_email: operator?.email ?? '',
    action,
    resource,
    resource_id: String(resourceId ?? ''),
    method,
    path: path || `/api/v1/admin/${resource}`,
    status_code: statusCode,
    success,
    ip: '10.0.0.12',
    user_agent: DEMO_UA,
    detail: detail ?? {},
    created_at: nowIso(),
  })
}

/* -------------------------------------------------------------------------- */
/* 写操作辅助                                                                  */
/* -------------------------------------------------------------------------- */

/** JobDetailView 里可人工编辑的字段（= service.JobEditInput，handler 用类型别名直接复用） */
const JOB_EDIT_FIELDS = [
  'title',
  'category',
  'industry',
  'employment_type',
  'department',
  'headcount',
  'experience',
  'education',
  'work_mode',
  'salary_text',
  'salary_min',
  'salary_max',
  'salary_months',
  'country',
  'province',
  'city',
  'district',
  'address',
  'location_text',
  'summary',
  'description',
  'responsibilities',
  'requirements',
  'requirements_list',
  'benefits',
  'tags',
  'contact_person',
  'contact_email',
  'contact_phone',
  'contact_wechat',
  'apply_url',
  'apply_method',
  'published_at',
  'deadline_at',
]

/**
 * 施加人工编辑，模仿 `JobService.applyEdits`：
 *   - 只处理请求体里出现的键（传什么改什么）
 *   - **JSON null 一律表示「不修改」**：JobEditInput 的每个字段都是指针，
 *     null 只会把指针置为 nil，而 service 里是 `if in.X != nil` 才写入。
 *     想清空某个可空字段得传 ""（字符串）或 []（数组），传 null 是没用的。
 */
function applyJobEdits(job, body) {
  for (const field of JOB_EDIT_FIELDS) {
    if (!Object.prototype.hasOwnProperty.call(body, field)) continue
    const value = body[field]
    if (value === null) continue
    if (field === 'title' && String(value).trim() === '') {
      throw badRequest('岗位名称不能为空')
    }
    job[field] = value
  }
}

/** contentFields 里发生变化的字段名（按字典序，与 changedFieldNames 一致） */
function changedJobFields(before, after) {
  return Object.keys(after)
    .filter((key) => JSON.stringify(before[key]) !== JSON.stringify(after[key]))
    .sort()
}

/** 实质性变更字段：已发布岗位命中其一就要退回待审核（revision.go substantiveFields） */
const SUBSTANTIVE_FIELDS = new Set([
  'title',
  'requirements',
  'requirements_list',
  'responsibilities',
  'salary_text',
  'employment_type',
  'address',
  'city',
])

/** 信息完整度评分：规则抄自 service.scoreOfJob，保证新建岗位的分数不是拍脑袋的 */
function scoreJob(job) {
  let score = 0
  if (String(job.title ?? '').trim()) score += 10
  if (runeLength(job.description) >= 20) score += 10
  const requirementLength = runeLength(job.requirements)
  if (requirementLength >= 50) score += 15
  else if (requirementLength >= 10) score += 8
  if ((job.requirements_list ?? []).length >= 3) score += 5
  if (runeLength(job.responsibilities) >= 20) score += 10
  if (String(job.address ?? '').trim()) score += 6
  if (String(job.city ?? '').trim()) score += 6
  if (job.published_at) score += 10
  if (job.salary_min !== null && job.salary_min !== undefined) score += 5
  else if (job.salary_max !== null && job.salary_max !== undefined) score += 5
  else if (String(job.salary_text ?? '').trim()) score += 3
  if (String(job.contact_email ?? '').trim()) score += 5
  if (String(job.contact_phone ?? '').trim()) score += 3
  if (String(job.apply_url ?? '').trim()) score += 4
  if (String(job.employment_type ?? '').trim()) score += 3
  if (String(job.category ?? '').trim()) score += 3
  if (String(job.experience ?? '').trim()) score += 3
  if (String(job.education ?? '').trim()) score += 2
  if ((job.tags ?? []).length > 0 || String(job.benefits ?? '').trim()) score += 2
  return Math.min(score, 100)
}

/**
 * 邮件配置校验：逐条复刻 `config.Mail.Validate()`。
 *
 * 文案必须一字不差（前端会直接把 message 弹出来），并且用 `；` 连接多个问题。
 * 注意错误码：后端包了一层 `apperr.ErrSettingInvalid`，它是 **50002**（HTTP 400），
 * 不是通用参数错误 10001 —— 视图里按 10001 判断会漏掉这条分支。
 */
function validateMailConfig(config) {
  if (config.driver === 'console') return
  if (config.driver !== 'smtp') {
    throw new ApiError({
      code: 50002,
      message: `邮件驱动只支持 console/smtp，当前为 "${config.driver}"`,
      status: 400,
    })
  }
  const problems = []
  if (!String(config.smtp.host ?? '').trim()) problems.push('smtp 主机不能为空')
  if (!(config.smtp.port > 0) || config.smtp.port > 65535) problems.push('smtp 端口必须在 1~65535 之间')
  if (!String(config.from_address ?? '').trim()) problems.push('发件人邮箱不能为空')
  else if (!config.from_address.includes('@')) problems.push('发件人邮箱格式不正确')
  const hasUser = String(config.smtp.username ?? '').trim() !== ''
  const hasPass = String(config.smtp.password ?? '') !== ''
  if (hasUser !== hasPass) {
    problems.push('SMTP 用户名与密码需要同时填写（若服务器无需认证请都留空）')
  }
  if (problems.length) {
    throw new ApiError({ code: 50002, message: `邮件配置有误：${problems.join('；')}`, status: 400 })
  }
}

/**
 * 收集范围写入：口径与 `SettingService.upsert` 一致。
 *
 * 注意「缺字段 = 清空」而不是「保持原值」：crawlScopeRequest 全是非指针字段，
 * 请求里没写的维度会以空数组落库，从而取消该维度限制。
 * 若最终值与现值完全相同，后端会**直接返回旧行**（不刷新 updated_at / updated_by）。
 */
function saveCrawlScope(body, operator) {
  const next = {
    job_types: Array.isArray(body?.job_types) ? body.job_types.map(String) : [],
    industries: Array.isArray(body?.industries) ? body.industries.map(String) : [],
    employment_types: Array.isArray(body?.employment_types) ? body.employment_types.map(String) : [],
    locations: Array.isArray(body?.locations) ? body.locations.map(String) : [],
    keywords: Array.isArray(body?.keywords) ? body.keywords.map(String) : [],
    exclude_keywords: Array.isArray(body?.exclude_keywords) ? body.exclude_keywords.map(String) : [],
    min_quality_score: queryInt(body?.min_quality_score, 0),
  }
  if (next.min_quality_score < 0) {
    throw new ApiError({
      code: 50002,
      message: '最低质量分不能为负数',
      details: { field: 'min_quality_score', value: next.min_quality_score },
      status: 400,
    })
  }
  const setting = scopeSetting()
  const previous = { ...setting.value }
  if (JSON.stringify(previous) === JSON.stringify(next)) {
    // 与后端一样：无实质变更时不写库、不记审计，直接返回旧行
    return settingView(setting)
  }
  setting.value = next
  setting.updated_by = operator.email
  setting.updated_at = nowIso()
  recordAudit({
    action: 'setting.update',
    resource: 'setting',
    resourceId: setting.key,
    method: 'PUT',
    path: '/api/v1/admin/crawl-scope',
    // 后端记的是 {old, new} 两段值，便于审计里直接看「改了什么」
    detail: { value: { old: previous, new: next } },
  })
  return settingView(setting)
}

/* -------------------------------------------------------------------------- */
/* 系统初始化（首次安装向导）                                                   */
/* -------------------------------------------------------------------------- */

/**
 * 可用管理员：口径抄自 `service.availableAdminIDs` —— **直接**拥有 admin 角色，且账号未被禁用。
 *
 * 两个坑：
 *   - 不能只看角色图。真实后端要用 Casbin 查一次、再逐个复查「直接拥有 admin」，
 *     正是因为角色是传递的（admin ⊃ reviewer ⊃ user），只查角色图会把审核员误判成管理员。
 *     mock 里 roles 就是直接数组，因此必须显式 `includes('admin')`；
 *   - 「可用」只排除 disabled：locked（连续登录失败被临时锁定）与 pending 仍算可用管理员，
 *     与后端的 `StatusNEQ(StatusDisabled)` 一致。
 */
function availableAdminIds() {
  return db.users
    .filter((user) => user.roles.includes('admin') && user.status !== 'disabled')
    .map((user) => user.id)
}

/**
 * DecideSetupAvailable（纯函数），文案与 `internal/service/setup.go` 逐字一致。
 *
 * 为什么「配置关闭」必须排在「已初始化」前面：两者同时成立时，后端先看配置
 * （assertSetupAvailable 在关闭时连数据库都不查），返回的也是配置那条原因。
 * 顺序反了，演示里弹出的 409 提示就与真后端不是同一句话。
 */
function setupDecision(availableAdminCount) {
  if (!mockAuthConfig.setupEnabled) {
    return { available: false, reason: '安装向导已在配置中关闭（auth.setup.enabled=false）' }
  }
  if (availableAdminCount > 0) {
    return {
      available: false,
      reason: `系统已完成初始化（已存在 ${availableAdminCount} 名可用管理员），安装向导永久失效；如需新增管理员请登录后台操作`,
    }
  }
  return { available: true, reason: '系统尚未初始化，安装向导可用' }
}

/** ErrSetupClosed：409 + 「无法使用安装向导：<原因>」（10005 = apperr.ErrConflict） */
function errSetupClosed(reason) {
  return new ApiError({ code: 10005, message: `无法使用安装向导：${reason}`, status: 409 })
}

/**
 * assertSetupAvailable：安装向导的第一道门。
 *
 * 必须在**任何校验与写入之前**执行，理由有三：
 *   1. 与 service.assertSetupAvailable 的调用位置一致（setup.go:181 在邮箱/密码校验之前），
 *      因此「已初始化 + 请求体非法」返回的是 409 而不是 10001，前端看到的分支与真后端相同；
 *   2. 系统一旦存在可用管理员，这两个接口就必须 409 —— 状态冲突优先于参数错误；
 *   3. 「已初始化后无副作用」这条规则正是靠它守住的：抛错时后面一行都没执行。
 */
function assertSetupAvailable() {
  const decision = setupDecision(availableAdminIds().length)
  if (!decision.available) throw errSetupClosed(decision.reason)
  return decision
}

/**
 * SystemStatusView（GET /system/status）。
 *
 * 每个字段都从 mock 自己的状态现算，不写死任何一个数字：
 *   - user_count 是**用户表总数**（含 pending/disabled，后端是 `User.Query().Count()`）；
 *   - admin_count / initialized 走 availableAdminIds()，与安装向导同一套口径 ——
 *     两处口径一旦分叉，就会出现「status 说没初始化、向导说已初始化」的自相矛盾；
 *   - registration_open = !register_need_verify || mail.smtp_configured，
 *     即「要求邮箱验证却没配可用 SMTP 时注册实际走不通」；
 *   - bootstrap_credential_file_exists 在真实后端是 os.Stat，这里由配置开关模拟。
 *
 * 该视图没有 omitempty 字段：所有键恒存在（公开接口，只暴露「系统是否已就绪」）。
 */
function systemStatusView() {
  const adminCount = availableAdminIds().length
  const mail = systemMailStatus()
  return {
    initialized: adminCount > 0,
    user_count: db.users.length,
    admin_count: adminCount,
    setup_available: setupDecision(adminCount).available,
    registration_open: !mockAuthConfig.registerNeedVerify || mail.smtp_configured,
    mail,
    bootstrap_credential_file_exists: mockAuthConfig.bootstrapCredentialFileExists,
  }
}

/** 与 service.defaultNickname 同口径：取邮箱前缀，没有前缀时用「用户」 */
function defaultNickname(email) {
  const at = String(email).indexOf('@')
  return at > 0 ? String(email).slice(0, at) : '用户'
}

/**
 * setupMailRequest → 邮件配置，口径抄自 `handler.setupMailRequest.toConfig`：
 * 除密码外一律 trim；密码**刻意不 trim**（service 会专门拒绝首尾带空白的授权码，
 * 在这里悄悄裁掉反而会把「复制时多带了一个空格」这个真实原因藏起来）。
 *
 * 返回 null 表示「本次不配置邮件服务」（请求体 mail 为 null / 省略），
 * 与「配置了一个空 SMTP」在 service 层是完全不同的两件事。
 */
function setupMailConfig(raw) {
  if (raw === null || raw === undefined) return null
  if (typeof raw !== 'object' || Array.isArray(raw)) {
    throw badRequest('请求参数有误', { reason: 'mail 必须是对象' })
  }
  return {
    driver: String(raw.driver ?? '').trim(),
    from_name: String(raw.from_name ?? '').trim(),
    from_address: String(raw.from_address ?? '').trim(),
    smtp: {
      host: String(raw.host ?? '').trim(),
      port: queryInt(raw.port, 0),
      username: String(raw.username ?? '').trim(),
      password: String(raw.password ?? ''),
      // TLS 是 *bool：请求里不传就等于 false（后端是 `in.TLS != nil && *in.TLS`）
      tls: raw.tls === true,
    },
  }
}

/**
 * 安装向导返回的管理员视图。
 *
 * UserView.must_change_password **没有 omitempty**，真实响应里这个键恒存在；
 * 而安装向导建出来的账号必然是 false（密码是运维当场设定的，不需要再改一次）。
 * userView() 服务的是读接口的常见形状，这里补上这个键，
 * 避免 /system/setup 的返回体比真后端少一个字段。
 */
function setupAdminView(user) {
  return { ...userView(user), must_change_password: Boolean(user.must_change_password) }
}

/**
 * 写一条安装向导的审计。
 *
 * 为什么不复用 recordAudit：安装向导用的是 SetupActor，只有 user_id 与 email，
 * **没有 method / path / ip / user_agent**；这四个字段在 AuditLogView 里都是 omitempty，
 * 真响应里因此整个键消失。recordAudit 默认会填 POST 与一条路径，
 * 那会凭空造出真后端不存在的方法与路径。
 */
function recordSetupAudit({ action, resource, resourceId, operator, detail }) {
  db.logs.unshift({
    id: nextUlid(),
    actor_id: operator.id,
    actor_email: operator.email,
    action,
    resource,
    resource_id: String(resourceId ?? ''),
    status_code: 200,
    success: true,
    detail: detail ?? {},
    created_at: nowIso(),
  })
}

/**
 * 落库邮件配置（对应 `MailService.Save` + persist）。
 *
 * 密码语义抄的是**代码**而不是 setup.go 的注释：Mail.Save 的分支是
 * 「空值或占位符 = 沿用已保存的密码」，而 setup.go 的注释说空值表示
 * 「该 SMTP 不需要认证」。两种解释对同一入参结论不同，真正生效的是 Mail.Save，
 * 因此这里按它实现；空用户名+空密码的「无需认证」场景本来就由 Validate 放行。
 *
 * 另注两点：Save 会再校验一次**合并后**的配置；审计只记可公开字段与
 * password_changed，绝不写入口令本身。
 */
function saveSetupMailConfig(input, operator) {
  const previous = db.mail.override ?? mailConfigBaseline
  const password =
    input.smtp.password === '' || input.smtp.password === MAIL_PASSWORD_MASK
      ? previous.smtp.password
      : input.smtp.password

  const merged = {
    driver: input.driver,
    from_name: input.from_name,
    from_address: input.from_address,
    smtp: {
      host: input.smtp.host,
      port: input.smtp.port,
      username: input.smtp.username,
      password,
      tls: input.smtp.tls,
    },
  }
  // Mail.Save 会再校验一次**合并后**的配置，而「用户名与密码要么都填、要么都空」
  // 这条规则可能被合并结果打破：运维清空用户名与密码表示「该 SMTP 无需认证」，
  // 但库里还留着上一版密码，合并后就成了「有密码没用户名」。
  // 此时 Save 返回 50002，SetupAdmin 把它包成 10007（账号已创建，邮件配置保存失败）。
  validateMailConfig(merged)

  db.mail.override = {
    ...merged,
    updated_by: operator.email,
    updated_at: nowIso(),
  }

  recordSetupAudit({
    action: 'mail.update',
    resource: 'setting',
    resourceId: 'mail.smtp',
    operator,
    detail: {
      driver: merged.driver,
      from_address: merged.from_address,
      host: merged.smtp.host,
      port: merged.smtp.port,
      username: merged.smtp.username,
      tls: merged.smtp.tls,
      password_changed: input.smtp.password !== '' && input.smtp.password !== MAIL_PASSWORD_MASK,
    },
  })
}

/**
 * SetupAdmin（POST /system/setup）。执行顺序刻意与 `service.SetupService.SetupAdmin` 对齐：
 *   1. handler 的 binding:"required"（缺字段是 10001，进不了 service）；
 *   2. 门控 assertSetupAvailable —— 见上面的注释，409 优先于任何参数校验；
 *   3. 邮箱 / 密码强度 / 邮件配置校验 —— **全部在任何写入之前**完成，
 *      避免「管理员建好了但邮件配置非法」这种半成品状态；
 *   4. 复查门控 + 邮箱占用 —— 对应咨询锁内的 availableAdminCountForTx 与 EmailEQ 预检。
 *      mock 是单线程，保留这一步不是为了并发安全，而是让「判定与创建之间必须再确认一次」
 *      这条规则在演示实现里同样存在；
 *   5. 建号（ULID 主键）→ 写 user.setup_admin 审计 → 可选落邮件配置 → 返回 SetupResult。
 *
 * 口令**不落任何地方**：mock 没有密码库，连字段都不建；
 * 审计 detail 与响应体里因此不可能出现任何口令材料（连哈希都没有）。
 */
function setupAdmin(body) {
  if (!body.admin_email) throw badRequest('请求参数有误', { reason: 'admin_email 为必填项' })
  if (!body.admin_password) throw badRequest('请求参数有误', { reason: 'admin_password 为必填项' })

  assertSetupAvailable()

  const email = String(body.admin_email).trim().toLowerCase() // service.normalizeEmail
  if (!looksLikeEmail(email)) throw badRequest('管理员邮箱格式不正确')
  // 密码强度必须复用与其他入口同一套规则，否则会出现「向导能设的密码在别处过不了」
  validatePassword(body.admin_password, 8)
  const mail = setupMailConfig(body.mail)
  if (mail) validateMailConfig(mail)

  // 复查：此刻仍必须「不存在可用管理员」，且邮箱没有被既有账号占用
  assertSetupAvailable()
  if (db.users.some((user) => user.email === email)) {
    throw new ApiError({
      code: 10005,
      message: `该邮箱已被既有账号占用，无法作为初始管理员：${email}（请换一个邮箱，或先清理该账号）`,
      status: 409,
    })
  }

  const now = nowIso()
  const nickname = String(body.nickname ?? '').trim() || defaultNickname(email)
  const user = {
    id: nextUlid(),
    email,
    nickname,
    avatar_url: '',
    phone: '',
    status: 'active',
    email_verified: true,
    roles: ['admin'],
    last_login_at: null,
    remark: '',
    // 以下两个是内部字段，userView() 逐字段挑选，不会进响应：
    // register_source 记 setup 便于事后与「注册」「自举」区分；向导不强制改密
    // （密码是运维自己设的，与自举生成的随机口令不是一回事）
    register_source: 'setup',
    must_change_password: false,
    created_at: now,
    updated_at: now,
  }
  db.users.push(user)

  // 审计先记「建号」：即使后面的邮件配置写入失败，管理员账号也是客观存在的，
  // 漏记会让审计与实际状态不一致。detail 只有可公开事实。
  recordSetupAudit({
    action: 'user.setup_admin',
    resource: 'user',
    resourceId: user.id,
    operator: user,
    detail: {
      email: user.email,
      register_source: user.register_source,
      must_change_password: user.must_change_password,
      email_verified: user.email_verified,
      roles: ['admin'],
      mail_configured: mail !== null,
    },
  })

  let mailConfigured = false
  if (mail) {
    // 邮件配置保存失败**不回滚账号**：账号已经可用，运维登录后台补配
    // 比把刚建好的管理员删掉更合理。后端把 Save 的错误包成 ErrInternal，
    // 因此这里同样是 500 / 10007，且 cause 不外泄（details 为空）。
    try {
      saveSetupMailConfig(mail, user)
    } catch {
      throw new ApiError({ code: 10007, message: '服务器内部错误', status: 500 })
    }
    mailConfigured = true
  }

  // registration_open 直接复用 /system/status 的结论，绝不在这里重算一遍语义
  const status = systemStatusView()
  return {
    admin: setupAdminView(user),
    mail: status.mail,
    registration_open: status.registration_open,
    mail_configured: mailConfigured,
  }
}

/**
 * 认证失败的对外文案，逐字抄自 `internal/service/mail_hint.go`（smtpAuthSteps）。
 *
 * 结构固定为「结论 + 四条通用排查步骤」，多行是有意的：前端把 message 原样渲染在
 * 测试按钮下方（`err.message` + white-space: pre-line），运维照着第 1) 条就能解决绝大多数 535。
 *
 * 服务商专属的第 5) 条（smtpHostHint 的 QQ/163/Gmail/… 表）没有搬过来：演示用主机名
 * 不在那张表里时真后端也不会输出这一条，两边文本因此逐字相同。
 */
const SMTP_AUTH_STEPS = `请按以下顺序检查
1) 密码是否是邮箱服务商生成的「授权码」——QQ/163/腾讯企业邮都必须用授权码，不能用登录密码；
2) 该账号是否已在邮箱网页端开启 SMTP/POP3 服务；
3) 账号是否填写为完整邮箱地址（如 xxx@qq.com），不要只填前缀；
4) 服务器与端口是否匹配（个人 QQ 邮箱 smtp.qq.com:465，腾讯企业邮 smtp.exmail.qq.com:465）。`

/** smtpAuthFailureMessage：与 mail_hint.go 同构——「服务器原文」原样保留，绝不改写或截断 */
function smtpAuthFailureMessage() {
  return (
    `SMTP 认证失败（服务器返回 535）：${SMTP_AUTH_STEPS}` +
    '\n服务器原文：535 "Login fail. Please use the authorization code instead of the login password."'
  )
}

/**
 * SendSetupTestMail（POST /system/setup/mail/test）。
 *
 * 三条硬性行为：
 *   - **什么都不落库**：不改 db.mail.override、不写审计、不热替换任何东西。
 *     这一步的意义正是「在正式提交之前验证参数」：先写库再试发的话，
 *     一个填错的 host/授权码就已经生效，系统立刻处于「验证码发不出去」的状态；
 *   - 校验顺序与 service 一致：门控 → 收件人 → 驱动回落 console → Mail.Validate → 密码首尾空白；
 *   - 收件人留空时回落到 auth.setup.test_mail_to（mockAuthConfig.setupTestMailTo）。
 *
 * 演示环境没有 SMTP 客户端，但**认证失败这条分支必须有办法复现**：它正是
 * 「授权码填成了登录密码」这类工单的唯一出口，前端要把多行排查步骤原样渲染出来。
 * 因此这里约定一条主机名规则（走到这一步时 driver 已回落过 console）：
 *   - console                          → 成功（保留原行为）；
 *   - 主机名含 "mock"（smtp.mock.local）→ 成功（模拟一个连得上且认证通过的 SMTP）；
 *   - 其它 smtp 主机                    → 10009 + HTTP 503 + 认证失败文案。
 */
function sendSetupTestMail(body) {
  // handler 的 binding:"required" 发生在 service 之前：mail 缺失是 10001，不是 409
  if (body.mail === null || body.mail === undefined) {
    throw badRequest('请求参数有误', { reason: 'mail 为必填项' })
  }

  assertSetupAvailable()

  const mail = setupMailConfig(body.mail)
  let to = String(body.to ?? '').trim()
  if (!to) to = String(mockAuthConfig.setupTestMailTo ?? '').trim()
  if (!looksLikeEmail(to)) throw badRequest('收件邮箱格式不正确')

  // 只有测试发信会在驱动为空时回落到 console（保存路径不做这个回落，见 setupAdmin）
  if (!mail.driver) mail.driver = 'console'
  validateMailConfig(mail)
  // 首尾空白几乎总是复制授权码时带进来的，而 SMTP 认证会因此失败（535），
  // 且失败原因完全指不到「多了个空格」这个真实原因上，所以单独拦一次
  if (mail.smtp.password !== mail.smtp.password.trim()) {
    throw new ApiError({
      code: 50002,
      message: 'SMTP 密码/授权码首尾不能包含空白字符（复制时容易多带一个空格）',
      status: 400,
    })
  }

  // 除 console 之外，只有主机名含 "mock" 的主机视为「连得上且认证通过」；
  // 其余 smtp 主机一律走真实后端那条认证失败分支（apperr.CodeMailAuthFailed → 10009 + 503）。
  if (String(mail.driver ?? '').trim().toLowerCase() === 'smtp' && !String(mail.smtp.host ?? '').toLowerCase().includes('mock')) {
    throw new ApiError({ code: 10009, message: smtpAuthFailureMessage(), status: 503 })
  }

  // 文案由 handler 拼装（固定文案，不带收件人地址）
  return { message: '测试邮件已发送，请查收（含垃圾箱）' }
}

/**
 * 把演示内存表切回「全新安装」状态，可选地覆盖配置开关。
 *
 * 为什么需要它：默认演示数据里已有管理员，/system/status 因此恒为 initialized、
 * 安装向导永远关闭 —— 这在生产上是正确默认，但离线验收 /setup 必须能走一遍
 * 「未初始化 → 提交 → 已初始化 → 再次提交被 409 拒绝」的完整链路，
 * 而这条链路无法通过任何公开接口构造（后端刻意不提供「删掉最后一名管理员」的能力）。
 * 真实契约里没有这个入口，它只服务于演示与自检脚本。
 *
 * 同时清空邮件配置覆盖层：全新安装还没有后台配置，source 因此是 config，
 * 这样 setup 里「配置 SMTP → smtp_configured 由 false 变 true → registration_open 变 true」
 * 这条因果关系才看得见。岗位/公司数据保持不变，其他页面照常可演示。
 *
 * 与其它写操作一样：改的是内存副本，**刷新页面即还原**（它不会把种子用户找回来）。
 */
export function resetMockSystemState({
  setupEnabled = mockAuthConfig.setupEnabled,
  registerNeedVerify = mockAuthConfig.registerNeedVerify,
  bootstrapCredentialFileExists = mockAuthConfig.bootstrapCredentialFileExists,
} = {}) {
  mockAuthConfig.setupEnabled = setupEnabled
  mockAuthConfig.registerNeedVerify = registerNeedVerify
  mockAuthConfig.bootstrapCredentialFileExists = bootstrapCredentialFileExists
  db.users = []
  db.logs = []
  db.mail.override = null
  db.identityEmail = ''
  return systemStatusView()
}

/* -------------------------------------------------------------------------- */
/* 路由表                                                                      */
/* -------------------------------------------------------------------------- */

const routes = [
  /* —— 认证 —— */
  [
    'POST /auth/codes',
    { auth: false },
    ({ body }) => {
      const purpose = String(body?.purpose ?? '')
      if (!body?.email) throw badRequest('请求参数有误', { reason: 'email 为必填项' })
      if (!purpose) throw badRequest('请求参数有误', { reason: 'purpose 为必填项' })
      if (!CODE_PURPOSES.includes(purpose)) {
        throw badRequest('验证码用途不合法')
      }
      const email = String(body.email).trim().toLowerCase()
      if (!looksLikeEmail(email)) throw badRequest('邮箱格式不正确')
      return sendCodeResult(email, purpose)
    },
  ],
  [
    'POST /auth/register',
    { auth: false },
    ({ body }) => {
      if (!body?.email) throw badRequest('请求参数有误', { reason: 'email 为必填项' })
      if (!body?.password) throw badRequest('请求参数有误', { reason: 'password 为必填项' })
      const email = String(body.email).trim().toLowerCase()
      if (!looksLikeEmail(email)) throw badRequest('邮箱格式不正确')
      validatePassword(body.password, 8)
      if (db.users.some((user) => user.email === email)) {
        throw new ApiError({ code: 20001, message: '该邮箱已被注册', status: 409 })
      }
      // register_need_verify=true 时验证码必填（configs/config.yaml 默认开启）
      if (!String(body.code ?? '').trim()) {
        throw new ApiError({ code: 20005, message: '请填写邮箱验证码', status: 400 })
      }
      if (String(body.code).trim() !== DEMO_CODE) {
        throw new ApiError({ code: 20005, message: '验证码错误或已过期', status: 400 })
      }
      const now = nowIso()
      const nickname = String(body.nickname ?? '').trim() || email.split('@')[0]
      const user = {
        id: nextUlid(),
        email,
        nickname,
        avatar_url: '',
        phone: '',
        status: 'active',
        email_verified: true,
        roles: ['user'],
        last_login_at: now,
        remark: '',
        created_at: now,
        updated_at: now,
      }
      db.users.push(user)
      db.identityEmail = email
      recordAudit({
        action: 'auth.register',
        resource: 'user',
        resourceId: user.id,
        path: '/api/v1/auth/register',
        operator: user,
        detail: { email, email_verified: true },
      })
      return { user: userView(user), token: makeToken(email) }
    },
  ],
  [
    'POST /auth/login',
    { auth: false },
    ({ body }) => {
      if (!body?.email) throw badRequest('请求参数有误', { reason: 'email 为必填项' })
      if (!body?.password) throw badRequest('请求参数有误', { reason: 'password 为必填项' })
      const email = String(body.email).trim().toLowerCase()
      // 账号不存在时也返回 20002（不泄露账号是否存在）；格式非法同样走这里
      if (!looksLikeEmail(email)) throw new ApiError({ code: 20002, message: '邮箱或密码错误', status: 401 })
      const user = identityFor(email)
      // 状态检查在密码校验之前：被锁定/禁用的账号即使密码正确也不能登录
      if (user.status === 'locked') {
        throw new ApiError({ code: 20010, message: '账号因多次登录失败已被临时锁定', status: 409 })
      }
      if (user.status === 'disabled') {
        throw new ApiError({ code: 20003, message: '账号已被禁用，请联系管理员', status: 403 })
      }
      if (!user.email_verified) {
        throw new ApiError({ code: 20004, message: '邮箱尚未验证', status: 403 })
      }
      // 演示环境没有密码库：≥8 位即视为密码正确
      if (String(body.password).length < 8) {
        throw new ApiError({ code: 20002, message: '邮箱或密码错误', status: 401 })
      }
      db.identityEmail = user.email
      const stored = db.users.find((item) => item.email === user.email)
      if (stored) {
        stored.last_login_at = nowIso()
        stored.updated_at = stored.last_login_at
      }
      recordAudit({ action: 'auth.login', resource: 'user', resourceId: user.id, path: '/api/v1/auth/login', operator: user, detail: {} })
      return { user: userView(stored ?? user), token: makeToken(user.email) }
    },
  ],
  [
    'POST /auth/refresh',
    { auth: false },
    ({ body }) => {
      if (!body?.refresh_token) throw badRequest('请求参数有误', { reason: 'refresh_token 为必填项' })
      // mock 无法验签：只要带了刷新令牌就换发一对新令牌（真实后端还会在这里轮换旧令牌）
      return makeToken(db.identityEmail)
    },
  ],
  [
    'POST /auth/logout',
    { auth: false },
    ({ body }) => {
      recordAudit({ action: 'auth.logout', resource: 'user', resourceId: currentUser().id, path: '/api/v1/auth/logout', detail: {} })
      // 后端返回 NoContent（信封里没有 data），因此 mock 也必须返回 undefined
      return undefined
    },
  ],
  [
    'POST /auth/password/forgot',
    { auth: false },
    ({ body }) => {
      if (!body?.email) throw badRequest('请求参数有误', { reason: 'email 为必填项' })
      const email = String(body.email).trim().toLowerCase()
      if (!looksLikeEmail(email)) throw badRequest('邮箱格式不正确')
      return sendCodeResult(email, 'reset_password')
    },
  ],
  [
    'POST /auth/password/reset',
    { auth: false },
    ({ body }) => {
      if (!body?.email) throw badRequest('请求参数有误', { reason: 'email 为必填项' })
      if (!body?.code) throw badRequest('请求参数有误', { reason: 'code 为必填项' })
      if (!body?.new_password) throw badRequest('请求参数有误', { reason: 'new_password 为必填项' })
      const email = String(body.email).trim().toLowerCase()
      if (!looksLikeEmail(email)) throw badRequest('邮箱格式不正确')
      validatePassword(body.new_password, 8)
      if (String(body.code).trim() !== DEMO_CODE) {
        throw new ApiError({ code: 20005, message: '验证码错误或已过期', status: 400 })
      }
      recordAudit({ action: 'auth.reset_password', resource: 'user', resourceId: '0', path: '/api/v1/auth/password/reset', detail: { email } })
      return { message: '密码已重置，请使用新密码登录' }
    },
  ],

  /* —— 字典与探针 —— */
  ['GET /meta/dictionaries', { auth: false }, () => metaView()],
  // /healthz、/readyz 挂在 API 前缀之外，响应体是 HealthView 本身（没有 {code,message,data} 信封）
  ['GET /healthz', { auth: false }, () => healthView()],
  ['GET /readyz', { auth: false }, () => healthView()],

  /* —— 公开岗位 —— */
  [
    'GET /jobs',
    { auth: false },
    ({ query }) => pageOf(jobFilter(query, { publicOnly: true }), query, jobListView),
  ],
  [
    'GET /jobs/:id/related',
    { auth: false },
    ({ params, query }) => {
      const job = findJobRecord(requireUlid(params.id))
      if (!job) throw new ApiError({ code: 30003, message: '岗位不存在', status: 404 })
      // limit 缺省 6；<=0 或 >20 也回落到 6
      let limit = queryInt(query.limit, 6)
      if (limit <= 0 || limit > 20) limit = 6
      const items = db.jobs
        .filter((item) => item.status === 'approved' && item.id !== job.id)
        .filter((item) => item.company_id === job.company_id || (job.category && item.category === job.category))
        .sort((a, b) => b.quality_score - a.quality_score || byIdDesc(a, b))
        .slice(0, limit)
      return { items: items.map(jobListView) }
    },
  ],
  [
    'GET /jobs/:id',
    { auth: false },
    ({ params }) => {
      const job = findJobRecord(requireUlid(params.id))
      // 未发布岗位一律按「不存在」处理，不泄露存在性
      if (!job || job.status !== 'approved') {
        throw new ApiError({ code: 30003, message: '岗位不存在', status: 404 })
      }
      // 浏览量自增发生在读之后：返回值仍是自增前的计数（与后端一致）
      const view = jobDetailView(job)
      job.view_count += 1
      return view
    },
  ],

  /* —— 公开公司 —— */
  [
    'GET /companies',
    { auth: false },
    ({ query }) => pageOf(companyFilter(query, { publicOnly: true }), query, briefView),
  ],
  [
    'GET /companies/:id',
    { auth: false },
    ({ params }) => {
      const company = requireCompany(params.id)
      if (!company.enabled) {
        throw new ApiError({ code: 30001, message: '公司不存在或已下线', status: 404 })
      }
      // 前台只暴露简要字段，不返回抓取配置等内部信息
      return briefView(company)
    },
  ],

  /* —— 系统初始化（首次安装向导，公开接口） —— */
  //
  // 三个接口都不鉴权：它们要回答的场景正是「系统里还没有任何账号可登录」。
  // 真实后端的安全边界由配置开关 + 「不存在可用管理员」判定 + PostgreSQL 咨询锁
  // + 路由级限流共同构成；mock 复刻的是**可见行为**：一旦存在可用管理员，
  // 两个写接口都返回 409，且不产生任何副作用（账号、邮件配置、审计都不动）。
  ['GET /system/status', { auth: false }, () => systemStatusView()],
  ['POST /system/setup', { auth: false }, ({ body }) => setupAdmin(body ?? {})],
  ['POST /system/setup/mail/test', { auth: false }, ({ body }) => sendSetupTestMail(body ?? {})],

  /* —— 登录用户 —— */
  ['GET /me', {}, () => userView(currentUser())],
  [
    'PATCH /me',
    {},
    ({ body }) => {
      const user = currentUser()
      if (Object.prototype.hasOwnProperty.call(body ?? {}, 'nickname')) {
        const nickname = String(body.nickname ?? '').trim()
        if (!nickname) throw badRequest('昵称不能为空')
        user.nickname = nickname
      }
      if (Object.prototype.hasOwnProperty.call(body ?? {}, 'avatar_url')) {
        user.avatar_url = String(body.avatar_url ?? '').trim()
      }
      if (Object.prototype.hasOwnProperty.call(body ?? {}, 'phone')) {
        user.phone = String(body.phone ?? '').trim()
      }
      user.updated_at = nowIso()
      return userView(user)
    },
  ],
  [
    'POST /me/password',
    {},
    ({ body }) => {
      if (!body?.old_password) throw badRequest('请求参数有误', { reason: 'old_password 为必填项' })
      if (!body?.new_password) throw badRequest('请求参数有误', { reason: 'new_password 为必填项' })
      // 演示环境没有密码库：原密码非空即视为正确，其余规则与后端一致
      if (body.old_password === body.new_password) {
        throw new ApiError({ code: 20012, message: '新密码不能与当前密码相同', status: 400 })
      }
      validatePassword(body.new_password, 8)
      recordAudit({ action: 'auth.change_password', resource: 'user', resourceId: currentUser().id, path: '/api/v1/me/password', detail: {} })
      // 改密会递增密码版本，当前设备必须换成新令牌，否则会被自己踢下线
      return { message: '密码修改成功，其他设备已退出登录', token: makeToken(currentUser().email) }
    },
  ],
  ['GET /me/sessions', {}, () => ({ items: db.sessions.map(sessionView) })],
  [
    'DELETE /me/sessions/:sid',
    {},
    ({ params }) => {
      const index = db.sessions.findIndex((session) => session.id === params.sid)
      if (index < 0) throw new ApiError({ code: 10004, message: '会话不存在或已过期', status: 404 })
      db.sessions.splice(index, 1)
      recordAudit({ action: 'auth.revoke_session', resource: 'session', resourceId: params.sid, method: 'DELETE', path: `/api/v1/me/sessions/${params.sid}`, detail: {} })
      return undefined
    },
  ],

  /* —— 管理端：统计 —— */
  ['GET /admin/stats', {}, () => statsView()],

  /* —— 管理端：岗位 —— */
  ['GET /admin/jobs', {}, ({ query }) => pageOf(jobFilter(query, { publicOnly: false }), query, jobListView)],
  [
    'GET /admin/jobs/:id/revisions',
    {},
    ({ params }) => {
      const job = requireJob(params.id)
      return { items: revisionsFor(job).map(revisionView) }
    },
  ],
  ['GET /admin/jobs/:id', {}, ({ params }) => jobDetailView(requireJob(params.id))],
  [
    'PATCH /admin/jobs/:id',
    {},
    ({ params, body }) => {
      const job = requireJob(params.id)
      const before = contentSnapshot(job)
      applyJobEdits(job, body ?? {})
      const after = contentSnapshot(job)
      const changed = changedJobFields(before, after)
      if (changed.length) {
        job.revision += 1
        job.last_updated_at = nowIso()
        job.updated_at = job.last_updated_at
        // 与抓取路径同口径：已发布岗位的实质性变更退回待审核，避免绕过审核改已发布内容
        if (job.status === 'approved' && changed.some((field) => SUBSTANTIVE_FIELDS.has(field))) {
          job.status = 'pending_review'
          job.reject_reason = ''
        }
        revisionsFor(job).unshift({
          // 新修订的 id 是新生成的 ULID（ULID 单调递增，所以它天然排在旧修订之后；
          // 列表本身按 revision 倒序展示，不依赖 id 顺序）
          id: nextUlid(),
          revision: job.revision,
          changed_fields: changed,
          content_hash: demoHash(`${job.id}:${job.revision}`),
          source_url: job.source_url,
          trigger: 'manual',
          snapshot: after,
          created_at: job.last_updated_at,
        })
      }
      recordAudit({
        action: 'job.update',
        resource: 'job',
        resourceId: job.id,
        method: 'PATCH',
        path: `/api/v1/admin/jobs/${job.id}`,
        detail: { changed_fields: changed },
      })
      return jobDetailView(job)
    },
  ],
  [
    'POST /admin/jobs/:id/review',
    {},
    ({ params, body }) => {
      const job = requireJob(params.id)
      const decision = String(body?.decision ?? '')
      // handler 的 binding 是 oneof=approve reject，其它取值在进 service 之前就被拒
      if (!body?.decision) throw badRequest('请求参数有误', { reason: 'decision 为必填项' })
      if (decision !== 'approve' && decision !== 'reject') {
        throw badRequest('请求参数有误', { reason: 'decision 只能是 approve 或 reject' })
      }
      if (job.status === 'approved') {
        throw new ApiError({ code: 30004, message: '该岗位已被审核，不能重复审核', status: 409 })
      }
      if (job.status !== 'pending_review' && job.status !== 'rejected') {
        throw new ApiError({ code: 30005, message: `当前状态不允许审核：${job.status}`, status: 400 })
      }
      const now = nowIso()
      job.reviewer_id = currentUser().id
      job.reviewed_at = now
      job.updated_at = now
      if (decision === 'reject') {
        const reason = String(body?.reject_reason ?? '').trim()
        if (!reason) throw new ApiError({ code: 10001, message: '驳回必须填写原因', status: 400 })
        job.status = 'rejected'
        job.reject_reason = reason
      } else {
        job.status = 'approved'
        job.reject_reason = ''
      }
      recordAudit({
        action: `job.${decision}`,
        resource: 'job',
        resourceId: job.id,
        path: `/api/v1/admin/jobs/${job.id}/review`,
        detail: { title: job.title, decision, reject_reason: body?.reject_reason ?? '' },
      })
      return jobDetailView(job)
    },
  ],
  [
    'POST /admin/jobs/:id/status',
    {},
    ({ params, body }) => {
      const job = requireJob(params.id)
      if (!body?.status) throw badRequest('请求参数有误', { reason: 'status 为必填项' })
      const target = String(body.status)
      if (!['approved', 'offline', 'expired', 'pending_review'].includes(target)) {
        throw badRequest(`不支持的岗位状态：${target}`)
      }
      const previous = job.status
      job.status = target
      if (target === 'approved') {
        // 重新上线视为一次人工确认：补审核人与时间，并清空驳回原因
        job.reviewer_id = currentUser().id
        job.reviewed_at = nowIso()
        job.reject_reason = ''
      }
      if (target === 'expired') job.expired_at = nowIso()
      job.updated_at = nowIso()
      recordAudit({
        action: 'job.set_status',
        resource: 'job',
        resourceId: job.id,
        path: `/api/v1/admin/jobs/${job.id}/status`,
        detail: { old: previous, new: target },
      })
      return jobDetailView(job)
    },
  ],
  [
    'POST /admin/jobs',
    {},
    ({ body }) => {
      // company_id 是 ULID 字符串而不是数字：空值 → 10001「必须指定所属公司」；
      // 非空但查不到（含格式非法）→ 30001，与后端 `Where(company.IDEQ(in.CompanyID))` 的分支一致
      const companyId = String(body?.company_id ?? '').trim()
      if (!companyId) throw badRequest('必须指定所属公司')
      if (!String(body?.title ?? '').trim()) throw badRequest('岗位名称不能为空')
      const sourceUrl = String(body?.source_url ?? '').trim()
      if (!sourceUrl) throw badRequest('必须提供岗位来源地址（官网链接）')
      if (db.jobs.some((job) => job.source_url === sourceUrl)) {
        throw new ApiError({ code: 30006, message: '岗位已存在', status: 409 })
      }
      const company = findCompanyRecord(companyId)
      if (!company) throw new ApiError({ code: 30001, message: '公司不存在', status: 404 })

      const now = nowIso()
      const record = makeJob({
        company,
        title: String(body.title).trim(),
        category: '',
        city: '',
        salary_min: null,
        salary_max: null,
        salary_months: null,
        job_code: '',
        published_at: null,
        status: body?.auto_approve ? 'approved' : 'pending_review',
        headcount: null,
        quality_score: 0,
        view_count: 0,
      })
      // makeJob 会用职位模板补全长文本（那是抓取岗位的演示内容），但手工新增必须严格对齐
      // ent 默认值：没填的字段就是空串/空数组，否则「后台新建一个空岗位」会凭空长出一段岗位职责。
      Object.assign(record, {
        summary: '',
        description: '',
        responsibilities: '',
        requirements: '',
        requirements_list: [],
        benefits: '',
        tags: [],
        department: '',
        work_mode: '',
        education: '',
        experience: '',
        employment_type: '',
        province: '',
        district: '',
        address: '',
        location_text: '',
        contact_person: '',
        contact_email: '',
        contact_phone: '',
        contact_wechat: '',
        apply_url: '',
        apply_method: '',
        source_updated_at: null,
        deadline_at: null,
      })
      // 手工录入的元信息与后端 Create 完全一致
      record.source_url = sourceUrl
      record.source_site = hostOf(sourceUrl)
      record.source_company_name = company.name
      record.industry = company.industry
      record.is_direct_hire = true
      record.extractor = 'manual'
      record.parse_confidence = 1
      record.crawled_at = now
      record.last_seen_at = now
      record.last_updated_at = now
      record.created_at = now
      record.updated_at = now
      record.revision = 1
      record.reviewer_id = null
      // auto_approve 时后端只写 reviewed_at，不写 reviewer_id
      record.reviewed_at = body?.auto_approve ? now : null
      applyJobEdits(record, body ?? {})
      record.quality_score = scoreJob(record)
      // 用新生成的 ULID 覆盖工厂里的编写用序号：ULID 单调递增，因此它一定排在所有旧岗位之前
      // （后端也是创建时生成主键，删除后再新增不会撞 id）
      record.id = nextUlid()
      db.jobs.unshift(record)
      // Create 不写修订记录（只有抓取入库与人工编辑会写）
      db.revisions.set(record.id, [])
      recordAudit({
        action: 'job.create',
        resource: 'job',
        resourceId: record.id,
        path: '/api/v1/admin/jobs',
        detail: { title: record.title, company: company.name },
      })
      return jobDetailView(record)
    },
  ],
  [
    'DELETE /admin/jobs/:id',
    {},
    ({ params }) => {
      const job = requireJob(params.id)
      const index = db.jobs.findIndex((item) => item.id === job.id)
      db.jobs.splice(index, 1)
      db.revisions.delete(job.id)
      recordAudit({ action: 'job.delete', resource: 'job', resourceId: job.id, method: 'DELETE', path: `/api/v1/admin/jobs/${job.id}`, detail: { title: job.title } })
      return undefined
    },
  ],

  /* —— 管理端：公司 —— */
  ['GET /admin/companies', {}, ({ query }) => pageOf(companyFilter(query, { publicOnly: false }), query, companyView)],
  // 详情只返回 CompanyView：后端刻意不预加载 jobs / crawl_runs，需要时另调列表接口
  ['GET /admin/companies/:id', {}, ({ params }) => companyView(requireCompany(params.id))],
  [
    'PATCH /admin/companies/:id',
    {},
    ({ params, body }) => {
      const company = requireCompany(params.id)
      const changed = {}
      for (const [key, value] of Object.entries(body ?? {})) {
        // 未知键与 companyUpdateRequest 一样被忽略；所有字段都是指针，JSON null 表示不修改
        if (!COMPANY_UPDATE_FIELDS.includes(key)) continue
        if (value === null || value === undefined) continue
        if (key === 'name' && !String(value).trim()) throw badRequest('公司名称不能为空', { field: 'name' })
        if (key === 'website' || key === 'careers_url') validateHttpUrl(key, value)
        if (key === 'crawl_interval_seconds' && Number(value) <= 0) {
          throw badRequest('抓取间隔必须大于 0 秒', { field: 'crawl_interval_seconds' })
        }
        changed[key] = { old: company[key], new: value }
        company[key] = value
      }
      if (Object.keys(changed).length) company.updated_at = nowIso()
      recordAudit({ action: 'company.update', resource: 'company', resourceId: company.id, method: 'PATCH', path: `/api/v1/admin/companies/${company.id}`, detail: changed })
      return companyView(company)
    },
  ],
  [
    'POST /admin/companies/:id/enabled',
    {},
    ({ params, body }) => {
      const company = requireCompany(params.id)
      // 真实后端的 setEnabledRequest.Enabled 是 bool + binding:"required"，
      // 传 false 会被 validator 当成「未提供」直接 10001 —— 那是后端契约瑕疵，
      // 这里按字段语义放过 false，否则演示里「停用公司」这个按钮永远失败。
      const enabled = typeof body?.enabled === 'boolean' ? body.enabled : !company.enabled
      if (company.enabled === enabled) return companyView(company)
      company.enabled = enabled
      company.updated_at = nowIso()
      recordAudit({
        action: 'company.set_enabled',
        resource: 'company',
        resourceId: company.id,
        path: `/api/v1/admin/companies/${company.id}/enabled`,
        detail: { enabled: { old: !enabled, new: enabled } },
      })
      return companyView(company)
    },
  ],
  [
    'POST /admin/companies/:id/discover',
    {},
    ({ params }) => {
      const company = requireCompany(params.id)
      // 没有官网就没法探测入口；真实后端此时返回 40006
      if (!String(company.website ?? '').trim()) {
        throw new ApiError({ code: 40006, message: '未能定位到该公司的招聘页面入口', status: 404 })
      }
      const careersUrl = `${company.website.replace(/\/$/, '')}/join-us`
      const detail = {}
      if (careersUrl !== company.careers_url) {
        detail.careers_url = { old: company.careers_url, new: careersUrl }
        company.careers_url = careersUrl
      }
      if (company.discovery_url !== company.website) {
        detail.discovery_url = { old: company.discovery_url, new: company.website }
        company.discovery_url = company.website
      }
      if (!company.careers_url || company.source !== 'discovered') {
        detail.source = { old: company.source, new: 'discovered' }
        company.source = 'discovered'
      }
      company.updated_at = nowIso()
      recordAudit({
        action: 'company.set_careers_url',
        resource: 'company',
        resourceId: company.id,
        path: `/api/v1/admin/companies/${company.id}/discover`,
        detail,
      })
      return companyView(company)
    },
  ],
  [
    'POST /admin/companies/:id/crawl',
    {},
    ({ params }) => {
      const company = requireCompany(params.id)
      if (!company.enabled) throw badRequest('该公司已停用，无法抓取')
      if (!company.crawl_enabled) throw badRequest('该公司未开启抓取')
      const run = createRun(company, 'manual', 'success')
      recordAudit({
        action: 'crawl.run',
        resource: 'company',
        resourceId: company.id,
        path: `/api/v1/admin/companies/${company.id}/crawl`,
        detail: { run_id: run.id, status: run.status, jobs_created: run.jobs_created, jobs_updated: run.jobs_updated, jobs_filtered: run.jobs_filtered, trigger: 'manual' },
      })
      return runView(run)
    },
  ],
  [
    'POST /admin/companies',
    {},
    ({ body }) => {
      const name = String(body?.name ?? '').trim()
      if (!name) throw badRequest('公司名称不能为空', { field: 'name' })
      if (db.companies.some((company) => company.name === name)) {
        throw new ApiError({ code: 30002, message: `公司名称已存在：${name}`, status: 409 })
      }
      validateHttpUrl('website', body?.website)
      validateHttpUrl('careers_url', body?.careers_url)
      if (body?.crawl_interval_seconds !== undefined && Number(body.crawl_interval_seconds) <= 0) {
        throw badRequest('抓取间隔必须大于 0 秒', { field: 'crawl_interval_seconds' })
      }
      const created = makeCompany({
        // makeCompany 的 id 只是「编写用序号」占位，这里直接给 ULID（实体 id 由服务端生成）
        id: nextUlid(),
        name,
        short_name: body?.short_name,
        website: body?.website,
        careers_url: body?.careers_url,
        logo_url: body?.logo_url,
        industry: body?.industry,
        sub_industry: body?.sub_industry,
        scale: body?.scale,
        nature: body?.nature,
        location: body?.location,
        description: body?.description,
        contact_email: body?.contact_email,
        contact_phone: body?.contact_phone,
        enabled: body?.enabled ?? true,
        verified: body?.verified ?? false,
        priority: body?.priority ?? 0,
        crawl_enabled: body?.crawl_enabled ?? true,
        crawl_interval_seconds: body?.crawl_interval_seconds ?? 21600,
        crawl_rules: body?.crawl_rules ?? {},
        force_headless: body?.force_headless ?? false,
        respect_robots: body?.respect_robots ?? true,
        last_crawled_at: null,
        last_crawl_status: '',
        robots_checked_at: null,
        remark: body?.remark,
        created_at: nowIso(),
        updated_at: nowIso(),
      })
      db.companies.unshift(created)
      recordAudit({
        action: 'company.create',
        resource: 'company',
        resourceId: created.id,
        path: '/api/v1/admin/companies',
        detail: { name: created.name, website: created.website, careers_url: created.careers_url, industry: created.industry, enabled: created.enabled, crawl_enabled: created.crawl_enabled, priority: created.priority },
      })
      return companyView(created)
    },
  ],
  [
    'DELETE /admin/companies/:id',
    {},
    ({ params }) => {
      const company = requireCompany(params.id)
      // 有岗位时后端拒绝删除：注意错误码是 10001（ErrInvalidParams），不是 30005
      const jobCount = db.jobs.filter((job) => job.company_id === company.id).length
      if (jobCount > 0) {
        throw new ApiError({
          code: 10001,
          message: '该公司下仍有岗位，请先删除或转移岗位后再删除公司',
          details: { company_id: company.id, job_count: jobCount },
          status: 400,
        })
      }
      const deletedRuns = db.runs.filter((run) => run.company_id === company.id).length
      db.companies = db.companies.filter((item) => item.id !== company.id)
      db.runs = db.runs.filter((run) => run.company_id !== company.id)
      recordAudit({
        action: 'company.delete',
        resource: 'company',
        resourceId: company.id,
        method: 'DELETE',
        path: `/api/v1/admin/companies/${company.id}`,
        detail: { name: company.name, website: company.website, careers_url: company.careers_url, deleted_crawl_runs: deletedRuns },
      })
      return undefined
    },
  ],

  /* —— 管理端：抓取 —— */
  [
    'GET /admin/crawl-runs',
    {},
    ({ query }) => {
      // company_id 是字符串过滤器（handler.queryString）：空串不过滤，非空精确匹配
      const companyId = firstValue(query.company_id).trim()
      const status = firstValue(query.status).trim()
      const trigger = firstValue(query.trigger).trim()
      const list = db.runs
        .filter((run) => (companyId ? String(run.company_id) === companyId : true))
        .filter((run) => (status ? run.status === status : true))
        .filter((run) => (trigger ? run.trigger === trigger : true))
        .sort(byIdDesc)
      return pageOf(list, query, runView)
    },
  ],
  [
    'GET /admin/crawl-runs/:id',
    {},
    ({ params }) => {
      const id = requireUlid(params.id)
      const run = db.runs.find((item) => String(item.id) === id)
      if (!run) throw new ApiError({ code: 40007, message: '抓取记录不存在', status: 404 })
      // 详情是 { run, logs } 两段，不是把日志塞进 run
      return { run: runView(run), logs: buildCrawlLogs(run).map(crawlLogView) }
    },
  ],
  [
    'POST /admin/crawl/run-all',
    {},
    () => {
      const candidates = db.companies.filter((company) => company.enabled && company.crawl_enabled)
      const runs = candidates.map((company) => createRun(company, 'manual', 'success'))
      recordAudit({
        action: 'crawl.run',
        resource: 'company',
        resourceId: '0',
        path: '/api/v1/admin/crawl/run-all',
        detail: { runs: runs.length, trigger: 'manual' },
      })
      return { runs: runs.map(runView), count: runs.length }
    },
  ],

  /* —— 管理端：用户与角色 —— */
  ['GET /admin/users', {}, ({ query }) => pageOf(userFilter(query), query, userView)],
  ['GET /admin/users/:id', {}, ({ params }) => userView(requireUser(params.id))],
  [
    'POST /admin/users/:id/status',
    {},
    ({ params, body }) => {
      const user = requireUser(params.id)
      if (!body?.status) throw badRequest('请求参数有误', { reason: 'status 为必填项' })
      const status = String(body.status)
      if (!['pending', 'active', 'disabled', 'locked'].includes(status)) {
        throw badRequest(`不支持的用户状态：${status}`)
      }
      if (user.id === currentUser().id) throw badRequest('不能修改自己的账号状态')
      user.status = status
      user.updated_at = nowIso()
      recordAudit({
        action: 'user.set_status',
        resource: 'user',
        resourceId: user.id,
        path: `/api/v1/admin/users/${user.id}/status`,
        detail: { status },
      })
      return userView(user)
    },
  ],
  [
    'POST /admin/users/:id/roles',
    {},
    ({ params, body }) => {
      const user = requireUser(params.id)
      const roles = Array.isArray(body?.roles) ? body.roles.map(String) : []
      if (!roles.length) throw badRequest('至少需要保留一个角色')
      for (const role of roles) {
        if (!['admin', 'reviewer', 'user'].includes(role)) throw badRequest(`不支持的角色：${role}`)
      }
      const previous = [...user.roles]
      // 不能把最后一名可用管理员降级，否则系统永久失去管理入口
      if (previous.includes('admin') && !roles.includes('admin')) {
        const otherAdmins = db.users.filter((item) => item.id !== user.id && item.roles.includes('admin') && item.status !== 'disabled')
        if (!otherAdmins.length) throw badRequest('系统必须保留至少一名可用管理员')
      }
      user.roles = roles
      user.updated_at = nowIso()
      recordAudit({
        action: 'user.set_roles',
        resource: 'user',
        resourceId: user.id,
        path: `/api/v1/admin/users/${user.id}/roles`,
        detail: { old: previous, new: roles },
      })
      return userView(user)
    },
  ],
  [
    'POST /admin/users/:id/reset-password',
    {},
    ({ params, body }) => {
      const user = requireUser(params.id)
      if (!body?.new_password) throw badRequest('请求参数有误', { reason: 'new_password 为必填项' })
      validatePassword(body.new_password, 8)
      recordAudit({
        action: 'user.reset_password',
        resource: 'user',
        resourceId: user.id,
        path: `/api/v1/admin/users/${user.id}/reset-password`,
        detail: { email: user.email },
      })
      return { message: '密码已重置，该用户需重新登录' }
    },
  ],
  [
    'DELETE /admin/users/:id',
    {},
    ({ params }) => {
      const user = requireUser(params.id)
      if (user.id === currentUser().id) throw badRequest('不能删除自己的账号')
      if (user.roles.includes('admin')) {
        const otherAdmins = db.users.filter((item) => item.id !== user.id && item.roles.includes('admin') && item.status !== 'disabled')
        if (!otherAdmins.length) throw badRequest('系统必须保留至少一名可用管理员')
      }
      db.users = db.users.filter((item) => item.id !== user.id)
      recordAudit({
        action: 'user.delete',
        resource: 'user',
        resourceId: user.id,
        method: 'DELETE',
        path: `/api/v1/admin/users/${user.id}`,
        detail: { roles: user.roles },
      })
      return undefined
    },
  ],
  ['GET /admin/roles', {}, () => ({ items: roleViews() })],

  /* —— 管理端：配置与收集范围 —— */
  [
    'GET /admin/settings',
    {},
    ({ query }) => {
      const group = firstValue(query.group).trim()
      const list = db.settings
        .filter((setting) => (group ? setting.group === group : true))
        .sort((a, b) => String(a.group).localeCompare(String(b.group)) || String(a.key).localeCompare(String(b.key)))
      return { items: list.map(settingView) }
    },
  ],
  ['GET /admin/crawl-scope', {}, () => currentScope()],
  [
    'PUT /admin/crawl-scope',
    {},
    ({ body }) => saveCrawlScope(body, currentUser()),
  ],
  [
    'GET /admin/settings/:key',
    {},
    ({ params }) => {
      const setting = db.settings.find((item) => item.key === params.key)
      if (!setting) throw new ApiError({ code: 50001, message: '配置项不存在', status: 404 })
      return settingView(setting)
    },
  ],
  [
    'PUT /admin/settings/:key',
    {},
    ({ params, body }) => {
      const key = String(params.key ?? '').trim()
      if (!key) throw badRequest('配置键不能为空')
      if (runeLength(key) > 128) {
        throw new ApiError({ code: 50002, message: '配置键过长（最多 128 字符）', details: { field: 'key', length: runeLength(key) }, status: 400 })
      }
      // value 是 map[string]any 且 binding:"required"：缺失、非对象、或空对象都会被拒
      if (!body?.value || typeof body.value !== 'object' || Array.isArray(body.value) || !Object.keys(body.value).length) {
        throw badRequest('请求参数有误', { reason: 'value 必须是非空 JSON 对象' })
      }
      const current = db.settings.find((item) => item.key === key)
      const now = nowIso()
      if (current) {
        const changed = {}
        if (JSON.stringify(current.value) !== JSON.stringify(body.value)) {
          changed.value = { old: current.value, new: body.value }
          current.value = { ...body.value }
        }
        // description / group 传空串表示「不修改已有值」，避免只想改 value 却清掉了说明
        if (body.description && body.description !== current.description) {
          changed.description = { old: current.description, new: body.description }
          current.description = body.description
        }
        if (body.group && body.group !== current.group) {
          changed.group = { old: current.group, new: body.group }
          current.group = body.group
        }
        if (!Object.keys(changed).length) return settingView(current)
        current.updated_by = currentUser().email
        current.updated_at = now
        recordAudit({ action: 'setting.update', resource: 'setting', resourceId: key, method: 'PUT', path: `/api/v1/admin/settings/${key}`, detail: changed })
        return settingView(current)
      }
      const created = {
        id: nextUlid(),
        key,
        value: { ...body.value },
        description: body.description ?? '',
        group: body.group ?? '',
        // 普通 Upsert 永远不会把配置提升为内置项，只有 SaveCrawlScope 会
        is_system: false,
        updated_by: currentUser().email,
        updated_at: now,
      }
      db.settings.push(created)
      recordAudit({
        action: 'setting.create',
        resource: 'setting',
        resourceId: key,
        method: 'PUT',
        path: `/api/v1/admin/settings/${key}`,
        detail: { value: created.value, group: created.group, is_system: created.is_system },
      })
      return settingView(created)
    },
  ],
  [
    'DELETE /admin/settings/:key',
    {},
    ({ params }) => {
      const key = String(params.key ?? '').trim()
      const setting = db.settings.find((item) => item.key === key)
      if (!setting) throw new ApiError({ code: 50001, message: '配置项不存在', status: 404 })
      if (setting.is_system) {
        // 内置项被删会让依赖它的功能静默回落默认值，因此后端拒绝：错误码是 50002
        throw new ApiError({ code: 50002, message: '系统内置配置不允许删除', details: { key }, status: 400 })
      }
      db.settings = db.settings.filter((item) => item.key !== key)
      recordAudit({
        action: 'setting.delete',
        resource: 'setting',
        resourceId: key,
        method: 'DELETE',
        path: `/api/v1/admin/settings/${key}`,
        detail: { value: setting.value, group: setting.group },
      })
      return undefined
    },
  ],

  /* —— 管理端：邮件设置 —— */
  ['GET /admin/mail', {}, () => mailView()],
  [
    'PUT /admin/mail',
    {},
    ({ body }) => {
      // driver 是必填 + oneof=console|smtp：缺失或大小写不符都在 binding 阶段被拒
      if (!body?.driver) throw badRequest('请求参数有误', { reason: 'driver 为必填项，取值 console 或 smtp' })
      const driver = String(body.driver).trim().toLowerCase()
      if (driver !== 'console' && driver !== 'smtp') {
        throw badRequest('请求参数有误', { reason: 'driver 只能是 console 或 smtp' })
      }
      const current = db.mail.override ?? mailConfigBaseline
      const next = {
        driver,
        from_name: String(body.from_name ?? '').trim(),
        from_address: String(body.from_address ?? '').trim(),
        smtp: {
          host: String(body.host ?? '').trim(),
          port: queryInt(body.port, 0),
          username: String(body.username ?? '').trim(),
          // TLS 是 *bool：请求里不传就等于 false（后端是 `in.TLS != nil && *in.TLS`）
          tls: body.tls === true,
          password: '',
        },
      }
      // 密码语义：显式清空 > 新密码 > 沿用旧密码（占位符原样回传也表示不修改）
      if (body.clear_password === true) {
        next.smtp.password = ''
      } else if (!body.password || body.password === MAIL_PASSWORD_MASK) {
        next.smtp.password = current.smtp.password
      } else {
        next.smtp.password = String(body.password)
      }
      validateMailConfig(next)
      next.updated_by = currentUser().email
      next.updated_at = nowIso()
      db.mail.override = next
      recordAudit({
        action: 'mail.update',
        resource: 'setting',
        resourceId: 'mail.smtp',
        method: 'PUT',
        path: '/api/v1/admin/mail',
        detail: {
          driver: next.driver,
          from_address: next.from_address,
          host: next.smtp.host,
          port: next.smtp.port,
          username: next.smtp.username,
          tls: next.smtp.tls,
          password_changed: body.clear_password === true || Boolean(body.password && body.password !== MAIL_PASSWORD_MASK),
        },
      })
      return mailView()
    },
  ],
  [
    'DELETE /admin/mail',
    {},
    () => {
      // 丢弃数据库覆盖，回落到 configs/config.yaml
      db.mail.override = null
      recordAudit({ action: 'mail.revert', resource: 'setting', resourceId: 'mail.smtp', method: 'DELETE', path: '/api/v1/admin/mail', detail: {} })
      return mailView()
    },
  ],
  [
    'POST /admin/mail/test',
    {},
    ({ body }) => {
      if (!body?.to) throw badRequest('请求参数有误', { reason: 'to 为必填项' })
      const to = String(body.to)
      if (!looksLikeEmail(to)) throw badRequest('收件邮箱格式不正确')
      recordAudit({
        action: 'mail.test',
        resource: 'setting',
        resourceId: 'mail.smtp',
        path: '/api/v1/admin/mail/test',
        detail: { to: to.trim(), driver: mailView().active_driver },
      })
      // 文案由 handler 拼装，用的是请求里的原始 to（未 trim）
      return { message: `测试邮件已发送至 ${to}，请查收（含垃圾箱）` }
    },
  ],

  /* —— 管理端：审计与策略 —— */
  [
    'GET /admin/audit-logs',
    {},
    ({ query }) => {
      // actor_id 是字符串过滤器：真实后端只有「非空才过滤」这一条规则，且不做 ULID 合法性校验
      const actorId = firstValue(query.actor_id).trim()
      const action = firstValue(query.action).trim()
      const resource = firstValue(query.resource).trim()
      const resourceId = firstValue(query.resource_id).trim()
      const success = queryBool(query.success)
      const list = db.logs
        .filter((log) => (actorId ? String(log.actor_id ?? '') === actorId : true))
        // action 是模糊匹配（便于按 "company." 前缀筛一类操作），resource 是等值匹配
        .filter((log) => (action ? includesFold(log.action, action) : true))
        .filter((log) => (resource ? log.resource === resource : true))
        .filter((log) => (resourceId ? log.resource_id === resourceId : true))
        .filter((log) => (success === null ? true : Boolean(log.success) === success))
        .sort(byIdDesc)
      return pageOf(list, query, auditView)
    },
  ],
  [
    'GET /admin/authz/policies',
    {},
    // 策略本身就是 casbin 的 [sub, obj, act] 三元组数组，另外带一份规模统计
    () => ({ policies: db.policies.map((row) => [...row]), stats: authzStats() }),
  ],
  [
    'POST /admin/authz/policies',
    {},
    ({ body }) => {
      if (!body?.role) throw badRequest('请求参数有误', { reason: 'role 为必填项' })
      if (!body?.obj) throw badRequest('请求参数有误', { reason: 'obj 为必填项' })
      if (!body?.act) throw badRequest('请求参数有误', { reason: 'act 为必填项' })
      const role = String(body.role)
      if (!['admin', 'reviewer', 'user'].includes(role)) throw badRequest(`不支持的角色：${role}`)
      const row = [role, String(body.obj), String(body.act)]
      if (!db.policies.some((item) => item[0] === row[0] && item[1] === row[1] && item[2] === row[2])) {
        db.policies.push(row)
      }
      recordAudit({
        action: 'authz.add_policy',
        resource: 'policy',
        resourceId: role,
        path: '/api/v1/admin/authz/policies',
        detail: { obj: row[1], act: row[2] },
      })
      return { role: row[0], obj: row[1], act: row[2] }
    },
  ],
  [
    'DELETE /admin/authz/policies',
    {},
    ({ query, body }) => {
      // 参数优先取查询串，同时兼容请求体（PowerShell 等客户端不允许 DELETE 带 body）
      const role = String(firstValue(query.role) || body?.role || '').trim()
      const obj = String(firstValue(query.obj) || body?.obj || '').trim()
      const act = String(firstValue(query.act) || body?.act || '').trim()
      if (!role || !obj || !act) {
        throw badRequest('role、obj、act 均为必填（可用查询参数或请求体传递）')
      }
      db.policies = db.policies.filter((row) => !(row[0] === role && row[1] === obj && row[2] === act))
      recordAudit({
        action: 'authz.remove_policy',
        resource: 'policy',
        resourceId: role,
        method: 'DELETE',
        path: '/api/v1/admin/authz/policies',
        detail: { obj, act },
      })
      return undefined
    },
  ],
]

/** 手动触发抓取时新建一条运行记录（演示数据直接给「成功」结果） */
function createRun(company, trigger, status) {
  const id = nextUlid()
  const now = nowIso()
  const run = {
    id,
    company_id: company.id,
    company_name: company.name,
    trigger,
    status,
    skip_reason: '',
    careers_url: company.careers_url || company.website,
    started_at: now,
    finished_at: status === 'running' ? null : now,
    duration_ms: 1680,
    pages_fetched: 8,
    jobs_found: 8,
    jobs_created: 2,
    jobs_updated: 1,
    jobs_unchanged: 4,
    jobs_filtered: 1,
    jobs_expired: 0,
    used_headless: false,
    error_message: '',
    detail: {},
  }
  db.runs.unshift(run)
  return run
}

/* -------------------------------------------------------------------------- */
/* 匹配与派发                                                                  */
/* -------------------------------------------------------------------------- */

function matchRoute(method, path) {
  const target = path.split('?')[0].split('/').filter(Boolean)
  for (const [pattern, meta, handler] of routes) {
    const [patternMethod, patternPath] = pattern.split(' ')
    if (patternMethod !== method) continue
    const segments = patternPath.split('/').filter(Boolean)
    if (segments.length !== target.length) continue
    const params = {}
    const ok = segments.every((seg, i) => {
      if (seg.startsWith(':')) {
        params[seg.slice(1)] = decodeURIComponent(target[i])
        return true
      }
      return seg === target[i]
    })
    if (ok) return { handler, meta, params }
  }
  return null
}

export async function mockRequest(method, path, { query = {}, body, authed = false, auth = true } = {}) {
  await delay(60)

  const matched = matchRoute(method, path)
  if (!matched) {
    throw new ApiError({ code: 10004, message: `演示数据未覆盖该接口：${method} ${path}`, status: 404 })
  }
  if (auth && matched.meta?.auth !== false && !authed) {
    throw new ApiError({ code: 10002, message: '请先登录', status: 401 })
  }
  // 响应体只返回 data 部分：http.js 已经负责拆信封与非零码转换
  return matched.handler({ params: matched.params, query, body })
}

/* -------------------------------------------------------------------------- */
/* 演示入口：把演示数据切成「全新安装」                                          */
/* -------------------------------------------------------------------------- */

/**
 * 为什么需要这个入口：默认演示数据里已经有管理员，/system/status 因此恒为 initialized，
 * 而路由守卫对「已初始化」的处理是把 /setup 直接弹回登录页 —— 于是安装向导
 * （连同向导里的「测试邮件」按钮、成功/认证失败两条路径）在演示环境里**根本走不到**，
 * 三个 /system/* 接口也就永远不会被调用。
 *
 * 用法：在任意页面地址后加 `?mockSetup=reset` 再打开，例如
 *   http://127.0.0.1:5173/setup?mockSetup=reset
 * 本次页面加载会把内存表换成「全新安装」：initialized=false、admin_count=0、
 * setup_available=true、registration_open=false、mail={driver:'console',source:'config',smtp_configured:false}。
 *
 * 它只改内存，**刷新页面即复位**：去掉参数刷新就回到默认演示数据（种子用户、演示邮箱配置都回来）。
 * 这里不引入任何真实契约字段 —— 本模块只在 VITE_USE_MOCK=true 时由 http.js 动态载入。
 */
if (typeof window !== 'undefined' && new URLSearchParams(window.location.search).get('mockSetup') === 'reset') {
  resetMockSystemState()
}
