import { ref } from 'vue'

import { authApi } from '@/api'

/**
 * 「获取验证码」的倒计时。
 *
 * 抽出来是因为注册与找回密码两处都要用，而且冷却逻辑（服务端 60 秒同一邮箱同用途
 * 只能发一次）必须两端一致：前端不禁用按钮的话，用户只会收到一个错误提示。
 */
export function useCountdown(seconds = 60) {
  const remaining = ref(0)
  const sending = ref(false)
  let timer = null

  function start() {
    remaining.value = seconds
    clearInterval(timer)
    timer = setInterval(() => {
      remaining.value -= 1
      if (remaining.value <= 0) {
        clearInterval(timer)
        timer = null
      }
    }, 1000)
  }

  async function send(email, purpose) {
    sending.value = true
    try {
      const result = await authApi.sendCode({ email, purpose })
      start()
      return result
    } finally {
      sending.value = false
    }
  }

  const label = () => (remaining.value > 0 ? `${remaining.value}s 后重发` : '获取验证码')

  return { remaining, sending, send, label }
}
