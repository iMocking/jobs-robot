<script setup>
import AppButton from '@/components/AppButton.vue'
import AppIcon from '@/components/AppIcon.vue'
import TrustBadge from '@/components/TrustBadge.vue'

/**
 * 关于直招。
 *
 * 这一页没有任何接口调用 —— 它要回答的不是「有什么岗位」，而是
 * 「凭什么说这些岗位是直招的」。因此内容顺序是：定义 → 溯源能力 → 能看到什么 → 怎么采集 → 企业怎么进来。
 *
 * 页脚的「数据来源说明」「企业入驻」直接指向 #source / #join，
 * 这两个 id 是页脚契约的一部分，改动前必须同步 WebsiteFooter。
 */

/** 入库时一起留存的信息，逐项列出比写成一句话更容易核对 */
const SOURCE_FIELDS = ['来源站点', '来源页面 URL', '抓取时间', '内容变更历史']

/** 三个时间字段最容易混淆，用表格逐行解释，而不是塞进一段话里 */
const TIME_FIELDS = [
  {
    name: '发布时间',
    text: '岗位首次出现在企业官网《加入我们》页面上的时间，取自来源页面自身，不是我们抓取的时间。',
  },
  {
    name: '最后更新时间',
    text: '来源页面内容（标题、薪资、要求等）最近一次发生变化的时间；页面没变就不会被刷新。',
  },
  {
    name: '最后核查时间',
    text: '我们最近一次成功访问来源页面的时间。它只能证明「这条信息还在」，不能证明它一定还有效。',
  },
]

const DIMENSIONS = [
  { icon: 'document', title: '岗位介绍', text: '这个岗位做什么、属于哪条业务线，内容来自来源页面。' },
  { icon: 'clipboard', title: '任职要求', text: '学历、经验与技能门槛，不添加来源页面没有写过的条件。' },
  { icon: 'calendar', title: '发布时间', text: '岗位在官网《加入我们》页面上首次出现的时间。' },
  { icon: 'mail', title: '联系方式', text: '来源页面公开的投递邮箱或电话；页面未公开时如实留空。' },
  { icon: 'map-pin', title: '工作地址', text: '城市、区域与办公地点，与来源页面保持一致的写法。' },
  { icon: 'clock', title: '最后更新时间', text: '这条岗位内容最近一次发生变化的时间。' },
]

const CRAWL_RULES = [
  '只抓取公开可访问的页面，不绕过登录、验证码或任何访问控制。',
  '遵循站点的 robots.txt；robots 禁止的路径一律不抓。',
  '尊重站点通过 noai、noindex 或条款声明的拒绝抓取意愿，收到即停止。',
  '同一域名串行抓取并限速，尽量减少对目标站点的压力。',
  '被过滤掉的岗位同样保留记录与原因，便于事后复核，而不是悄悄丢弃。',
]

const JOIN_STEPS = [
  { title: '提交官网《加入我们》入口', text: '企业提供招聘入口地址，或由我们根据官网导航定位。' },
  { title: '自动定位并校验', text: '确认该页面确实是官网页面的招聘入口，而不是第三方转载页。' },
  { title: '岗位进入人工审核', text: '抓取到的岗位逐条与来源页面比对，信息不一致的不予展示。' },
  { title: '审核通过后展示，并保留来源链接', text: '每条岗位详情里都能点回它被发现的那个页面。' },
]
</script>

<template>
  <div class="container-page py-8">
    <div class="mx-auto max-w-[920px] space-y-6">
      <!-- 开篇：先把「直招」这个词的定义钉死，后面的章节才立得住 -->
      <section class="card p-8 sm:p-10">
        <TrustBadge variant="green" />
        <h1 class="mt-5 text-[30px] leading-tight font-bold text-ink-900 sm:text-[34px]">
          关于直招
        </h1>
        <p class="mt-4 text-[15px] leading-[1.95] text-ink-600">
          在招聘广告里，「直招」经常只是一个形容词：谁都可以说自己直招。我们不这么用它 ——
          在这里，「直招」是一条可以被验证的判断：
          <strong class="font-medium text-ink-900">
            只有当岗位出现在企业官网《加入我们》（或同等官方招聘入口）页面上时，才会被收录。
          </strong>
          所以每条岗位都带着它被发现的那个地址，你可以自己点开、自己核对，而不必相信我们的说法。
        </p>
        <p class="mt-3.5 text-[15px] leading-[1.95] text-ink-600">
          我们不做简历投递，也不参与招聘流程。你看到的信息属于企业，我们只负责把它从官网首页搬到更好用的界面上，
          并且把来源一并留下。
        </p>
      </section>

      <!-- 页脚「数据来源说明」的落点 -->
      <section id="source" class="card scroll-mt-24 p-8 sm:p-9">
        <h2 class="text-[20px] font-semibold text-ink-900">为什么这里的岗位可以溯源</h2>
        <p class="mt-3.5 text-[14px] leading-[1.9] text-ink-600">
          每条岗位入库时，都会连同它的「出身」一起存下来，而不是只存一份清洗后的文本：
        </p>

        <ul class="mt-4 grid gap-3 sm:grid-cols-2">
          <li
            v-for="item in SOURCE_FIELDS"
            :key="item"
            class="flex items-center gap-2.5 rounded-xl bg-ink-50 px-4 py-3 text-[13.5px] text-ink-700"
          >
            <AppIcon name="check" :size="15" class="text-brand-600" />
            {{ item }}
          </li>
        </ul>

        <p class="mt-5 text-[14px] leading-[1.9] text-ink-600">
          其中「内容变更历史」保存的是同一岗位的历次修订：薪资、要求或标题只要变过，
          旧版本就不会被覆盖。这意味着我们不会用一条新岗位去修补旧岗位的错误 ——
          改了什么、什么时候改的，都能查。
        </p>

        <p class="mt-5 text-[14px] leading-[1.9] text-ink-600">
          页面上有三个时间字段，含义各不相同，请按下面的口径理解：
        </p>

        <div class="mt-4 overflow-hidden rounded-xl border border-ink-200">
          <table class="w-full border-collapse text-left">
            <thead class="thead-row">
              <tr>
                <th class="th-cell w-[130px]">字段</th>
                <th class="th-cell">含义</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-ink-100">
              <tr v-for="row in TIME_FIELDS" :key="row.name">
                <td class="td-cell text-[13.5px] font-medium whitespace-nowrap text-ink-800">
                  {{ row.name }}
                </td>
                <td class="td-cell text-[13.5px] leading-relaxed text-ink-600">{{ row.text }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="card p-8 sm:p-9">
        <h2 class="text-[20px] font-semibold text-ink-900">你会看到哪些信息</h2>
        <p class="mt-3.5 text-[14px] leading-[1.9] text-ink-600">
          每条岗位都按同一套维度整理，方便横向比较，也方便你回到来源页面逐项核对。
        </p>

        <div class="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <div v-for="item in DIMENSIONS" :key="item.title" class="rounded-xl border border-ink-200 p-4">
            <span class="flex size-9 items-center justify-center rounded-lg bg-brand-50 text-brand-600">
              <AppIcon :name="item.icon" :size="18" />
            </span>
            <p class="mt-3 text-[14px] font-medium text-ink-900">{{ item.title }}</p>
            <p class="mt-1.5 text-[13px] leading-relaxed text-ink-500">{{ item.text }}</p>
          </div>
        </div>
      </section>

      <section class="card p-8 sm:p-9">
        <h2 class="text-[20px] font-semibold text-ink-900">我们如何采集</h2>
        <p class="mt-3.5 text-[14px] leading-[1.9] text-ink-600">
          采集的边界写在代码里，也写在这里：我们只做「公开页面的搬运工」，不做任何越界的事。
        </p>

        <ul class="mt-4 space-y-3">
          <li
            v-for="rule in CRAWL_RULES"
            :key="rule"
            class="flex gap-2.5 text-[14px] leading-relaxed text-ink-600"
          >
            <span class="mt-[9px] size-1.5 shrink-0 rounded-full bg-brand-500" />
            <span>{{ rule }}</span>
          </li>
        </ul>
      </section>

      <!-- 页脚「企业入驻」的落点 -->
      <section id="join" class="card scroll-mt-24 p-8 sm:p-9">
        <h2 class="text-[20px] font-semibold text-ink-900">企业入驻</h2>
        <p class="mt-3.5 text-[14px] leading-[1.9] text-ink-600">
          企业不需要付费即可被收录 —— 只要招聘入口在官网上公开可见，就会被纳入采集范围。
          如果你希望我们更快发现它，可以主动提交入口：
        </p>

        <ol class="mt-5 space-y-4">
          <li v-for="(step, index) in JOIN_STEPS" :key="step.title" class="flex gap-3.5">
            <span
              class="flex size-7 items-center justify-center rounded-full bg-brand-50 text-xs font-semibold text-brand-700"
            >
              {{ index + 1 }}
            </span>
            <div class="min-w-0">
              <p class="text-[14px] font-medium text-ink-900">{{ step.title }}</p>
              <p class="mt-1 text-[13px] leading-relaxed text-ink-500">{{ step.text }}</p>
            </div>
          </li>
        </ol>

        <p class="mt-6 flex gap-2.5 rounded-xl bg-ink-50 px-4 py-3.5 text-[13px] leading-relaxed text-ink-600">
          <AppIcon name="shield-check" :size="16" class="mt-px shrink-0 text-brand-600" />
          <span>
            企业无法通过付费隐藏来源链接，也无法要求我们改写岗位内容。
            来源页面 URL 是岗位信息的一部分，不是可选的装饰 —— 这条规则对任何规模的客户都成立。
          </span>
        </p>

        <div class="mt-6 flex flex-wrap items-center gap-3">
          <AppButton variant="secondary" disabled>企业入驻申请（演示环境暂未开放）</AppButton>
          <span class="text-[12.5px] text-ink-400">
            当前为界面演示版本，申请入口将在正式上线后开放。
          </span>
        </div>
      </section>
    </div>
  </div>
</template>
