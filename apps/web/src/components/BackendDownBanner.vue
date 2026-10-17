<script setup>
import { onMounted, ref } from 'vue'

import { systemApi } from '@/api'
import AppIcon from '@/components/AppIcon.vue'

/**
 * 全局「后端未启动」提示。
 *
 * 挂在根组件最顶部、所有外壳之上：无论你停在登录页、首页还是后台，
 * 只要连不上 cmd/api 的真实后端，就有一条醒目的红字横幅压在最上面，
 * 而不是让每个页面各自报「请求失败」。
 *
 * 判定口径：启动时调用公开的 /system/status 探活。
 * 只要这一步抛错（连接拒绝、代理 502、跨域失败）就认为后端不可达。
 * 注意它只回答「后端在不在」，不关心业务状态 —— 未初始化（needsSetup）
 * 由路由守卫处理，横幅不掺和。
 */
const API_TARGET = import.meta.env.VITE_API_BASE || '/api/v1'

const down = ref(false)
const checking = ref(false)

async function probe() {
  checking.value = true
  try {
    await systemApi.status()
    down.value = false
  } catch {
    // 网络层错误：后端没起、代理没转发、跨域拦截都会落到这里
    down.value = true
  } finally {
    checking.value = false
  }
}

onMounted(probe)
</script>

<template>
  <div
    v-if="down"
    class="flex items-center justify-center gap-2 border-b-2 border-red-200 bg-red-50 px-4 py-2.5 text-[13.5px] font-semibold text-red-700"
    role="alert"
  >
    <AppIcon name="alert" :size="18" class="shrink-0" />
    <span>
      无法连接后端服务（{{ API_TARGET }}）—— 请确认 cmd/api 已启动（127.0.0.1:8090），随后
    </span>
    <button
      type="button"
      class="shrink-0 rounded-md bg-red-600 px-3 py-1 text-[12.5px] font-semibold text-white transition hover:bg-red-700 disabled:opacity-60"
      :disabled="checking"
      @click="probe"
    >
      {{ checking ? '检测中…' : '重试连接' }}
    </button>
  </div>
</template>
