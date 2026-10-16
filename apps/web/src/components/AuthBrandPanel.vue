<script setup>
import AppLogo from '@/components/AppLogo.vue'
import TrustBadge from '@/components/TrustBadge.vue'

/**
 * 认证页品牌区。
 * 登录 / 注册 / 找回密码共用同一块版式，保证「同一产品」的连续感 ——
 * 这也是设计稿第 3~5 屏共用一套左右分栏的原因。
 *
 * 小屏时它退化成顶部压缩条，大屏时才是整列品牌墙。
 */
</script>

<template>
  <div
    class="relative flex flex-col justify-center overflow-hidden bg-hero px-6 py-9 sm:px-10 lg:px-14 lg:py-16 xl:px-20"
  >
    <!-- 装饰：右下角错位点阵，给大面积渐变一点材质 -->
    <div
      class="pointer-events-none absolute right-10 bottom-10 hidden grid-cols-6 gap-3.5 opacity-25 lg:grid"
      aria-hidden="true"
    >
      <span v-for="dot in 30" :key="dot" class="size-1.5 rounded-full bg-white" />
    </div>

    <div class="relative max-w-[520px]">
      <AppLogo size="lg" variant="light" class="hidden sm:inline-flex" />
      <AppLogo variant="light" class="sm:hidden" />

      <h1
        class="mt-8 text-[30px] leading-tight font-bold tracking-tight text-white lg:mt-12 lg:text-[46px]"
      >
        发现官网直招好工作
      </h1>
      <p class="mt-4 text-[15px] text-white/75 lg:mt-5 lg:text-[19px]">
        绕过猎头与中介，直达企业官方招聘入口
      </p>

      <div class="mt-7 lg:mt-11">
        <TrustBadge variant="green" />
      </div>
    </div>
  </div>
</template>
