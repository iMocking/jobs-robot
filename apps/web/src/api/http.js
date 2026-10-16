/**
 * 统一 HTTP 客户端。
 *
 * 与 docs/api.md 第 1 节的信封结构一一对应：
 *   { code, message, data, details, request_id }
 * 业务层拿到的永远是 data；错误一律抛 ApiError，调用方只判断 err.code。
 *
 * VITE_USE_MOCK=true（默认）时请求不发出网络，改由 src/api/mock 提供演示数据，
 * 这样在没有后端的情况下也能逐屏验收设计稿。
 */
const API_BASE = (import.meta.env.VITE_API_BASE || '/api/v1').replace(/\/+$/, '')

/**
 * 默认走真实后端。
 * 要脱离后端做纯界面开发/验收时，在 .env.local 里设 VITE_USE_MOCK=true。
 *
 * 之所以把默认值从 true 改成 false：mock 一旦默认开启，很容易在「演示数据能跑通」
 * 的错觉里积累与真实 DTO 不一致的字段，等到联调时才发现整页都要改。
 */
export const USE_MOCK = String(import.meta.env.VITE_USE_MOCK ?? 'false').toLowerCase() === 'true'

const TOKEN_KEY = 'jobhub.tokens'

/** 需要重新登录的业务码：10002 请先登录 / 20008 登录已过期 / 20009 凭证无效 / 20015 刷新令牌失效 */
const AUTH_EXPIRED_CODES = [10002, 20008, 20009, 20015]

export class ApiError extends Error {
  constructor({ code, message, details = null, status = 0, requestId = '' }) {
    super(message || '请求失败')
    this.name = 'ApiError'
    this.code = code
    this.details = details
    this.status = status
    this.requestId = requestId
  }

  /** 是否属于「登录态失效」，用于决定要不要弹登录框 / 跳登录页 */
  get isAuthError() {
    return AUTH_EXPIRED_CODES.includes(this.code)
  }
}

/* -------------------------------------------------------------------------- */
/* 令牌存取                                                                     */
/* -------------------------------------------------------------------------- */

function readJson(key, fallback) {
  try {
    const raw = localStorage.getItem(key)
    return raw ? JSON.parse(raw) : fallback
  } catch {
    return fallback
  }
}

export function loadTokens() {
  return readJson(TOKEN_KEY, null)
}

export function saveTokens(token) {
  if (!token) return
  const prev = loadTokens() || {}
  localStorage.setItem(TOKEN_KEY, JSON.stringify({ ...prev, ...token }))
}

export function clearTokens() {
  localStorage.removeItem(TOKEN_KEY)
}

export function accessToken() {
  return loadTokens()?.access_token || ''
}

/* -------------------------------------------------------------------------- */
/* 请求实现                                                                     */
/* -------------------------------------------------------------------------- */

function buildQuery(query) {
  if (!query) return ''
  const usp = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    // 空值不上送，避免后端把 '' 当成有效筛选条件
    if (value === undefined || value === null || value === '') continue
    if (Array.isArray(value)) {
      if (!value.length) continue
      usp.set(key, value.join(','))
    } else {
      usp.set(key, String(value))
    }
  }
  const qs = usp.toString()
  return qs ? `?${qs}` : ''
}

let refreshing = null

async function refreshTokens() {
  const current = loadTokens()
  if (!current?.refresh_token) throw new Error('no refresh token')
  // 并发请求同时 401 时只刷新一次
  refreshing ??= (async () => {
    const res = await fetch(`${API_BASE}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: current.refresh_token }),
    })
    const payload = await res.json().catch(() => null)
    if (!res.ok || !payload || payload.code !== 0) throw new Error('refresh failed')
    clearTokens()
    saveTokens(payload.data)
    return payload.data
  })().finally(() => {
    refreshing = null
  })
  return refreshing
}

async function rawFetch(method, path, { query, body, absolute = false } = {}) {
  const headers = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const token = accessToken()
  if (token) headers.Authorization = `Bearer ${token}`

  // absolute=true 用于 /healthz、/readyz 这类挂在 API 前缀之外的探针接口
  const url = absolute ? `${path}${buildQuery(query)}` : `${API_BASE}${path}${buildQuery(query)}`
  const res = await fetch(url, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  const payload = await res.json().catch(() => null)
  if (!payload) {
    throw new ApiError({ code: 10007, message: `服务响应异常（HTTP ${res.status}）`, status: res.status })
  }
  if (payload.code !== 0) {
    throw new ApiError({
      code: payload.code,
      message: payload.message,
      details: payload.details,
      status: res.status,
      requestId: payload.request_id,
    })
  }
  return payload.data
}

/**
 * 发起请求并直接返回信封里的 data。
 *
 * @param {string} method  GET / POST / PATCH / PUT / DELETE
 * @param {string} path    以 / 开头的路径，不含 API 前缀
 * @param {object} options query / body / auth
 */
export async function request(method, path, options = {}) {
  const { auth = true } = options

  if (USE_MOCK) {
    const { mockRequest } = await import('@/api/mock')
    return mockRequest(method, path, { ...options, authed: auth && Boolean(accessToken()) })
  }

  try {
    return await rawFetch(method, path, options)
  } catch (err) {
    // 访问令牌过期时静默续期一次；刷新失败则清空登录态交给路由守卫处理
    const canRetry =
      err instanceof ApiError &&
      AUTH_EXPIRED_CODES.includes(err.code) &&
      auth &&
      Boolean(loadTokens()?.refresh_token)

    if (!canRetry) throw err

    try {
      await refreshTokens()
    } catch {
      clearTokens()
      throw err
    }
    return rawFetch(method, path, options)
  }
}

export const http = {
  get: (path, options) => request('GET', path, options),
  post: (path, body, options) => request('POST', path, { ...options, body }),
  patch: (path, body, options) => request('PATCH', path, { ...options, body }),
  put: (path, body, options) => request('PUT', path, { ...options, body }),
  delete: (path, options) => request('DELETE', path, options),
}

/**
 * 探针接口（/healthz、/readyz）专用：**不套 {code,message,data} 信封**。
 *
 * handler.Health 直接 `c.JSON(status, view)`，降级时还会返回 503。
 * 如果复用 request()，http.js 会去找 payload.code（undefined !== 0）并抛错，
 * 于是「服务降级」这一最需要看到的信息反而变成了一句「请求失败」。
 * 因此这里把解析后的 payload 原样交回调用方，由页面自己决定怎么表达健康状态。
 */
export async function requestProbe(path) {
  if (USE_MOCK) {
    const { mockRequest } = await import('@/api/mock')
    return mockRequest('GET', path, { auth: false })
  }

  const res = await fetch(path, { headers: { Accept: 'application/json' } })
  const payload = await res.json().catch(() => null)
  if (!payload) {
    throw new ApiError({ code: 10008, message: `探针接口无响应（HTTP ${res.status}）`, status: res.status })
  }
  return payload
}
