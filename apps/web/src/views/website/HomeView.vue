<script setup>
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute } from 'vue-router'

import JobExplorer from '@/components/JobExplorer.vue'
import SearchHero from '@/components/SearchHero.vue'
import { useDictionaryStore } from '@/stores/dictionaries'

/**
 * 首页 = 设计稿第 1 屏。
 * Hero 建立「官网直招 = 真实可溯源」的第一印象，下方直接进入可筛选的职位流，
 * 不做多余的营销模块 —— 用户是来找岗位的。
 */
const route = useRoute()

const dictionary = useDictionaryStore()
const { data: dictionaries } = storeToRefs(dictionary)
dictionary.load()

/**
 * 热门关键词优先用后端字典里的职位类型（job_types），
 * 而不是前端写死一批：运营在后台调整收集范围后，热门标签会跟着变，
 * 点进去也一定搜得到岗位。字典为空（后端不可用）时退回一组通用词。
 */
const FALLBACK_KEYWORDS = ['人工智能', '前端开发', '数据分析', '产品经理']

const hotKeywords = computed(() => {
  const types = dictionaries.value.job_types ?? []
  return types.length ? types.slice(0, 4) : FALLBACK_KEYWORDS
})
</script>

<template>
  <div>
    <SearchHero :keyword="String(route.query.keyword ?? '')" :hot-keywords="hotKeywords" />

    <div class="container-page py-8">
      <JobExplorer />
    </div>
  </div>
</template>
