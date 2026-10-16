<script setup>
/**
 * 认证卡片外壳。
 * 登录 / 注册 / 找回密码 / 重置密码四屏共用同一张卡片，
 * 只换标题与表单内容 —— 这是设计稿第 3~5 屏共用一套版式的直接体现。
 */
defineProps({
  title: { type: String, required: true },
  subtitle: { type: String, default: '' },
})
</script>

<template>
  <section class="rounded-2xl border border-ink-100 bg-white px-7 py-9 shadow-auth sm:px-9">
    <header class="text-center">
      <h1 class="text-[26px] font-bold text-ink-900">{{ title }}</h1>
      <p v-if="subtitle" class="mt-2 text-[13.5px] text-ink-500">{{ subtitle }}</p>
      <div v-if="$slots.header" class="mt-4">
        <slot name="header" />
      </div>
    </header>

    <div class="mt-7">
      <slot />
    </div>

    <footer v-if="$slots.footer" class="mt-6 text-center text-[13px] text-ink-500">
      <slot name="footer" />
    </footer>
  </section>
</template>
