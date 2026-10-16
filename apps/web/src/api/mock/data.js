/**
 * 演示数据（demo fixtures）。
 *
 * 字段口径的唯一依据是 Go 后端 `internal/service/view.go` 的 json tag，
 * 不是设计稿、也不是「前端用着顺手」的名字：
 *   - companies → CompanyView（`job_count` 由 index.js 按 approved 岗位实时统计）
 *   - jobs      → JobDetailView（列表接口由 index.js 裁剪成 JobListView）
 *   - users     → UserView；roles → RoleView（permissions / user_count 运行时拼装）
 *   - settings  → SettingView（value 恒为对象；后端只播种 crawler.scope 一项）
 *   - crawlRuns → CrawlRunView；明细日志按 CrawlLogView 生成
 *   - auditLogs → AuditLogView（detail 是对象、actor_email 不是 actor）
 *   - mockAuthConfig → config.Default() 的 auth 段（配置项，不是数据）
 *
 * 为什么派生字段一律留给 index.js：真实后端里 job_count / stats / permissions
 * 都是从数据实时算出来的。把 186、835 这类数字写死在 fixture 里，
 * 数组一变长就对不上，于是又变回「演示能跑、联调对不上」的假契约。
 *
 * 数据全部为演示占位内容，联系邮箱/电话一律使用 example.com / 8888 号段，
 * 不构成任何真实招聘信息。
 */

const NOW = Date.now()
const DAY = 24 * 60 * 60 * 1000

/** 相对「现在」的 ISO 时间，让「3 天前」这类文案永远是新鲜的 */
function daysAgo(days, hour = 10, minute = 12) {
  const d = new Date(NOW - days * DAY)
  d.setHours(hour, minute, 0, 0)
  return d.toISOString()
}

function hoursAgo(hours) {
  return new Date(NOW - hours * 60 * 60 * 1000).toISOString()
}

/**
 * 只在有值时才写入键，模拟 Go 的 `omitempty`。
 *
 * 为什么必须显式模拟：`json:"logo_url,omitempty"` 在空值时会让键**整体消失**，
 * 而 `{ logo_url: '' }` 与 `{}` 对前端的 `v-if="company.logo_url"` 虽然等价，
 * 对 `'logo_url' in company`、`Object.keys(row)` 这类写法却不等价。
 * 更关键的是 `{ logo_url: undefined }` 这种「键存在但值为空」的第三态 —— 真实后端绝不会产生它。
 */
function setIf(target, key, value) {
  if (value === undefined || value === null || value === '') return target
  if (Array.isArray(value) && value.length === 0) return target
  target[key] = value
  return target
}

/**
 * 取默认值，但保留调用方显式传入的 null。
 *
 * 为什么不能用 `base.x ?? fallback`：`null ?? fallback` 会退化成 fallback，
 * 于是「从未抓取过」(last_crawled_at = null) 和「薪资面议」(salary_months = null)
 * 这两种**有意义**的空值会被悄悄填成默认值 —— 契约里的可空字段就是这么被弄丢的。
 */
function pick(base, key, fallback) {
  return base[key] === undefined ? fallback : base[key]
}

/**
 * 演示用的内容指纹。
 *
 * 真实后端是 sha256（64 位小写十六进制）。这里刻意保持同样的长度与字符集，
 * 因为前端可能按长度截断展示、或按 16 进制做高亮 —— 用 'rev-3' 这种假串会让这些代码在演示里失真。
 */
function demoHash(seed) {
  const text = String(seed)
  const parts = []
  let h = 0x811c9dc5
  for (let round = 0; round < 8; round += 1) {
    for (let i = 0; i < text.length; i += 1) {
      h = Math.imul(h ^ text.charCodeAt(i), 0x01000193) >>> 0
    }
    h = Math.imul(h ^ (round + 1), 0x85ebca6b) >>> 0
    parts.push(h.toString(16).padStart(8, '0'))
  }
  return parts.join('')
}

/* -------------------------------------------------------------------------- */
/* 主键：ULID（与 internal/pkg/ulidid 同形态）                                   */
/* -------------------------------------------------------------------------- */

/**
 * 与后端一致的主键形态：26 位 Crockford Base32（大写，字母不含 I/L/O/U）。
 *
 * 布局是「前 10 位 = 48 位毫秒时间戳，后 16 位 = 80 位熵」，因此**字典序即时间序**：
 * 「按 id 倒序」与「按创建时间倒序」是同一件事。这也是 internal/pkg/ulidid 的注释里
 * 写明的性质，前端任何按 id 排序的地方都要按字符串比较，不能做数字相减。
 */
const ULID_ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ'

/**
 * 固定的时间前缀：2026-01-01T00:00:00Z（而不是 Date.now()）。
 *
 * 演示数据的 id 必须**每次刷新都完全一样**：/admin/jobs/<id> 这类深链接、
 * 以及界面里缓存下来的 id，一旦刷新就换一批，演示会直接断在半路。
 * 熵同样取自固定计数器而不是随机数，于是「id 稳定」与「id 可排序」同时成立。
 */
const ULID_TIME_MS = Date.UTC(2026, 0, 1)

/**
 * 熵的固定基数：让生成的 id 也带一点真实 ULID 的「随机感」。
 *
 * 若 80 位熵直接放自增计数器，id 会是一长串 0（01KDVDNA00000000000000000F），
 * 一眼就能看出是演示数据。加上这个固定的 80 位基数后，只有低位随序号变化：
 * 仍然是「同一个基数上的单调递增」（字典序即生成顺序），但不会暴露「从 0 开始数」。
 */
const ULID_ENTROPY_BASE = 0x9e3779b97f4a7c15a5c3n

let ulidSeq = 0

/**
 * 生成下一个 ULID：48 位固定时间前缀 + 80 位（固定基数 + 自增计数器）。
 *
 * 计数器单调递增，所以先创建的实体 id 必然小于后创建的 —— index.js 里
 * 「id 越大越新」的排序语义（过去靠自增数字主键实现）因此原样保留，
 * 包括「新建的岗位排在最前」这类行为。
 */
export function nextUlid() {
  // 计数器相对基数极小，相加不会越出 80 位
  const value = (BigInt(ULID_TIME_MS) << 80n) | (ULID_ENTROPY_BASE + BigInt(ulidSeq))
  ulidSeq += 1
  let text = ''
  let rest = value
  for (let i = 0; i < 26; i += 1) {
    // Number(...) 只把 5 位分组（0~31）变成字符表下标，不参与任何 id 比较
    text = ULID_ALPHABET[Number(rest & 31n)] + text
    rest >>= 5n
  }
  return text
}

/**
 * ULID 形态校验：与 `ulidid.Valid` 同口径。
 *
 * 长度必须是 26，字符必须落在 Crockford Base32 内，而且**首字符不得大于 '7'**
 * （Base32 表示 130 位、ULID 只有 128 位，首字符越界即溢出）。
 * 后端写的是 `ulid.ParseStrict(strings.ToUpper(s))`：小写也能通过格式校验，
 * 但随后查询用的仍是原始字符串，所以小写 id 会走到「查不到」（404），
 * 而不是「参数非法」（10001）—— 这里刻意保持同样的分支。
 */
export function isUlid(raw) {
  return /^[0-7][0-9A-HJKMNP-TV-Z]{25}$/.test(String(raw ?? '').trim().toUpperCase())
}

/**
 * 给一批种子数据分配 ULID。
 *
 * 按「编写用序号」升序分配，让 id 的字典序与原来的数字主键完全一致：
 * index.js 里所有「id 越大越新」的排序在改成字符串比较后行为不变；
 * 抓取记录那种按 328→321 倒序书写的种子数组也不会被排反。
 *
 * 前置条件：传入数组里的 id 必须还是**编写用序号（数字）**，即每个数组只能调用一次
 * （第二次调用时 id 已经是 ULID，`a.id - b.id` 会变成 NaN，排序静默失效）。
 */
const seedIdMap = new Map()

function assignSeedIds(kind, records) {
  for (const record of [...records].sort((a, b) => a.id - b.id)) {
    seedIdMap.set(`${kind}:${record.id}`, nextUlid())
  }
  for (const record of records) record.id = seedIdMap.get(`${kind}:${record.id}`)
  return records
}

/**
 * 取某个种子实体的 ULID（按编写用序号）。
 *
 * 审计日志这类内嵌引用必须与实体自身的 id 一字不差：原来靠「两边都写 1」维持一致，
 * 现在统一由这里换算，序号写错会在加载期直接报错，而不是留下一个查不到的假外键。
 */
function seedRef(kind, seq) {
  const id = seedIdMap.get(`${kind}:${seq}`)
  if (!id) throw new Error(`演示数据缺少 ${kind} 序号 ${seq}（seedRef 写错了？）`)
  return id
}

/**
 * 按编写用序号取各实体真实的 ULID。
 *
 * 外键（job.company_id / crawlRun.company_id / job.reviewer_id / 审计里的 actor_id、
 * resource_id、path 里的 id、detail.run_id）统一走这里，保证「指向谁就是谁的 id」。
 * 名字带 Ref 后缀是为了不和 BULK 解构里的局部变量 companyId 撞名。
 */
const companyRef = (seq) => seedRef('company', seq)
const jobRef = (seq) => seedRef('job', seq)
const userRef = (seq) => seedRef('user', seq)
const runRef = (seq) => seedRef('run', seq)

export { daysAgo, hoursAgo, demoHash, setIf }

/* -------------------------------------------------------------------------- */
/* 公司（CompanyView）                                                          */
/* -------------------------------------------------------------------------- */

/**
 * 构造一条公司记录。
 *
 * 这里的默认值抄自 `internal/ent/schema/company.go` 的 ent default
 * （enabled=true、crawl_enabled=true、respect_robots=true、crawl_interval_seconds=21600 …），
 * 因为「新建公司后立刻 GET 出来」看到的正是这些值；默认值写错会让写路径与读路径对不上。
 *
 * `job_count` 刻意不在此处赋值：它是「已发布岗位数」的冗余统计，由 index.js 的 companyView() 实时算。
 *
 * `base.id` 是**编写用序号**（1、2、3…），不是实体 id：数组定义完之后由 assignSeedIds()
 * 统一换成 ULID，序号只用于种子数据内部互相引用（以及审计日志换算外键）。
 */
export function makeCompany(base) {
  const name = base.name ?? '未命名公司'
  return {
    id: base.id,
    name,
    short_name: base.short_name ?? name.slice(0, 1),
    website: base.website ?? '',
    careers_url: base.careers_url ?? '',
    discovery_url: base.discovery_url ?? '',
    logo_url: base.logo_url ?? '',
    industry: base.industry ?? '互联网',
    sub_industry: base.sub_industry ?? '',
    scale: base.scale ?? '1000-5000 人',
    nature: base.nature ?? '民营',
    location: base.location ?? '北京',
    description: base.description ?? `${name}（演示数据）：岗位由爬虫从官网《加入我们》页面采集，仅用于界面演示。`,
    contact_email: base.contact_email ?? 'hr@example.com',
    contact_phone: base.contact_phone ?? '010-8888-8888',
    enabled: base.enabled ?? true,
    verified: base.verified ?? false,
    priority: base.priority ?? 0,
    crawl_enabled: base.crawl_enabled ?? true,
    crawl_interval_seconds: base.crawl_interval_seconds ?? 21600,
    crawl_rules: base.crawl_rules ?? {},
    force_headless: base.force_headless ?? false,
    respect_robots: base.respect_robots ?? true,
    // 从未抓取过的公司必须是 null（omitempty 后整个键消失），不能是 '' 或 0
    last_crawled_at: pick(base, 'last_crawled_at', hoursAgo(3)),
    last_crawl_status: base.last_crawl_status ?? 'success',
    last_crawl_error: base.last_crawl_error ?? '',
    robots_allowed: base.robots_allowed ?? true,
    robots_checked_at: pick(base, 'robots_checked_at', hoursAgo(4)),
    source: base.source ?? 'manual',
    remark: base.remark ?? '',
    created_at: base.created_at ?? daysAgo(200, 9, 30),
    updated_at: base.updated_at ?? hoursAgo(3),
  }
}

export const companies = [
  makeCompany({ id: 1, name: '字节跳动', short_name: '字', industry: '互联网', sub_industry: '内容平台', location: '北京', priority: 100, scale: '10000 人以上', website: 'https://www.example.com', careers_url: 'https://www.example.com/join-us', discovery_url: 'https://www.example.com', verified: true }),
  makeCompany({ id: 2, name: '腾讯', short_name: '腾', industry: '互联网', sub_industry: '社交与游戏', location: '深圳', priority: 90, scale: '10000 人以上', website: 'https://www.example.com', careers_url: 'https://www.example.com/join-us', discovery_url: 'https://www.example.com', verified: true }),
  makeCompany({ id: 3, name: '阿里巴巴', short_name: '阿', industry: '互联网', sub_industry: '电商与云', location: '杭州', priority: 80, scale: '10000 人以上', website: 'https://www.example.com', careers_url: 'https://www.example.com/join-us', discovery_url: 'https://www.example.com', verified: true }),
  makeCompany({ id: 4, name: '华为', short_name: '华', industry: '智能制造', sub_industry: '通信设备', location: '武汉', priority: 70, scale: '10000 人以上', website: 'https://www.example.com', careers_url: 'https://www.example.com/join-us', discovery_url: 'https://www.example.com', verified: true }),
  makeCompany({ id: 5, name: '美团', short_name: '美', industry: '互联网', sub_industry: '本地生活', location: '北京', priority: 60, scale: '10000 人以上', website: 'https://www.example.com', careers_url: 'https://www.example.com/join-us', discovery_url: 'https://www.example.com', verified: true }),
  makeCompany({ id: 6, name: '小米', short_name: '米', industry: '智能制造', sub_industry: '消费电子', location: '北京', priority: 50, scale: '10000 人以上', website: 'https://www.example.com', careers_url: 'https://www.example.com/join-us', discovery_url: 'https://www.example.com', verified: true }),
  makeCompany({ id: 7, name: '网易', short_name: '网', industry: '互联网', sub_industry: '游戏与音乐', location: '杭州', priority: 40, scale: '10000 人以上', website: 'https://www.example.com', careers_url: 'https://www.example.com/join-us', discovery_url: 'https://www.example.com', last_crawl_status: 'partial', last_crawl_error: '部分岗位详情页解析失败，已跳过（40004）', verified: true }),
  makeCompany({ id: 8, name: '快手', short_name: '快', industry: '互联网', sub_industry: '短视频', location: '北京', priority: 30, scale: '10000 人以上', website: 'https://www.example.com', careers_url: 'https://www.example.com/join-us', discovery_url: 'https://www.example.com', verified: false }),
  makeCompany({ id: 9, name: '宁德时代', short_name: '宁', industry: '新能源', sub_industry: '动力电池', location: '宁德', priority: 20, scale: '10000 人以上', website: 'https://www.example.com', careers_url: 'https://www.example.com/join-us', discovery_url: 'https://www.example.com', crawl_enabled: false, verified: true }),
  makeCompany({ id: 10, name: '招商银行', short_name: '招', industry: '金融科技', sub_industry: '银行', location: '深圳', priority: 10, scale: '10000 人以上', website: 'https://www.example.com', careers_url: 'https://www.example.com/join-us', discovery_url: 'https://www.example.com', enabled: false, crawl_enabled: false, verified: true }),
  // 蔚来：官网已录入、招聘入口还没找到；robots 也还没探测过 —— 用来驱动「自动定位《加入我们》」这条演示路径
  makeCompany({ id: 11, name: '蔚来汽车', short_name: '蔚', industry: '新能源', sub_industry: '整车制造', location: '上海', priority: 0, scale: '10000 人以上', website: 'https://www.example.com', careers_url: '', discovery_url: '', crawl_enabled: false, last_crawled_at: null, last_crawl_status: '', robots_checked_at: null, verified: false }),
  makeCompany({ id: 12, name: '商汤科技', short_name: '商', industry: '人工智能', sub_industry: '计算机视觉', location: '上海', priority: 15, scale: '1000-5000 人', website: 'https://www.example.com', careers_url: 'https://www.example.com/join-us', discovery_url: 'https://www.example.com', verified: true, source: 'discovered' }),
]

/**
 * 编写期索引：键是上面写死的序号（1..12），**不是实体 id**。
 * 只供种子数据内部按序号取公司（makeJob 的 companyId）；按 id 查公司请用 findCompany()。
 */
const companyById = new Map(companies.map((c) => [c.id, c]))

// 实体 id 换成 ULID。必须在 companyById 建好之后执行：那张表的键是编写用序号。
assignSeedIds('company', companies)

/** 按 ULID 查公司。id 是不透明字符串，绝不做 Number() 转换（Number('') 是 0、Number('abc') 是 NaN） */
export function findCompany(id) {
  const target = String(id ?? '')
  return companies.find((company) => String(company.id) === target) || null
}

/* -------------------------------------------------------------------------- */
/* 岗位（JobDetailView；列表由 index.js 裁剪）                                   */
/* -------------------------------------------------------------------------- */

/** 各职位类别的通用描述模板，供批量生成的岗位复用（人工编写的 12 条会覆盖它） */
const CATEGORY_COPY = {
  技术: {
    summary: '负责核心业务系统的设计与研发，与产品、测试协作完成从需求到上线的完整闭环。',
    responsibilities:
      '参与业务需求评审，输出技术方案并落地；负责核心模块的编码、自测与上线；持续优化系统性能与稳定性，沉淀可复用的工程能力。',
    requirements_list: [
      '计算机相关专业本科及以上学历',
      '3 年以上相关开发经验，具备扎实的数据结构与算法基础',
      '熟悉主流工程化工具链，有性能优化实践经验',
      '具备良好的沟通协作能力与自驱力',
      '有大型分布式系统经验者优先',
    ],
    benefits: '五险一金、补充医疗、年度体检、弹性工作、免费三餐',
  },
  产品: {
    summary: '负责产品方向的规划与落地，串联用户需求、业务目标与研发资源。',
    responsibilities:
      '负责所辖方向的需求调研与竞品分析；输出产品方案与原型，推动研发按期交付；上线后跟踪核心指标并持续迭代。',
    requirements_list: [
      '本科及以上学历，3 年以上产品经验',
      '具备完整的产品从 0 到 1 落地经验',
      '逻辑清晰，能独立完成需求文档与原型',
      '数据敏感，能基于指标做决策',
      '有 B 端或企业服务产品经验者优先',
    ],
    benefits: '五险一金、补充医疗、年度体检、弹性工作',
  },
  设计: {
    summary: '负责产品界面的视觉与交互设计，输出可落地的高保真设计稿。',
    responsibilities:
      '负责产品界面的视觉与交互设计；与产品、前端协作推动设计还原；参与并维护设计规范与组件库。',
    requirements_list: [
      '设计相关专业本科及以上学历',
      '3 年以上 UI／交互设计经验，作品集完整',
      '熟练使用主流设计工具，具备组件化设计思维',
      '对细节与一致性有较高要求',
      '有 B 端产品设计经验者优先',
    ],
    benefits: '五险一金、补充医疗、年度体检、设计书籍报销',
  },
  运营: {
    summary: '负责内容与用户的日常运营，通过活动与内容策略带动增长。',
    responsibilities: '负责内容选题与生产排期；策划并执行运营活动，复盘数据效果；维护核心用户社群与创作者生态。',
    requirements_list: [
      '本科及以上学历，1 年以上内容或用户运营经验',
      '文字功底扎实，能独立完成内容策划',
      '对数据敏感，能通过复盘持续优化',
      '抗压能力强，可适应快节奏迭代',
      '有短视频平台运营经验者优先',
    ],
    benefits: '五险一金、补充医疗、年度体检、餐补',
  },
  数据: {
    summary: '负责业务数据的采集、建模与分析，为决策提供量化支撑。',
    responsibilities: '搭建并维护核心业务指标体系；完成专题分析并输出结论；与业务方共建数据看板与实验评估方法。',
    requirements_list: [
      '统计学、数学或计算机相关专业本科及以上学历',
      '熟练使用 SQL，掌握 Python 或 R 做数据分析',
      '具备指标体系搭建与归因分析经验',
      '能把分析结论讲清楚，推动业务落地',
      '有 A/B 实验设计经验者优先',
    ],
    benefits: '五险一金、补充医疗、年度体检、弹性工作',
  },
  市场: {
    summary: '负责品牌与产品的市场推广，拓展并维护渠道资源。',
    responsibilities: '制定并执行市场推广计划；负责渠道拓展与投放效果优化；策划行业活动，沉淀品牌内容资产。',
    requirements_list: [
      '本科及以上学历，1 年以上市场或渠道经验',
      '具备较强的资源整合与商务沟通能力',
      '能独立负责项目的策划与执行',
      '对投放数据敏感，能持续优化 ROI',
      '有 To B 市场经验者优先',
    ],
    benefits: '五险一金、补充医疗、年度体检、通讯补贴',
  },
}

const EDUCATION = {
  技术: '本科及以上',
  产品: '本科及以上',
  设计: '本科及以上',
  运营: '本科及以上',
  数据: '本科及以上',
  市场: '本科及以上',
}

let jobSeq = 0

/**
 * 构造一条岗位（字段集合 = JobDetailView）。
 *
 * 仅内部使用的键（**绝不出现在响应里**，index.js 的序列化器逐字段挑选）：
 *   - company_id           ：用于筛选与拼装 company 简要信息
 *   - changed_fields       ：给 revisionsOf() 生成演示修订用
 *   - revision_history     ：同上
 *
 * 薪资三兄弟的契约要小心：
 *   salary_min / salary_max 是 `*int`（可为 null → omitempty 后键消失），
 *   salary_text 才是给人看的原始文本；salary_months 只在 JobDetailView 里存在。
 */
export function makeJob(base) {
  // base.companyId 是编写用序号（1..12），不是实体 id：companyById 这张表就是按序号建的
  const company = base.company ?? companyById.get(base.companyId)
  const copy = CATEGORY_COPY[base.category] ?? CATEGORY_COPY.技术
  const publishedDaysAgo = base.publishedDaysAgo ?? 3
  const updatedHoursAgo = base.updatedHoursAgo ?? 26
  const months = pick(base, 'salary_months', 14)
  // null 表示「来源页面没写薪资」：此时 salary_min/max/salary_text 三个键在响应里全部消失
  const salaryMin = pick(base, 'salary_min', null)
  const salaryMax = pick(base, 'salary_max', null)
  const salaryText =
    salaryMin && salaryMax
      ? `${Math.round(salaryMin / 1000)}K-${Math.round(salaryMax / 1000)}K·${months}薪`
      : ''
  const status = base.status ?? 'approved'
  const publishedAt = base.published_at === null ? null : daysAgo(publishedDaysAgo)

  return {
    // 编写用序号：jobs 数组定义完之后由 assignSeedIds('job', jobs) 换成 ULID
    id: ++jobSeq,
    company_id: company.id,
    title: base.title,
    category: base.category,
    industry: company.industry,
    employment_type: base.employment_type ?? '全职',
    city: base.city,
    district: base.district ?? '',
    salary_text: salaryText,
    salary_min: salaryMin,
    salary_max: salaryMax,
    salary_months: months,
    experience: base.experience ?? '3-5年',
    education: base.education ?? EDUCATION[base.category] ?? '本科及以上',
    tags: base.tags ?? [],
    summary: base.summary ?? copy.summary,
    description: base.description ?? copy.summary,
    responsibilities: base.responsibilities ?? copy.responsibilities,
    requirements: (base.requirements_list ?? copy.requirements_list).join('；'),
    requirements_list: base.requirements_list ?? copy.requirements_list,
    benefits: base.benefits ?? copy.benefits,
    contact_person: base.contact_person ?? '招聘负责人',
    contact_email: 'hr@example.com',
    contact_phone: '010-8888-8888',
    contact_wechat: '',
    apply_method: '官网投递',
    apply_url: company.careers_url || '',
    published_at: publishedAt,
    deadline_at: pick(base, 'deadline_at', null),
    source_updated_at: hoursAgo(updatedHoursAgo + 2),
    last_updated_at: hoursAgo(updatedHoursAgo),
    last_seen_at: hoursAgo(2),
    crawled_at: hoursAgo(2),
    job_code: base.job_code ?? `JR-${String(jobSeq).padStart(4, '0')}`,
    department: base.department ?? '',
    headcount: pick(base, 'headcount', 2),
    work_mode: base.work_mode ?? '坐班',
    country: '中国',
    province: base.province ?? '',
    address: base.address ?? `${base.city}市${base.district || ''}中关村大街 1 号（演示地址）`,
    location_text: base.location_text ?? '',
    is_direct_hire: true,
    source_url: `${company.careers_url || 'https://www.example.com/join-us'}#job-${jobSeq}`,
    source_site: 'www.example.com',
    source_company_name: company.name,
    status,
    quality_score: base.quality_score ?? 88 + (jobSeq % 9),
    view_count: base.view_count ?? 120 + jobSeq * 7,
    revision: base.revision ?? 1,
    changed_fields: base.changed_fields,
    revision_history: base.revision_history,
    extractor: 'jsonld',
    parse_confidence: 0.86,
    // 审核信息：待审核岗位没有审核人；已发布/已驳回都有
    reviewer_id: pick(base, 'reviewer_id', status === 'approved' || status === 'rejected' ? 1 : null),
    reviewed_at: pick(base, 'reviewed_at', status === 'approved' || status === 'rejected' ? daysAgo(publishedDaysAgo, 15, 30) : null),
    reject_reason: base.reject_reason ?? '',
    expired_at: status === 'expired' ? hoursAgo(updatedHoursAgo) : null,
    created_at: daysAgo(publishedDaysAgo + 1, 9, 0),
    updated_at: hoursAgo(updatedHoursAgo),
  }
}

/* —— 12 条人工编写的岗位：内容、薪资、地点与设计稿一致 —— */
const handwritten = [
  makeJob({
    title: '高级前端开发工程师（React）',
    companyId: 1,
    category: '技术',
    city: '北京',
    district: '海淀区',
    address: '北京市海淀区中关村大街 1 号（演示地址）',
    salary_min: 25000,
    salary_max: 40000,
    salary_months: 14,
    experience: '3-5年',
    description:
      '负责企业级中后台产品的前端架构与研发，主导组件库与工程化建设，和产品、设计一起把复杂的业务场景做成好用、稳定的界面。',
    responsibilities:
      '负责中后台产品的前端架构设计与核心模块开发；主导组件库与工程化工具链建设，持续提升研发效率；推动性能与稳定性优化，建立前端质量度量体系；参与技术方案评审，指导初级工程师成长。',
    requirements_list: [
      '计算机相关专业本科及以上学历',
      '3 年以上前端开发经验，精通 React 及其生态',
      '熟悉 TypeScript、工程化与性能优化',
      '具备良好的沟通协作与自驱力',
      '有大型 Web 应用架构经验者优先',
    ],
    publishedDaysAgo: 3,
    updatedHoursAgo: 26,
    quality_score: 96,
    view_count: 1284,
    // revision=2 让变更历史页有「一次真实改动」可看；changed_fields 的口径与后端一致
    revision: 2,
    changed_fields: ['salary_max', 'salary_text'],
    revision_history: [{ revision: 1, changed_fields: null, created_at: daysAgo(9, 10, 12) }],
  }),
  makeJob({
    title: '产品经理（企业服务方向）',
    companyId: 2,
    category: '产品',
    city: '深圳',
    district: '南山区',
    address: '深圳市南山区科技园南区（演示地址）',
    salary_min: 20000,
    salary_max: 35000,
    salary_months: 16,
    experience: '3-5年',
    description:
      '面向企业客户设计协同与效率类产品，从行业调研、方案设计到客户交付全程参与，对产品的商业结果负责。',
    responsibilities:
      '负责企业服务方向的产品规划与需求管理；深入客户现场完成调研，输出可落地的产品方案；协调设计、研发、售前资源推动版本交付；跟踪客户使用数据与续约情况，持续迭代产品。',
    requirements_list: [
      '本科及以上学历，3 年以上 To B 产品经验',
      '有完整的企业级产品从 0 到 1 落地经验',
      '逻辑清晰，能独立完成需求文档与原型',
      '具备较强的客户沟通与方案讲解能力',
      '有 SaaS 或协同办公产品经验者优先',
    ],
    publishedDaysAgo: 5,
    updatedHoursAgo: 30,
    quality_score: 93,
    view_count: 862,
  }),
  makeJob({
    title: '算法工程师（NLP 方向）',
    companyId: 3,
    category: '技术',
    city: '杭州',
    district: '余杭区',
    address: '杭州市余杭区文一西路 969 号（演示地址）',
    salary_min: 30000,
    salary_max: 50000,
    salary_months: 16,
    experience: '3-5年',
    education: '硕士及以上',
    description:
      '负责自然语言处理算法在搜索、推荐与客服场景的落地，从数据建设、模型训练到线上效果优化全链路参与。',
    responsibilities:
      '负责文本理解、信息抽取与生成类算法的研发与落地；建设并维护训练与评测数据集；跟进线上效果，持续优化模型精度与推理成本；跟踪学术界与工业界前沿方案并推动应用。',
    requirements_list: [
      '计算机、人工智能相关专业硕士及以上学历',
      '3 年以上 NLP 算法研发经验',
      '熟悉 PyTorch 等框架，有大规模模型训练经验',
      '具备扎实的机器学习与统计学基础',
      '有顶会论文或开源项目贡献者优先',
    ],
    publishedDaysAgo: 1,
    updatedHoursAgo: 8,
    quality_score: 97,
    view_count: 2140,
    revision: 3,
    changed_fields: ['requirements_list', 'requirements'],
    revision_history: [
      { revision: 1, changed_fields: null, created_at: daysAgo(20, 10, 12) },
      { revision: 2, changed_fields: ['salary_min', 'salary_max', 'salary_text'], created_at: daysAgo(6, 14, 5) },
    ],
  }),
  makeJob({
    title: 'UI 设计师（企业云产品）',
    companyId: 4,
    category: '设计',
    city: '武汉',
    district: '光谷',
    province: '湖北',
    address: '武汉市东湖新技术开发区光谷大道 77 号（演示地址）',
    salary_min: 18000,
    salary_max: 30000,
    salary_months: 14,
    experience: '3-5年',
    description:
      '负责企业云产品的界面视觉与交互设计，参与设计规范与组件库建设，让复杂的控制台变得清晰易用。',
    responsibilities:
      '负责云产品控制台的视觉与交互设计；输出高保真设计稿并跟进前端还原；参与设计规范与组件库的维护；通过可用性测试持续改进关键流程。',
    requirements_list: [
      '设计相关专业本科及以上学历',
      '3 年以上 UI／交互设计经验，作品集完整',
      '熟练使用主流设计工具，具备组件化设计思维',
      '对细节与一致性有较高要求',
      '有 B 端或云产品设计经验者优先',
    ],
    publishedDaysAgo: 0,
    updatedHoursAgo: 3,
    quality_score: 91,
    view_count: 517,
  }),
  makeJob({
    title: '后端开发工程师（Go）',
    companyId: 5,
    category: '技术',
    city: '北京',
    district: '朝阳区',
    address: '北京市朝阳区望京街 10 号（演示地址）',
    salary_min: 28000,
    salary_max: 45000,
    salary_months: 15,
    experience: '3-5年',
    description:
      '负责交易与履约链路的后端服务研发，保障高峰期系统的稳定性与一致性，参与服务治理与容量规划。',
    responsibilities:
      '负责核心交易服务的设计、开发与维护；参与服务拆分与治理，提升系统可观测性；负责容量评估与稳定性演练；沉淀通用的中间件与工具能力。',
    requirements_list: [
      '计算机相关专业本科及以上学历',
      '3 年以上 Go 或 Java 后端开发经验',
      '熟悉 MySQL、Redis、消息队列等常用组件',
      '有高并发系统的设计与排查经验',
      '熟悉 Kubernetes 者优先',
    ],
    publishedDaysAgo: 6,
    updatedHoursAgo: 40,
    quality_score: 90,
    view_count: 733,
    deadline_at: daysAgo(-24, 23, 59),
  }),
  makeJob({
    title: '数据分析师（硬件业务）',
    companyId: 6,
    category: '数据',
    city: '北京',
    district: '海淀区',
    address: '北京市海淀区清河中街 68 号（演示地址）',
    salary_min: 22000,
    salary_max: 35000,
    salary_months: 14,
    experience: '1-3年',
    description:
      '负责硬件业务线的经营分析与用户洞察，搭建指标体系，用数据回答「卖得好不好、为什么」。',
    responsibilities: '搭建并维护硬件业务的指标体系与数据看板；完成销量、渠道、用户留存等专题分析；支持新品上市的效果评估与复盘。',
    requirements_list: [
      '统计学、数学或计算机相关专业本科及以上学历',
      '1 年以上数据分析经验，熟练使用 SQL',
      '掌握 Python 或 R 进行数据处理与建模',
      '能把分析结论讲清楚并推动落地',
      '有硬件或消费品行业经验者优先',
    ],
    publishedDaysAgo: 2,
    updatedHoursAgo: 20,
    quality_score: 87,
    view_count: 402,
  }),
  makeJob({
    title: '游戏客户端开发工程师（Unity）',
    companyId: 7,
    category: '技术',
    city: '杭州',
    district: '滨江区',
    province: '浙江',
    address: '杭州市滨江区网商路 599 号（演示地址）',
    salary_min: 26000,
    salary_max: 42000,
    salary_months: 15,
    experience: '3-5年',
    description:
      '负责手游客户端的玩法与系统开发，和策划、美术紧密协作，把设计意图还原成流畅的玩家体验。',
    responsibilities: '负责客户端玩法系统的设计与实现；参与性能优化与多机型适配；建设并维护客户端基础框架与工具链。',
    requirements_list: [
      '计算机相关专业本科及以上学历',
      '3 年以上 Unity 客户端开发经验',
      '熟悉 C#、常用数据结构与设计模式',
      '有完整的项目上线经验',
      '有性能优化或图形渲染经验者优先',
    ],
    publishedDaysAgo: 4,
    updatedHoursAgo: 28,
    quality_score: 89,
    view_count: 655,
  }),
  makeJob({
    title: '内容运营（短视频）',
    companyId: 8,
    category: '运营',
    city: '北京',
    district: '海淀区',
    address: '北京市海淀区上地西路 6 号（演示地址）',
    salary_min: 15000,
    salary_max: 25000,
    salary_months: 14,
    experience: '1-3年',
    description: '负责短视频垂类的内容运营，通过选题策划与创作者扶持带动内容供给与消费增长。',
    responsibilities: '负责垂类内容选题与排期；策划并执行话题活动，复盘数据效果；维护核心创作者关系，推动优质内容产出。',
    requirements_list: [
      '本科及以上学历，1 年以上内容运营经验',
      '文字功底扎实，能独立完成内容策划',
      '熟悉短视频平台的内容分发逻辑',
      '对数据敏感，能通过复盘持续优化',
      '有垂类社区或 MCN 经验者优先',
    ],
    publishedDaysAgo: 7,
    updatedHoursAgo: 50,
    quality_score: 84,
    view_count: 388,
  }),
  makeJob({
    title: '测试开发工程师（质量平台）',
    companyId: 1,
    category: '技术',
    city: '上海',
    district: '杨浦区',
    address: '上海市杨浦区政立路 485 号（演示地址）',
    salary_min: 24000,
    salary_max: 38000,
    salary_months: 14,
    experience: '3-5年',
    description: '负责质量平台与自动化测试体系建设，用工具和平台把重复的验证工作交给机器。',
    responsibilities: '负责自动化测试框架与质量平台的研发；建设持续集成流水线中的质量卡点；参与线上问题复盘并推动质量改进。',
    requirements_list: [
      '计算机相关专业本科及以上学历',
      '3 年以上测试开发经验，熟悉 Java 或 Go',
      '有自动化测试框架或平台建设经验',
      '熟悉 CI/CD 流程与常用工具',
      '有性能测试经验者优先',
    ],
    publishedDaysAgo: 9,
    updatedHoursAgo: 60,
    quality_score: 86,
    view_count: 296,
  }),
  makeJob({
    title: '前端开发工程师（增长方向）',
    companyId: 2,
    category: '技术',
    city: '深圳',
    district: '南山区',
    address: '深圳市南山区科技园南区（演示地址）',
    salary_min: 22000,
    salary_max: 36000,
    salary_months: 15,
    experience: '1-3年',
    status: 'pending_review',
    publishedDaysAgo: 1,
    updatedHoursAgo: 5,
    quality_score: 82,
  }),
  makeJob({
    title: '硬件结构工程师（终端）',
    companyId: 4,
    category: '技术',
    city: '东莞',
    district: '松山湖',
    province: '广东',
    address: '东莞市松山湖园区（演示地址）',
    salary_min: 20000,
    salary_max: 34000,
    salary_months: 14,
    experience: '3-5年',
    status: 'pending_review',
    publishedDaysAgo: 2,
    updatedHoursAgo: 12,
    quality_score: 80,
  }),
  makeJob({
    title: '市场推广专员（云业务）',
    companyId: 3,
    category: '市场',
    city: '杭州',
    district: '余杭区',
    salary_min: 16000,
    salary_max: 28000,
    salary_months: 14,
    experience: '1-3年',
    status: 'rejected',
    reject_reason: '岗位信息与官网《加入我们》页面不一致，缺少任职要求',
    publishedDaysAgo: 12,
    updatedHoursAgo: 80,
    quality_score: 46,
  }),
]

/* —— 批量补充的岗位：用一行数据描述一条，凑出有说服力的分页与筛选效果 —— */
const BULK = [
  ['资深 Java 开发工程师（交易）', 5, '技术', '北京', '朝阳区', 30000, 48000, 15, '3-5年', 'approved'],
  ['推荐算法工程师', 1, '技术', '北京', '海淀区', 32000, 55000, 14, '3-5年', 'approved'],
  ['iOS 开发工程师', 2, '技术', '深圳', '南山区', 24000, 38000, 15, '3-5年', 'approved'],
  ['云原生平台工程师', 3, '技术', '杭州', '余杭区', 28000, 46000, 16, '3-5年', 'approved'],
  ['嵌入式软件工程师', 4, '技术', '武汉', '光谷', 20000, 33000, 14, '3-5年', 'approved'],
  ['数据仓库工程师', 5, '数据', '北京', '朝阳区', 26000, 40000, 15, '3-5年', 'approved'],
  ['服务端开发工程师（Node.js）', 8, '技术', '北京', '海淀区', 23000, 36000, 14, '1-3年', 'approved'],
  ['视觉设计师（品牌方向）', 7, '设计', '杭州', '滨江区', 16000, 26000, 15, '1-3年', 'approved'],
  ['用户运营（社区方向）', 8, '运营', '北京', '海淀区', 14000, 22000, 14, '1-3年', 'approved'],
  ['商业分析师', 6, '数据', '北京', '海淀区', 20000, 32000, 14, '1-3年', 'approved'],
  ['产品运营（B 端）', 2, '运营', '深圳', '南山区', 17000, 28000, 15, '1-3年', 'approved'],
  ['交互设计师', 3, '设计', '杭州', '余杭区', 19000, 31000, 16, '3-5年', 'approved'],
  ['安全工程师（应用安全）', 1, '技术', '上海', '杨浦区', 27000, 43000, 14, '3-5年', 'approved'],
  ['测试工程师（自动化）', 4, '技术', '东莞', '松山湖', 18000, 29000, 14, '1-3年', 'approved'],
  ['供应链数据分析师', 6, '数据', '北京', '海淀区', 19000, 30000, 14, '1-3年', 'approved'],
  ['渠道市场经理', 3, '市场', '杭州', '余杭区', 21000, 34000, 16, '3-5年', 'approved'],
  ['前端开发工程师（小程序）', 5, '技术', '北京', '朝阳区', 21000, 34000, 15, '1-3年', 'approved'],
  ['运维开发工程师（SRE）', 2, '技术', '深圳', '南山区', 25000, 41000, 15, '3-5年', 'approved'],
  ['品牌视觉设计师', 8, '设计', '北京', '海淀区', 15000, 24000, 14, '1-3年', 'approved'],
  ['机器学习平台工程师', 12, '技术', '上海', '徐汇区', 30000, 50000, 15, '3-5年', 'approved'],
  ['自动驾驶感知算法工程师', 11, '技术', '上海', '嘉定区', 32000, 52000, 15, '3-5年', 'approved'],
  ['电芯研发工程师', 9, '技术', '宁德', '蕉城区', 22000, 36000, 14, '3-5年', 'approved'],
  ['风控策略分析师', 10, '数据', '深圳', '福田区', 24000, 38000, 16, '3-5年', 'approved'],
  ['招聘专员（技术招聘）', 12, '运营', '上海', '徐汇区', 13000, 20000, 14, '1-3年', 'approved'],
  ['后端开发工程师（支付）', 10, '技术', '深圳', '福田区', 26000, 42000, 16, '3-5年', 'pending_review'],
  ['内容审核专员', 8, '运营', '北京', '海淀区', 11000, 17000, 14, '1-3年', 'pending_review'],
  ['市场活动策划', 6, '市场', '北京', '海淀区', 15000, 24000, 14, '1-3年', 'pending_review'],
  ['资深产品经理（增长）', 1, '产品', '北京', '海淀区', 30000, 48000, 14, '5-10年', 'pending_review'],
  ['技术项目经理', 4, '产品', '武汉', '光谷', 23000, 36000, 14, '3-5年', 'offline'],
  ['海外市场经理', 9, '市场', '宁德', '蕉城区', 25000, 40000, 14, '3-5年', 'offline'],
  ['资深测试专家', 7, '技术', '杭州', '滨江区', 28000, 44000, 15, '5-10年', 'rejected'],
  // 薪资留空（null）：驱动「面议」这条真实分支 —— salary_min/max/salary_text 三个键都会消失
  ['行政专员', 10, '运营', '深圳', '福田区', null, null, null, '1-3年', 'rejected'],
]

const bulkJobs = BULK.map(([title, companyId, category, city, district, min, max, months, experience, status]) =>
  makeJob({
    title,
    companyId,
    category,
    city,
    district,
    salary_min: min,
    salary_max: max,
    salary_months: months,
    experience,
    status,
    reject_reason: status === 'rejected' ? '岗位描述缺少关键信息，或与来源页面不一致' : '',
    publishedDaysAgo: 1 + (title.length % 12),
    updatedHoursAgo: 6 + (title.length % 60),
    // 未发布的岗位常常还没有「来源页标注的发布时间」；published_at 为空时该键整体消失
    published_at: status === 'pending_review' ? null : undefined,
  }),
)

export const jobs = [...handwritten, ...bulkJobs]

/** 岗位 id 同样是 ULID（按编写用序号升序分配，保证「id 越大越新」仍然成立） */
assignSeedIds('job', jobs)

export function findAllJob(id) {
  const target = String(id ?? '')
  return jobs.find((job) => String(job.id) === target) || null
}

/* -------------------------------------------------------------------------- */
/* 岗位变更历史（RevisionView）                                                  */
/* -------------------------------------------------------------------------- */

/**
 * 内容快照的键集合 = `internal/service/revision.go` 的 contentFields.snapshot()。
 * 多一个键、少一个键都会让「变更字段」的展示与真实后端不一致。
 */
export function contentSnapshot(job) {
  return {
    title: job.title,
    category: job.category,
    employment_type: job.employment_type,
    city: job.city,
    district: job.district,
    address: job.address,
    salary_text: job.salary_text,
    experience: job.experience,
    education: job.education,
    description: job.description,
    responsibilities: job.responsibilities,
    requirements: job.requirements,
    requirements_list: job.requirements_list,
    benefits: job.benefits,
    contact_email: job.contact_email,
    contact_phone: job.contact_phone,
    apply_url: job.apply_url,
    published_at: job.published_at,
  }
}

/**
 * 生成某岗位的变更记录（服务端按 revision desc 返回，最新在前）。
 *
 * 两个刻意的口径：
 *   - 初始修订（revision=1）的 changed_fields 为 **null**：真实后端记录创建时传入的是 nil，
 *     序列化后该键整体消失，而不是 ['__created__'] 这种自造标记；
 *   - 每条修订的 snapshot 都用当前内容快照（演示数据不做历史版本归档），
 *     形状与真实后端一致，只是值不会随时间倒流。
 *
 * 修订 id 也是 ULID：同一批种子数据在模块加载期按固定顺序生成一次，刷新后完全一致
 * （不能像原来那样 `job.id * 100 + revision`——ULID 是字符串，做不了算术）。
 */
export function revisionsOf(job) {
  const snapshot = contentSnapshot(job)
  const current = job.revision ?? 1
  const rows = [
    {
      id: nextUlid(),
      revision: current,
      changed_fields: job.changed_fields ?? null,
      content_hash: demoHash(`${job.id}:${current}`),
      source_url: job.source_url,
      trigger: 'crawl',
      snapshot,
      created_at: job.last_updated_at,
    },
  ]
  for (const item of job.revision_history ?? []) {
    rows.push({
      id: nextUlid(),
      revision: item.revision,
      changed_fields: item.changed_fields ?? null,
      content_hash: demoHash(`${job.id}:${item.revision}`),
      source_url: job.source_url,
      trigger: item.trigger ?? 'crawl',
      snapshot,
      created_at: item.created_at,
    })
  }
  return rows.sort((a, b) => b.revision - a.revision)
}

/* -------------------------------------------------------------------------- */
/* 用户 / 角色 / 策略                                                          */
/* -------------------------------------------------------------------------- */

/**
 * 构造一个用户（UserView）。
 *
 * 注意这里没有 role_labels / tone / initial / registered_at：
 * 前者由 utils/format.js 从 roles 派生，后者在真实契约里叫 created_at。
 */
function makeUser(base) {
  return {
    id: base.id,
    email: base.email,
    nickname: base.nickname,
    avatar_url: base.avatar_url ?? '',
    phone: base.phone ?? '',
    status: base.status ?? 'active',
    email_verified: base.email_verified ?? true,
    roles: base.roles,
    last_login_at: base.last_login_at ?? null,
    remark: base.remark ?? '',
    created_at: base.created_at,
    updated_at: base.updated_at ?? base.last_login_at ?? base.created_at,
  }
}

export const users = [
  makeUser({ id: 1, email: 'zhangwei@example.com', nickname: '张伟', roles: ['admin'], status: 'active', created_at: daysAgo(320, 9, 30), last_login_at: hoursAgo(2), phone: '138-8888-0001', remark: '系统初始管理员（演示数据）' }),
  makeUser({ id: 2, email: 'li@example.com', nickname: '李娜', roles: ['admin'], status: 'active', created_at: daysAgo(320, 9, 30), last_login_at: hoursAgo(2) }),
  makeUser({ id: 3, email: 'wang@example.com', nickname: '王强', roles: ['reviewer'], status: 'disabled', created_at: daysAgo(320, 9, 30), last_login_at: daysAgo(1, 18, 20), remark: '违反操作规范，已停用' }),
  makeUser({ id: 4, email: 'chen@example.com', nickname: '陈晨', roles: ['reviewer'], status: 'active', created_at: daysAgo(180, 14, 5), last_login_at: daysAgo(3, 14, 5) }),
  makeUser({ id: 5, email: 'liu@example.com', nickname: '刘洋', roles: ['user'], status: 'active', created_at: daysAgo(320, 11, 40), last_login_at: daysAgo(3, 11, 40) }),
  makeUser({ id: 6, email: 'sunqi@example.com', nickname: '孙琪', roles: ['reviewer'], status: 'active', created_at: daysAgo(150, 10, 0), last_login_at: hoursAgo(6) }),
  makeUser({ id: 7, email: 'zhaomin@example.com', nickname: '赵敏', roles: ['user'], status: 'active', created_at: daysAgo(96, 16, 20), last_login_at: hoursAgo(20) }),
  makeUser({ id: 8, email: 'zhouhang@example.com', nickname: '周航', roles: ['user'], status: 'active', created_at: daysAgo(74, 21, 10), last_login_at: daysAgo(2, 21, 10) }),
  makeUser({ id: 9, email: 'wuyan@example.com', nickname: '吴燕', roles: ['reviewer'], status: 'active', created_at: daysAgo(210, 9, 50), last_login_at: hoursAgo(9) }),
  // locked = 连续登录失败被临时锁定，与 disabled 是两种不同状态（真实枚举里都存在）
  makeUser({ id: 10, email: 'zhengkai@example.com', nickname: '郑凯', roles: ['user'], status: 'locked', created_at: daysAgo(300, 10, 0), last_login_at: daysAgo(26, 10, 0) }),
  makeUser({ id: 11, email: 'fenghua@example.com', nickname: '冯华', roles: ['user'], status: 'active', created_at: daysAgo(45, 12, 30), last_login_at: hoursAgo(30) }),
  // pending = 注册后还没验证邮箱：登录会被 20004 拦下，正好演示这条分支
  makeUser({ id: 12, email: 'hejuan@example.com', nickname: '何娟', roles: ['user'], status: 'pending', email_verified: false, created_at: daysAgo(12, 8, 15), last_login_at: hoursAgo(48) }),
]

/** 用户 id 换成 ULID（下面审计日志里的 actor_id 也按同一批 ULID 换算） */
assignSeedIds('user', users)

/**
 * 回填岗位的审核人外键。
 *
 * jobs 数组比 users 数组先定义，所以 makeJob 里写的 `reviewer_id: 1` 是**编写用序号**，
 * 只有在用户 id 分配完成之后才能换算成真实 ULID。漏掉这一步，
 * 岗位详情里的 reviewer_id 就会留下一个既不是字符串主键、也查不到实体的 1。
 */
for (const job of jobs) {
  if (job.reviewer_id !== null && job.reviewer_id !== undefined) {
    job.reviewer_id = userRef(job.reviewer_id)
  }
}

/**
 * 角色元数据（RoleView 的静态部分）。
 *
 * name/display_name/description/level 抄自 `cmd/seed/main.go` 的 roleSeeds：
 * 权限由 Casbin 策略决定，roles 表只放展示信息 —— 两边写重复的权限描述一定会打架。
 * permissions / user_count 由 index.js 从 db.policies、db.users 实时拼装。
 */
export const roles = [
  {
    name: 'admin',
    display_name: '管理员',
    description: '拥有全部权限：管理公司、系统配置、用户与角色，并可按需调整审核结果',
    is_system: true,
    level: 100,
  },
  {
    name: 'reviewer',
    display_name: '审核员',
    description: '审核爬取到的岗位（通过/驳回），只读查看公司与抓取记录',
    is_system: true,
    level: 50,
  },
  {
    name: 'user',
    display_name: '用户',
    description: '浏览已发布的直招岗位，维护个人资料与登录会话',
    is_system: true,
    level: 10,
  },
]

/**
 * Casbin 权限策略：每行是 [角色, 接口路径, HTTP 方法]（`internal/casbin/model.go` 的 defaultPolicies）。
 *
 * 为什么保持「三元组数组」而不是 {role, obj, act} 对象：
 * `/admin/authz/policies` 返回的 policies 是 `[][]string`（Go 直接吐 casbin 的原始规则），
 * 而 RoleView.permissions 也是同一份数据。用对象会把两处都带偏。
 */
export const policies = [
  ['user', '/api/v1/jobs', 'GET'],
  ['user', '/api/v1/jobs/:id', 'GET'],
  ['user', '/api/v1/companies', 'GET'],
  ['user', '/api/v1/companies/:id', 'GET'],
  ['user', '/api/v1/meta/*', 'GET'],
  ['user', '/api/v1/me', 'GET'],
  ['user', '/api/v1/me/*', '*'],
  ['reviewer', '/api/v1/admin/jobs', 'GET'],
  ['reviewer', '/api/v1/admin/jobs/:id', 'GET'],
  ['reviewer', '/api/v1/admin/jobs/:id/review', 'POST'],
  ['reviewer', '/api/v1/admin/jobs/:id/revisions', 'GET'],
  ['reviewer', '/api/v1/admin/companies', 'GET'],
  ['reviewer', '/api/v1/admin/crawl-runs', 'GET'],
  ['reviewer', '/api/v1/admin/crawl-runs/:id', 'GET'],
  ['reviewer', '/api/v1/admin/stats', 'GET'],
  ['admin', '/api/v1/*', '*'],
]

/** 角色继承关系（g 规则）：reviewer ⊃ user，admin ⊃ reviewer */
export const groupingRules = [
  ['reviewer', 'user'],
  ['admin', 'reviewer'],
]

/* -------------------------------------------------------------------------- */
/* 系统配置（SettingView）                                                     */
/* -------------------------------------------------------------------------- */

/**
 * 爬虫收集范围。字段与 `internal/crawler/scope.go` 的 Scope 一一对应。
 * 注意这里没有 updated_at —— 在真实契约里 updated_at 属于 SettingView，不属于 Scope。
 */
export const crawlScope = {
  job_types: ['技术', '产品', '设计', '运营', '市场', '职能'],
  industries: ['互联网', '人工智能', '金融科技', '智能制造', '新能源'],
  employment_types: ['全职', '实习', '校招', '社招'],
  locations: [],
  keywords: [],
  exclude_keywords: ['外包', '劳务派遣', '中介'],
  min_quality_score: 40,
}

/**
 * 配置项。
 *
 * **只有 crawler.scope 一项** —— `SettingService.EnsureDefaults` 的注释写得很清楚：
 * 「目前仅 crawler.scope 一项内置配置」。早期 mock 里那 9 条 site.name / crawler.delay_ms
 * 是凭空造的，接真后端会整页空白。
 * value 恒为 JSON 对象（Go 是 map[string]any）；标量配置在这个系统里不存在。
 */
export const settings = [
  {
    id: 1,
    key: 'crawler.scope',
    value: { ...crawlScope },
    description: '爬虫收集的工作类型与行业范围',
    group: 'crawler',
    is_system: true,
    updated_by: 'zhangwei@example.com',
    updated_at: hoursAgo(4),
  },
]

/** 配置项 id 换成 ULID（按 id 查询配置的能力在真实后端并不存在：走的是 :key） */
assignSeedIds('setting', settings)

/* -------------------------------------------------------------------------- */
/* 抓取运行记录 / 明细日志                                                      */
/* -------------------------------------------------------------------------- */

function makeRun(base) {
  return {
    id: base.id,
    company_id: base.company_id,
    company_name: base.company_name,
    trigger: base.trigger,
    status: base.status,
    skip_reason: '', // 真实后端从未写入该字段（finishRun 只写 error_message），因此始终为空
    careers_url: base.careers_url,
    started_at: base.started_at,
    finished_at: base.status === 'running' ? null : base.finished_at,
    duration_ms: base.status === 'running' ? 0 : (base.duration_ms ?? 0),
    pages_fetched: base.pages_fetched ?? 0,
    jobs_found: base.jobs_found ?? 0,
    jobs_created: base.jobs_created ?? 0,
    jobs_updated: base.jobs_updated ?? 0,
    jobs_unchanged: base.jobs_unchanged ?? 0,
    jobs_filtered: base.jobs_filtered ?? 0,
    jobs_expired: base.jobs_expired ?? 0,
    used_headless: base.used_headless ?? false,
    error_message: base.error_message ?? '',
    // detail 也是「后端从未写入」的字段（CrawlRun.Detail 目前没有赋值点），保持为空对象
    detail: base.detail ?? {},
  }
}

export const crawlRuns = [
  makeRun({ id: 328, company_id: companyRef(1), company_name: '字节跳动', trigger: 'schedule', status: 'success', careers_url: 'https://www.example.com/join-us', started_at: daysAgo(0, 10, 12), finished_at: daysAgo(0, 10, 12), duration_ms: 2140, pages_fetched: 12, jobs_found: 12, jobs_created: 8, jobs_updated: 3, jobs_filtered: 1 }),
  makeRun({ id: 327, company_id: companyRef(2), company_name: '腾讯', trigger: 'manual', status: 'success', careers_url: 'https://www.example.com/join-us', started_at: daysAgo(0, 9, 41), finished_at: daysAgo(0, 9, 41), duration_ms: 1830, pages_fetched: 9, jobs_found: 9, jobs_created: 6, jobs_updated: 1, jobs_filtered: 2 }),
  makeRun({ id: 326, company_id: companyRef(3), company_name: '阿里巴巴', trigger: 'manual', status: 'running', careers_url: 'https://www.example.com/join-us', started_at: daysAgo(0, 9, 40), pages_fetched: 5, jobs_found: 5, jobs_created: 3 }),
  makeRun({ id: 325, company_id: companyRef(4), company_name: '华为', trigger: 'schedule', status: 'failed', careers_url: 'https://www.example.com/join-us', started_at: daysAgo(0, 8, 2), finished_at: daysAgo(0, 8, 2), duration_ms: 940, error_message: '抓取目标页面失败：连接超时（40003）' }),
  makeRun({ id: 324, company_id: companyRef(5), company_name: '美团', trigger: 'schedule', status: 'success', careers_url: 'https://www.example.com/join-us', started_at: daysAgo(1, 10, 12), finished_at: daysAgo(1, 10, 12), duration_ms: 2610, pages_fetched: 14, jobs_found: 14, jobs_created: 5, jobs_updated: 6, jobs_unchanged: 2, jobs_filtered: 1, used_headless: true }),
  makeRun({ id: 323, company_id: companyRef(7), company_name: '网易', trigger: 'schedule', status: 'partial', careers_url: 'https://www.example.com/join-us', started_at: daysAgo(1, 8, 30), finished_at: daysAgo(1, 8, 30), duration_ms: 4120, pages_fetched: 7, jobs_found: 7, jobs_created: 4, jobs_updated: 1, jobs_unchanged: 1, jobs_expired: 1, error_message: '部分岗位详情页解析失败，已跳过（40004）' }),
  makeRun({ id: 322, company_id: companyRef(6), company_name: '小米', trigger: 'manual', status: 'success', careers_url: 'https://www.example.com/join-us', started_at: daysAgo(2, 15, 20), finished_at: daysAgo(2, 15, 20), duration_ms: 1980, pages_fetched: 8, jobs_found: 8, jobs_created: 3, jobs_updated: 2, jobs_unchanged: 3 }),
  makeRun({ id: 321, company_id: companyRef(9), company_name: '宁德时代', trigger: 'schedule', status: 'skipped', careers_url: 'https://www.example.com/join-us', started_at: daysAgo(2, 8, 0), finished_at: daysAgo(2, 8, 0), duration_ms: 320, error_message: 'robots.txt 不允许抓取该路径（40002）' }),
]

// 抓取记录 id 换成 ULID。注意数组本身是 328→321 倒序写的，assignSeedIds 会按序号升序分配，
// 这样「id 越大越新」（列表默认倒序）与原来完全一致。
assignSeedIds('run', crawlRuns)

/**
 * 生成一次运行的明细日志（CrawlLogView）。
 *
 * 用函数而不是常量表：手动触发抓取会新建 run，新 run 也必须能查到明细，
 * 写死一份 map 就只能覆盖预置的那几条。
 *
 * 日志 id 是 ULID，按 (run, 行号) 生成一次后缓存：既不能像原来的 `run.id * 100 + n` 那样做算术
 * （ULID 是字符串），也不能每次请求都调 nextUlid()（那样同一个 run 查两次会拿到不同的日志 id）。
 *
 * stage / level / 文案都照抄 `internal/crawler/pipeline.go` 的 logf 调用：
 * stage 只有 discover / fetch / parse / filter / done / robots 这几种，
 * 而且**没有耗时字段** —— 早期 mock 里的 duration_ms 根本不在 CrawlLogView 里。
 * context 同样不写：真实 LogEntry 从不设置它，omitempty 后该键恒不出现。
 */
const crawlLogIds = new Map()

function logIdsFor(run, count) {
  let ids = crawlLogIds.get(run.id)
  if (!ids) {
    ids = []
    crawlLogIds.set(run.id, ids)
  }
  // 行数变多时只追加新 id，已有行的 id 不漂移
  while (ids.length < count) ids.push(nextUlid())
  return ids
}

export function buildCrawlLogs(run) {
  const entry = (level, stage, message, url = run.careers_url || '') => ({
    run_id: run.id,
    level,
    stage,
    url,
    message,
    context: null,
    created_at: run.started_at,
  })
  const logs = [
    entry('info', 'discover', `从该页面定位到招聘入口：${run.careers_url || '（未配置）'}`),
    entry('debug', 'parse', `已抽取岗位 ${run.jobs_found} 条（抽取器 dom）`),
  ]
  if (run.jobs_filtered > 0) {
    logs.push(entry('info', 'filter', `岗位信息完整度不足或命中排除关键词，已丢弃 ${run.jobs_filtered} 条`))
  }
  if (run.used_headless) {
    logs.push(entry('warn', 'fetch', '静态抓取正文过短，已切换无头浏览器渲染'))
  }
  if (run.error_message) {
    logs.push(entry('warn', 'fetch', run.error_message))
  }
  logs.push(entry('info', 'done', `本轮抓取完成：页面 ${run.pages_fetched} 个，岗位 ${run.jobs_found} 条`))
  const ids = logIdsFor(run, logs.length)
  return logs.map((log, index) => ({ id: ids[index], ...log }))
}

// 种子运行的日志 id 在这里一次性预生成：否则「谁先请求谁先分配」会让刷新后的 id 顺序漂移。
for (const run of crawlRuns) buildCrawlLogs(run)

/* -------------------------------------------------------------------------- */
/* 审计日志（AuditLogView）                                                    */
/* -------------------------------------------------------------------------- */

/**
 * 审计日志。
 *
 * 两个必须踩准的形状：
 *   - 操作人是 `actor_email`（不是 actor），系统任务的 `actor_id` 直接消失（可空外键 + omitempty）；
 *   - `detail` 是结构化对象（Go 是 map[string]any），不是一句中文 —— 后端从来没写过字符串。
 *
 * 说明：真实 `AuditService.Record` 目前只写成功记录（status_code 恒为 200）。
 * 这里保留一条 success=false 的登录失败记录，仅仅是为了让「只看失败」这个筛选有数据可看，
 * 字段形状仍然合法（schema 与查询都支持 success/status_code）。
 *
 * 外键（actor_id / resource_id，以及 path 里的 id）必须是实体**真实的 ULID**：
 * 真实后端写的是 `user.ID` / `job.ID` 这类主键，而它们现在是字符串。
 * 因此这里统一用 userRef / jobRef / companyRef / runRef 按编写用序号换算，
 * 杜绝出现查不到的假外键。
 * 例外只有两处本就非实体的标识：resource_id='user'（策略的角色名）、
 * resource_id='crawler.scope'（配置键），以及那条合成登录失败记录的 '-'（失败登录没有会话）。
 */
export const auditLogs = [
  { id: 1024, actor_id: userRef(1), actor_email: 'zhangwei@example.com', action: 'job.approve', resource: 'job', resource_id: jobRef(1), method: 'POST', path: `/api/v1/admin/jobs/${jobRef(1)}/review`, status_code: 200, success: true, ip: '10.0.0.12', user_agent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0.0.0', detail: { title: '高级前端开发工程师（React）', decision: 'approve', reject_reason: '' }, created_at: hoursAgo(1) },
  { id: 1023, actor_id: userRef(2), actor_email: 'li@example.com', action: 'authz.add_policy', resource: 'policy', resource_id: 'user', method: 'POST', path: '/api/v1/admin/authz/policies', status_code: 200, success: true, ip: '10.0.0.18', user_agent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15', detail: { obj: '/api/v1/me', act: 'GET' }, created_at: hoursAgo(2) },
  { id: 1022, actor_id: userRef(1), actor_email: 'zhangwei@example.com', action: 'setting.update', resource: 'setting', resource_id: 'crawler.scope', method: 'PUT', path: '/api/v1/admin/crawl-scope', status_code: 200, success: true, ip: '10.0.0.12', user_agent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0.0.0', detail: { value: { old: { exclude_keywords: ['外包'] }, new: { exclude_keywords: ['外包', '劳务派遣', '中介'] } } }, created_at: hoursAgo(4) },
  { id: 1021, actor_email: 'system', action: 'crawl.run', resource: 'company', resource_id: companyRef(1), method: 'POST', path: '/api/v1/admin/crawl/run-all', status_code: 200, success: true, ip: '127.0.0.1', user_agent: 'JobsRobotBot/1.0', detail: { run_id: runRef(328), status: 'success', jobs_created: 8, jobs_updated: 3, jobs_filtered: 1, trigger: 'schedule' }, created_at: daysAgo(0, 10, 12) },
  { id: 1020, actor_id: userRef(4), actor_email: 'chen@example.com', action: 'job.reject', resource: 'job', resource_id: jobRef(12), method: 'POST', path: `/api/v1/admin/jobs/${jobRef(12)}/review`, status_code: 200, success: true, ip: '10.0.0.31', user_agent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0.0.0', detail: { title: '市场推广专员（云业务）', decision: 'reject', reject_reason: '岗位信息与官网《加入我们》页面不一致，缺少任职要求' }, created_at: daysAgo(0, 9, 5) },
  { id: 1019, actor_id: userRef(2), actor_email: 'li@example.com', action: 'user.set_roles', resource: 'user', resource_id: userRef(9), method: 'POST', path: `/api/v1/admin/users/${userRef(9)}/roles`, status_code: 200, success: true, ip: '10.0.0.18', user_agent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15', detail: { old: ['user'], new: ['reviewer'] }, created_at: daysAgo(1, 16, 40) },
  { id: 1018, actor_id: userRef(1), actor_email: 'zhangwei@example.com', action: 'user.set_status', resource: 'user', resource_id: userRef(3), method: 'POST', path: `/api/v1/admin/users/${userRef(3)}/status`, status_code: 200, success: true, ip: '10.0.0.12', user_agent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0.0.0', detail: { status: 'disabled' }, created_at: daysAgo(1, 15, 10) },
  // 登录失败：actor_email 为空（匿名请求没有身份），因此该键整体消失
  { id: 1017, action: 'auth.login', resource: 'session', resource_id: '-', method: 'POST', path: '/api/v1/auth/login', status_code: 401, success: false, ip: '203.0.113.44', user_agent: 'curl/8.4.0', detail: { reason: '邮箱或密码错误', code: 20002 }, created_at: daysAgo(1, 3, 22) },
  // actor_email 是 zhaomin@example.com，而它正是第 7 个用户：原来写的 13 在用户表里并不存在，
  // 换成 ULID 后统一指向 userRef(7)，否则这条记录会引用一个查不到的 id。
  { id: 1016, actor_id: userRef(7), actor_email: 'zhaomin@example.com', action: 'auth.register', resource: 'user', resource_id: userRef(7), method: 'POST', path: '/api/v1/auth/register', status_code: 200, success: true, ip: '203.0.113.9', user_agent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) Safari/604.1', detail: { email: 'zhaomin@example.com', email_verified: true }, created_at: daysAgo(2, 20, 5) },
  { id: 1015, actor_id: userRef(1), actor_email: 'zhangwei@example.com', action: 'company.create', resource: 'company', resource_id: companyRef(11), method: 'POST', path: '/api/v1/admin/companies', status_code: 200, success: true, ip: '10.0.0.12', user_agent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0.0.0', detail: { name: '蔚来汽车', website: 'https://www.example.com', careers_url: '', industry: '新能源', enabled: true, crawl_enabled: false, priority: 0 }, created_at: daysAgo(2, 14, 30) },
  { id: 1014, actor_id: userRef(1), actor_email: 'zhangwei@example.com', action: 'company.set_careers_url', resource: 'company', resource_id: companyRef(12), method: 'POST', path: `/api/v1/admin/companies/${companyRef(12)}/discover`, status_code: 200, success: true, ip: '10.0.0.12', user_agent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0.0.0', detail: { careers_url: { old: '', new: 'https://www.example.com/join-us' }, source: { old: 'manual', new: 'discovered' } }, created_at: daysAgo(2, 14, 32) },
  { id: 1013, actor_id: userRef(2), actor_email: 'li@example.com', action: 'auth.logout', resource: 'user', resource_id: userRef(2), method: 'POST', path: '/api/v1/auth/logout', status_code: 200, success: true, ip: '10.0.0.18', user_agent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15', detail: {}, created_at: daysAgo(3, 19, 0) },
]

/** 审计日志自身的 id 也是 ULID：按原有数字序号升序分配，列表默认倒序（最新在前）不变 */
assignSeedIds('log', auditLogs)

/* -------------------------------------------------------------------------- */
/* 字典（MetaView 的静态枚举部分）                                              */
/* -------------------------------------------------------------------------- */

/**
 * MetaView 里与数据库无关的枚举。
 *
 * 键名严格对齐 `internal/service/meta.go`：是 crawl_triggers / code_purposes，
 * 不是 triggers / purposes；状态类一律是 `{value, label}`，**没有 tone 字段**
 * （色彩已下沉到 utils/format.js，接口不再教前端配色）。
 * job_types / industries / employment_types / locations 来自收集范围，由 index.js 现算。
 */
export const META_ENUMS = {
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

/* -------------------------------------------------------------------------- */
/* 邮件配置                                                                     */
/* -------------------------------------------------------------------------- */

/** SMTP 密码占位符：与 `service.MailPasswordMask` 必须完全一致，原样回传代表「不修改」 */
export const MAIL_PASSWORD_MASK = '********'

/**
 * 配置文件基线（configs/config.yaml 的 mail 段，也是 config.Default()）。
 * `DELETE /admin/mail` 会丢弃数据库覆盖，回落到这里。
 */
export const mailConfigBaseline = {
  driver: 'console',
  from_name: '直招好工作',
  from_address: 'no-reply@jobs-robot.local',
  smtp: { host: '', port: 465, username: '', password: '', tls: true },
}

/**
 * 数据库覆盖层（settings 表的 mail.smtp 行，真实后端启动时由 MailService.EnsureDefaults 播种）。
 *
 * 演示数据刻意填成「管理员已经配好 SMTP」：这样邮件设置页能同时展示
 * has_password=true、password_mask、source=database、updated_by/updated_at 这几条真实分支。
 * 密码是假值，且**永远不会出现在任何响应里**（只有 has_password 这个布尔外泄）。
 */
export const mailConfigSeed = {
  driver: 'smtp',
  from_name: '直招好工作',
  from_address: 'no-reply@jobs-robot.local',
  smtp: {
    host: 'smtp.example.com',
    port: 465,
    username: 'no-reply@jobs-robot.local',
    password: 'demo-only-not-a-real-password',
    tls: true,
  },
  updated_by: 'zhangwei@example.com',
  updated_at: hoursAgo(4),
}

/* -------------------------------------------------------------------------- */
/* 服务端配置开关（auth 段）                                                    */
/* -------------------------------------------------------------------------- */

/**
 * 演示环境里的 `auth` 配置段，取值与 `internal/config/config.go` 的 Default() 一致。
 *
 * 为什么这几条必须放在数据层而不是写死在 index.js 里：它们**不是数据，而是服务端配置**，
 * 却直接决定两个接口字段的取值：
 *   - setup_available    = auth.setup.enabled && 不存在可用管理员
 *   - registration_open  = !auth.register_need_verify || mail.smtp_configured
 * 前端（路由守卫 / SetupView）无法自己推断这两条，只能照搬后端结论。
 *
 * `setupEnabled` 默认 true（与后端一致）。它存在的意义是让演示能同时覆盖**两种状态**：
 * 置为 false 时 /system/status 的 setup_available 恒为 false，于是 SetupView 的
 * 「系统尚未初始化，但向导已被配置关闭」那一页才可能出现 —— 否则那一页在演示环境里
 * 永远走不到（默认数据里已有管理员）。自检脚本可直接改写本对象，
 * 或调用 index.js 导出的 resetMockSystemState()。
 *
 * `bootstrapCredentialFileExists` 在真实后端是「auth.bootstrap.credential_file 指向的
 * 文件是否存在」（os.Stat），演示环境没有文件系统可言，默认 false；
 * 置 true 可以演示初始化完成页上「凭据文件仍在，请登录后删除」的运维提示。
 */
export const mockAuthConfig = {
  setupEnabled: true,
  registerNeedVerify: true,
  setupTestMailTo: '',
  bootstrapCredentialFileExists: false,
}

/** 演示账号：任意 ≥8 位密码均可登录，用于在没有后端时走通登录流程 */
export const demoAccount = {
  email: 'zhangwei@example.com',
  user: users[0],
  roles: ['admin'],
}
