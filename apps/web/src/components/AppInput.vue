<script setup>
import { computed, useId } from 'vue'

import AppIcon from '@/components/AppIcon.vue'

/**
 * 文本输入。图标、错误提示、右侧插槽（如「获取验证码」）都在这里统一。
 */
const props = defineProps({
  modelValue: { type: [String, Number], default: '' },
  label: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  type: { type: String, default: 'text' },
  icon: { type: String, default: '' },
  hint: { type: String, default: '' },
  error: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  autocomplete: { type: String, default: 'off' },
  inputmode: { type: String, default: undefined },
  maxlength: { type: [String, Number], default: undefined },
})

const emit = defineEmits(['update:modelValue', 'enter'])

const uid = useId()
const inputId = computed(() => `field-${uid}`)

const model = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})
</script>

<template>
  <div class="w-full">
    <label v-if="label" :for="inputId" class="mb-1.5 block text-[13px] font-medium text-ink-700">
      {{ label }}
    </label>

    <div
      class="flex items-center gap-2.5 rounded-xl border bg-white px-3.5 transition"
      :class="
        error
          ? 'border-red-300 focus-within:border-red-400'
          : 'border-ink-200 focus-within:border-brand-400 focus-within:ring-2 focus-within:ring-brand-500/12'
      "
    >
      <AppIcon v-if="icon" :name="icon" :size="18" class="text-ink-400" />

      <input
        :id="inputId"
        v-model="model"
        :type="type"
        :placeholder="placeholder"
        :disabled="disabled"
        :autocomplete="autocomplete"
        :inputmode="inputmode"
        :maxlength="maxlength"
        class="h-11 min-w-0 flex-1 bg-transparent text-sm text-ink-900 placeholder:text-ink-400 focus:outline-none disabled:cursor-not-allowed"
        @keyup.enter="emit('enter')"
      />

      <slot name="suffix" />
    </div>

    <p v-if="error" class="mt-1.5 flex items-center gap-1 text-[12.5px] text-red-600">
      <AppIcon name="alert" :size="14" />
      {{ error }}
    </p>
    <p v-else-if="hint" class="mt-1.5 text-[12.5px] text-ink-400">{{ hint }}</p>
  </div>
</template>
