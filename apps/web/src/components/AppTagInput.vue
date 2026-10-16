<script setup>
import { computed, ref } from 'vue'

import AppIcon from '@/components/AppIcon.vue'

/**
 * 多选标签输入。
 * 系统配置的「收集范围」用它维护 工作类型/行业/用工性质/地点 这些白名单：
 * 已选项是可删除的胶囊，末尾「+」展开候选，也支持直接输入自定义值。
 */
const props = defineProps({
  modelValue: { type: Array, default: () => [] },
  options: { type: Array, default: () => [] },
  placeholder: { type: String, default: '添加' },
  disabled: { type: Boolean, default: false },
})

const emit = defineEmits(['update:modelValue'])

const open = ref(false)
const draft = ref('')

const remaining = computed(() => props.options.filter((option) => !props.modelValue.includes(option)))

function add(value) {
  const text = String(value).trim()
  if (!text || props.modelValue.includes(text)) return
  emit('update:modelValue', [...props.modelValue, text])
  draft.value = ''
}

function remove(value) {
  emit(
    'update:modelValue',
    props.modelValue.filter((item) => item !== value),
  )
}
</script>

<template>
  <div class="flex flex-wrap items-center gap-2">
    <span
      v-for="item in modelValue"
      :key="item"
      class="inline-flex items-center gap-1.5 rounded-lg bg-ink-100 px-3 py-1.5 text-[13px] text-ink-700"
    >
      {{ item }}
      <button
        v-if="!disabled"
        type="button"
        class="text-ink-400 transition hover:text-red-500"
        :aria-label="`移除 ${item}`"
        @click="remove(item)"
      >
        <AppIcon name="x" :size="13" />
      </button>
    </span>

    <div v-if="!disabled" class="relative">
      <button
        type="button"
        class="flex size-8 items-center justify-center rounded-lg border border-dashed border-ink-300 text-ink-400 transition hover:border-brand-400 hover:text-brand-600"
        :aria-label="placeholder"
        @click="open = !open"
      >
        <AppIcon name="plus" :size="15" />
      </button>

      <template v-if="open">
        <div class="fixed inset-0 z-10" @click="open = false" />
        <div class="absolute top-10 left-0 z-20 w-64 rounded-xl border border-ink-200 bg-white p-3 shadow-panel">
          <div class="flex items-center gap-1.5 rounded-lg border border-ink-200 px-2.5 py-1.5">
            <input
              v-model="draft"
              class="h-7 min-w-0 flex-1 bg-transparent text-[13px] placeholder:text-ink-400 focus:outline-none"
              :placeholder="`${placeholder}，回车确认`"
              @keyup.enter="add(draft)"
            />
            <button
              type="button"
              class="text-brand-600 transition hover:text-brand-700"
              aria-label="确认添加"
              @click="add(draft)"
            >
              <AppIcon name="check" :size="15" />
            </button>
          </div>

          <div v-if="remaining.length" class="mt-2.5 flex flex-wrap gap-1.5">
            <button
              v-for="option in remaining"
              :key="option"
              type="button"
              class="rounded-md bg-ink-50 px-2 py-1 text-[12.5px] text-ink-600 transition hover:bg-brand-50 hover:text-brand-700"
              @click="add(option)"
            >
              {{ option }}
            </button>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
