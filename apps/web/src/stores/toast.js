import { ref } from 'vue'
import { defineStore } from 'pinia'

/**
 * 轻量提示条。
 * 接口错误一律走这里冒泡，避免每个页面各写一套 alert。
 */
export const useToastStore = defineStore('toast', () => {
  const items = ref([])
  let seq = 0

  function push(message, tone = 'info', timeout = 3200) {
    const id = ++seq
    items.value.push({ id, message, tone })
    if (timeout) setTimeout(() => dismiss(id), timeout)
    return id
  }

  function dismiss(id) {
    items.value = items.value.filter((t) => t.id !== id)
  }

  const success = (message) => push(message, 'success')
  /**
   * 错误提示。
   *
   * 多行文案（后端给的排查步骤 1) 2) 3) + 服务器原文）按默认的 4.2 秒根本读不完，
   * 这里按行数放宽停留时间——它不是装饰，而是运维真正要照着做的内容。
   */
  const error = (message) => {
    const multiline = typeof message === 'string' && message.includes('\n')
    return push(message, 'error', multiline ? 20000 : 4200)
  }
  const info = (message) => push(message, 'info')

  /** 把 ApiError 转成用户能读懂的一句话 */
  function fromError(err, fallback = '操作失败，请稍后重试') {
    const message = err?.message || fallback
    return error(message)
  }

  return { items, push, dismiss, success, error, info, fromError }
})
