<script setup>
import { computed } from 'vue'

import AppIcon from '@/components/AppIcon.vue'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  label: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
})

const emit = defineEmits(['update:modelValue'])

const model = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})
</script>

<template>
  <label
    class="inline-flex items-start gap-2 text-sm text-ink-700"
    :class="disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'"
  >
    <span class="relative flex shrink-0 items-center justify-center pt-px">
      <input v-model="model" type="checkbox" :disabled="disabled" class="peer sr-only" />
      <span
        class="flex size-[18px] items-center justify-center rounded-[5px] border transition peer-checked:border-brand-600 peer-checked:bg-brand-600 peer-focus-visible:ring-2 peer-focus-visible:ring-brand-500/50 peer-focus-visible:ring-offset-1"
        :class="model ? 'border-brand-600 bg-brand-600' : 'border-ink-300 bg-white'"
      >
        <AppIcon
          v-if="model"
          name="check"
          :size="13"
          :stroke-width="2.6"
          class="text-white"
        />
      </span>
    </span>
    <span v-if="label || $slots.default" class="leading-[18px]">
      <slot>{{ label }}</slot>
    </span>
  </label>
</template>
