<script setup>
import { computed, reactive, ref } from 'vue'

import { adminApi } from '@/api'
import AdminTable from '@/components/AdminTable.vue'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppEmpty from '@/components/AppEmpty.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppModal from '@/components/AppModal.vue'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { useAsync } from '@/composables/useAsync'
import { useToastStore } from '@/stores/toast'
import { formatDateTime } from '@/utils/format'

/**
 * 高级配置：settings 表的通用读写入口。
 *
 * 为什么保留这个「原始」面板：收集范围与邮件都有专用表单，但那两张表之外的配置项
 * （后续新增的开关、灰度参数）没有界面，运维只能直连数据库改 —— 那是最容易出事的方式。
 * 这里给出一个受控入口：看得见有哪些键、谁改的、什么时候改的。
 *
 * 一个必须守住的约束：**value 只能是 JSON 对象**。
 * 后端 SettingView.Value 是 map[string]any，提交数组或标量会被 gin 直接判为 10001，
 * 因此这里在提交前先本地解析并明确提示，而不是把必然失败的请求发出去。
 */
const toast = useToastStore()

const columns = [
  { key: 'key', label: '配置键' },
  { key: 'group', label: '分组', class: 'w-[110px]' },
  { key: 'description', label: '说明' },
  { key: 'updated', label: '最后修改', class: 'w-[210px]' },
  { key: 'actions', label: '操作', class: 'w-[130px]' },
]

const { data, loading, error, ready, run } = useAsync(() => adminApi.settings.list(), {
  immediate: false,
})
useAdminRefresh(run)

const items = computed(() => data.value?.items ?? [])

/* —— 编辑 / 新建 —— */

const editing = ref(null)
const creating = ref(false)
const saving = ref(false)

const draft = reactive({ key: '', group: '', description: '', valueText: '' })
const valueError = ref('')

function pretty(value) {
  try {
    return JSON.stringify(value ?? {}, null, 2)
  } catch {
    return '{}'
  }
}

function openEdit(row) {
  editing.value = row
  creating.value = false
  draft.key = row.key
  draft.group = row.group || ''
  draft.description = row.description || ''
  draft.valueText = pretty(row.value)
  valueError.value = ''
}

function openCreate() {
  editing.value = null
  creating.value = true
  draft.key = ''
  draft.group = ''
  draft.description = ''
  draft.valueText = '{\n  \n}'
  valueError.value = ''
}

const dialogOpen = computed({
  get: () => Boolean(editing.value) || creating.value,
  set: (open) => {
    if (!open) {
      editing.value = null
      creating.value = false
    }
  },
})

/** 解析并强制要求是对象：数组/标量/null 都不符合后端 map[string]any 的契约 */
function parseValue() {
  const text = draft.valueText.trim()
  if (!text) return {}
  let parsed
  try {
    parsed = JSON.parse(text)
  } catch (err) {
    valueError.value = `JSON 解析失败：${err.message}`
    return null
  }
  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    valueError.value = '配置值必须是一个 JSON 对象，例如 {"enabled": true}'
    return null
  }
  valueError.value = ''
  return parsed
}

async function save() {
  if (creating.value && !draft.key.trim()) {
    valueError.value = '配置键不能为空'
    return
  }
  const value = parseValue()
  if (value === null) return

  saving.value = true
  try {
    await adminApi.settings.save(draft.key.trim(), {
      value,
      group: draft.group.trim(),
      description: draft.description.trim(),
    })
    toast.success(`配置已保存：${draft.key.trim()}`)
    editing.value = null
    creating.value = false
    run()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

/* —— 删除 —— */

const removeTarget = ref(null)
const removing = ref(false)

async function submitRemove() {
  removing.value = true
  try {
    await adminApi.settings.remove(removeTarget.value.key)
    toast.success('配置已删除')
    removeTarget.value = null
    run()
  } catch (err) {
    toast.error(err.message)
  } finally {
    removing.value = false
  }
}
</script>

<template>
  <div>
    <div class="mb-2 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="text-[16px] font-semibold text-ink-900">高级配置</h2>
        <p class="mt-1 text-[12.5px] leading-relaxed text-ink-400">
          settings 表的原始视图。收集范围与邮件服务有专用表单，这里用于查看与维护其余键。
          配置值必须是 JSON 对象，保存后下一次读取即生效。
        </p>
      </div>
      <AppButton @click="openCreate">
        <AppIcon name="plus" :size="16" />
        新增配置
      </AppButton>
    </div>

    <div v-if="loading && !ready" class="card h-64 animate-pulse" />

    <AppEmpty
      v-else-if="error"
      icon="alert"
      tone="danger"
      title="配置加载失败"
      :description="error.message"
    >
      <AppButton @click="run()">重新加载</AppButton>
    </AppEmpty>

    <AdminTable
      v-else
      :columns="columns"
      :empty="!items.length"
      empty-text="暂无配置项"
      min-width="860px"
    >
      <tr v-for="row in items" :key="row.key" class="transition hover:bg-ink-50/60">
        <td class="td-cell">
          <div class="flex items-center gap-2">
            <code class="rounded bg-ink-50 px-2 py-1 font-mono text-[12.5px] text-ink-700">
              {{ row.key }}
            </code>
            <AppBadge v-if="row.is_system" tone="brand" size="sm">内置</AppBadge>
          </div>
        </td>

        <td class="td-cell">
          <AppBadge tone="ink" size="sm">{{ row.group || '未分组' }}</AppBadge>
        </td>

        <td class="td-cell">
          <p class="max-w-[380px] text-[13px] text-ink-500">{{ row.description || '—' }}</p>
        </td>

        <td class="td-cell text-[12.5px] text-ink-500">
          <p class="tabular">{{ formatDateTime(row.updated_at) }}</p>
          <p v-if="row.updated_by" class="mt-0.5 truncate text-ink-400">{{ row.updated_by }}</p>
        </td>

        <td class="td-cell">
          <div class="flex items-center gap-1.5">
            <button
              type="button"
              class="flex size-8 items-center justify-center rounded-lg border border-ink-200 text-ink-500 transition hover:border-brand-300 hover:text-brand-600"
              title="编辑配置"
              @click="openEdit(row)"
            >
              <AppIcon name="pencil" :size="15" />
            </button>
            <button
              type="button"
              class="flex size-8 items-center justify-center rounded-lg border border-ink-200 transition"
              :class="
                row.is_system
                  ? 'cursor-not-allowed text-ink-300'
                  : 'text-ink-500 hover:border-red-300 hover:text-red-600'
              "
              :disabled="row.is_system"
              :title="row.is_system ? '系统内置配置不可删除' : '删除配置'"
              @click="removeTarget = row"
            >
              <AppIcon name="trash" :size="15" />
            </button>
          </div>
        </td>
      </tr>
    </AdminTable>

    <!-- 编辑 / 新建 -->
    <AppModal
      v-model="dialogOpen"
      :title="creating ? '新增配置' : `编辑配置 · ${draft.key}`"
      description="配置值必须是 JSON 对象；保存后立即生效，无需重启服务。"
      size="lg"
    >
      <div class="space-y-4">
        <AppInput
          v-model="draft.key"
          label="配置键"
          placeholder="例如 feature.flag"
          :disabled="!creating"
          hint="键名最多 128 个字符；已存在的键会被覆盖。"
        />

        <div class="grid gap-4 sm:grid-cols-2">
          <AppInput v-model="draft.group" label="分组" placeholder="例如 mail / crawler" />
          <AppInput v-model="draft.description" label="说明" placeholder="这个配置用来做什么" />
        </div>

        <div>
          <label class="mb-1.5 block text-[13px] font-medium text-ink-700">配置值（JSON 对象）</label>
          <textarea
            v-model="draft.valueText"
            rows="9"
            spellcheck="false"
            class="w-full rounded-xl border border-ink-200 bg-white px-3.5 py-3 font-mono text-[12.5px] leading-relaxed text-ink-800 transition focus:border-brand-400 focus:ring-2 focus:ring-brand-500/12 focus:outline-none"
          />
          <p v-if="valueError" class="mt-1.5 flex items-center gap-1 text-[12.5px] text-red-600">
            <AppIcon name="alert" :size="14" />
            {{ valueError }}
          </p>
          <p v-else class="mt-1.5 text-[12px] text-ink-400">
            必须是对象字面量。数组或标量会被后端判为参数错误（10001）。
          </p>
        </div>
      </div>

      <template #footer>
        <AppButton variant="secondary" @click="dialogOpen = false">取消</AppButton>
        <AppButton :loading="saving" @click="save">保存</AppButton>
      </template>
    </AppModal>

    <!-- 删除确认 -->
    <AppModal
      :model-value="Boolean(removeTarget)"
      title="删除配置"
      size="sm"
      @update:model-value="removeTarget = null"
    >
      <p class="text-[14px] leading-relaxed text-ink-600">
        确定删除
        <code class="rounded bg-ink-100 px-1.5 py-0.5 font-mono text-[12.5px]">
          {{ removeTarget?.key }}
        </code>
        吗？删除后该配置会回落到代码或配置文件中的默认值。
      </p>
      <template #footer>
        <AppButton variant="secondary" @click="removeTarget = null">取消</AppButton>
        <AppButton variant="danger" :loading="removing" @click="submitRemove">确认删除</AppButton>
      </template>
    </AppModal>
  </div>
</template>
