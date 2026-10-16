# JobHub 直招 · Web 端

`.creative-design/jobhub-ui/` 里 10 张设计稿的 Vue 3 实现：前台（职位列表 / 职位详情 / 公司库 / 关于直招）、
认证（登录 / 注册 / 找回密码 / 重置密码）、后台（仪表盘 / 岗位管理 / 审核队列 / 公司管理 / 用户管理 /
角色权限 / 系统配置 / 系统监控 / 审计日志）。

**默认对接真实后端**（`cmd/api`），字段与错误码口径完全来自 `internal/service/view.go` 的 DTO。

## 技术栈

| 关注点 | 选择 | 说明 |
|---|---|---|
| 框架 | Vue 3（`<script setup>`，JavaScript） | 与既定路由模板保持同一写法，不引入 TS 编译层 |
| 构建 | Vite 8 | |
| 样式 | Tailwind CSS v4 | CSS-first 配置，设计令牌写在 `src/style.css` 的 `@theme` 里 |
| 路由 | vue-router 5 | |
| 状态 | Pinia | `session` / `toast` / `dictionaries` / `passwordReset` |
| 图标 | 自绘内联 SVG | 见 `src/components/AppIcon.vue`，不引图标库 |
| 图表 | 自绘 SVG / DOM | 见 `DonutChart.vue` / `BarChart.vue`，不引图表库 |

运行时依赖只有 `vue`、`vue-router`、`pinia` 三个。

## 起服务

### 1. 后端依赖与 API

```bash
# 在仓库根目录：PostgreSQL 17 + Redis 7
docker compose -f deployments/docker-compose.yml up -d

# 初始化角色、管理员与内置配置项（幂等）
go run ./cmd/seed -admin-email admin@jobs-robot.local -admin-password 'Jobhub12345'

# 启动 API（configs/config.yaml 里 server.port = 8090）
go run ./cmd/api
```

### 2. 前端

```bash
cd apps/web
npm install
npm run dev        # http://localhost:5173
npm run build      # 产物在 dist/
```

`vite.config.js` 已把 `/api`、`/healthz`、`/readyz` 代理到 `http://127.0.0.1:8090`。
浏览器只访问 5173，因此不存在跨域问题（后端 CORS 也已放行 `localhost:5173`）。

### 3. 环境变量

复制 `.env.example` 为 `.env.local` 后按需修改：

```dotenv
VITE_API_BASE=/api/v1
VITE_USE_MOCK=false    # 默认值：真实后端
```

`VITE_USE_MOCK=true` 时改用 `src/api/mock` 的演示数据，可在没有后端的情况下逐屏验收设计稿。
**mock 的输出形状必须与 `internal/service/view.go` 的 DTO 逐字段一致**（包括 Go `omitempty`
会丢掉的键）。这是硬约束：早期版本的 mock 自己发明了 `company.tone`、`registered_at`、
`status_distribution` 等字段，导致「演示跑得通、一联调整页错位」。

### 初始化（首次启动）

前端与后端是联动的：**系统没有任何可用管理员时，前端会把所有入口强制拉到 `/setup`**，
不需要运维知道有这么一个页面。

```
GET  /api/v1/system/status          公开；initialized / setup_available / registration_open / mail
POST /api/v1/system/setup           公开但只在「无可用管理员」时可调用，否则 409
POST /api/v1/system/setup/mail/test 用向导表单里的参数试发测试邮件（不落库）
```

前端侧：

- `src/stores/system.js` 缓存 `/system/status`，路由守卫在**每次导航前**据此判断；
- `src/router/index.js`：`needsSetup`（无可用管理员）时一律跳 `/setup`；
  已初始化时 `/setup` 直接失效（未登录跳登录页，已登录跳「我的」）；
- `src/views/setup/SetupView.vue` 三步向导：管理员账号 → 邮件服务 → 确认。
  第 2 步可以直接「稍后到后台补配」，此时会明确提示注册与找回密码暂不可用。

三条值得知道的行为：

1. **状态查询失败时不强制跳转**。`/system/status` 拿不到就按「已初始化」处理，
   否则后端一挂，所有人都会被锁在一个永远提交不了的安装页上。
2. **测试邮件不落库**。它用的正是表单里尚未保存的参数——若先写库再试发，
   一个填错的授权码就已经热替换了投递器，系统立刻处于「验证码发不出去」的状态。
3. **向导关闭时不是死路**。若 `auth.setup.enabled=false`（例如由外部系统预置账号），
   页面会给出可执行的 `cmd/seed` 命令，而不是留一个永远提交不了的按钮。

无界面部署（容器 / CI）走命令行，两者互斥：

```bash
go run ./cmd/seed -admin-email you@example.com -admin-password 'YourPassw0rd'
```

`auth.setup.enabled` 与 `auth.bootstrap.enabled` 不能同时为 true（启动时会直接报配置错误）：
自举会先建号，系统随即变成「已初始化」，向导就永远用不上了。

> 已知缺口：安装向导只创建管理员与配置，**不写入 `roles` 表的内置角色元数据**
> （只有 `cmd/seed` 的 `seedRoles` 会写）。在新库上通过向导完成初始化后，
> 登录与后台功能均正常，但「角色权限」页的权限策略表有内容、角色卡片为空
> （`GET /admin/roles` 返回 `items: []`）。要么先跑一次 `cmd/seed`，
> 要么把角色元数据的播种从 `cmd/seed` 提到 bootstrap 的 `EnsureDefaults` 里。

### 演示账号

| 角色 | 账号 | 密码 |
|---|---|---|
| 管理员 | `admin@jobs-robot.local` | `Jobhub12345` |

后端在 `auth.expose_code_in_response=true`（联调模式）时会在发码接口回传 `debug_code`，
登录页/注册页的演示提示依赖它；生产环境该开关必须关闭。

## 目录

```
src/
├── api/            接口封装（http.js 统一信封与续期；mock/ 与真实 DTO 同形）
├── components/     通用组件（UI 原子 + 业务组件；settings/ 放配置面板）
├── composables/    useAsync / useAdminRefresh / useCountdown
├── layouts/        LayWebsite / LayAuth / LayWorktable —— 三套外壳
├── router/         routes + 全局守卫
├── stores/         Pinia
├── utils/format.js 时间、薪资、状态语义色，以及后端 DTO → 界面的唯一适配层
├── views/          页面（auth/ 与 admin/ 分子目录）
└── style.css       @theme 设计令牌 + 自定义工具类
```

## 路由设计

沿用既定模板的三条约定，`src/router/index.js` 里有逐条注释：

1. **按外壳分顶级分支**：前台 `LayWebsite`、认证 `LayAuth`、后台 `LayWorktable` 三个顶级记录，
   子路由以父级 `path: '/'` 为基准拼出绝对路径。换导航即换 layout，页面组件不受影响。
2. **meta 驱动行为**：`title` 由 `afterEach` 写入 `document.title`；
   `requiresAuth` / `guestOnly` 由 `beforeEach` 统一处理，页面里不写跳转逻辑。
3. **懒加载**：所有视图 `() => import()`。

`beforeEach` 会先 `await session.restore()` 再判断登录态 —— 少了这一步，刷新页面时
`user` 尚未拉回，已登录用户会被误判成游客并弹回登录页。

### 路径命名与冲突规避

模板里 `/products` 与前台 `/product` 的冲突，在本项目会重演（前台「职位」`/jobs`、
「公司库」`/companies` 与后台「岗位管理」「公司管理」同路径），因此后台改用不与前台重叠的名字：

| 前台 | 后台 |
|---|---|
| `/jobs`、`/job/:id` | `/postings`、`/postings/:id`（岗位管理） |
| `/companies`、`/company/:id` | `/sources`、`/sources/:id`（公司管理 = 来源官网） |

`reset-password` 刻意**不加** `guestOnly`：已登录用户点开邮件里的重置链接时也应该能完成重置。

## 后端 DTO → 界面的适配层

`src/utils/format.js` 是唯一适配点。后端只返回业务字段，**不返回**「用哪个颜色、取哪个字做头像」，
因此这些派生值全部在这里计算：

- `companyInitial()` / `userInitial()` —— 头像首字（`short_name` → 名称首字 → 邮箱首字母）
- `toneFor(seed)` —— 由 id/邮箱**稳定**推出的头像配色；刻意不含 accent，绿色是「官网直招」专用语义
- `jobStatus()` / `userStatus()` / `crawlStatus()` —— 状态码 → 中文标签 + 语义色
- `salaryText()` / `locationText()` / `sourceLabel()` / `relativeTime()` / `auditDetailText()`

页面里不再出现裸字段名做展示逻辑，新增派生需求请加在这里。

### 主键是不透明的 ULID 字符串

后端所有实体主键都是 ULID 字符串（26 位 Crockford base32，例如 `01M52SV84SZSWVD7RQDXQDCBGP`），
**不是自增整数**。因此：

- 路由参数、`:key`、接口路径里的 id 一律当作字符串处理，**禁止 `Number(id)` / `parseInt`**。
  字符串 id 做数值比较不会报错，只会静默地永不相等 —— 这是最难查的一类问题。
- 需要比较时统一用 `String(a) === String(b)`（`LogDetailView` 就是这么做的）。
- 「是不是我自己的账号」这类判断必须拿当前登录用户的 id 比，不能写 `id === 1`。
- 幂等脚本不要靠 id 大小排序或自增；ULID 本身按字典序可排序，需要顺序时用它。

演示数据（mock）同样生成 ULID 形状的 id，且**跨刷新稳定**，否则深链接会指向不存在的记录。

## 邮件（SMTP）服务配置

后台「系统配置 → 邮件服务」对应 `internal/service/mail.go`，与 `crawler.scope` 同为
**配置文件为基线、数据库覆盖、保存后热生效**的两层模型（键名 `mail.smtp`，密码用
`internal/pkg/secret` 加密存储）。

| 操作 | 接口 |
|---|---|
| 读取生效配置（密码脱敏） | `GET /api/v1/admin/mail` |
| 保存并热替换投递器 | `PUT /api/v1/admin/mail` |
| 回落到 `configs/config.yaml` | `DELETE /api/v1/admin/mail` |
| 发送测试邮件 | `POST /api/v1/admin/mail/test` |

界面上的三条硬约束：

1. **密码永不回显**：只显示 `password_mask`（`********`）与 `has_password`；原样回传或留空
   都表示「不修改」，另有 `clear_password` 显式清空。
2. **校验规则与服务端逐条对齐**（`config.Mail.Validate`）：`console/smtp` 二选一、
   smtp 主机必填、端口 1~65535、发件人邮箱必填且含 `@`、用户名与密码必须同时填写或同时留空。
   前端校验只是省一次往返，服务端仍是权威。
3. **测试邮件用的是已保存的配置**，不是表单里未保存的改动 —— 因此按钮旁边明确写出了这一点，
   否则管理员会误以为「改了没生效」。

`configs/config.yaml` 的 `mail.driver` 默认是 `console`（只把邮件内容打进日志，不真实发信），
上线前请在后台切到 `smtp`。

## 系统配置的页签划分

三个页签对应后端三种不同的配置模型，划分本身就是信息：

- **邮件服务** —— 影响下一次请求（验证码、通知邮件）；
- **收集范围**（`crawler.scope`）—— 影响下一次**抓取**，白名单语义，留空表示不限该维度；
- **高级配置** —— `settings` 表的原始读写入口，用于维护前两者之外的键。
  配置值必须是 JSON 对象（后端 `SettingView.Value` 是 `map[string]any`），界面会先本地解析再提交。

把生效时机不同的配置放在同一屏连续滚动（照搬设计稿的做法）会让人分不清「我刚改的这条什么时候生效」，
因此拆成页签，并在右上角统一说明生效时机。

## 与设计稿的差异（都是刻意的）

1. **筛选改为服务端真正支持的维度**。设计稿把「职位类型 / 薪资范围 / 经验要求」画成多选复选框，
   但公开接口对 `category`/`employment_type`/`experience` 只接受单值。工作地点 / 职位类型 /
   行业 / 用工性质 四组由 `GET /meta/dictionaries` 驱动；`experience` 与薪资区间后端**没有字典**，
   是前端便利项（接口分别是自由文本包含匹配与数值区间相交），界面上有注释说明。
2. **筛选状态放进地址栏**。`JobExplorer` 以 `route.query` 为唯一事实来源，
   筛选结果可直接分享、刷新不丢、后退可回到上一次筛选。
3. **近 7 日「抓取趋势」改为「新增岗位」并标注口径**。后端没有聚合接口，
   该图由 `GET /admin/crawl-runs` 在客户端按天分桶 `jobs_created` 得出，
   图上写明了指标、样本量与「超过 100 次运行不覆盖更早数据」。
4. **统计卡不再显示环比**。`StatsView` 只是快照，没有历史值；改为显示同一份数据里的另一个真实数字。
5. **认证页是独立外壳**。登录类页面不带站点导航与页脚，硬塞进前台外壳会让外壳充满条件判断。
6. **补齐了设计稿未出稿的页面**，以保证导航与侧边栏没有死链。
7. **修正设计稿的渲染错误**：设计稿把 `PostgreSQL` 渲染成 `PostGeeSQL`；
   详情页重复出现两次的「3天前发布」只保留一次。

## 验证

```bash
npm run build          # 产物构建
npm run dev            # 逐屏核对设计稿（需后端与依赖容器在跑）
```

已做的端到端核对（真实后端 + 无头浏览器）：

- 全部 27 条路由渲染，控制台零报错；详情页用真实 id 复核（`/job/37`、`/sources/34` 等）。
- 认证守卫：未登录访问 `/account` → `/login?redirect=/account`；登出后 `/dashboard` 再次被拦。
- 审核：审核通过后队列行数 9 → 8 且弹出成功提示。
- 岗位筛选：切到「待审核」后 `?status=pending_review`，行数 10 → 6，与页签计数一致。
- 邮件配置：控制台驱动下发测试邮件成功；切到 SMTP 保存后接口确认
  `driver=smtp / host / port=465 / tls / has_password=true / password_mask=******** / source=database /
  active_driver=smtp`，且响应体中**不含明文密码**；随后「回落到配置文件」回到 `source=config`。
