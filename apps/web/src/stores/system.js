import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { systemApi } from '@/api'

/**
 * 系统初始化状态。
 *
 * 路由守卫在**每次导航**前都要知道「这个系统装好了没有」，因此这里做一次性缓存，
 * 只在首次调用时发请求；初始化完成后由向导显式 refresh()。
 *
 * 失败时的取舍（很重要）：状态查询失败时按「已初始化」处理，也就是**不强制跳 /setup**。
 * 反过来做的话，后端一挂（或代理没起来），所有人都会被锁死在一个永远提交不了的
 * 安装页上——那比看到一个能解释原因的登录页糟糕得多。真正的 401/网络错误会由
 * 正常页面暴露出来。
 */
export const useSystemStore = defineStore('system', () => {
  const status = ref(null)
  const loading = ref(false)
  const error = ref(null)
  const loaded = ref(false)

  let pending = null

  /** 是否已存在可用管理员。状态缺失时乐观地当作 true（见文件头注释） */
  const initialized = computed(() => (status.value ? Boolean(status.value.initialized) : true))

  /** 安装向导是否可用：未初始化 且 配置里开着 auth.setup.enabled */
  const canSetup = computed(() => status.value?.setup_available === true)

  /** 系统没有任何可用管理员：无论向导开没开，都不该让人停在登录页 */
  const needsSetup = computed(() => !initialized.value)

  const userCount = computed(() => status.value?.user_count ?? 0)
  const adminCount = computed(() => status.value?.admin_count ?? 0)
  const registrationOpen = computed(() => status.value?.registration_open ?? false)
  const mail = computed(() => status.value?.mail ?? { driver: '', source: '', ready: false, smtp_configured: false })
  const bootstrapCredentialFileExists = computed(
    () => status.value?.bootstrap_credential_file_exists === true,
  )

  function load() {
    if (loaded.value) return Promise.resolve(status.value)
    pending ??= systemApi
      .status()
      .then((result) => {
        status.value = result
        error.value = null
        return result
      })
      .catch((err) => {
        error.value = err
        // 失败也要标记「问过了」，否则每次导航都会重试并卡住页面
        return null
      })
      .finally(() => {
        loaded.value = true
        loading.value = false
        pending = null
      })
    loading.value = true
    return pending
  }

  /** 初始化完成后重新拉一次，让守卫立刻停止强制 /setup */
  async function refresh() {
    loaded.value = false
    return load()
  }

  return {
    status,
    loading,
    error,
    loaded,
    initialized,
    canSetup,
    needsSetup,
    userCount,
    adminCount,
    registrationOpen,
    mail,
    bootstrapCredentialFileExists,
    load,
    refresh,
  }
})
