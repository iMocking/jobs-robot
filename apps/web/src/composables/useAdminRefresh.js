import { inject, onMounted, watch } from 'vue'

/** LayWorktable 提供的刷新信号 key */
export const ADMIN_REFRESH_KEY = 'admin:refresh'

/**
 * 让后台页面同时响应「挂载时加载」与「顶栏刷新按钮」。
 * 少了这个，每个页面都要自己写一遍 onMounted + watch(inject(...))。
 */
export function useAdminRefresh(handler) {
  const tick = inject(ADMIN_REFRESH_KEY, null)

  onMounted(() => handler())

  if (tick) {
    watch(tick, () => handler())
  }
}
