/**
 * 接口封装：按业务域分组，组件里只写 `jobApi.list()` 这样的调用，
 * 不出现裸路径，方便日后统一加缓存、埋点或灰色发布开关。
 * 路径与 docs/api.md 一一对应。
 */
import { http, requestProbe, upload } from '@/api/http'

export const authApi = {
  sendCode: (payload) => http.post('/auth/codes', payload, { auth: false }),
  register: (payload) => http.post('/auth/register', payload, { auth: false }),
  login: (payload) => http.post('/auth/login', payload, { auth: false }),
  refresh: (refreshToken) => http.post('/auth/refresh', { refresh_token: refreshToken }, { auth: false }),
  logout: (refreshToken) => http.post('/auth/logout', { refresh_token: refreshToken }, { auth: false }),
  forgotPassword: (payload) => http.post('/auth/password/forgot', payload, { auth: false }),
  resetPassword: (payload) => http.post('/auth/password/reset', payload, { auth: false }),
}

export const jobApi = {
  list: (query) => http.get('/jobs', { query, auth: false }),
  detail: (id) => http.get(`/jobs/${id}`, { auth: false }),
  related: (id, limit = 6) => http.get(`/jobs/${id}/related`, { query: { limit }, auth: false }),
  dictionaries: () => http.get('/meta/dictionaries', { auth: false }),
}

export const companyApi = {
  list: (query) => http.get('/companies', { query, auth: false }),
  detail: (id) => http.get(`/companies/${id}`, { auth: false }),
}

/**
 * 系统初始化（首次安装向导）。
 *
 * 三个接口都是公开的：它们要回答的场景正是「系统里还没有任何账号可登录」。
 * 安全边界在服务端（配置开关 + 「不存在可用管理员」判定 + 咨询锁 + 路由限流），
 * 前端只负责把状态如实呈现出来。
 */
export const systemApi = {
  /** 初始化状态；前端启动时无条件调用，决定是否强制进入 /setup */
  status: () => http.get('/system/status', { auth: false }),
  /** 一次性初始化：创建管理员，可选地落邮件配置 */
  setup: (payload) => http.post('/system/setup', payload, { auth: false }),
  /** 用向导表单里的参数试发测试邮件（不落库、不热替换投递器） */
  testSetupMail: (payload) => http.post('/system/setup/mail/test', payload, { auth: false }),
}

export const meApi = {
  profile: () => http.get('/me'),
  updateProfile: (payload) => http.patch('/me', payload),
  changePassword: (payload) => http.post('/me/password', payload),
  sessions: () => http.get('/me/sessions'),
  revokeSession: (sid) => http.delete(`/me/sessions/${sid}`),
  /** 上传当前用户头像：multipart 字段 file，返回更新后的用户视图 */
  uploadAvatar: (file) => {
    const form = new FormData()
    form.append('file', file)
    return upload('/me/avatar', form)
  },
}

export const adminApi = {
  stats: () => http.get('/admin/stats'),
  /**
   * /healthz 挂在 API 前缀之外，且**不套统一信封**、降级时返回 503，
   * 因此走 requestProbe 而不是 http.get。
   */
  health: () => requestProbe('/healthz'),

  jobs: {
    list: (query) => http.get('/admin/jobs', { query }),
    detail: (id) => http.get(`/admin/jobs/${id}`),
    revisions: (id) => http.get(`/admin/jobs/${id}/revisions`),
    create: (payload) => http.post('/admin/jobs', payload),
    update: (id, payload) => http.patch(`/admin/jobs/${id}`, payload),
    remove: (id) => http.delete(`/admin/jobs/${id}`),
    review: (id, payload) => http.post(`/admin/jobs/${id}/review`, payload),
    setStatus: (id, status) => http.post(`/admin/jobs/${id}/status`, { status }),
  },

  companies: {
    list: (query) => http.get('/admin/companies', { query }),
    detail: (id) => http.get(`/admin/companies/${id}`),
    create: (payload) => http.post('/admin/companies', payload),
    update: (id, payload) => http.patch(`/admin/companies/${id}`, payload),
    remove: (id) => http.delete(`/admin/companies/${id}`),
    setEnabled: (id, enabled) => http.post(`/admin/companies/${id}/enabled`, { enabled }),
    discover: (id) => http.post(`/admin/companies/${id}/discover`),
    crawl: (id) => http.post(`/admin/companies/${id}/crawl`),
  },

  crawl: {
    runs: (query) => http.get('/admin/crawl-runs', { query }),
    run: (id) => http.get(`/admin/crawl-runs/${id}`),
    runAll: () => http.post('/admin/crawl/run-all'),
  },

  users: {
    list: (query) => http.get('/admin/users', { query }),
    detail: (id) => http.get(`/admin/users/${id}`),
    setStatus: (id, status) => http.post(`/admin/users/${id}/status`, { status }),
    setRoles: (id, roles) => http.post(`/admin/users/${id}/roles`, { roles }),
    resetPassword: (id, newPassword) => http.post(`/admin/users/${id}/reset-password`, { new_password: newPassword }),
    remove: (id) => http.delete(`/admin/users/${id}`),
  },

  roles: () => http.get('/admin/roles'),

  settings: {
    list: (query) => http.get('/admin/settings', { query }),
    scope: () => http.get('/admin/crawl-scope'),
    saveScope: (payload) => http.put('/admin/crawl-scope', payload),
    get: (key) => http.get(`/admin/settings/${encodeURIComponent(key)}`),
    save: (key, payload) => http.put(`/admin/settings/${encodeURIComponent(key)}`, payload),
    remove: (key) => http.delete(`/admin/settings/${encodeURIComponent(key)}`),
  },

  /**
   * 邮件（SMTP）服务配置。
   *
   * 与 crawler.scope 一样是「configs/config.yaml 为基线 + 数据库 mail.smtp 覆盖、
   * 保存后热替换投递器立即生效」的两层模型，因此单独一组端点而不是通用 settings/:key：
   * 密码需要脱敏读取、留空即表示不修改，这两条通用配置接口表达不了。
   *
   * 对应 internal/service/mail.go 的 MailConfigView / MailConfigInput。
   */
  mail: {
    config: () => http.get('/admin/mail'),
    saveConfig: (payload) => http.put('/admin/mail', payload),
    /** 删除后台设置，回落到 configs/config.yaml 的邮件配置 */
    revertConfig: () => http.delete('/admin/mail'),
    sendTest: (to) => http.post('/admin/mail/test', { to }),
  },

  /**
   * 图片服务配置（本地目录 / OSS）。
   *
   * 与 mail 同款两层模型：configs/config.yaml 的 image 段为基线、被数据库
   * storage.image 覆盖，保存后立即热生效。OSS AccessKeySecret 永不回显，
   * 留空或回传占位符即表示不修改。
   *
   * 对应 internal/service/image.go 的 ImageConfigView / ImageConfigInput。
   */
  image: {
    config: () => http.get('/admin/image-storage'),
    saveConfig: (payload) => http.put('/admin/image-storage', payload),
    /** 删除后台设置，回落到 configs/config.yaml 的图片存储配置 */
    revertConfig: () => http.delete('/admin/image-storage'),
  },

  policies: {
    list: () => http.get('/admin/authz/policies'),
    add: (payload) => http.post('/admin/authz/policies', payload),
    remove: (query) => http.delete('/admin/authz/policies', { query }),
  },

  auditLogs: (query) => http.get('/admin/audit-logs', { query }),
}
