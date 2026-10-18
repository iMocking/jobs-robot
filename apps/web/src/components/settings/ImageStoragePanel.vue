<script setup>
import { computed, reactive, ref, watch } from 'vue'

import { adminApi } from '@/api'
import AppBadge from '@/components/AppBadge.vue'
import AppButton from '@/components/AppButton.vue'
import AppIcon from '@/components/AppIcon.vue'
import AppInput from '@/components/AppInput.vue'
import AppModal from '@/components/AppModal.vue'
import { useAsync } from '@/composables/useAsync'
import { useAdminRefresh } from '@/composables/useAdminRefresh'
import { formatDateTime } from '@/utils/format'
import { useToastStore } from '@/stores/toast'

/**
 * 图片服务设置：本地磁盘 / 阿里云 OSS。
 *
 * 对应 internal/service/image.go，与邮件配置同款两层模型：
 * configs/config.yaml 的 image 段为基线、被数据库 storage.image 覆盖，
 * 保存后立即热生效（下一次头像上传即用新配置），无需重启服务。
 *
 * 三条刻意的设计约束（与 MailConfigPanel 保持一致）：
 * 1. **密钥永不回显**：后端只返回 has_secret 与占位符，这里也绝不把
 *    AccessKeySecret 放进 value；「留空 = 不修改」是接口约定。
 * 2. **校验规则与服务端逐条对齐**（config.ImageStorage.Validate）：
 *    local 必须给路径，oss 必须给 endpoint/bucket/AK；修改 AK 时必须同时
 *    填新 SK（服务端会拒绝「改 AK 沿用旧 SK」的组合，前端提前点破）。
 * 3. **回落到配置文件**只删除后台设置，不改 configs/config.yaml 本身。
 */
const props = defineProps({
  /** 配置保存成功后通知父级 */
  onSaved: { type: Function, default: null },
})

const toast = useToastStore()

const { data, loading, error, ready, run } = useAsync(() => adminApi.image.config(), {
  immediate: false,
})
useAdminRefresh(run)

const form = reactive({
  driver: 'local',
  local_dir: '',
  public_base_url: '',
  oss_endpoint: '',
  oss_bucket: '',
  oss_access_key_id: '',
  oss_access_key_secret: '',
  clear_secret: false,
})

const errors = reactive({
  driver: '',
  local_dir: '',
  oss_endpoint: '',
  oss_bucket: '',
  oss_access_key_id: '',
  oss_access_key_secret: '',
})

const baseline = ref('')
const changingSecret = ref(false)
const saving = ref(false)

function hydrate(view) {
  if (!view) return
  data.value = view
  form.driver = view.driver || 'local'
  form.local_dir = view.local_dir || ''
  form.public_base_url = view.public_base_url || ''
  form.oss_endpoint = view.oss_endpoint || ''
  form.oss_bucket = view.oss_bucket || ''
  form.oss_access_key_id = view.oss_access_key_id || ''
  form.oss_access_key_secret = ''
  form.clear_secret = false
  changingSecret.value = false
  Object.keys(errors).forEach((key) => {
    errors[key] = ''
  })
  baseline.value = JSON.stringify({
    driver: form.driver,
    local_dir: form.local_dir,
    public_base_url: form.public_base_url,
    oss_endpoint: form.oss_endpoint,
    oss_bucket: form.oss_bucket,
    oss_access_key_id: form.oss_access_key_id,
    clear_secret: false,
  })
}

watch(data, (value) => hydrate(value), { immediate: true })

const snapshot = computed(() =>
  JSON.stringify({
    driver: form.driver,
    local_dir: form.local_dir,
    public_base_url: form.public_base_url,
    oss_endpoint: form.oss_endpoint,
    oss_bucket: form.oss_bucket,
    oss_access_key_id: form.oss_access_key_id,
    clear_secret: form.clear_secret,
  }),
)

const secretTouched = computed(() => form.oss_access_key_secret !== '' || form.clear_secret)
const dirty = computed(() => baseline.value !== '' && snapshot.value !== baseline.value)

const isLocal = computed(() => form.driver === 'local')
const isOSS = computed(() => form.driver === 'oss')

const sourceLabel = computed(() => (data.value?.source === 'database' ? '后台设置' : '配置文件'))
const sourceHint = computed(() =>
  data.value?.source === 'database'
    ? '当前生效的是后台保存的配置，已覆盖 configs/config.yaml'
    : '当前生效的是 configs/config.yaml 中的配置，尚未在后台改过',
)

/**
 * 是否最终会有密钥：想清空就是没有；填了新密钥就是有；否则沿用已保存的状态。
 */
const willHaveSecret = computed(() => {
  if (form.clear_secret) return false
  if (form.oss_access_key_secret) return true
  return Boolean(data.value?.has_secret)
})

/**
 * 修改 AccessKeyID 却沿用旧密钥，几乎总是配置失误（新账号配旧口令必然上传失败）。
 * 与服务端 image.go 的同一规则对齐，在提交前点破。
 */
const akChanged = computed(
  () =>
    isOSS.value &&
    form.oss_access_key_id.trim() !== '' &&
    form.oss_access_key_id.trim() !== (data.value?.oss_access_key_id ?? '') &&
    !form.oss_access_key_secret &&
    !form.clear_secret &&
    Boolean(data.value?.has_secret),
)

function validate() {
  errors.driver = ''
  errors.local_dir = ''
  errors.oss_endpoint = ''
  errors.oss_bucket = ''
  errors.oss_access_key_id = ''
  errors.oss_access_key_secret = ''

  if (!['local', 'oss'].includes(form.driver)) {
    errors.driver = '存储驱动只支持 local/oss'
  }

  if (isLocal.value) {
    if (!form.local_dir.trim()) errors.local_dir = '本地存储路径不能为空'
  }

  if (isOSS.value) {
    if (!form.oss_endpoint.trim()) errors.oss_endpoint = 'OSS Endpoint 不能为空'
    if (!form.oss_bucket.trim()) errors.oss_bucket = 'OSS Bucket 不能为空'
    if (!form.oss_access_key_id.trim()) errors.oss_access_key_id = 'AccessKey ID 不能为空'
    if (akChanged.value) {
      errors.oss_access_key_secret =
        '修改 AccessKey ID 时必须同时填写新的 AccessKey Secret（不能沿用旧密钥）'
    }
  }

  return Object.values(errors).every((message) => !message)
}

async function save() {
  if (!validate()) {
    toast.error('请先修正表单中标红的问题')
    return
  }

  saving.value = true
  try {
    const view = await adminApi.image.saveConfig({
      driver: form.driver,
      local_dir: form.local_dir.trim(),
      public_base_url: form.public_base_url.trim(),
      oss_endpoint: form.oss_endpoint.trim(),
      oss_bucket: form.oss_bucket.trim(),
      oss_access_key_id: form.oss_access_key_id.trim(),
      // 空字符串即「不修改已保存的密钥」；这是接口约定而不是省略字段
      secret: form.clear_secret ? '' : form.oss_access_key_secret,
      clear_secret: form.clear_secret,
    })
    hydrate(view)
    toast.success('图片服务配置已保存并立即生效')
    props.onSaved?.()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

/* —— 回落到配置文件 —— */
const revertOpen = ref(false)
const reverting = ref(false)

async function submitRevert() {
  reverting.value = true
  try {
    const view = await adminApi.image.revertConfig()
    hydrate(view)
    toast.success('已删除后台设置，回落到 configs/config.yaml')
    revertOpen.value = false
    props.onSaved?.()
  } catch (err) {
    toast.error(err.message)
  } finally {
    reverting.value = false
  }
}

const DRIVERS = [
  {
    value: 'local',
    label: '本地磁盘',
    description: '图片写入服务器本地目录，通过本服务 /files 路由或公网前缀访问。适合单机部署。',
    icon: 'globe',
  },
  {
    value: 'oss',
    label: '阿里云 OSS',
    description: '图片上传到对象存储，由 OSS 公网域名提供访问。适合多机部署与 CDN 加速。',
    icon: 'cloud',
  },
]
</script>

<template>
  <div>
    <div v-if="loading && !ready" class="space-y-2">
      <div class="h-24 animate-pulse rounded-card bg-ink-100" />
      <div class="h-64 animate-pulse rounded-card bg-ink-100" />
    </div>

    <div v-else-if="error" class="rounded-card border border-red-200 bg-red-50 p-5">
      <p class="flex items-center gap-2 text-[14px] font-medium text-red-700">
        <AppIcon name="alert" :size="17" />
        图片服务配置加载失败
      </p>
      <p class="mt-1.5 text-[13px] text-red-600">{{ error.message }}</p>
      <AppButton variant="secondary" size="sm" class="mt-3" @click="run()">重新加载</AppButton>
    </div>

    <template v-else>
      <!-- 当前生效状态 -->
      <section class="card mb-2.5 p-5">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-[16px] font-semibold text-ink-900">当前生效配置</h2>
              <AppBadge :tone="data?.source === 'database' ? 'brand' : 'ink'" size="sm">
                来源：{{ sourceLabel }}
              </AppBadge>
              <AppBadge :tone="data?.ready ? 'accent' : 'amber'" size="sm" dot>
                驱动：{{ data?.driver === 'oss' ? 'OSS' : '本地磁盘' }}{{ data?.ready ? '（可用）' : '（未就绪）' }}
              </AppBadge>
            </div>

            <p class="mt-2 text-[12.5px] text-ink-500">{{ sourceHint }}</p>

            <p v-if="data?.updated_at" class="mt-1 text-[12.5px] text-ink-400">
              最后保存：{{ formatDateTime(data.updated_at) }}
              <span v-if="data?.updated_by">· {{ data.updated_by }}</span>
            </p>
          </div>

          <AppButton
            v-if="data?.source === 'database'"
            variant="secondary"
            size="sm"
            @click="revertOpen = true"
          >
            <AppIcon name="refresh" :size="15" />
            回落到配置文件
          </AppButton>
        </div>
      </section>

      <!-- 驱动选择 -->
      <section class="card mb-2.5 p-6">
        <h2 class="text-[16px] font-semibold text-ink-900">存储方式</h2>
        <p class="mt-1 mb-4 text-[12.5px] text-ink-400">
          头像等用户上传图片都走这里。单机部署用「本地磁盘」即可，多机/上云切到 OSS。
        </p>

        <div class="grid gap-3 sm:grid-cols-2">
          <button
            v-for="item in DRIVERS"
            :key="item.value"
            type="button"
            class="rounded-xl border p-4 text-left transition"
            :class="
              form.driver === item.value
                ? 'border-brand-500 bg-brand-50/60 ring-1 ring-brand-200'
                : 'border-ink-200 bg-white hover:border-brand-300'
            "
            :aria-pressed="form.driver === item.value"
            @click="form.driver = item.value"
          >
            <span class="flex items-center gap-2">
              <AppIcon
                :name="item.icon"
                :size="18"
                :class="form.driver === item.value ? 'text-brand-600' : 'text-ink-400'"
              />
              <span class="text-[14px] font-medium text-ink-900">{{ item.label }}</span>
              <AppIcon
                v-if="form.driver === item.value"
                name="check-circle"
                :size="16"
                class="ml-auto text-brand-600"
              />
            </span>
            <span class="mt-2 block text-[12.5px] leading-relaxed text-ink-500">
              {{ item.description }}
            </span>
          </button>
        </div>
      </section>

      <!-- 本地参数 -->
      <section v-if="isLocal" class="card mb-2.5 p-6">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 class="text-[16px] font-semibold text-ink-900">本地存储参数</h2>
            <p class="mt-1 text-[12.5px] text-ink-400">
              相对路径以项目目录为基准；目录不存在时服务会自动创建。
            </p>
          </div>
        </div>

        <div class="divide-y divide-ink-100">
          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">
              本地存储路径
              <span class="text-red-500">*</span>
            </label>
            <div class="max-w-[520px]">
              <AppInput
                v-model="form.local_dir"
                icon="folder"
                placeholder="var/uploads"
                :error="errors.local_dir"
              />
              <p class="mt-1.5 text-[12px] text-ink-400">
                头像等图片会写入该目录；已存在文件不会被影响。
              </p>
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">公网访问前缀</label>
            <div class="max-w-[520px]">
              <AppInput
                v-model="form.public_base_url"
                icon="globe"
                placeholder="https://cdn.example.com/uploads（留空走本服务 /files 路由）"
              />
              <p class="mt-1.5 text-[12px] text-ink-400">
                部署在网关 / CDN / 独立域名后时填写，图片 URL 将以此开头；留空则使用本服务的
                <code class="rounded bg-ink-100 px-1 py-0.5 font-mono text-[11.5px]">/api/v1/files</code>
                路由。
              </p>
            </div>
          </div>
        </div>
      </section>

      <!-- OSS 参数 -->
      <section v-else class="card mb-2.5 p-6">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 class="text-[16px] font-semibold text-ink-900">OSS 参数</h2>
            <p class="mt-1 text-[12.5px] text-ink-400">
              图片通过 PutObject 上传，公网访问地址由 Endpoint + Bucket 推导，也可单独配置前缀。
            </p>
          </div>
        </div>

        <div class="divide-y divide-ink-100">
          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">
              Endpoint
              <span class="text-red-500">*</span>
            </label>
            <div class="max-w-[520px]">
              <AppInput
                v-model="form.oss_endpoint"
                icon="globe"
                placeholder="oss-cn-hangzhou.aliyuncs.com"
                :error="errors.oss_endpoint"
              />
              <p class="mt-1.5 text-[12px] text-ink-400">可不带 https://，服务端自动补全。</p>
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">
              Bucket
              <span class="text-red-500">*</span>
            </label>
            <div class="max-w-[520px]">
              <AppInput
                v-model="form.oss_bucket"
                icon="box"
                placeholder="jobs-robot-images"
                :error="errors.oss_bucket"
              />
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">
              AccessKey ID
              <span class="text-red-500">*</span>
            </label>
            <div class="max-w-[520px]">
              <AppInput
                v-model="form.oss_access_key_id"
                icon="key"
                placeholder="LTAI5t..."
                autocomplete="off"
                :error="errors.oss_access_key_id"
              />
              <p class="mt-1.5 text-[12px] text-ink-400">
                建议使用 RAM 子账号密钥，仅授予目标 Bucket 的写权限。
              </p>
            </div>
          </div>

          <div class="form-row">
            <label class="text-[13.5px] text-ink-600">AccessKey Secret</label>
            <div class="max-w-[520px]">
              <template v-if="data?.has_secret && !changingSecret && !form.clear_secret">
                <div
                  class="flex h-11 items-center gap-2 rounded-xl border border-ink-200 bg-ink-50 px-3.5"
                >
                  <AppIcon name="lock" :size="17" class="text-ink-400" />
                  <span class="flex-1 font-mono text-sm tracking-widest text-ink-500">
                    {{ data.secret_mask }}
                  </span>
                  <AppBadge tone="accent" size="sm">已配置</AppBadge>
                  <button
                    type="button"
                    class="text-[12.5px] text-brand-600 transition hover:text-brand-700"
                    @click="changingSecret = true"
                  >
                    修改
                  </button>
                  <button
                    type="button"
                    class="text-[12.5px] text-ink-400 transition hover:text-red-600"
                    @click="form.clear_secret = true"
                  >
                    清空
                  </button>
                </div>
                <p class="mt-1.5 text-[12px] text-ink-400">
                  出于安全考虑，已保存的密钥不会回显。留空提交即表示保持原密钥不变。
                </p>
              </template>

              <template v-else>
                <AppInput
                  v-model="form.oss_access_key_secret"
                  icon="key"
                  type="password"
                  autocomplete="new-password"
                  placeholder="输入新的 AccessKey Secret"
                  :disabled="form.clear_secret"
                  :error="errors.oss_access_key_secret"
                />
                <div class="mt-1.5 flex flex-wrap items-center gap-3">
                  <button
                    v-if="data?.has_secret && changingSecret"
                    type="button"
                    class="text-[12.5px] text-ink-400 transition hover:text-ink-700"
                    @click="changingSecret = false; form.oss_access_key_secret = ''"
                  >
                    取消修改
                  </button>
                  <button
                    v-if="form.clear_secret"
                    type="button"
                    class="text-[12.5px] text-brand-600 transition hover:text-brand-700"
                    @click="form.clear_secret = false"
                  >
                    撤销「清空密钥」
                  </button>
                </div>

                <p
                  v-if="form.clear_secret"
                  class="mt-1.5 flex items-center gap-1.5 text-[12.5px] text-amber-600"
                >
                  <AppIcon name="alert" :size="14" />
                  保存后将清空已存密钥；OSS 驱动没有密钥将无法上传。
                </p>
                <p v-else-if="akChanged" class="mt-1.5 text-[12.5px] text-amber-600">
                  已修改 AccessKey ID：必须填写新的 AccessKey Secret，否则服务端会拒绝保存。
                </p>
                <p v-else class="mt-1.5 text-[12px] text-ink-400">
                  密钥以密文存储，接口永不回显明文。
                </p>
              </template>
            </div>
          </div>
        </div>
      </section>

      <!-- 操作区 -->
      <div class="card flex flex-wrap items-center justify-between gap-3 p-5">
        <p v-if="dirty" class="flex items-center gap-1.5 text-[12.5px] font-medium text-amber-600">
          <AppIcon name="alert" :size="14" />
          有未保存的修改
        </p>
        <p v-else class="text-[12.5px] text-ink-400">配置修改后立即生效，无需重启服务。</p>

        <AppButton :loading="saving" :disabled="!dirty" @click="save">保存配置</AppButton>
      </div>
    </template>

    <!-- 回落到配置文件 -->
    <AppModal v-model="revertOpen" title="回落到配置文件" size="sm">
      <p class="text-[14px] leading-relaxed text-ink-600">
        将删除数据库中的 <code class="rounded bg-ink-100 px-1.5 py-0.5 font-mono text-[12.5px]">storage.image</code>
        配置，改用 <code class="rounded bg-ink-100 px-1.5 py-0.5 font-mono text-[12.5px]">configs/config.yaml</code>
        里的图片服务配置，并立即热生效。已保存的 OSS 密钥会一并删除。
      </p>
      <template #footer>
        <AppButton variant="secondary" @click="revertOpen = false">取消</AppButton>
        <AppButton variant="danger" :loading="reverting" @click="submitRevert">确认回落</AppButton>
      </template>
    </AppModal>
  </div>
</template>
