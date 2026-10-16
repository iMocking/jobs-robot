<script setup>
import { computed, ref } from 'vue'

import AppAvatar from '@/components/AppAvatar.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppIcon from '@/components/AppIcon.vue'
import {
  companyInitial,
  jobTags,
  locationText,
  relativeTime,
  salaryText,
  sourceLabel,
  toneFor,
} from '@/utils/format'

/**
 * 职位卡片。
 * 结构照设计稿分三层：公司与薪资 → 职位名与标签 → 溯源信息与操作。
 * 「来源：XX官网《加入我们》」与「最后更新」是本品类的信任核心，不能被折叠进详情页。
 */
const props = defineProps({
  job: { type: Object, required: true },
  /** 已收藏态由父级持有，卡片本身不碰接口 */
  saved: { type: Boolean, default: false },
})

const emit = defineEmits(['save'])

const saved = ref(props.saved)

/**
 * 公司头像的首字与配色都由前端派生：
 * 后端 CompanyBriefView 只有 name / short_name / logo_url，没有 initial、更没有 tone。
 * 配色种子用 company.id（同一家公司在列表与详情页必须同色），没有 id 时退回名称。
 */
const companyAvatar = computed(() => ({
  initial: companyInitial(props.job.company),
  tone: toneFor(props.job.company?.id ?? props.job.company?.name),
}))

/** 标签组（用工性质 + 经验）也走 format.js，避免卡片与详情页各写一套拼接逻辑 */
const tags = computed(() => jobTags(props.job))

const meta = computed(() => [
  { icon: 'map-pin', text: locationText(props.job) || '地点待补充' },
  { icon: 'globe', text: `来源：${sourceLabel(props.job.company)}` },
  { icon: 'clock', text: `最后更新：${relativeTime(props.job.last_updated_at)}` },
])

function toggleSave() {
  saved.value = !saved.value
  emit('save', saved.value)
}
</script>

<template>
  <article class="card card-hover p-5">
    <div class="flex gap-4">
      <AppAvatar
        :name="job.company?.name"
        :initial="companyAvatar.initial"
        :tone="companyAvatar.tone"
        size="md"
      />

      <div class="min-w-0 flex-1">
        <div class="flex items-start justify-between gap-5">
          <div class="min-w-0">
            <p class="flex items-center gap-1.5 text-[13.5px] text-ink-500">
              <span class="truncate">{{ job.company?.name }}</span>
              <span
                v-if="job.company?.verified"
                class="text-brand-500"
                title="已核实企业官网《加入我们》页面"
              >
                <AppIcon name="check-circle" :size="15" />
              </span>
            </p>

            <h3 class="mt-1.5 truncate text-[19px] font-semibold text-ink-900">
              <RouterLink :to="{ name: 'job', params: { id: job.id } }" class="hover:text-brand-700">
                {{ job.title }}
              </RouterLink>
            </h3>

            <div class="mt-2.5 flex flex-wrap items-center gap-2">
              <AppBadge v-if="job.is_direct_hire" tone="accent">官网直招</AppBadge>
              <AppBadge v-for="tag in tags" :key="tag" tone="ink">{{ tag }}</AppBadge>
              <span v-if="locationText(job)" class="text-[12.5px] text-ink-400">
                {{ locationText(job) }}
              </span>
            </div>
          </div>

          <div class="shrink-0 text-right">
            <p class="tabular text-[17px] font-semibold text-brand-700">{{ salaryText(job) }}</p>
            <p class="mt-1.5 text-[12.5px] text-ink-400">{{ relativeTime(job.published_at) }}发布</p>
          </div>
        </div>

        <div
          class="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2.5 border-t border-ink-100 pt-3.5"
        >
          <span
            v-for="item in meta"
            :key="item.icon"
            class="inline-flex items-center gap-1.5 text-[12.5px] text-ink-500"
          >
            <AppIcon :name="item.icon" :size="14" class="text-ink-400" />
            {{ item.text }}
          </span>

          <div class="ml-auto flex items-center gap-2">
            <button
              type="button"
              class="flex size-8 items-center justify-center rounded-lg border transition"
              :class="
                saved
                  ? 'border-brand-300 bg-brand-50 text-brand-600'
                  : 'border-ink-200 text-ink-400 hover:border-brand-300 hover:text-brand-600'
              "
              :aria-pressed="saved"
              :aria-label="saved ? '取消收藏' : '收藏该职位'"
              @click="toggleSave"
            >
              <AppIcon name="bookmark" :size="16" />
            </button>

            <AppButton :to="{ name: 'job', params: { id: job.id } }" size="sm" class="px-4">
              查看详情
            </AppButton>
          </div>
        </div>
      </div>
    </div>
  </article>
</template>
