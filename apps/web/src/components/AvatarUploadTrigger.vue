<script setup>
import { ref } from 'vue'

import { meApi } from '@/api'
import AppAvatar from '@/components/AppAvatar.vue'
import AppIcon from '@/components/AppIcon.vue'
import AvatarCropModal from '@/components/AvatarCropModal.vue'
import { useSessionStore } from '@/stores/session'
import { useToastStore } from '@/stores/toast'

/**
 * 头像上传触发器（前后台共用）。
 *
 * 经典 hover 蒙版交互：移入头像显示压暗蒙版 + 居中摄像机图标，
 * 点击唤起上传 —— 选图 → B 站式裁剪弹窗 → 确认后上传 → 全站回显。
 * 无头像/加载失败时由 AppAvatar 兜底显示首字母色块。
 *
 * 用法：
 *   <AvatarUploadTrigger :avatar-url="..." :initial="..." :name="..." size="xl" circle />
 *   需要程序化触发时：const trigger = ref(null); trigger.value?.openPicker()
 */
const props = defineProps({
  avatarUrl: { type: String, default: '' },
  initial: { type: String, default: '' },
  name: { type: String, default: '' },
  tone: { type: String, default: 'brand' },
  size: { type: String, default: 'md' }, // sm | md | lg | xl
  circle: { type: Boolean, default: false },
})

const session = useSessionStore()
const toast = useToastStore()

const fileInput = ref(null)
const uploading = ref(false)
const cropOpen = ref(false)
const cropFile = ref(null)

/** 摄像机图标随头像尺寸缩放 */
const CAMERA_SIZE = { sm: 16, md: 20, lg: 24, xl: 26 }

function openPicker() {
  if (uploading.value) return
  fileInput.value?.click()
}

async function onAvatarPicked(event) {
  const file = event.target.files?.[0]
  // 先清空，允许再次选择同一文件（change 事件只在值变化时触发）
  event.target.value = ''
  if (!file) return

  // 前端先做一次快速校验，与后端 storage.SniffImage / MaxImageBytes 对齐
  if (!['image/jpeg', 'image/png', 'image/gif', 'image/webp'].includes(file.type)) {
    toast.error('仅支持 PNG / JPEG / GIF / WebP 图片')
    return
  }
  if (file.size > 10 * 1024 * 1024) {
    toast.error('图片不能超过 10MB')
    return
  }

  // 先进入裁剪弹窗，不直接上传
  cropFile.value = file
  cropOpen.value = true
}

/** 上传裁剪后的头像：成功关闭弹窗并全站回显，失败保留弹窗便于重试/取消 */
async function uploadCropped(cropped) {
  uploading.value = true
  try {
    const view = await meApi.uploadAvatar(cropped)
    session.applyUser(view)
    cropOpen.value = false
    toast.success('头像已更新')
  } catch (err) {
    toast.fromError(err, '头像上传失败，请稍后重试')
  } finally {
    uploading.value = false
  }
}

defineExpose({ openPicker, uploading })
</script>

<template>
  <button
    type="button"
    class="group relative flex cursor-pointer items-center justify-center rounded-full disabled:cursor-not-allowed disabled:opacity-60"
    :disabled="uploading"
    :title="uploading ? '上传中…' : '更换头像'"
    :aria-label="uploading ? '上传中' : '更换头像'"
    @click="openPicker"
  >
    <AppAvatar
      :name="name"
      :initial="initial"
      :tone="tone"
      :avatar-url="avatarUrl"
      :size="size"
      :circle="circle"
    />
    <span
      class="pointer-events-none absolute inset-0 flex items-center justify-center rounded-full bg-ink-900/45 opacity-0 transition-opacity duration-150 group-hover:opacity-100"
    >
      <AppIcon name="camera" :size="CAMERA_SIZE[size] ?? 20" class="text-white" />
    </span>
    <input
      ref="fileInput"
      type="file"
      accept="image/png,image/jpeg,image/gif,image/webp"
      class="hidden"
      @change="onAvatarPicked"
    />
  </button>

  <AvatarCropModal
    v-model="cropOpen"
    :file="cropFile"
    :uploading="uploading"
    @confirm="uploadCropped"
  />
</template>
