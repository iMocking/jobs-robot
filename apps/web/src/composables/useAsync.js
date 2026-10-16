import { ref, shallowRef } from 'vue'

/**
 * 统一「加载 → 数据 / 错误」三态。
 *
 * 页面里最常见的样板就是 loading/data/error 三个 ref 加一段 try/catch，
 * 26 个页面各写一遍既啰嗦又容易漏掉竞态处理（快速切筛选时旧响应后到会覆盖新数据），
 * 这里用序号做一次「只认最后一次请求」的保护。
 *
 * @param {Function} loader 返回 Promise 的取数函数
 * @param {object} options immediate=true 时立即执行一次；onError 用于统一弹提示
 */
export function useAsync(loader, { immediate = true, onError } = {}) {
  const data = shallowRef(null)
  const loading = ref(false)
  const error = ref(null)
  /** ready 为 true 表示至少跑完过一次，用来区分「首次骨架屏」与「刷新中」 */
  const ready = ref(false)

  let seq = 0

  async function run(...args) {
    const current = (seq += 1)
    loading.value = true
    error.value = null
    try {
      const result = await loader(...args)
      if (current === seq) data.value = result
      return result
    } catch (err) {
      if (current === seq) error.value = err
      onError?.(err)
      return undefined
    } finally {
      if (current === seq) {
        loading.value = false
        ready.value = true
      }
    }
  }

  if (immediate) run()

  return { data, loading, error, ready, run }
}
