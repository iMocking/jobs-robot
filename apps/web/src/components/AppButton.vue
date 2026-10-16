<script setup>
import { computed } from 'vue'
import { RouterLink } from 'vue-router'

/**
 * 统一按钮。
 * 全站只允许出现这里定义的几种 variant，避免「每个页面一个蓝色」。
 */
const props = defineProps({
  variant: { type: String, default: 'primary' }, // primary | secondary | subtle | ghost | success | danger
  size: { type: String, default: 'md' }, // sm | md | lg
  to: { type: [String, Object], default: null },
  href: { type: String, default: '' },
  type: { type: String, default: 'button' },
  disabled: { type: Boolean, default: false },
  loading: { type: Boolean, default: false },
  block: { type: Boolean, default: false },
})

const VARIANTS = {
  primary: 'bg-brand-600 text-white hover:bg-brand-700 active:bg-brand-800 shadow-sm',
  secondary: 'bg-white text-ink-700 border border-ink-200 hover:border-brand-300 hover:text-brand-700',
  subtle: 'bg-brand-50 text-brand-700 hover:bg-brand-100',
  ghost: 'text-ink-600 hover:bg-ink-100 hover:text-ink-800',
  success: 'bg-accent-600 text-white hover:bg-accent-700 shadow-sm',
  danger: 'bg-white text-red-600 border border-red-200 hover:bg-red-50',
}

const SIZES = {
  sm: 'h-8 gap-1.5 rounded-lg px-3 text-[13px]',
  md: 'h-10 gap-2 rounded-lg px-4 text-sm',
  lg: 'h-12 gap-2 rounded-xl px-6 text-[15px]',
}

const tag = computed(() => {
  if (props.to) return RouterLink
  if (props.href) return 'a'
  return 'button'
})

const linkProps = computed(() => {
  if (props.to) return { to: props.to }
  if (props.href) return { href: props.href, target: '_blank', rel: 'noopener noreferrer' }
  return {}
})

const classes = computed(() => [
  'inline-flex select-none items-center justify-center font-medium transition',
  'disabled:cursor-not-allowed disabled:opacity-55',
  SIZES[props.size],
  VARIANTS[props.variant],
  props.block ? 'w-full' : '',
])
</script>

<template>
  <component
    :is="tag"
    v-bind="linkProps"
    :type="tag === 'button' ? type : undefined"
    :disabled="tag === 'button' ? disabled || loading : undefined"
    :aria-busy="loading || undefined"
    :class="classes"
  >
    <svg
      v-if="loading"
      class="size-4 animate-spin"
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity="0.25" stroke-width="3" />
      <path
        d="M21 12a9 9 0 0 0-9-9"
        stroke="currentColor"
        stroke-width="3"
        stroke-linecap="round"
      />
    </svg>
    <slot />
  </component>
</template>
