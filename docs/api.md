# API 文档 · 直招好工作

- **服务地址**：`http://127.0.0.1:8090`
- **接口前缀**：`/api/v1`（可用 `app.api_prefix` 调整）
- **编码**：请求与响应均为 `application/json; charset=utf-8`
- **时间格式**：RFC 3339，例如 `2025-01-08T00:00:00+08:00`

---

## 1. 通用约定

### 1.1 统一响应信封

所有接口（含错误）都返回同一结构，前端可只写一套处理逻辑：

```json
{
  "code": 0,
  "message": "ok",
  "data": { },
  "details": null,
  "request_id": "0f5b1e3a-..."
}
```

- `code`：业务错误码，`0` 表示成功
- `message`：面向用户的可读提示
- `details`：可选的结构化补充信息（例如字段级校验失败原因、剩余尝试次数）
- `request_id`：链路 ID，与响应头 `X-Request-Id` 一致，报障时请提供

### 1.2 分页

请求参数：`page`（默认 1）、`page_size`（默认 20，上限 100）。

```json
{
  "items": [],
  "total": 137,
  "page": 1,
  "page_size": 20,
  "pages": 7
}
```

### 1.3 鉴权

除公开接口外，均需在请求头携带访问令牌：

```
Authorization: Bearer <access_token>
```

- 访问令牌有效期默认 2 小时（`jwt.access_ttl`）
- 刷新令牌有效期默认 7 天（`jwt.refresh_ttl`），刷新时**一次性轮换**
- 登出、改密、重置密码、角色变更、账号禁用都会立即使相关令牌失效

### 1.4 首次安装：安装向导（默认）与启动自举（无界面部署）

配置 SMTP 必须先登录后台，登录又必须先有账号。为避免全新部署卡在
「没账号 → 进不了后台 → 配不了邮件 → 注册走不通」的死循环里，系统提供**两条互斥的路径**：

| 路径 | 适用场景 | 谁提供密码 | 开关 | 默认 |
|---|---|---|---|---|
| **安装向导**（前端 `/setup`） | 有界面的部署 | **运维自己设**，并现场配置 + 验证 SMTP | `auth.setup.enabled` | **开启** |
| **启动自举**（服务端建号） | 无界面场景：纯容器、CI、只跑 API 的镜像 | 系统随机生成，打印在启动日志并写凭据文件 | `auth.bootstrap.enabled` | **关闭** |

> **默认不再启动自动建号**：全新部署第一次打开前端会被路由守卫强制送到 `/setup`，
> 由运维在一个页面里完成「建管理员 → 填真实 SMTP → 发送测试邮件 → 进入后台」。
>
> `auth.setup.enabled` 与 `auth.bootstrap.enabled` **不能同时为 true**：
> 启动自举会先建号，系统立刻变成「已初始化」，安装向导因此永久失效（`409`）。
> 同时开启会在**启动时**直接报配置错误，而不是让运维对着一个必然 409 的向导页困惑。
> 若两者都关（`false`/`false`），表示账号由外部系统预置。

**安装向导相关配置**：

| 配置项 | 默认值 | 说明 |
|---|---|---|
| `auth.setup.enabled` | `true` | 是否开放安装向导接口与 `setup_available` |
| `auth.setup.test_mail_to` | 空 | 测试邮件的默认收件人；留空表示由请求体指定 |

环境变量前缀：`JOBSROBOT_AUTH_SETUP_*`。接口说明见 [4.7](#47-首次安装向导)。

**启动自举（`auth.bootstrap.*`）配置**——仅在显式开启时生效：

| 配置项 | 默认值 | 说明 |
|---|---|---|
| `auth.bootstrap.enabled` | `false` | 是否启用启动自举 |
| `auth.bootstrap.email` | `admin@jobs-robot.local` | 初始管理员邮箱（小写存储） |
| `auth.bootstrap.password` | 空 | 留空 → 自动生成 16 位随机强密码 |
| `auth.bootstrap.nickname` | `初始管理员` | 昵称 |
| `auth.bootstrap.credential_file` | `var/bootstrap-admin.txt` | 凭据文件（权限 `0600`），留空表示不写文件 |

环境变量前缀：`JOBSROBOT_AUTH_BOOTSTRAP_*`。

**密码来源与是否打印**：

| 路径 | 启动日志中的 `password` 字段 | 凭据文件内容 |
|---|---|---|
| 新建账号，`auth.bootstrap.password` 为空（随机生成） | 明文打印一次 | 写入明文 |
| 新建账号，`auth.bootstrap.password` 非空（配置/环境变量） | `(来自配置，未打印)` | 写入明文 |
| 提升既有账号（邮箱已被占用） | `(既有账号，密码未修改)` | **不写文件**（密码未变，写了也登不上） |

即：**运维自己设置的口令不会进入日志**，只有系统随机生成的初始口令会打印一次；
提升既有账号时既不改密码也不写凭据文件，请使用该账号原有密码登录。

创建成功时使用 **WARN 级** 打印，字段包含 `email`、`password`、`user_id`、
`credential_file`、`must_change_password=true`、`created`：

```text
WARN 初始管理员账号已创建，请登录后台配置邮件（SMTP）服务，并立即修改密码
     email=admin@jobs-robot.local password=Xk7... user_id=01JD7Q9K2M3N4P5R6S7T8V9W0X
     credential_file=var/bootstrap-admin.txt must_change_password=true created=true
```

已存在可用管理员时只在 INFO 级打印一行并跳过：

```text
INFO 已存在可用管理员，跳过初始账号自举 admin_id=01JD... available_admins=1
```

**幂等与既有数据安全**：

- 只要存在 1 名「可登录的管理员」（Casbin 中直接拥有 `admin` 角色，且用户行存在、状态非 `disabled`）就跳过，
  **绝不修改任何既有账号**（不动密码、不动状态、不动角色）；
- 若配置的邮箱已被占用但不是管理员（历史降级、管理员被清空等），走**「提升既有账号」**路径：
  补 `admin` 角色 + 置 `must_change_password=true` + 放开为 `active`，**不重置密码**。
  这样不会撞 `users.email` 唯一约束，也不会把自动自举变成一次意外的凭据变更；
- 该字段是纯新增列，ent 自动迁移以 `ALTER TABLE ADD COLUMN` 补齐（默认 `false`），不重建表、不影响既有行。

创建出的账号带有 `must_change_password=true`，登录后应在 UI 提示「请修改初始密码」；
用户成功改密（或管理员重置其密码）后该标记自动清零。

**运维建议顺序（自举路径）**：登录 → 配置 SMTP 并测试发信 → 修改密码 → 删除 `var/bootstrap-admin.txt`。

> 走安装向导时**不会写任何凭据文件**：密码是运维自己在页面上设的，
> 把它落盘只会多一处明文泄漏面。

### 1.5 限流

公开的认证类接口按客户端 IP 限流，超限返回 `429`（`code=10006`）并带
`Retry-After`、`X-RateLimit-Limit`、`X-RateLimit-Remaining`、`X-RateLimit-Reset`：

| 接口 | 默认限额 | 配置项 |
|---|---|---|
| `POST /auth/codes` | 10 次/小时 | `auth.rate_limit.send_code_*` |
| `POST /auth/register` | 20 次/小时 | `auth.rate_limit.register_*` |
| `POST /auth/login` | 30 次/10 分钟 | `auth.rate_limit.login_*` |
| `POST /auth/password/forgot` | 10 次/小时 | `auth.rate_limit.forgot_*` |
| `POST /auth/password/reset` | 20 次/小时 | `auth.rate_limit.reset_*` |
| `POST /system/setup` | 10 次/小时 | `auth.rate_limit.setup_*` |
| `POST /system/setup/mail/test` | 10 次/小时 | `auth.rate_limit.setup_*` |

阈值可通过配置或环境变量（`JOBSROBOT_AUTH_RL_*`）调整；置 `0` 或
`auth.rate_limit.enabled=false` 表示不限流——本地联调与自动化测试常需要放宽，
这也避免测试与手工使用争抢同一 IP 额度。

另有基于 Redis 的**验证码发送冷却**（同一邮箱同一用途 60 秒内只能发一次，`code=20006`），
以及同一邮箱的**每日发送上限**（默认 20 次，`code=20007`）。

限流组件在 Redis 故障时**放行（fail-open）**并记录告警：限流不可用不应导致登录注册整体不可用。

### 1.6 标识（ID）约定：全部是 ULID 字符串

**系统内所有实体标识都是 ULID 字符串，不再是数字自增 id**，这一点适用于所有请求体、查询参数、
路径参数与响应字段（如 `id`、`company_id`、`reviewer_id`、`actor_id`、`run_id`、`job_id`）。

| 特性 | 说明 |
|---|---|
| 形态 | 26 位 Crockford Base32，大写字母与数字，不含 `I`/`L`/`O`/`U`，例如 `01JD7Q9K2M3N4P5R6S7T8V9W0X` |
| 存储 | `VARCHAR(26)`，字符串比较即可，字典序即时间序 |
| 排序 | 前 48 位为毫秒时间戳，**同一毫秒内生成的 ID 严格单调递增**，因此按 `id` 倒序分页不会重复或漏项 |
| 生成 | 服务端创建记录时自动生成，客户端**不要也不允许**指定 `id`；历史数据导入可用 `SetID` 显式保留原标识 |
| 校验 | 路径参数必须是合法 ULID，否则返回 `400`（`code=10001`），提示形如「路径参数 id 必须是合法的 ULID」 |

> 之所以不用数字自增 id：自增 id 会把业务规模（「你是第几个用户/第几条岗位」）写进 URL，
> 既泄露增长数据，也让遍历接口变得容易；ULID 同时具备「不暴露规模」与「可按时间排序」两个特性。

查询参数中的标识（`company_id`、`actor_id`、`source_url` 等）为可选过滤器：缺省或空串表示不过滤。

---

## 2. 认证接口

### 2.1 发送邮箱验证码

`POST /api/v1/auth/codes`

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `email` | string | 是 | 接收邮箱 |
| `purpose` | string | 是 | `register` / `reset_password` / `change_email` / `login` |

```json
{ "email": "user@example.com", "purpose": "register" }
```

响应 `data`：

```json
{
  "email": "user@example.com",
  "purpose": "register",
  "expires_in": 600,
  "message": "验证码已发送，请查收邮件",
  "debug_code": "135790"
}
```

> `debug_code` 仅在 `auth.expose_code_in_response=true`（联调模式）时返回，生产环境必须关闭。

**隐私说明**：`purpose=reset_password` 时，无论邮箱是否已注册都返回相同话术；
未注册邮箱不会真正发信，也不会返回 `debug_code`，避免被用于枚举平台用户。

### 2.2 注册

`POST /api/v1/auth/register`

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `email` | string | 是 | 登录邮箱（统一小写存储） |
| `password` | string | 是 | 至少 8 位，需同时包含字母与数字，最长 72 字节 |
| `nickname` | string | 否 | 留空时取邮箱前缀 |
| `code` | string | 条件 | `auth.register_need_verify=true` 时必填 |

响应 `201`，`data` 为 `{ user, token }`；新用户自动获得 `user` 角色。

### 2.3 登录

`POST /api/v1/auth/login`

```json
{ "email": "user@example.com", "password": "Passw0rd123" }
```

响应 `data`：

```json
{
  "user": { "id": "01JD7Q9K2M3N4P5R6S7T8V9W0X", "email": "user@example.com", "roles": ["user"], "status": "active", "...": "..." },
  "token": {
    "access_token": "eyJhbGciOi...",
    "refresh_token": "kZ8m...",
    "token_type": "Bearer",
    "expires_in": 7200,
    "expires_at": "2025-01-08T12:00:00+08:00",
    "refresh_expires_at": "2025-01-15T10:00:00+08:00",
    "session_id": "3f9c..."
  }
}
```

失败：`20002` 邮箱或密码错误、`20003` 账号已禁用、`20010` 账号被临时锁定、`20004` 邮箱未验证。

> 账号不存在时同样执行一次 bcrypt 校验，避免通过响应时间枚举已注册邮箱。

### 2.4 刷新令牌

`POST /api/v1/auth/refresh`

```json
{ "refresh_token": "kZ8m..." }
```

响应为新的 `token` 对象（**旧刷新令牌立即作废**）。刷新时会重新解析用户角色，因此角色调整会随之生效。

### 2.5 登出

`POST /api/v1/auth/logout`

```json
{ "refresh_token": "kZ8m..." }
```

登出后该会话的访问令牌与刷新令牌同时失效。

### 2.6 找回密码

`POST /api/v1/auth/password/forgot` → 同 2.1（`purpose=reset_password`）

`POST /api/v1/auth/password/reset`

```json
{ "email": "user@example.com", "code": "135790", "new_password": "NewPassw0rd" }
```

重置成功后会吊销该用户的**全部**会话，并发送通知邮件。

---

## 3. 用户自助接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/me` | 当前用户信息（含角色） |
| PATCH | `/me` | 更新昵称 / 头像 / 手机号 |
| POST | `/me/password` | 修改密码（需原密码） |
| GET | `/me/sessions` | 列出活跃会话（设备/IP/时间） |
| DELETE | `/me/sessions/:sid` | 下线指定设备 |

`POST /me/password` 请求 `{ "old_password": "...", "new_password": "..." }`。

响应会**换发当前设备的新令牌**：

```json
{ "message": "密码修改成功，其他设备已退出登录", "token": { "access_token": "...", "refresh_token": "..." } }
```

> 为什么必须换发：改密会递增密码版本，使所有既有访问令牌立即失效（否则改密无法踢掉已泄漏的令牌）。
> 客户端收到响应后应替换本地令牌，这样当前设备无需重新登录。

### 3.1 `UserView` 契约

登录、注册、`GET /me`、`PATCH /me` 以及管理端用户列表/详情返回的用户对象结构一致：

```json
{
  "id": "01JD7Q9K2M3N4P5R6S7T8V9W0X",
  "email": "admin@jobs-robot.local",
  "nickname": "初始管理员",
  "avatar_url": "",
  "phone": null,
  "status": "active",
  "email_verified": true,
  "roles": ["admin"],
  "last_login_at": "2025-01-08T10:12:31+08:00",
  "remark": "",
  "must_change_password": true,
  "created_at": "2025-01-08T09:00:00+08:00",
  "updated_at": "2025-01-08T10:12:31+08:00"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `must_change_password` | bool | 是否**必须修改密码**。为 `true` 时前端应在登录后展示「请修改初始密码」提示条 |

`must_change_password=true` 只会在两种情况下出现：首次启动自举创建的初始管理员、
或被自举「提升为管理员」的既有账号（`auth.bootstrap.*`，见 1.4）。
用户成功调用 `POST /me/password` 改密后，该标记会被清零（管理员通过
`POST /admin/users/:id/reset-password` 重置密码时同样清零）。

---

## 4. 公开接口（无需登录）

### 4.1 岗位列表

`GET /api/v1/jobs`

| 参数 | 说明 |
|---|---|
| `keyword` | 匹配岗位名称 / 简介 / 任职要求 / 来源公司名 |
| `category` | 职位类别（工作类型） |
| `industry` | 行业 |
| `employment_type` | 用工性质 |
| `experience` | 经验要求（包含匹配，兼容「3-5年」「3年以上」等写法） |
| `city` | 城市（包含匹配，兼容「北京」「北京市」） |
| `company_id` | 指定公司 |
| `source_url` | 精确匹配来源地址，用于确认「这个官网链接是否已采集过」 |
| `is_direct_hire` | 是否仅看官网直招 |
| `min_quality_score` | 最低信息完整度评分 |
| `salary_min` / `salary_max` | 月薪范围（单位：元），语义为**区间相交**：岗位薪资区间与查询区间有交集即命中 |
| `order_by` | `last_updated_at`（默认）/ `published_at` / `quality_score` / `created_at` |
| `page` / `page_size` | 分页 |

返回 `JobListView` 列表（含公司简要信息、薪资、城市、发布时间、最后更新时间、来源地址）。

### 4.2 岗位详情

`GET /api/v1/jobs/:id`

返回 `JobDetailView`，包含用户关心的全部字段：

```json
{
  "id": "01JD7Q9K2M3N4P5R6S7T8V9W0X",
  "title": "后端开发工程师（Go）",
  "company": { "id": "01JD7Q9K2M3N4P5R6S7T8V9W0Y", "name": "示例科技有限公司", "verified": false },
  "category": "技术",
  "industry": "智能制造",
  "employment_type": "全职",
  "city": "北京",
  "district": "朝阳区",
  "address": "北京市朝阳区望京SOHO T1 座 20 层",
  "salary_text": "",
  "salary_min": 30000,
  "salary_max": 50000,
  "experience": "3-5 年",
  "education": "本科及以上",
  "headcount": 2,
  "summary": "",
  "description": "负责公司工业互联网平台核心服务的设计与开发。",
  "responsibilities": "负责核心交易与设备接入服务的架构设计与编码实现；…",
  "requirements": "本科及以上学历，计算机相关专业，3 年以上 Go 开发经验；…",
  "requirements_list": ["本科及以上学历…", "熟悉 PostgreSQL、Redis…", "熟悉 Kubernetes…"],
  "benefits": "五险一金、补充医疗、弹性工作…",
  "contact_person": "",
  "contact_email": "hr@example-tech.com",
  "contact_phone": "010-88886666",
  "apply_url": "",
  "published_at": "2025-01-08T00:00:00+08:00",
  "last_updated_at": "2025-01-08T10:12:31+08:00",
  "last_seen_at": "2025-01-08T10:12:31+08:00",
  "source_url": "http://…/jobs/backend.html",
  "source_site": "127.0.0.1:8899",
  "is_direct_hire": true,
  "quality_score": 95,
  "status": "approved",
  "view_count": 1
}
```

> 仅返回 `status=approved` 的岗位；未发布岗位一律返回 `404`（不泄露存在性）。

### 4.3 相关职位

`GET /api/v1/jobs/:id/related?limit=6`

返回同公司或同职位类别的其它已发布岗位（按信息完整度排序），用于详情页底部的「相关职位」推荐。

```json
{ "items": [ { "id": "01JD7Q9K2M3N4P5R6S7T8V9W0Z", "title": "…", "company": { }, "salary_text": "…" } ] }
```

### 4.4 公司列表 / 详情

- `GET /api/v1/companies` —— 仅返回启用中的公司简要信息
- `GET /api/v1/companies/:id` —— 公司简要信息（不暴露抓取配置等内部字段）

### 4.5 字典

`GET /api/v1/meta/dictionaries` —— 工作类型、行业、用工性质、地点、岗位状态、用户状态、角色、抓取状态等枚举，供前端渲染筛选项。

### 4.6 系统初始化状态

`GET /api/v1/system/status` —— **公开接口，无需登录**。前端启动时调用，用于决定是否展示「初始化向导」。

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "initialized": false,
    "user_count": 0,
    "admin_count": 0,
    "setup_available": true,
    "registration_open": false,
    "mail": {
      "driver": "smtp",
      "source": "config",
      "ready": false,
      "smtp_configured": false
    },
    "bootstrap_credential_file_exists": false
  }
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `initialized` | bool | 是否存在**可用管理员**（直接拥有 `admin` 角色，且账号存在、未被禁用） |
| `user_count` | int | 用户总数 |
| `admin_count` | int | 可用管理员数量（与 `initialized` 同源，`admin_count > 0` 即 `initialized=true`） |
| `setup_available` | bool | **安装向导当前是否可用**：`auth.setup.enabled && !initialized` |
| `registration_open` | bool | 注册功能当前是否**真的可用** |
| `mail.driver` | string | 当前生效的投递器：`console` / `smtp` |
| `mail.source` | string | 配置来源：`config`（配置文件/环境变量）/ `database`（后台在线设置） |
| `mail.ready` | bool | 当前投递器是否可用 |
| `mail.smtp_configured` | bool | 是否已配置可用的 SMTP（`driver=smtp` 且 `host` 与 `from_address` 均非空） |
| `bootstrap_credential_file_exists` | bool | 初始凭据文件是否仍然存在（存在则提示运维登录后删除） |

**语义澄清（前端提示文案的依据）**：

```
registration_open = !auth.register_need_verify || mail.smtp_configured
setup_available   = auth.setup.enabled && !initialized
```

也就是说：**要求邮箱验证（`auth.register_need_verify=true`）但没配好 SMTP 时，注册实际走不通**
——验证码根本发不出去，用户永远拿不到码。此时 `registration_open=false`，前端应提示
「请管理员先在后台『系统设置 - 邮件设置』中配置 SMTP 并发送测试邮件」。

`setup_available=true` 时前端应**强制跳转** `/setup`（除 `/setup` 本身外的任何路由）；
`setup_available=false` 而 `initialized=false`，说明向导被配置关闭，
此时应给出「请用 `cmd/seed` 或打开 `auth.bootstrap.enabled`」的可执行说明，
而不是把用户送去一个永远登不进去的登录页。

`bootstrap_credential_file_exists=true` 时提示运维：登录 → 配 SMTP → 改密码 → 删除该文件。

本接口只暴露「系统是否就绪」这类信息，**不含任何用户邮箱、口令或业务数据**，因此可以公开访问。
邮件配置损坏时不会返回错误，而是如实报告 `mail.ready=false`，保证初始化向导本身不会因 500 白屏。

### 4.7 首次安装向导

这两个接口与 `/system/status` 一样是**公开**的——它们要回答的正是
「系统里还没有任何账号可登录」这一场景，此时不可能存在有效令牌。

公开不等于无保护，服务端做**三重门控**：

1. `auth.setup.enabled` 为真（配置层开关，默认开）；
2. 系统当前**不存在可用管理员**（与启动自举共用同一套判定：直接拥有 `admin` 角色 + 账号存在 + 未被禁用）；
3. 不满足则返回 **`409`**（`code=10005`）+ 可读原因，例如
   `无法使用安装向导：系统已完成初始化（已存在 1 名可用管理员），安装向导永久失效；如需新增管理员请登录后台操作`。

**一旦存在管理员即永久失效**：判定与创建之间用事务 + `pg_advisory_xact_lock`
（固定 key）串行化，避免两个并发请求各建一个管理员。
两个接口都按客户端 IP 限流（默认 10 次/小时，`auth.rate_limit.setup_*`）。

因为都在公开分组，**不需要**在 Casbin 策略里为它们添加任何规则（公开接口不走授权中间件）。

#### `POST /api/v1/system/setup`

一次性完成初始化：创建管理员，可选地写入并**热生效**邮件配置。

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `admin_email` | string | 是 | 管理员邮箱（小写归一化后存储）；必须是邮箱格式 |
| `admin_password` | string | 是 | 至少 8 位且同时含字母与数字（与注册/改密同一套 `auth.ValidatePasswordStrength`） |
| `nickname` | string | 否 | 留空则取邮箱前缀 |
| `mail` | object | 否 | 省略或 `null` 表示「先跳过邮件服务」，此时只建管理员 |
| `mail.driver` | string | 否 | `smtp` / `console` |
| `mail.host` / `mail.port` | string / int | smtp 必填 | SMTP 主机与端口（1~65535） |
| `mail.username` / `mail.password` | string | 否 | 要么都填、要么都空；密码/授权码首尾空白会被自动去除（不报错） |
| `mail.tls` | bool | 否 | **`true` = 隐式 TLS（465）**；`false` = 明文连接后自动尝试 STARTTLS（587/25） |
| `mail.from_name` / `mail.from_address` | string | `from_address` 必填 | 发件人显示名与地址 |

```json
{
  "admin_email": "ops@example.com",
  "admin_password": "Setup2026pass",
  "nickname": "运维管理员",
  "mail": {
    "driver": "smtp",
    "host": "smtp.exmail.qq.com",
    "port": 465,
    "username": "ops@example.com",
    "password": "授权码",
    "tls": true,
    "from_name": "直招好工作",
    "from_address": "ops@example.com"
  }
}
```

响应 `201` + `data`：

```json
{
  "admin": {
    "id": "01JD7Q9K2M3N4P5R6S7T8V9W0X",
    "email": "ops@example.com",
    "nickname": "运维管理员",
    "status": "active",
    "email_verified": true,
    "roles": ["admin"],
    "must_change_password": false,
    "created_at": "2026-01-08T10:12:31+08:00",
    "updated_at": "2026-01-08T10:12:31+08:00"
  },
  "mail": { "driver": "smtp", "source": "database", "ready": true, "smtp_configured": true },
  "registration_open": true,
  "mail_configured": true
}
```

创建出的账号：`status=active`、`email_verified=true`、`register_source="setup"`、
**`must_change_password=false`**（密码是运维自己设的，不必再逼他改一遍），并授予 `admin` 角色。
审计写两条：`user.setup_admin`（必要时 `mail.update`），detail **不含任何密码**，
也**不写任何凭据文件**。

`mail` 省略时 `registration_open` 仍为 `false`，前端应提示
「未配置邮件服务，用户注册与找回密码暂不可用，可稍后在后台补配」。

可能的状态码：

| 状态 | code | 场景 |
|---|---|---|
| `201` | 0 | 创建成功 |
| `400` | 10001 / 20011 / 50002 | 邮箱格式、密码强度、邮件配置不合法 |
| `409` | 10005 | 向导已关闭 / 系统已完成初始化 / 邮箱已被既有账号占用 |
| `429` | 10006 | 触发限流 |
| `500` | 10007 | 管理员已创建但邮件配置保存失败（响应 message 会说明「请登录后台重新配置」） |

#### `POST /api/v1/system/setup/mail/test`

用**表单里正在填的**参数试发一封测试邮件，**只发信、不持久化、不热替换投递器**。

这样才能做到「先验证、后保存」：若先写库再试发，一个填错的 host/授权码
就已经替换了生效中的投递器，系统立刻处于「验证码发不出去」的状态——
而这正是安装向导要避免的。因为走的是同一个 `mail.New` + 同一份 SMTP 实现，
「测试通过」对「保存后可用」是有意义的预测。

门控与 `/system/setup` 完全相同（见上）。

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `mail` | object | 是 | 一份完整邮件配置（字段同 `/system/setup` 的 `mail`） |
| `to` | string | 否 | 收件人；留空时回落到 `auth.setup.test_mail_to` |

```json
{
  "mail": { "driver": "smtp", "host": "smtp.exmail.qq.com", "port": 465,
            "username": "ops@example.com", "password": "授权码", "tls": true,
            "from_address": "ops@example.com" },
  "to": "ops@example.com"
}
```

成功：

```json
{ "code": 0, "message": "ok", "data": { "message": "测试邮件已发送，请查收（含垃圾箱）" } }
```

失败返回 **`503`**，`message` 直接展示给运维以便判断该改哪一项。
**认证失败**用专用码 `10009`，`message` 内含排查步骤、服务商专属提示与**服务器原文**：

```json
{
  "code": 10009,
  "message": "SMTP 认证失败（服务器返回 535）：请按以下顺序检查\n1) 密码是否是邮箱服务商生成的「授权码」——QQ/163/腾讯企业邮都必须用授权码，不能用登录密码；\n2) 该账号是否已在邮箱网页端开启 SMTP/POP3 服务；\n3) 账号是否填写为完整邮箱地址（如 xxx@qq.com），不要只填前缀；\n4) 服务器与端口是否匹配（个人 QQ 邮箱 smtp.qq.com:465，腾讯企业邮 smtp.exmail.qq.com:465）。\n5) 你填的是 QQ 邮箱（smtp.qq.com）：请到 QQ 邮箱网页端「设置 → 账户 → POP3/IMAP/SMTP 服务」开启服务，按提示用手机发送短信后获取 16 位「授权码」——登录密码一定不行，且授权码只在生成时显示一次。\n服务器原文：535 \"Login fail. Account is abnormal, service is not open, password is incorrect, login frequency limited, or system is busy. More information at https://help.mail.qq.com/detail/108/1023\"",
  "request_id": "..."
}
```

**连接/加密方式错配**（例如 465 端口选了 STARTTLS：服务器等不到 TLS 客户端问候就把连接关掉，
客户端只读到 `EOF`）沿用 `10008`，但会回显**当前提交的 host/port/加密方式**：

```json
{
  "code": 10008,
  "message": "无法建立 SMTP 连接（EOF）：端口与加密方式很可能不匹配\n1) 465 端口必须选「SSL/TLS 直连」（隐式 TLS）；\n2) 587 端口必须选「STARTTLS」（不要选 SSL/TLS 直连）；\n3) 25 端口通常被云厂商与运营商封禁，建议改用 465 或 587。\n当前提交：smtp.qq.com:465，加密方式=STARTTLS 或不加密（tls=false）—— 该组合下服务器会因收到明文而立即断开，请在加密方式里改选「SSL/TLS 直连」。\n服务器原文：EOF",
  "request_id": "..."
}
```

同样返回 `10008` 的还有 DNS 解析失败（`无法解析 SMTP 主机名 <host>`）、端口拒绝
（`SMTP 服务器拒绝连接`）与超时（`连接 SMTP 服务器超时`），三者都会带上「当前提交：host:port，加密方式=…」。

> 契约说明：**认证失败 = `503` + 专用码 `10009`**，其余发信失败仍是 `503` + `10008`
> （HTTP 状态没变，因此按 `503` 展示 `message` 的前端无需改动）。
> 所有 message 都**不再包含「服务器内部错误」**：那是对服务端的诊断，
> 甩给运维只会把「配置问题」误导成「程序坏了」。
> `message` 中永不出现 SMTP 密码/授权码（服务器若回显了 AUTH 凭据，也会被替换为 `****`）。

> 密码/授权码首尾的空白（复制粘贴产物）会被**自动去除**并在服务端记一条 WARN
> （`检测到 SMTP 密码/授权码首尾含空白字符，已自动去除`，日志不含口令），
> 不会因此拒绝保存或发信。

> `driver=console` 时不会真实发信，邮件内容只打印到服务端日志（适合本地联调）。

---

## 5. 管理端：岗位与审核

> 需 `admin` 或 `reviewer` 角色；权限由 Casbin 按接口路径 + HTTP 方法判定。

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/admin/jobs` | admin, reviewer | 岗位列表（**不传 `status` 即返回全部状态**，可传逗号分隔多值） |
| GET | `/admin/jobs/:id` | admin, reviewer | 岗位详情（含修订历史） |
| POST | `/admin/jobs` | admin | 手工新增岗位 |
| PATCH | `/admin/jobs/:id` | admin | 人工编辑岗位 |
| DELETE | `/admin/jobs/:id` | admin | 删除岗位 |
| POST | `/admin/jobs/:id/review` | admin, reviewer | 审核（通过 / 驳回） |
| POST | `/admin/jobs/:id/status` | admin | 切换状态（上线 / 下线 / 标记过期） |
| GET | `/admin/jobs/:id/revisions` | admin, reviewer | 内容变更历史 |
| GET | `/admin/stats` | admin, reviewer | 概览统计 |

### 5.1 审核

`POST /api/v1/admin/jobs/:id/review`

```json
{ "decision": "approve" }
{ "decision": "reject", "reject_reason": "岗位信息与官网不一致" }
```

- `pending_review` 与 `rejected` 状态可审核（允许改判）
- 已发布（`approved`）岗位不可重复审核，需先下线：返回 `30004`
- 驳回必须填写原因

### 5.2 人工编辑与「退回待审核」

`PATCH /api/v1/admin/jobs/:id`（字段均为可选，传什么改什么）

可编辑字段：`title`、`category`、`industry`、`employment_type`、`department`、`headcount`、
`experience`、`education`、`work_mode`、`salary_text`、`salary_min/max/months`、
`country`、`province`、`city`、`district`、`address`、`location_text`、`summary`、`description`、
`responsibilities`、`requirements`、`requirements_list`、`benefits`、`tags`、
`contact_person`、`contact_email`、`contact_phone`、`contact_wechat`、`apply_url`、`apply_method`、
`published_at`、`deadline_at`。

行为：
1. 与修改前快照比对，生成一条 `trigger=manual` 的修订记录（含变更字段列表与快照）
2. `revision` 自增，`content_hash` 重算，`last_updated_at` 刷新
3. **若变更属于实质性变更**（标题、任职要求、岗位职责、薪资、用工性质、城市、地址）
   且岗位已发布，则自动退回 `pending_review` —— 与抓取路径口径一致，避免绕过审核改动已发布内容

---

## 6. 管理端：公司与抓取

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/admin/companies` | admin, reviewer | 公司列表（`keyword`/`industry`/`enabled`/`crawl_enabled`/`verified`/`order_by`） |
| POST | `/admin/companies` | admin | 新增公司 |
| GET | `/admin/companies/:id` | admin, reviewer | 公司详情 |
| PATCH | `/admin/companies/:id` | admin | 更新公司 |
| DELETE | `/admin/companies/:id` | admin | 删除公司（有岗位时拒绝，需先清理岗位） |
| POST | `/admin/companies/:id/enabled` | admin | 启用/停用 |
| POST | `/admin/companies/:id/discover` | admin | **自动定位《加入我们》入口并回填** |
| POST | `/admin/companies/:id/crawl` | admin | 手动触发该公司抓取 |
| POST | `/admin/crawl/run-all` | admin | 手动触发全量抓取 |
| GET | `/admin/crawl-runs` | admin, reviewer | 抓取运行记录（`company_id`/`status`/`trigger`） |
| GET | `/admin/crawl-runs/:id` | admin, reviewer | 运行详情 + 逐条明细日志 |

### 6.1 新增公司

```json
{
  "name": "示例科技有限公司",
  "website": "https://www.example.com",
  "industry": "智能制造",
  "location": "北京",
  "careers_url": "",
  "crawl_enabled": true,
  "crawl_interval_seconds": 21600,
  "crawl_rules": {},
  "force_headless": false,
  "respect_robots": true
}
```

`crawl_rules` 支持的键（对结构特殊的站点做定向覆盖，缺省时走通用启发式）：

```json
{
  "careers_url": "https://www.example.com/join-us",
  "careers_link_selector": "nav a.join",
  "careers_link_texts": ["加入我们", "Join Us"],
  "list_selector": ".job-list",
  "item_selector": ".job-item",
  "link_selector": "a.job-link",
  "detail_fields": { "title": "h1", "requirements": ".requirements" },
  "force_headless": true,
  "max_pages": 100
}
```

> 未知键会被保留但记录警告日志；已知键类型不符时返回 `10001` 并在 `details` 指明具体键。

### 6.2 抓取运行记录

```json
{
  "id": "01JD7Q9K2M3N4P5R6S7T8V9W0X",
  "company_id": "01JD7Q9K2M3N4P5R6S7T8V9W0Y",
  "company_name": "示例科技有限公司",
  "trigger": "manual",
  "status": "success",
  "careers_url": "http://127.0.0.1:8899/join-us.html",
  "started_at": "2025-01-08T10:12:29+08:00",
  "finished_at": "2025-01-08T10:12:31+08:00",
  "duration_ms": 2140,
  "pages_fetched": 4,
  "jobs_found": 3,
  "jobs_created": 2,
  "jobs_updated": 0,
  "jobs_unchanged": 0,
  "jobs_filtered": 1,
  "jobs_expired": 0,
  "used_headless": false,
  "error_message": ""
}
```

口径说明（避免误读）：
- `jobs_found`：招聘页上**发现**的岗位链接数
- `jobs_created` / `jobs_updated` / `jobs_unchanged`：落库结果
- `jobs_filtered`：被「收集范围」过滤掉的岗位数（原因见明细日志 `stage=filter`）
- `status`：`running` / `success` / `partial` / `failed` / `skipped`（被 robots 或 noai 拒绝）

---

## 7. 管理端：用户与角色

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/admin/users` | 用户列表（`keyword`/`status`/`role`/`order_by`） |
| GET | `/admin/users/:id` | 用户详情 |
| POST | `/admin/users/:id/status` | 启用/禁用（`{"status":"active"}` / `disabled`） |
| POST | `/admin/users/:id/roles` | 设置角色（`{"roles":["reviewer"]}`） |
| POST | `/admin/users/:id/reset-password` | 重置密码（`{"new_password":"…"}`） |
| DELETE | `/admin/users/:id` | 删除用户 |
| GET | `/admin/roles` | 角色列表（含权限策略与用户数） |

安全约束（违反返回 `400` / `10001`）：
- 不能禁用自己的账号、不能删除自己的账号
- 不能移除系统中的**最后一名可用管理员**
- 角色变更、禁用账号、重置密码都会立即使目标用户的令牌失效

> 用户对象结构见「3.1 `UserView` 契约」，其中 `must_change_password` 可用于在用户列表里
> 标记「仍在使用初始密码」的账号（通常就是初始管理员）。

---

## 8. 管理端：系统配置与收集范围

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/admin/settings` | 配置列表（可按 `group` 过滤） |
| GET | `/admin/settings/:key` | 读取配置 |
| PUT | `/admin/settings/:key` | 新建/覆盖配置 |
| DELETE | `/admin/settings/:key` | 删除配置（系统内置项不可删） |
| GET | `/admin/crawl-scope` | 读取**最终生效**的收集范围 |
| PUT | `/admin/crawl-scope` | 保存收集范围（立即生效，无需重启） |

### 收集范围

```json
{
  "job_types": ["技术", "产品", "设计", "运营", "市场", "职能"],
  "industries": ["互联网", "人工智能", "金融科技", "智能制造", "新能源"],
  "employment_types": ["全职", "实习", "校招", "社招"],
  "locations": [],
  "keywords": [],
  "exclude_keywords": ["外包", "劳务派遣", "中介"],
  "min_quality_score": 40
}
```

语义：
- **工作类型 / 行业 / 用工性质 / 地点**：白名单，不在其中即不采集
- **keywords**：非空时，岗位必须命中至少一个才采集
- **exclude_keywords**：命中任一即丢弃（优先级最高）
- **min_quality_score**：信息完整度低于该分值不进入审核队列
- 行业无法判断时**放行**（交由人工审核），避免误杀大量合规岗位
- 合并规则：以 `configs/config.yaml` 的 `crawler.scope` 为基线，被数据库中的
  `crawler.scope` 逐字段覆盖；显式空数组表示取消该维度限制

---

## 9. 管理端：邮件设置（在线配置 SMTP）

后台「系统设置 - 邮件设置」对应的接口。**仅管理员可用**，审核员访问返回 `403`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/admin/mail` | 读取当前生效的邮件配置（密码以占位符返回） |
| PUT | `/admin/mail` | 保存配置并**立即热生效**（无需重启服务） |
| POST | `/admin/mail/test` | 用当前配置发送测试邮件到指定地址 |
| DELETE | `/admin/mail` | 删除后台设置，回落到 `configs/config.yaml` |

### 读取配置

`GET /api/v1/admin/mail`

```json
{
  "driver": "smtp",
  "from_name": "JobHub 直招",
  "from_address": "no-reply@example.com",
  "host": "smtp.example.com",
  "port": 465,
  "username": "no-reply@example.com",
  "tls": true,
  "has_password": true,
  "password_mask": "********",
  "source": "database",
  "active_driver": "smtp",
  "ready": true,
  "updated_by": "admin@example.com",
  "updated_at": "2026-10-17T00:12:03+08:00"
}
```

- `has_password`：是否已保存密码（前端据此显示「已配置 / 未配置」）
- `password_mask`：占位符。**明文密码永远不会返回给客户端**
- `source`：`database`（后台设置生效）或 `config`（配置文件生效）
- `active_driver` / `ready`：当前实际装载的投递器与是否可用

### 保存配置

`PUT /api/v1/admin/mail`

```json
{
  "driver": "smtp",
  "from_name": "JobHub 直招",
  "from_address": "no-reply@example.com",
  "host": "smtp.example.com",
  "port": 465,
  "username": "no-reply@example.com",
  "password": "授权码或密码",
  "tls": true,
  "clear_password": false
}
```

密码字段的三种语义：

| 传入值 | 行为 |
|---|---|
| 新密码字符串 | 更新密码（落库前用 AES-256-GCM 加密） |
| 空字符串 或 `********`（占位符原样回传） | **沿用已保存的密码**（避免前端不知道明文时误清空） |
| `clear_password: true` | 显式清空密码 |

校验规则（不满足返回 `400` / `code=50002`）：
`driver` 只能为 `console`/`smtp`；`smtp` 时 `host`、`1~65535` 的 `port`、
`from_address` 必填；用户名与密码需同时填写或同时留空。

保存成功后**立即生效**：正在运行的进程会切换到新配置，无需重启。

### 发送测试邮件

`POST /api/v1/admin/mail/test`

```json
{ "to": "admin@example.com" }
```

- 成功：`200`，邮件内容包含发送时间与投递通道，便于确认「收到的就是刚配置的这次」
- 失败：`503`，`message` 直接给出可照做的排查步骤（与安装向导的
  `/system/setup/mail/test` **完全一致**，两处共用一个文案生成器）：
  - **认证失败**（535/534/530/454，或错误文本含 `auth` / `login fail`）→ `code=10009`，
    message 含四条通用排查步骤 + 按主机名追加的服务商提示 + 服务器原文；
  - **连接/加密方式错配**（EOF、connection reset、`first record does not look like a TLS handshake`）
    → `code=10008`，message 含 465↔SSL、587↔STARTTLS 的对照关系、当前提交的
    `host:port` 与加密方式、以及服务器原文；
  - **DNS 失败 / 端口拒绝 / 超时** → `code=10008`，message 直说是主机名、端口还是网络问题；
  - 任何分支都不出现「服务器内部错误」，也不出现密码/授权码
- `console` 驱动下「发送」即打印到服务日志，同样返回 `200`，可用于本地联调

> 密码/授权码首尾空白（复制粘贴最常见）在保存与试发时都会**自动去除**，
> 并在服务端记一条不含口令的 WARN。

> 安全性：密码在响应中只以占位符出现；保存与读取都会写入审计（`mail.update` / `mail.test`），
> 审计明细只记录主机、端口、发件人等可公开字段与「密码是否变化」，**不记录口令本身**。
> 加密密钥由 `jwt.secret` 派生，因此更换主密钥后需要在后台重新填写 SMTP 密码。

---

## 10. 管理端：审计与授权策略

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/admin/audit-logs` | 审计日志（`actor_id`/`action`/`resource`/`resource_id`/`success`） |
| GET | `/admin/authz/policies` | 授权策略列表 + 策略规模统计 |
| POST | `/admin/authz/policies` | 新增策略（`{"role":"reviewer","obj":"/api/v1/admin/x","act":"GET"}`） |
| DELETE | `/admin/authz/policies` | 删除策略，参数可用查询串：`?role=&obj=&act=` |

审计记录覆盖：登录/登出、注册、改密、重置密码、公司增删改、抓取运行、
岗位审核/编辑/删除/状态变更、角色与状态变更、配置变更、策略变更，
以及首次安装向导创建管理员（`user.setup_admin`）与启动自举创建/提升初始管理员
（`user.bootstrap_admin`）——两条的 detail 都只含邮箱、注册来源与标记位，**不含口令**。

---

## 11. 健康检查

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 依赖可用性（PostgreSQL、Redis、爬虫开关、策略规模）；异常返回 `503` |
| GET | `/readyz` | 就绪探针 |

---

## 12. 错误码总表

| 码 | HTTP | 含义 |
|---|---|---|
| 0 | 200 | 成功 |
| 10000 | 500 | 服务异常 |
| 10001 | 400 | 请求参数有误（`details` 给出具体原因） |
| 10002 | 401 | 请先登录 |
| 10003 | 403 | 没有权限执行该操作 |
| 10004 | 404 | 资源不存在 |
| 10005 | 409 | 资源冲突 |
| 10006 | 429 | 请求过于频繁 |
| 10007 | 500 | 服务器内部错误 |
| 10008 | 503 | 服务暂不可用（发信失败、连接失败也用它） |
| 10009 | 503 | SMTP 认证失败（授权码/服务开关问题，message 内含排查步骤与服务器原文） |
| 20001 | 409 | 该邮箱已被注册 |
| 20002 | 401 | 邮箱或密码错误 |
| 20003 | 403 | 账号已被禁用 |
| 20004 | 403 | 邮箱尚未验证 |
| 20005 | 400 | 验证码错误或已过期 |
| 20006 | 429 | 验证码发送过于频繁 |
| 20007 | 429 | 今日验证码发送次数已达上限 |
| 20008 | 401 | 登录已过期，请重新登录 |
| 20009 | 401 | 身份凭证无效 |
| 20010 | 409 | 账号因多次登录失败已被临时锁定 |
| 20011 | 400 | 密码强度不足（`details.problems` 列出原因） |
| 20012 | 400 | 新密码不能与当前密码相同 |
| 20013 | 400 | 原密码不正确 |
| 20014 | 404 | 用户不存在 |
| 20015 | 401 | 刷新令牌无效或已失效 |
| 30001 | 404 | 公司不存在 |
| 30002 | 409 | 公司已存在 |
| 30003 | 404 | 岗位不存在 |
| 30004 | 409 | 该岗位已被审核，不能重复审核 |
| 30005 | 400 | 当前状态不允许该操作 |
| 30006 | 409 | 岗位已存在 |
| 30007 | 400 | 招聘页地址无效 |
| 40001 | 503 | 爬虫功能当前已关闭 |
| 40002 | 503 | 目标站点 robots.txt 不允许抓取（或声明拒绝 AI 抓取） |
| 40003 | 503 | 抓取目标页面失败 |
| 40004 | 503 | 解析页面内容失败 |
| 40005 | 409 | 该公司的抓取任务正在执行中 |
| 40006 | 404 | 未能定位到该公司的招聘页面入口 |
| 40007 | 404 | 抓取记录不存在 |
| 50001 | 404 | 配置项不存在 |
| 50002 | 400 | 配置项内容不合法（安装向导的邮件参数也走这个码） |

> `409`（`code=10005`）在安装向导场景下还有一个专门含义：**向导已失效**
> （`auth.setup.enabled=false`、系统已完成初始化、或邮箱已被既有账号占用）。
> 前端据此把「系统状态不允许」与真正的鉴权失败（`401`/`403`）区分开。

---

## 13. 前端路由与守卫行为（安装向导）

前端（`apps/web`）启动后由路由守卫统一处理初始化状态，规则只有两条：

| 条件 | 行为 |
|---|---|
| `setup_available === true` | 除 `/setup` 外的**任何**路由一律重定向到 `/setup` |
| `setup_available === false` 且 `to.name === 'setup'` | 重定向到 `/login`（向导已永久失效） |

```
GET /api/v1/system/status  →  setup_available
```

- 该状态在应用启动时调用一次并**缓存**（`stores/system.js`），
  初始化成功后由向导页显式 `refresh()`，让守卫立刻停止强制跳转。
- `/setup` 路由**不加** `requiresAuth` / `guestOnly`：
  它存在的意义就是「系统里还没有任何可用账号」，任何鉴权前置都会自相矛盾。
- **后端不可用时不做任何强制跳转**：状态查询失败按「已初始化」处理，
  只给出可读提示。否则后端一挂（或反向代理没起来），所有人都会被锁死在一个
  永远提交不了的安装页上——那比看到一个能解释原因的登录页糟糕得多。
