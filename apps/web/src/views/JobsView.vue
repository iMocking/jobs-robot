<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import JobExplorer from '@/components/JobExplorer.vue'

/**
 * 职位列表页（不带 Hero）。
 * 与首页共用 JobExplorer，差别只在有没有 Hero：
 * 从首页搜索进来后，用户需要的是结果和筛选，而不是再看一次大标题。
 */
const route = useRoute()

const keyword = computed(() => String(route.query.keyword ?? ''))
</script>

<template>
  <div class="container-page py-8">
    <div class="mb-6">
      <h1 class="text-[24px] font-semibold text-ink-900">
        {{ keyword ? `“${keyword}” 的搜索结果` : '全部职位' }}
      </h1>
      <p class="mt-1.5 text-[13.5px] text-ink-500">
        每一个岗位都来自企业官网《加入我们》页面，可点开来源逐条核对。
      </p>
    </div>

    <JobExplorer />
  </div>
</template>
