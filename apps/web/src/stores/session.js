import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { authApi, meApi } from '@/api'
import { clearTokens, loadTokens, saveTokens } from '@/api/http'

/**
 * 登录态。
 *
 * 路由守卫会 `await session.restore()` 之后再判断 isAuthenticated —— 这是必须的：
 * 刷新页面时 access_token 还在 localStorage 里，但 user 还没拉回来，
 * 如果先判断就会把已登录用户误判成游客并弹回登录页。
 */
export const useSessionStore = defineStore('session', () => {
  const user = ref(null)
  const accessToken = ref('')
  const refreshToken = ref('')

  /** restore 完成前，restored 为 false，守卫不应据此做任何跳转 */
  const restored = ref(false)
  let restoring = null

  const isAuthenticated = computed(() => Boolean(user.value && accessToken.value))
  const roles = computed(() => user.value?.roles ?? [])
  const isStaff = computed(() => roles.value.includes('admin') || roles.value.includes('reviewer'))
  const displayName = computed(() => user.value?.nickname || user.value?.email || '未登录')
  const initial = computed(() => (user.value?.nickname || user.value?.email || '游').slice(0, 1))

  function applyTokens(token) {
    if (!token) return
    accessToken.value = token.access_token || accessToken.value
    refreshToken.value = token.refresh_token || refreshToken.value
    saveTokens(token)
  }

  function clear() {
    user.value = null
    accessToken.value = ''
    refreshToken.value = ''
    clearTokens()
  }

  /** 幂等：同一时刻只发一次 /me，多个守卫并发调用会共享同一个 Promise */
  function restore() {
    if (restored.value) return Promise.resolve()
    restoring ??= (async () => {
      const stored = loadTokens()
      if (stored?.access_token) {
        accessToken.value = stored.access_token
        refreshToken.value = stored.refresh_token || ''
        try {
          user.value = await meApi.profile()
        } catch {
          // 令牌已失效：清空本地登录态，交给守卫跳登录页
          clear()
        }
      }
      restored.value = true
      restoring = null
    })()
    return restoring
  }

  async function login(payload) {
    const data = await authApi.login(payload)
    applyTokens(data.token)
    user.value = data.user
    restored.value = true
    return data
  }

  async function register(payload) {
    const data = await authApi.register(payload)
    if (data?.token) {
      applyTokens(data.token)
      user.value = data.user
      restored.value = true
    }
    return data
  }

  async function logout() {
    const token = refreshToken.value
    clear()
    restored.value = true
    if (token) {
      // 登出失败不影响本地状态：服务端会话会自行过期
      await authApi.logout(token).catch(() => {})
    }
  }

  async function updateProfile(payload) {
    user.value = await meApi.updateProfile(payload)
    return user.value
  }

  /** 用接口返回的最新用户视图同步本地登录态（头像上传等） */
  function applyUser(view) {
    if (view) user.value = view
    return view
  }

  return {
    user,
    accessToken,
    roles,
    isAuthenticated,
    isStaff,
    displayName,
    initial,
    restored,
    restore,
    login,
    register,
    logout,
    updateProfile,
    applyUser,
    clear,
  }
})
