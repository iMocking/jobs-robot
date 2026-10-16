<script setup>
import { computed } from 'vue'

/** 开关。后台配置页用它表达「布尔型配置项」，比下拉更直观。 */
const props = defineProps({
  modelValue: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  label: { type: String, default: '' },
})

const emit = defineEmits(['update:modelValue'])

const model = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})
</script>

<template>
  <label
    class="inline-flex items-center gap-2.5"
    :class="disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'"
  >
    <button
      type="button"
      role="switch"
      :aria-checked="model"
      :disabled="disabled"
      class="relative inline-flex h-[22px] w-[42px] shrink-0 items-center rounded-full transition"
      :class="model ? 'bg-brand-600' : 'bg-ink-300'"
      @click="model = !model"
    >
      <span
        class="inline-block size-[18px] transform rounded-full bg-white shadow-sm transition"
        :class="model ? 'translate-x-[22px]' : 'translate-x-[2px]'"
      />
    </button>
    <span v-if="label || $slots.default" class="text-sm text-ink-700">
      <slot>{{ label }}</slot>
    </span>
  </label>
</template>
