<script setup>
import { onBeforeUnmount, watch } from 'vue'

import AppIcon from '@/components/AppIcon.vue'

/**
 * 对话框。
 * 后台的审核、新增用户、新增策略都用它，避免跳页打断操作上下文。
 */
const props = defineProps({
  modelValue: { type: Boolean, default: false },
  title: { type: String, default: '' },
  description: { type: String, default: '' },
  size: { type: String, default: 'md' }, // sm | md | lg
  closeOnBackdrop: { type: Boolean, default: true },
})

const emit = defineEmits(['update:modelValue'])

const SIZES = {
  sm: 'max-w-[420px]',
  md: 'max-w-[560px]',
  lg: 'max-w-[760px]',
}

function close() {
  emit('update:modelValue', false)
}

function onKeydown(event) {
  if (event.key === 'Escape') close()
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      document.addEventListener('keydown', onKeydown)
      document.body.style.overflow = 'hidden'
    } else {
      document.removeEventListener('keydown', onKeydown)
      document.body.style.overflow = ''
    }
  },
)

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown)
  document.body.style.overflow = ''
})
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="opacity-0"
      leave-active-class="transition duration-100 ease-in"
      leave-to-class="opacity-0"
    >
      <div v-if="modelValue" class="fixed inset-0 z-[90] flex items-start justify-center overflow-y-auto p-4 sm:p-8">
        <div
          class="fixed inset-0 bg-ink-900/45 backdrop-blur-[2px]"
          @click="closeOnBackdrop && close()"
        />

        <div
          class="relative z-10 my-auto w-full rounded-card bg-white shadow-auth"
          :class="SIZES[size] ?? SIZES.md"
          role="dialog"
          aria-modal="true"
        >
          <header class="flex items-start justify-between gap-4 border-b border-ink-100 px-6 py-4.5">
            <div>
              <h2 class="text-[17px] font-semibold text-ink-900">{{ title }}</h2>
              <p v-if="description" class="mt-1 text-[13px] text-ink-500">{{ description }}</p>
            </div>
            <button
              type="button"
              class="rounded-lg p-1.5 text-ink-400 transition hover:bg-ink-100 hover:text-ink-700"
              aria-label="关闭"
              @click="close"
            >
              <AppIcon name="x" :size="18" />
            </button>
          </header>

          <div class="px-6 py-5">
            <slot />
          </div>

          <footer
            v-if="$slots.footer"
            class="flex items-center justify-end gap-3 border-t border-ink-100 px-6 py-4"
          >
            <slot name="footer" />
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
