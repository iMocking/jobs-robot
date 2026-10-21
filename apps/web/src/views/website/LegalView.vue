<script setup>
import { computed, watch } from 'vue'

import AppIcon from '@/components/AppIcon.vue'

/**
 * 用户服务协议 / 隐私政策。
 *
 * 两条路由（/legal/terms、/legal/privacy）共用这一个组件，用 props.doc 区分文档 ——
 * 两份文件的版式、章节编号规则完全一致，拆成两个文件只会让样式慢慢跑偏。
 *
 * 内容策略：这是**演示占位文本**。真实协议必须由执业律师按实际业务书写，
 * 因此这里刻意不写任何具体的赔偿上限、保留期限或响应时限，
 * 一律用「将在正式版本中明确」把口子留出来，避免演示文案被当成承诺。
 */
const props = defineProps({
  doc: { type: String, default: 'terms' },
})

const TERMS = {
  title: '用户服务协议',
  subtitle: '本协议说明你在使用 JobHub 直招（以下称「本站」）时双方的权利与义务，请在使用前完整阅读。',
  updated: '2026-01-08',
  sections: [
    {
      title: '服务说明',
      paragraphs: [
        '本站提供的是岗位信息检索服务：我们把企业官网《加入我们》（或同等官方招聘入口）页面上公开的岗位整理成便于检索与比较的形式，并标注每一条岗位的来源页面。',
        '本站不参与招聘流程，不提供简历投递、面试安排或入职协助，也不向求职者收取任何费用。岗位信息的权利属于发布它的企业，本站只做整理与来源标注。',
      ],
    },
    {
      title: '账号与安全',
      paragraphs: [
        '部分功能需要注册账号后使用。你应当提供真实、可用的邮箱地址，并妥善保管账号与密码。',
        '通过你的账号进行的操作，除有相反证据外视为你本人的行为。如发现账号被他人使用，请立即修改密码，并通过「我的 → 活跃设备」下线不认识的登录设备。',
      ],
      bullets: [
        '不要与他人共用账号，也不要把密码告知任何人（本站工作人员不会向你索取密码）。',
        '建议使用长度不少于 8 位、同时包含字母与数字的密码，并避免与其它网站重复。',
      ],
    },
    {
      title: '用户行为规范',
      paragraphs: [
        '你可以为求职目的浏览、检索本站内容。未经许可，不得使用自动化程序批量抓取本站数据，不得对本站进行扫描、渗透或干扰其正常运行。',
        '本站目前不提供用户发布岗位或评论的功能。若未来开放相关内容功能，你需对发布的内容负责，并保证不侵犯他人权利。',
      ],
      bullets: [
        '不得将本站数据用于转售、二次聚合或训练商业模型。',
        '不得冒用企业、招聘方或本站工作人员的名义发布信息。',
      ],
    },
    {
      title: '岗位信息与来源说明',
      paragraphs: [
        '本站岗位信息来自企业官网公开页面。我们会对抓取结果做人工审核，并保留来源页面地址、内容变更历史与核查时间，但无法保证信息与企业官网时刻完全一致。',
        '岗位是否仍在招聘、薪资与要求是否已调整，请以企业官网《加入我们》页面为准。若你发现某条岗位与来源页面不符，欢迎通过本页底部的方式反馈，我们会复核并在必要时下线该岗位。',
      ],
    },
    {
      title: '知识产权',
      paragraphs: [
        '本站的界面设计、栏目结构、文案与程序代码归本站运营方所有；岗位内容及其它由企业提供的信息归相应权利人所有。',
        '在注明来源并保留原始链接的前提下，你可以为个人求职目的引用岗位信息。超出该范围的转载、汇编或商业使用，需事先取得权利人许可。',
      ],
    },
    {
      title: '免责声明',
      paragraphs: [
        '本站按「现状」提供服务。我们会尽合理努力保持服务可用与信息准确，但不对服务的连续性、及时性或岗位信息的绝对完整性作出担保。',
        '因企业官网内容变更、第三方网站不可访问、网络故障或不可抗力导致的服务中断或信息偏差，本站不承担由此产生的求职结果责任。责任限制的具体范围与方式，将在正式版本中依法明确。',
      ],
    },
    {
      title: '协议变更',
      paragraphs: [
        '本协议可能随功能调整或法律要求而更新。更新后的版本会在本页公示，并同步修改页面顶部的「最后更新」日期。',
        '若变更涉及你的重要权益，我们会以站内提示等显著方式告知。变更生效后继续使用本站，即视为接受更新后的协议。',
      ],
    },
    {
      title: '联系我们',
      paragraphs: [
        '对本协议或本站服务有疑问、意见或投诉，可通过站点页脚公布的方式与我们联系。',
        '演示环境不提供真实的客服渠道与响应时限；正式上线时将在此补充专门的联系邮箱、响应时限与争议解决方式。',
      ],
    },
  ],
}

const PRIVACY = {
  title: '隐私政策',
  subtitle: '本政策说明 JobHub 直招（以下称「本站」）会处理哪些信息、为什么处理，以及你可以如何管理这些信息。',
  updated: '2026-01-08',
  sections: [
    {
      title: '我们收集哪些信息',
      paragraphs: [
        '我们只收集为提供岗位检索与账号服务所必需的信息，不收集与求职无关的敏感个人信息。',
        '本站不提供在线投递功能，因此我们不会收到你的简历、身份证件或学历证明。',
      ],
      bullets: [
        '账号信息：注册邮箱、昵称，以及你自愿填写的手机号。',
        '登录信息：登录设备与浏览器标识、IP 地址、最近活跃时间，用于展示与管理活跃设备。',
        '使用信息：搜索关键词、筛选条件与浏览记录等，用于改进检索效果。',
      ],
    },
    {
      title: '信息如何使用',
      paragraphs: [
        '我们使用上述信息来创建与维护你的账号、保障账号安全、展示与恢复你的筛选条件，以及统计整体使用情况以改进产品。',
        '我们不会基于你的个人信息进行个性化广告投放，也不会将其用于与招聘信息检索无关的用途。',
      ],
    },
    {
      title: 'Cookie 与本地存储',
      paragraphs: [
        '本站使用浏览器本地存储保存登录令牌与少量界面偏好，避免你每次访问都要重新登录。这些数据保存在你自己的浏览器中，清除浏览器数据即可删除。',
        '演示环境不接入第三方统计或广告脚本。若正式版本引入此类服务，我们会在本政策中列出其名称、用途与关闭方式。',
      ],
    },
    {
      title: '信息共享与披露',
      paragraphs: [
        '我们不会出售你的个人信息。仅在下列情形下才可能对外提供：取得你的单独同意；为履行法定义务或响应有权机关的合法要求；为保护你或他人的合法权益所必需。',
        '若我们使用云服务商等受托处理方来承载服务，将与其签署数据处理约定，并在正式版本中列明处理方的类别与处理范围。',
      ],
    },
    {
      title: '数据安全',
      paragraphs: [
        '我们采取传输加密、访问权限最小化、账号密码不以明文形式保存等常规措施来保护你的信息，并仅在有业务必要时保留这些信息。',
        '各类信息的具体保存期限与技术措施，将在正式版本中结合业务实际与法律要求逐项明确。',
      ],
    },
    {
      title: '你的权利',
      paragraphs: [
        '你可以随时在「我的」页面查看与修改昵称、手机号，也可以修改密码或下线其它登录设备。',
        '如需查询、更正、删除账号信息或注销账号，可通过本页底部的方式提出。账号注销的入口与处理时限将在正式版本中提供。',
      ],
    },
    {
      title: '未成年人保护',
      paragraphs: [
        '本站面向有求职需求的成年人。若你未满 18 周岁，请在监护人陪同下使用，并由监护人阅读同意本政策。',
        '我们不会主动收集未成年人的个人信息；如发现误收集，会在核实后尽快删除。',
      ],
    },
    {
      title: '联系方式',
      paragraphs: [
        '如对本政策有疑问，或希望行使上述权利，可通过站点页脚公布的方式与我们联系。',
        '演示环境不提供真实的隐私事务受理渠道；正式上线时将在此补充专门的联系邮箱与响应时限，并说明个人信息保护负责人的联系方式。',
      ],
    },
  ],
}

const DOCS = { terms: TERMS, privacy: PRIVACY }

const activeDoc = computed(() => DOCS[props.doc] ?? TERMS)
const title = computed(() => activeDoc.value.title)
const subtitle = computed(() => activeDoc.value.subtitle)
const updated = computed(() => activeDoc.value.updated)
const sections = computed(() => activeDoc.value.sections)

// 两条路由复用同一个组件，props 变化时组件不会重新挂载，
// 若不手动滚动，用户会停在上一份文档的中段。
watch(
  () => props.doc,
  () => window.scrollTo({ top: 0, behavior: 'smooth' }),
)
</script>

<template>
  <div class="container-page py-8">
    <article class="card mx-auto w-full max-w-[860px] p-8 sm:p-10">
      <header class="border-b border-ink-100 pb-6">
        <h1 class="text-[26px] font-semibold text-ink-900">{{ title }}</h1>
        <p class="mt-3 text-[14px] leading-[1.9] text-ink-500">{{ subtitle }}</p>
        <p class="mt-3 text-[12.5px] text-ink-400">最后更新：{{ updated }} · 演示文本</p>
      </header>

      <!-- 演示环境必须显式声明文本性质，避免占位内容被当作真实法律文件 -->
      <div class="mt-6 rounded-xl border border-warn-500/30 bg-warn-50 p-5">
        <p class="flex items-center gap-2 text-[13.5px] font-medium text-warn-600">
          <AppIcon name="alert" :size="16" />
          演示占位文本 · 不构成法律意见
        </p>
        <p class="mt-2 text-[13px] leading-[1.9] text-ink-600">
          以下内容为演示环境的占位文本，仅用于展示文档版式，不构成任何法律意见，也不构成对服务的承诺。
          正式上线前，本文件必须由执业律师根据实际业务与适用法律逐条审阅、修订后发布。
        </p>
      </div>

      <div class="mt-8 space-y-8">
        <section v-for="(section, index) in sections" :key="section.title">
          <h2 class="text-[17px] font-semibold text-ink-900">
            {{ index + 1 }}. {{ section.title }}
          </h2>

          <div class="mt-3 space-y-3 text-[14px] leading-[1.95] text-ink-600">
            <p v-for="paragraph in section.paragraphs" :key="paragraph">{{ paragraph }}</p>
          </div>

          <ul v-if="section.bullets?.length" class="mt-3 space-y-2.5">
            <li
              v-for="bullet in section.bullets"
              :key="bullet"
              class="flex gap-2.5 text-[14px] leading-relaxed text-ink-600"
            >
              <span class="mt-[9px] size-1.5 shrink-0 rounded-full bg-brand-500" />
              <span>{{ bullet }}</span>
            </li>
          </ul>
        </section>
      </div>

      <footer class="mt-9 border-t border-ink-100 pt-6 text-[13px] leading-relaxed text-ink-400">
        本文件为演示占位版本，未标注生效日期。正式版本将补充生效日期、联系渠道与争议解决方式。
      </footer>
    </article>
  </div>
</template>
