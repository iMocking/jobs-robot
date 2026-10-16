import { createRouter, createWebHistory } from 'vue-router'

import { useSessionStore } from '@/stores/session'
import { useSystemStore } from '@/stores/system'

/**
 * 路由设计沿用既定的三条约定：
 *
 * 1. **按「外壳」分顶级分支**，而不是按角色分文件：
 *    前台 LayWebsite / 认证 LayAuth / 后台 LayWorktable 各自是一个顶级记录，
 *    业务页面是它们的 children。换一套导航就是换一个 layout，页面组件不用改。
 * 2. **meta 驱动行为**：title 由 afterEach 写入 document.title，
 *    requiresAuth / guestOnly 由 beforeEach 统一处理，页面里不再写跳转逻辑。
 * 3. **懒加载**：所有视图都用 () => import()，首屏只加载当前外壳。
 *
 * 三个顶级分支的 path 都是 '/'，与既有模板保持一致：
 * 子路由以父级 path 为基准拼出 /home、/login、/dashboard 这样的绝对路径，
 * 精确匹配到 '/' 时由注册顺序在前的分支（前台）接管并 redirect。
 *
 * 命名上刻意避开同路径冲突（模板里 /products 与 /product 的教训）：
 *   - 前台职位「列表 /jobs」「详情 /job/:id」
 *   - 后台岗位管理用 /postings、/postings/:id
 *   - 前台公司库 /companies、/company/:id；后台公司管理用 /sources（来源官网）
 * 否则按注册顺序会被前台抢先匹配，后台对应页面将永远打不开。
 */
const routes = [
  {
    path: '/',
    name: 'website',
    component: () => import('@/layouts/LayWebsite.vue'),
    redirect: { name: 'home' },
    meta: { title: 'JobHub 直招 · 发现官网直招好工作' },
    children: [
      {
        path: 'home',
        name: 'home',
        component: () => import('@/views/HomeView.vue'),
        meta: { title: '首页 · JobHub 直招' },
      },
      {
        path: 'jobs',
        name: 'jobs',
        component: () => import('@/views/JobsView.vue'),
        meta: { title: '职位 · JobHub 直招' },
      },
      {
        path: 'job/:id',
        name: 'job',
        component: () => import('@/views/JobDetailView.vue'),
        meta: { title: '职位详情 · JobHub 直招' },
      },
      {
        path: 'companies',
        name: 'companies',
        component: () => import('@/views/CompaniesView.vue'),
        meta: { title: '公司库 · JobHub 直招' },
      },
      {
        path: 'company/:id',
        name: 'company',
        component: () => import('@/views/CompanyDetailView.vue'),
        meta: { title: '公司详情 · JobHub 直招' },
      },
      {
        path: 'about',
        name: 'about',
        component: () => import('@/views/AboutView.vue'),
        meta: { title: '关于直招 · JobHub 直招' },
      },
      {
        path: 'legal/terms',
        name: 'terms',
        component: () => import('@/views/LegalView.vue'),
        props: { doc: 'terms' },
        meta: { title: '用户服务协议 · JobHub 直招' },
      },
      {
        path: 'legal/privacy',
        name: 'privacy',
        component: () => import('@/views/LegalView.vue'),
        props: { doc: 'privacy' },
        meta: { title: '隐私政策 · JobHub 直招' },
      },
      {
        path: 'account',
        name: 'account',
        component: () => import('@/views/AccountView.vue'),
        meta: { title: '我的 · JobHub 直招', requiresAuth: true },
      },
    ],
  },
  {
    path: '/',
    name: 'auth',
    component: () => import('@/layouts/LayAuth.vue'),
    meta: { title: 'JobHub 直招 · 发现官网直招好工作' },
    children: [
      {
        path: 'login',
        name: 'login',
        component: () => import('@/views/auth/LoginView.vue'),
        meta: { title: '登录 · JobHub 直招', guestOnly: true },
      },
      {
        path: 'register',
        name: 'register',
        component: () => import('@/views/auth/RegisterView.vue'),
        meta: { title: '注册 · JobHub 直招', guestOnly: true },
      },
      {
        path: 'forgot-password',
        name: 'forgot-password',
        component: () => import('@/views/auth/ForgotPasswordView.vue'),
        meta: { title: '找回密码 · JobHub 直招', guestOnly: true },
      },
      {
        // 首次安装向导。刻意**不加** guestOnly / requiresAuth：
        // 它存在的意义就是「系统里还没有任何可用账号」，任何鉴权前置都会自相矛盾。
        path: 'setup',
        name: 'setup',
        component: () => import('@/views/setup/SetupView.vue'),
        meta: { title: '初始化设置 · JobHub 直招' },
      },
      {
        // 不加 guestOnly：已登录用户点开邮件里的重置链接时，
        // 也应该能完成重置，而不是被直接弹回「我的」。
        path: 'reset-password',
        name: 'reset-password',
        component: () => import('@/views/auth/ResetPasswordView.vue'),
        meta: { title: '重置密码 · JobHub 直招' },
      },
    ],
  },
  {
    path: '/',
    name: 'admin',
    component: () => import('@/layouts/LayWorktable.vue'),
    redirect: { name: 'dashboard' },
    meta: { title: '后台管理 · JobHub 直招' },
    children: [
      {
        path: 'dashboard',
        name: 'dashboard',
        component: () => import('@/views/admin/DashboardView.vue'),
        meta: { title: '仪表盘 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'postings',
        name: 'postings',
        component: () => import('@/views/admin/PostingsView.vue'),
        meta: { title: '岗位管理 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'postings/:id',
        name: 'posting',
        component: () => import('@/views/admin/PostingDetailView.vue'),
        meta: { title: '岗位详情 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'review',
        name: 'review',
        component: () => import('@/views/admin/ReviewView.vue'),
        meta: { title: '审核队列 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'sources',
        name: 'sources',
        component: () => import('@/views/admin/SourcesView.vue'),
        meta: { title: '公司管理 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'sources/:id',
        name: 'source',
        component: () => import('@/views/admin/SourceDetailView.vue'),
        meta: { title: '公司详情 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'users',
        name: 'users',
        component: () => import('@/views/admin/UsersView.vue'),
        meta: { title: '用户管理 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'user/:id',
        name: 'user',
        component: () => import('@/views/admin/UserDetailView.vue'),
        meta: { title: '用户详情 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'roles',
        name: 'roles',
        component: () => import('@/views/admin/RolesView.vue'),
        meta: { title: '角色权限 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'settings',
        name: 'settings',
        component: () => import('@/views/admin/SettingsView.vue'),
        meta: { title: '系统配置 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'monitor',
        name: 'monitor',
        component: () => import('@/views/admin/MonitorView.vue'),
        meta: { title: '系统监控 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'logs',
        name: 'logs',
        component: () => import('@/views/admin/LogsView.vue'),
        meta: { title: '审计日志 · JobHub 直招', requiresAuth: true },
      },
      {
        path: 'log/:id',
        name: 'log',
        component: () => import('@/views/admin/LogDetailView.vue'),
        meta: { title: '日志详情 · JobHub 直招', requiresAuth: true },
      },
    ],
  },
  {
    path: '/404',
    name: 'not-found',
    component: () => import('@/views/NotFoundView.vue'),
    meta: { title: '页面走丢了 · JobHub 直招' },
  },
  { path: '/:pathMatch(.*)*', redirect: '/404' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(to, from, savedPosition) {
    if (savedPosition) return savedPosition
    if (to.hash) return { el: to.hash, behavior: 'smooth', top: 96 }
    return { top: 0 }
  },
})

// 守卫顺序：登录态恢复 → 初始化状态 → 页面级要求。
//
// 必须先等 restore() 完成，否则刚打开页面时会把已登录用户误判成游客。
// 初始化判定要放在 requiresAuth 之前：系统还没有任何可用账号时，
// 「没登录」不是用户的问题，把他送去登录页只会看到一个永远登不进去的界面。
router.beforeEach(async (to) => {
  const session = useSessionStore()
  const system = useSystemStore()
  await session.restore()
  await system.load()

  // —— 首次启动：尚无可用管理员 ——
  // 此时注册/登录都走不通（没有账号可登，且缺 SMTP 时验证码发不出去）。
  // 无论安装向导是否在配置里开启，都先去 /setup：那里要么是向导，
  // 要么是「向导已关闭，请用 cmd/seed 自举」的可执行说明。
  if (system.needsSetup) {
    if (to.name === 'setup') return true
    // 只记住有意义的回跳目标，避免把根路径也塞进查询串
    return { name: 'setup', query: to.fullPath === '/' ? {} : { redirect: to.fullPath } }
  }

  // —— 已初始化：安装向导永久失效 ——
  // 直接弹回登录页，不留下一个看起来可提交、实际必然 409 的建号入口。
  if (to.name === 'setup') {
    return { name: 'login' }
  }

  if (to.meta?.requiresAuth && !session.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.meta?.guestOnly && session.isAuthenticated) {
    return { name: 'account' }
  }
  return true
})

router.afterEach((to) => {
  const base = 'JobHub 直招'
  document.title = to.meta?.title ?? base
})

export default router
