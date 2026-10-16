<script setup>
import AppIcon from '@/components/AppIcon.vue'

/**
 * 分步指示条。
 *
 * 文案由调用方传入：默认是「找回密码」的三步，首次安装向导传自己的三步进来。
 * 之前这里写死了 STEPS，导致安装向导要么复用「邮箱验证/重置密码」这种不相关的文案，
 * 要么自己再画一套指示条——两条路径迟早会走样，所以改成参数。
 *
 * 已完成的步骤用对勾替换序号，让用户一眼看出「还剩几步」。
 */
defineProps({
  current: { type: Number, default: 1 },
  steps: { type: Array, default: () => ['邮箱验证', '重置密码', '完成'] },
})
</script>

<template>
  <ol class="flex flex-wrap items-center justify-center gap-x-2 gap-y-2 text-[13px]">
    <li v-for="(label, index) in steps" :key="label" class="flex items-center gap-2">
      <span
        class="flex items-center gap-1.5"
        :class="index + 1 <= current ? 'font-medium text-brand-700' : 'text-ink-400'"
      >
        <span
          class="flex size-5 items-center justify-center rounded-full text-[11.5px]"
          :class="index + 1 <= current ? 'bg-brand-600 text-white' : 'bg-ink-200 text-ink-500'"
        >
          <AppIcon
            v-if="index + 1 < current"
            name="check"
            :size="12"
            :stroke-width="3"
            aria-hidden="true"
          />
          <template v-else>{{ index + 1 }}</template>
        </span>
        {{ label }}
      </span>
      <span v-if="index < steps.length - 1" class="h-px w-6 bg-ink-200" />
    </li>
  </ol>
</template>
