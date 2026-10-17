<script setup>
import { ref } from 'vue'

import AppIcon from '@/components/AppIcon.vue'
import AppTabs from '@/components/AppTabs.vue'
import AdvancedSettingsPanel from '@/components/settings/AdvancedSettingsPanel.vue'
import CrawlScopePanel from '@/components/settings/CrawlScopePanel.vue'
import MailConfigPanel from '@/components/settings/MailConfigPanel.vue'

/**
 * 系统配置。
 *
 * 三个页签对应后端三种不同的配置模型，这个划分本身就是信息：
 *   邮件服务 —— configs/config.yaml 的 mail 段为基线，被数据库 mail.smtp 覆盖，保存后热替换投递器；
 *   收集范围 —— 同样是两层覆盖，但它影响的是「下一次抓取」，不是「下一次请求」；
 *   高级配置 —— settings 表的原始读写，用于维护前两者之外的键。
 *
 * 把生效时机不同的配置放在同一屏连续滚动（早期版本照搬设计稿的做法）会让人分不清
 * 「我刚改的这条什么时候生效」，因此拆成页签，并在右上角统一说明生效时机。
 */
const TAB = ref('mail')

const TABS = [
  { key: 'mail', label: '邮件服务' },
  { key: 'scope', label: '收集范围' },
  { key: 'advanced', label: '高级配置' },
]
</script>

<template>
  <div>
    <div class="mb-5 flex flex-wrap items-center justify-between gap-3">
      <AppTabs v-model="TAB" :tabs="TABS" />
      <p class="flex items-center gap-1.5 text-[12.5px] text-ink-400">
        <AppIcon name="info" :size="14" />
        配置保存后立即生效，无需重启服务
      </p>
    </div>

    <!-- 按页签挂载：未激活的面板不发请求，也避免三份表单同时持有脏状态 -->
    <MailConfigPanel v-if="TAB === 'mail'" />
    <CrawlScopePanel v-else-if="TAB === 'scope'" />
    <AdvancedSettingsPanel v-else />
  </div>
</template>
