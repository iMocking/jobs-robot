import { ref } from 'vue'
import { defineStore } from 'pinia'

/**
 * 找回密码流程的跨步骤暂存。
 *
 * 为什么不把验证码放进地址栏（/reset-password?code=123456）：
 * 查询串会进入浏览器历史、可能被 Referer 带给第三方，而验证码是一次性凭据。
 * 放在内存里只活一次导航，刷新即失效 —— 用户重新走一遍第一步即可，
 * 代价远小于凭据泄漏。
 */
export const usePasswordResetStore = defineStore('passwordReset', () => {
  const email = ref('')
  const code = ref('')

  function set(payload) {
    email.value = payload?.email ?? ''
    code.value = payload?.code ?? ''
  }

  function clear() {
    email.value = ''
    code.value = ''
  }

  return { email, code, set, clear }
})
