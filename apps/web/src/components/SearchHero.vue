<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'

import AppIcon from '@/components/AppIcon.vue'
import TrustBadge from '@/components/TrustBadge.vue'

/**
 * 首页 Hero：深蓝渐变 + 信任徽章 + 大搜索框 + 热门关键词。
 * 第一视觉焦点是搜索框与「官网直招」信任徽章 —— 设计稿的注意力路径如此。
 */
const props = defineProps({
  keyword: { type: String, default: '' },
  hotKeywords: { type: Array, default: () => [] },
})

const router = useRouter()
const draft = ref(props.keyword)

function search(value) {
  const keyword = String(value ?? draft.value).trim()
  router.push({ name: 'jobs', query: keyword ? { keyword } : {} })
}
</script>

<template>
  <section class="bg-hero py-14 lg:py-16">
    <div class="container-page flex flex-col items-center text-center">
      <TrustBadge />

      <h1 class="mt-7 text-[36px] leading-tight font-bold tracking-tight text-white lg:text-[46px]">
        发现官网直招好工作
      </h1>
      <p class="mt-4 text-[16px] text-white/75 lg:text-[18px]">
        绕过猎头与中介，直达企业官方招聘入口
      </p>

      <!-- 搜索：白色胶囊里嵌蓝色按钮，视觉上只有一个可操作焦点 -->
      <div
        class="mt-8 flex w-full max-w-[720px] items-center gap-2 rounded-xl bg-white p-1.5 shadow-lg shadow-brand-950/25"
      >
        <span class="pl-3.5 text-ink-400">
          <AppIcon name="search" :size="19" />
        </span>
        <input
          v-model="draft"
          type="search"
          placeholder="输入职位关键词，如“前端工程师”“产品经理”"
          class="h-11 min-w-0 flex-1 bg-transparent text-[15px] text-ink-900 placeholder:text-ink-400 focus:outline-none"
          @keyup.enter="search()"
        />
        <button
          type="button"
          class="h-11 shrink-0 rounded-lg bg-brand-600 px-7 text-[15px] font-medium text-white transition hover:bg-brand-700"
          @click="search()"
        >
          搜索
        </button>
      </div>

      <div v-if="hotKeywords.length" class="mt-5 flex flex-wrap items-center justify-center gap-2.5">
        <button
          v-for="item in hotKeywords"
          :key="item"
          type="button"
          class="rounded-full bg-white/12 px-4 py-1.5 text-[13px] text-white/85 ring-1 ring-white/20 transition hover:bg-white/20 hover:text-white"
          @click="search(item)"
        >
          {{ item }}
        </button>
      </div>
    </div>
  </section>
</template>
