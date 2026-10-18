<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

import AppButton from '@/components/AppButton.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppModal from '@/components/AppModal.vue'

/**
 * B 站式头像裁剪弹窗。
 *
 * 流程与 B 站一致：选中图片后先进入裁剪（而不是直接上传）——
 * 拖动图片调整位置、滚轮或滑块缩放，圆形裁剪框固定居中，
 * 确认后用 canvas 把裁剪区域输出为圆形 PNG，再交给父组件上传。
 *
 * 纯前端裁剪：不改后端接口（上传的仍是图片文件）。
 * 输出 512×512 圆形 PNG（透明背景），展示端 object-cover 圆形容器直接可用。
 */
const props = defineProps({
  modelValue: { type: Boolean, default: false },
  /** 待裁剪的原始图片文件 */
  file: { type: File, default: null },
  /** 父组件上传中：确定按钮转圈并禁用，防止重复提交 */
  uploading: { type: Boolean, default: false },
})

const emit = defineEmits(['update:modelValue', 'confirm'])

/** v-model 代理：AppModal 的开关状态 */
const open = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const STAGE = 300 // 可视区域边长
const BOX = 200 // 圆形裁剪框直径
const OUTPUT = 512 // 输出头像边长

const imgEl = ref(null)
const objectUrl = ref('')
const natural = ref({ w: 0, h: 0 })
const fitScale = ref(1)
const scaleFactor = ref(1)
const offset = ref({ x: 0, y: 0 })
const loaded = ref(false)

const dragging = ref(false)
const dragStart = ref({ px: 0, py: 0, ox: 0, oy: 0 })

/** 打开时重置并加载新图片 */
watch(
  () => props.modelValue,
  async (open) => {
    if (!open) return
    loaded.value = false
    scaleFactor.value = 1
    offset.value = { x: 0, y: 0 }
    if (objectUrl.value) URL.revokeObjectURL(objectUrl.value)
    objectUrl.value = props.file ? URL.createObjectURL(props.file) : ''
    await nextTick()
  },
)

onBeforeUnmount(() => {
  if (objectUrl.value) URL.revokeObjectURL(objectUrl.value)
})

function onImageLoad() {
  const img = imgEl.value
  if (!img || !img.naturalWidth) return
  natural.value = { w: img.naturalWidth, h: img.naturalHeight }
  // 初始比例：图片最短边恰好等于裁剪框，保证画面上没有「空白缺口」
  fitScale.value = BOX / Math.min(img.naturalWidth, img.naturalHeight)
  loaded.value = true
}

const displayW = computed(() => natural.value.w * fitScale.value * scaleFactor.value)
const displayH = computed(() => natural.value.h * fitScale.value * scaleFactor.value)

/** 拖动边界：图片任意时刻都必须盖住整个裁剪框 */
const maxX = computed(() => Math.max(0, (displayW.value - BOX) / 2))
const maxY = computed(() => Math.max(0, (displayH.value - BOX) / 2))

const imgStyle = computed(() => ({
  width: `${displayW.value}px`,
  height: `${displayH.value}px`,
  left: `${(STAGE - displayW.value) / 2 + offset.value.x}px`,
  top: `${(STAGE - displayH.value) / 2 + offset.value.y}px`,
}))

const scalePct = computed({
  get: () => Math.round(scaleFactor.value * 100),
  set: (value) => zoomTo(value / 100),
})

function clampOffset() {
  offset.value.x = Math.min(maxX.value, Math.max(-maxX.value, offset.value.x))
  offset.value.y = Math.min(maxY.value, Math.max(-maxY.value, offset.value.y))
}

function zoomTo(factor) {
  scaleFactor.value = Math.min(3, Math.max(1, factor))
  clampOffset()
}

function startDrag(event) {
  if (!loaded.value) return
  dragging.value = true
  dragStart.value = {
    px: event.clientX,
    py: event.clientY,
    ox: offset.value.x,
    oy: offset.value.y,
  }
  imgEl.value?.setPointerCapture(event.pointerId)
}

function onDrag(event) {
  if (!dragging.value) return
  offset.value.x = dragStart.value.ox + (event.clientX - dragStart.value.px)
  offset.value.y = dragStart.value.oy + (event.clientY - dragStart.value.py)
  clampOffset()
}

function endDrag() {
  dragging.value = false
}

function onWheel(event) {
  if (!loaded.value) return
  event.preventDefault()
  zoomTo(scaleFactor.value * (event.deltaY < 0 ? 1.06 : 1 / 1.06))
}

function cancel() {
  emit('update:modelValue', false)
}

/** 把裁剪框对应区域绘制为 512×512 圆形 PNG，交给父组件上传 */
async function confirm() {
  const img = imgEl.value
  if (!img || !loaded.value) return

  const imgLeft = (STAGE - displayW.value) / 2 + offset.value.x
  const imgTop = (STAGE - displayH.value) / 2 + offset.value.y
  // 裁剪中心在图片自然像素坐标系中的位置
  const centerX = ((STAGE / 2 - imgLeft) * natural.value.w) / displayW.value
  const centerY = ((STAGE / 2 - imgTop) * natural.value.h) / displayH.value
  const cropW = (BOX * natural.value.w) / displayW.value
  const cropH = (BOX * natural.value.h) / displayH.value

  const canvas = document.createElement('canvas')
  canvas.width = OUTPUT
  canvas.height = OUTPUT
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  // 圆形裁剪（与 B 站头像一致）：透明背景 + 圆形主体
  ctx.beginPath()
  ctx.arc(OUTPUT / 2, OUTPUT / 2, OUTPUT / 2, 0, Math.PI * 2)
  ctx.clip()
  ctx.drawImage(img, centerX - cropW / 2, centerY - cropH / 2, cropW, cropH, 0, 0, OUTPUT, OUTPUT)

  const blob = await new Promise((resolve) => canvas.toBlob(resolve, 'image/png'))
  if (!blob) return
  const baseName = props.file?.name.replace(/\.[^.]+$/, '') || 'avatar'
  emit('confirm', new File([blob], `${baseName}-cropped.png`, { type: 'image/png' }))
}
</script>

<template>
  <AppModal
    v-model="open"
    title="调整头像"
    size="md"
    :close-on-backdrop="false"
    description="拖动图片调整位置，滚轮或滑块缩放后点击确定"
  >
    <div>
      <!-- 裁剪舞台：图片绝对定位在圆框之下，四周被暗色遮罩压暗 -->
      <div
        class="relative mx-auto h-[300px] w-[300px] touch-none select-none overflow-hidden rounded-xl bg-ink-100"
        @wheel.prevent="onWheel"
      >
        <img
          v-if="objectUrl"
          ref="imgEl"
          :src="objectUrl"
          alt="待裁剪头像"
          class="absolute max-w-none cursor-move"
          :class="dragging ? 'cursor-grabbing' : ''"
          draggable="false"
          :style="imgStyle"
          @pointerdown="startDrag"
          @pointermove="onDrag"
          @pointerup="endDrag"
          @pointercancel="endDrag"
          @load="onImageLoad"
        />

        <!-- 圆形裁剪框 + 框外压暗 -->
        <div
          class="pointer-events-none absolute left-1/2 top-1/2 size-[200px] -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow-[0_0_0_1000px_rgba(11,16,33,0.45)]"
        />
      </div>

      <!-- 缩放滑块 -->
      <div class="mt-4 flex items-center gap-3 px-2">
        <AppIcon name="minus" :size="16" class="shrink-0 text-ink-400" />
        <input
          v-model.number="scalePct"
          type="range"
          min="100"
          max="300"
          step="1"
          class="h-1.5 flex-1 cursor-pointer appearance-none rounded-full bg-ink-200 accent-brand-600"
          aria-label="缩放"
        />
        <AppIcon name="plus" :size="16" class="shrink-0 text-ink-400" />
      </div>
      <p class="mt-2 text-center text-[12.5px] text-ink-400">
        拖动调整位置，滚轮或滑块缩放（{{ scalePct }}%）
      </p>
    </div>

    <template #footer>
      <AppButton variant="secondary" @click="cancel">取消</AppButton>
      <AppButton :loading="uploading" :disabled="!loaded" @click="confirm">确定</AppButton>
    </template>
  </AppModal>
</template>
