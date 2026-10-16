<script setup>
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-page px-6">
    <div class="w-full max-w-[520px] text-center">
      <p class="tabular text-[80px] leading-none font-bold text-brand-100">404</p>
      <div class="mt-2">
        <AppEmpty
          icon="search"
          title="页面走丢了"
          description="这个地址可能已经下线，或者你输入了一个不存在的页面。回到首页继续找工作吧。"
        >
          <div class="flex flex-wrap items-center justify-center gap-3">
            <AppButton :to="{ name: 'home' }">回到首页</AppButton>
            <AppButton variant="secondary" :to="{ name: 'jobs' }">浏览全部职位</AppButton>
          </div>
        </AppEmpty>
      </div>
    </div>
  </div>
</template>
