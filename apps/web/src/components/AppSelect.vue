<script setup>
import { computed } from 'vue'

import AppIcon from '@/components/AppIcon.vue'

/**
 * 下拉选择。
 * 用原生 <select> 而不是自绘浮层：键盘操作、移动端滚轮、表单语义都免费拿到。
 */
const props = defineProps({
  modelValue: { type: [String, Number, Boolean], default: '' },
  label: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  options: { type: Array, default: () => [] }, // [{ value, label }] 或 ['a','b']
  compact: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
})

const emit = defineEmits(['update:modelValue'])

const normalized = computed(() =>
  props.options.map((option) =>
    typeof option === 'object' && option !== null
      ? { value: option.value ?? option.key, label: option.label ?? option.name }
      : { value: option, label: option },
  ),
)

const model = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})
</script>

<template>
  <div class="w-full">
    <label v-if="label" class="mb-1.5 block text-[13px] font-medium text-ink-700">{{ label }}</label>

    <div
      class="relative flex items-center rounded-xl border border-ink-200 bg-white transition focus-within:border-brand-400 focus-within:ring-2 focus-within:ring-brand-500/12"
      :class="compact ? 'h-9' : 'h-11'"
    >
      <select
        v-model="model"
        :disabled="disabled"
        class="h-full w-full cursor-pointer appearance-none bg-transparent pr-9 pl-3.5 text-sm text-ink-800 focus:outline-none disabled:cursor-not-allowed disabled:text-ink-400"
      >
        <option v-if="placeholder" value="">{{ placeholder }}</option>
        <option v-for="option in normalized" :key="String(option.value)" :value="option.value">
          {{ option.label }}
        </option>
      </select>
      <AppIcon
        name="chevron-down"
        :size="16"
        class="pointer-events-none absolute right-3 text-ink-400"
      />
    </div>
  </div>
</template>
