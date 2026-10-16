# 直招好工作 · jobs-robot

采集**企业官网《加入我们》页面**上的直招岗位，聚合成可检索、可审核、可追溯的岗位库。

> 「直招」在本项目中特指：从公司官网的《加入我们 / Join Us / Careers》超链接跳转后，
> 由企业自己发布的岗位。这类岗位不经中介、信息源可回溯，因此采集时会强制记录来源 URL、
> 判定 `is_direct_hire`，并保留内容变更历史。

当前交付范围：**后端 API**（前端 UI 待设计稿介入后对接）。

---

## 1. 能力概览

| 模块 | 能力 |
|---|---|
| 账号体系 | 邮箱注册、登录、找回密码（邮箱验证码）、改密、刷新令牌、会话管理（可下线指定设备） |
| 身份权限 | 预设 **管理员 / 审核员 / 用户** 三种角色，Casbin 做接口级鉴权，策略可在线调整 |
| 爬虫采集 | 配置化公司清单 → 自动定位《加入我们》入口 → 发现岗位链接 → 抽取字段 → 白名单过滤 |
| 抓取礼仪 | 标准 **AI 机器人身份请求头**、遵循 robots.txt、同站点串行 + 延时抖动、尊重 `noai` 声明 |
| 岗位信息 | 任职要求、岗位介绍、岗位职责、发布时间、联系方式、工作地址、最后更新时间等 60+ 字段 |
| 变更追溯 | 内容指纹 + 修订记录，能回答「何时、因哪次抓取、改了哪些字段」 |
| 审核工作流 | 待审核 → 已发布 / 已驳回；实质性变更自动退回待审核；长期未再出现自动判定过期 |
| 收集范围 | 通过配置（静态 YAML + 数据库热更新）规定只收集哪些**工作类型**与**行业** |
| 邮件配置 | SMTP 可在后台页面在线设置，**保存即热生效**；密码加密存储、接口只回传占位符，并支持一键发送测试邮件 |
| 可观测性 | 每次抓取有运行记录与逐条明细日志；管理操作全量审计留痕 |

技术栈：Go 1.27 · gin · ent · PostgreSQL · Redis · Casbin。

> **主键统一使用 ULID**（26 位 Crockford Base32 字符串，例如 `01JD7Q9K2M3N4P5R6S7T8V9W0X`），
> **杜绝数字自增 id**：既不把业务规模写进 URL，又能按时间单调递增排序；
> 十张业务表全部通过 `internal/ent/schema/mixins.ULID{}` 自动获得该主键，
> 外键字段（`company_id` / `reviewer_id` / `actor_id` / `run_id` / `job_id` 等）同样是
> `VARCHAR(26)`。详见 `internal/pkg/ulidid` 与 `docs/api.md` 的「1.6 标识（ID）约定」。

---

## 2. 快速开始

### 2.1 启动依赖

```bash
docker compose -f deployments/docker-compose.yml up -d
```

> 宿主机端口刻意使用 **55432**（PostgreSQL）与 **56379**（Redis），避免与本机其他项目的
> 5432/6379 冲突。如已有可用的 PG/Redis，跳过本步并修改 `configs/config.yaml`。

### 2.2 全新部署：打开前端 `/setup` 完成初始化

```bash
go run ./cmd/api -config configs/config.yaml
# 单机便利模式：在 API 进程内一并启动定时抓取
go run ./cmd/api -scheduler
```

启动 API 后打开前端，请按这个顺序走（**这就是默认的首次安装路径**）：

1. **启动 API** —— 不需要先跑 `cmd/seed`，也不会有任何自动生成的账号；
2. **打开前端 `/setup`** —— 系统尚无可用管理员时，路由守卫会把**除 `/setup` 外的任何
   页面**都重定向到这里。若你直接落到 `/login`，表单上方也会给出「前往初始化」的入口；
3. **创建管理员并配置 SMTP** —— 一个页面三步：
   - 第 1 步：管理员邮箱 + 密码（至少 8 位且含字母与数字，与注册/改密同一套规则）+ 昵称；
   - 第 2 步：邮件服务。可选 `SMTP`（推荐）/ `console`（验证码只打印到服务端日志）/
     「稍后到后台补配」。加密方式三选一，**与后端实现严格对应**：

     | 选项 | 端口 | 请求里的 `tls` | 后端行为（`internal/mail/smtp.go`） |
     |---|---|---|---|
     | SSL/TLS 直连 | 465 | `true` | `tls.Dial` —— 整条连接即 TLS（隐式 TLS） |
     | STARTTLS | 587 | `false` | 先明文 `smtp.Dial`，服务端通告 STARTTLS 时升级 |
     | 不加密 | 25 | `false` | 同上，但服务端通常不通告 STARTTLS，于是全程明文 |

     页面上有腾讯企业邮 / QQ / 163 / Gmail / 阿里云企业邮 / Outlook 的一键填入参考，
     避免填错主机与端口；密码框支持显示切换，并提示「请填写服务商生成的**授权码**（非登录密码）；
     首尾空白会被自动忽略」；
   - **发送测试邮件**：用**表单里正在填的、尚未保存的**参数真实投递一封，
     收件人默认取管理员邮箱。失败会原样展示后端返回的可读原因与**排查步骤**
     （认证失败 535 → `code=10009`；连接/加密方式错配、DNS、端口、超时 → `code=10008`），
     据此改参数再试。**测试失败也允许继续提交**，但会醒目提示；
4. **进入后台** —— 提交成功后展示结果摘要（管理员邮箱、邮件驱动与是否就绪、
   `registration_open` 状态），点「进入后台」跳登录页即可。

> 测试邮件**只发信、不落库、不热替换投递器**。这正是「先验证再保存」的意义：
> 若先写库再试发，一个填错的主机或授权码就已经替换了生效中的投递器，
> 系统立刻处于「验证码发不出去」的状态——而这恰恰是向导要避免的。

> 跳过 SMTP 直接提交也可以：此时 `registration_open` 仍为 `false`，
> 用户注册与找回密码暂不可用（验证码发不出去），稍后在后台「系统配置 - 邮件服务」补配即可。

安装向导的三重门控：`auth.setup.enabled` 为真 + **系统当前不存在可用管理员** + 否则返回 `409`。
一旦存在管理员即**永久失效**（访问 `/setup` 会被重定向到登录页），接口按 IP 限流（默认 10 次/小时），
并且**不写任何凭据文件**（密码是你自己设的）。

可用 `auth.setup.enabled=false` 完全关闭向导（`setup_available` 随之恒为 false，
适用于由外部系统预置账号的部署）。

### 2.3 无界面 / 容器部署：启动自动建号（`auth.bootstrap.*`）

纯容器、CI、只跑 API 的镜像没有 `/setup` 可用，此时把自举显式打开：

```yaml
auth:
  bootstrap:
    enabled: true          # 默认 false
  setup:
    enabled: false         # 与上者互斥
```

打开后 API（以及 worker）启动时会检查「可登录的管理员」是否存在，没有就创建一个，并：

1. 用 **WARN 级**在启动日志打印一次初始凭据（默认级别是 info，WARN 保证醒目）：
   `初始管理员账号已创建，请登录后台配置邮件（SMTP）服务，并立即修改密码`；
2. 把邮箱/密码/生成时间写入 `var/bootstrap-admin.txt`（权限 `0600`，目录自动创建）；
3. 给账号打上 `must_change_password=true`，登录后前端会提示「请修改初始密码」。

配置项见 `auth.bootstrap.*`（`JOBSROBOT_AUTH_BOOTSTRAP_*`）：可自定义邮箱、昵称、
凭据文件路径，或用 `password` 指定密码（此时日志只打印 `(来自配置，未打印)`，
不会把你设置的口令写进日志）。

> **`auth.bootstrap.*` 与 `auth.setup.*` 的区别**：
>
> | | `auth.setup.*`（安装向导） | `auth.bootstrap.*`（启动自举） |
> |---|---|---|
> | 谁提供密码 | **运维自己设** | 系统随机生成 |
> | 在哪操作 | 浏览器 `/setup` 页面 | 启动日志 + 凭据文件 |
> | 能顺便验证 SMTP | **能**（发送测试邮件） | 不能，需登录后到后台配 |
> | 默认 | **开启** | **关闭** |
> | 适用 | 有界面的部署 | 无界面：纯容器、CI、只跑 API 的镜像 |
>
> **两者不能同时为 true**：启动自举会先建号，系统立刻变成「已初始化」，
> 安装向导就永远用不上了。同时开启会在启动时直接报配置错误。

> **幂等且不改动既有账号**：只要系统里还有一名可用管理员（角色存在且账号未被禁用），
> 自举就直接跳过。因此重复重启、滚动升级都不会产生多余管理员，也不会重置任何人的密码。
> 若邮箱已被占用但不是管理员（例如管理员被降级过），则走「提升既有账号」路径：
> 只补 `admin` 角色并把 `must_change_password` 置 true，**不改密码、也不写凭据文件**
> （那个随机密码并未生效，写进文件只会让人照着登录却失败），请用该账号原有密码登录。

**首次登录后请按这个顺序处理**：

1. 在「系统设置 - 邮件设置」中配置 SMTP，并点「发送测试邮件」确认能收到；
2. 再在「个人中心」修改初始密码；
3. 最后**删除 `var/bootstrap-admin.txt`**。

> 顺序很重要：先配邮件再改密码，能避免「注册/找回密码走不通时还要再改一次密码」的来回折腾。
> 前端可调用公开接口 `GET /api/v1/system/status` 读取初始化状态
> （是否已有管理员、`setup_available`、注册通道是否真的可用、凭据文件是否仍未删除）
> 来渲染初始化向导。

#### 仍可按需显式创建管理员

`cmd/seed` 依然保留，用于**指定邮箱/密码**或为已有部署**补建**管理员
（它显式关闭了启动自举，只按自己的参数动作）：

```bash
go run ./cmd/seed -admin-email admin@example.com -admin-password 'YourPassw0rd'
```

不带 `-admin-password` 时会生成一个随机强密码并**只打印一次**。该命令幂等，可重复执行。
它会完成三件事：

1. 建表（ent 自动迁移）；
2. 写入三种角色的展示元数据，并向 Casbin 播种默认策略；
3. 创建初始管理员账号并授予 `admin` 角色。

### 2.4 访问接口

服务默认监听 `0.0.0.0:8090`，接口前缀 `/api/v1`。

```bash
# 健康检查
curl http://127.0.0.1:8090/healthz

# 登录
curl -s -X POST http://127.0.0.1:8090/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"YourPassw0rd"}'

# 浏览已发布岗位（公开接口）
curl 'http://127.0.0.1:8090/api/v1/jobs?page=1&page_size=10'
```

### 2.5 独立抓取 worker（推荐生产使用）

```bash
go run ./cmd/worker            # 按 crawler.schedule 周期执行
go run ./cmd/worker -once      # 立即跑一轮全量抓取后退出
go run ./cmd/worker -expire    # 只执行一次岗位过期判定
```

抓取是长耗时且受目标站点影响的任务，与 API 分离部署后可独立扩容与重启。
worker 启动时同样会执行初始管理员自举（**仅在 `auth.bootstrap.enabled=true` 时**，
默认关闭；只跑抓取的部署也需要能进后台），逻辑幂等。

---

## 3. 配置

优先级：**内置默认值 < `configs/config.yaml` < 环境变量（前缀 `JOBSROBOT_`）**。

完整示例见 [configs/config.yaml](configs/config.yaml) 与 [configs/.env.example](configs/.env.example)。关键项：

| 配置项 | 说明 |
|---|---|
| `jwt.secret` | 至少 32 字节；生产环境必须替换（`openssl rand -base64 48`） |
| `jwt.access_ttl` / `refresh_ttl` | 访问令牌 2 小时、刷新令牌 7 天；刷新令牌存 Redis，可主动吊销 |
| `mail.driver` | `console`（验证码打印到日志，便于联调）/ `smtp`（真实发信） |
| `auth.expose_code_in_response` | 仅联调期可置 `true`，接口会直接回显验证码 |
| `auth.rate_limit.*` | 认证接口的 IP 维度限流阈值（置 `0` 或 `enabled=false` 即不限流） |
| `auth.setup.enabled` | **首次安装向导**是否开放（默认 `true`）；`test_mail_to` 可指定测试邮件默认收件人 |
| `auth.bootstrap.*` | **启动自举管理员**：开关、邮箱、密码、昵称、凭据文件路径（默认**关闭**，仅无界面部署开启；与 `auth.setup.enabled` 互斥） |
| `crawler.identity.*` | **AI 机器人身份**：名称、版本、联系邮箱、主页、用途、是否 AI |
| `crawler.schedule` | 全量抓取周期（cron 5 段式），默认每 6 小时 |
| `crawler.delay` / `random_delay` | 同站点最小间隔与随机抖动（默认 2s + 1s） |
| `crawler.respect_robots` | 是否遵循 robots.txt，**默认 true，请勿关闭** |
| `crawler.job_expire_after` | 连续多久未再出现即判定岗位下线，默认 30 天 |
| `crawler.scope.*` | 收集范围：工作类型、行业、用工性质、城市、关键词白名单与排除词 |

### 收集范围可热更新

`crawler.scope` 可被数据库中的配置项覆盖，通过接口修改后**下一次抓取立即生效，无需重启**：

```bash
curl -X PUT http://127.0.0.1:8090/api/v1/admin/crawl-scope \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"job_types":["技术","产品"],"industries":["人工智能"],"exclude_keywords":["外包","派遣"],"min_quality_score":50}'
```

### 邮件配置可在后台在线修改

SMTP 配置不必改文件、不必重启：后台「系统设置 - 邮件设置」保存后**立即生效**。

```bash
# 读取当前生效配置（密码以 ******** 占位符返回，明文永不出服务端）
curl http://127.0.0.1:8090/api/v1/admin/mail -H "Authorization: Bearer $TOKEN"

# 保存在线配置（password 留空或回传占位符 = 不修改已保存的密码）
curl -X PUT http://127.0.0.1:8090/api/v1/admin/mail \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"driver":"smtp","from_name":"JobHub 直招","from_address":"no-reply@example.com",
       "host":"smtp.example.com","port":465,"username":"no-reply@example.com",
       "password":"授权码","tls":true}'

# 发一封测试邮件确认配置真的可用
curl -X POST http://127.0.0.1:8090/api/v1/admin/mail/test \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"to":"admin@example.com"}'
```

- 优先级：**数据库中的后台设置 > `configs/config.yaml` > 环境变量**
- 密码用 AES-256-GCM 加密存储，密钥由 `jwt.secret` 派生（更换主密钥后需重新填写）
- `DELETE /api/v1/admin/mail` 可一键回落到配置文件中的配置
- 仅管理员可访问；审核员访问返回 `403`

### SMTP 常见故障排查

点「发送测试邮件」失败时，后端返回的 `message` 已经写明原因与动作，这里给出对照表备查。

**先记住端口与加密方式的对应关系**（这是最高频的填错项）：

| 端口 | 界面上的加密方式 | 请求里的 `tls` | 后端行为（`internal/mail/smtp.go`） |
|---|---|---|---|
| **465** | **SSL/TLS 直连** | `true` | `tls.Dial`：整条连接即 TLS（隐式 TLS） |
| **587** | **STARTTLS** | `false` | 先明文 `smtp.Dial`，服务端通告 STARTTLS 时升级 |
| 25 | 不加密 | `false` | 同上，但服务端通常不通告 STARTTLS，且多数云厂商已封禁 |

> 465 选了 STARTTLS/不加密 → 服务器等不到 TLS 客户端问候会直接断开，表现为 `EOF`；
> 反过来 587 选了 SSL/TLS 直连 → 表现为 `first record does not look like a TLS handshake`。

**各服务商的服务器、端口与密码类型**：

| 服务商 | 服务器 | 端口 / 加密方式 | 这里的「密码」填什么 | 在哪里开启 |
|---|---|---|---|---|
| QQ 邮箱（个人） | `smtp.qq.com` | `465` / SSL/TLS 直连 | **16 位授权码**（不是登录密码） | 邮箱网页端「设置 → 账户 → POP3/IMAP/SMTP 服务」 |
| 腾讯企业邮 | `smtp.exmail.qq.com` | `465` / SSL/TLS 直连 | **客户端专用密码**（与个人 QQ 邮箱的授权码不通用） | 企业邮管理端「邮箱设置 → 客户端设置」 |
| 163 / 126 邮箱 | `smtp.163.com` / `smtp.126.com` | `465` / SSL/TLS 直连 | **客户端授权密码** | 邮箱网页端「设置 → POP3/SMTP/IMAP」 |
| Gmail | `smtp.gmail.com` | `587` / STARTTLS | **应用专用密码**（需先开启两步验证；组织禁用时改用 OAuth） | Google 账号「安全性 → 两步验证 → 应用专用密码」 |
| Outlook / Microsoft 365 | `smtp.office365.com` | `587` / STARTTLS | **应用密码 / 现代认证**（账号须为完整邮箱地址） | 租户中启用 SMTP AUTH（安全默认值会禁用） |
| 阿里云企业邮 | `smtp.qiye.aliyun.com` | `465` / SSL/TLS 直连 | **客户端专用密码** | 阿里云邮箱「设置 → 客户端设置」 |

**按报错关键词定位原因**：

| 报错关键词（服务器原文里出现的） | 真实原因 | 怎么改 |
|---|---|---|
| `EOF`、`unexpected EOF`、`connection reset by peer`、`broken pipe`、`first record does not look like a TLS handshake` | **端口与加密方式不匹配** | 465 必须选「SSL/TLS 直连」；587 必须选「STARTTLS」 |
| `no such host` | **主机名拼错** | 填 `smtp.qq.com` 这种主机名，不要带 `https://`、路径或端口 |
| `connection refused` | **端口不对，或服务商侧 SMTP 未开启** | 核对 465/587，并确认邮箱网页端已开启 SMTP 服务 |
| `i/o timeout`、`deadline exceeded` | **网络不通 / 被安全组或防火墙拦截** | 放行出站 465/587；**25 端口多数云厂商默认封禁**，不要用它发公网邮件 |
| `535 Login fail...`、`authentication failed`、`534`、`530` | **授权码或服务开关问题**（不是程序 bug） | 用服务商生成的**授权码**而非登录密码；网页端开启 POP3/SMTP；账号填**完整邮箱地址** |

> 两条容易忽略的细节：
> 1. 授权码从网页复制时**常带一个尾随空格或换行**，后端会自动去除并记一条 WARN
>    （`检测到 SMTP 密码/授权码首尾含空白字符，已自动去除`），日志里不含口令；
> 2. 535 这类失败返回 `code=10009`（HTTP 仍是 `503`），`message` 内含排查步骤、
>    按主机名追加的服务商提示与**服务器原文**，且不会出现「服务器内部错误」。

---

## 4. 抓取合规设计

本项目把「不越界」当成功能的一部分，而不是可选项：

1. **标准 AI 机器人身份**：每个出站请求（含 robots.txt 请求）都携带

   ```
   User-Agent: JobsRobotBot/1.0 (+https://…/bot; AI-crawler; contact: crawler@…)
   From: crawler@…
   X-Robot-Name / X-Robot-Version / X-Robot-Contact / X-Robot-Homepage
   X-Robot-Purpose: 聚合企业官网直招岗位信息，供求职者检索
   X-Robot-AI: true
   X-Robot-Type: ai-crawler
   X-Robot-Respect-Robots: true
   ```

   HTTP 头只允许可见 ASCII，中文用途说明会被百分号编码（`internal/crawler/identity.go`）。

2. **遵循 robots.txt**：站点禁止即跳过并记录原因；站点声明的 `Crawl-Delay` 会被采纳为
   该域名的最小请求间隔。`robots.txt` 不可达时**保守处理**（暂时不抓取，短 TTL 后重试）。

3. **尊重 AI 拒绝声明**：页面 meta 或 `X-Robots-Tag` 中出现 `noai` / `nogptbot` / `noimageai`
   时主动放弃该页。

4. **抓取限速**：同域名串行（`per_host_limit`）、固定延时 + 随机抖动、
   单站点页面上限（`max_pages_per_site`）、失败指数退避、429 时遵守 `Retry-After`。

5. **单实例抓取**：同一公司同时只允许一个抓取在跑（Redis 分布式锁 + 安全释放的 Lua 脚本），
   定时任务与手动触发不会叠加对目标站点施压。

6. **入口探测有界**：为找到《加入我们》入口最多探测 12 个页面，不会把整站爬一遍。

---

## 5. 权限模型

授权事实来源唯一：Casbin 的 `casbin_rule` 表。

- `p` 规则 = 谁能在哪个接口上做什么（`p, reviewer, /api/v1/admin/jobs/:id/review, POST`）
- `g` 规则 = 用户属于哪个角色（`g, user:12, reviewer`），并带角色继承 `admin ⊃ reviewer ⊃ user`

数据库中的 `roles` 表**只是角色的展示元数据**（中文名、描述、层级），不参与鉴权判定——
这样不会出现两套互相矛盾的权限数据。

| 角色 | 权限范围 |
|---|---|
| `admin` 管理员 | 全部管理能力：公司、抓取、配置、用户与角色、审核、授权策略 |
| `reviewer` 审核员 | 查看并审核岗位；只读查看公司、抓取记录与统计 |
| `user` 用户 | 浏览已发布岗位、维护个人资料与登录会话 |

安全约束：不允许禁用/删除自己；不允许移除系统中的最后一名可用管理员；
角色调整、改密、禁用账号都会使既有令牌立即失效。

---

## 6. 数据模型

所有表的主键都是 **ULID 字符串**（`VARCHAR(26)`，非自增数字），外键字段同为 `VARCHAR(26)`：
新增实体只要在 schema 的 `Mixin()` 中加入 `mixins.ULID{}` 即可获得统一主键，
无需业务层手动赋值（ent 在创建时自动填充）。

| 表 | 用途 |
|---|---|
| `users` | 用户（含登录失败计数、锁定时间、邮箱验证状态） |
| `roles` | 角色展示元数据 |
| `casbin_rule` | Casbin 策略（p 与 g 规则），授权唯一事实来源 |
| `companies` | 来源公司，含 `careers_url`（《加入我们》链接）、`discovery_url`、站点抓取规则 |
| `job_postings` | 岗位（60+ 字段，含任职要求、联系方式、工作地址、发布时间、最后更新时间） |
| `job_revisions` | 岗位内容修订记录（变更字段、快照、触发方） |
| `crawl_runs` | 每次抓取的运行记录与统计 |
| `crawl_logs` | 抓取过程逐条明细日志（阶段、URL、原因） |
| `verification_codes` | 验证码发放审计（只存 HMAC 摘要，不存明文） |
| `audit_logs` | 管理操作审计 |

「最后更新时间」有三层含义，字段刻意分开：

- `published_at`：来源页面标注的**发布时间**
- `last_updated_at`：本系统感知到的**内容最后变更时间**（内容指纹变化时才刷新）
- `last_seen_at`：最近一次抓取时该岗位**仍在架**（用于判定过期）

---

## 7. 项目结构

```
cmd/
  api/          HTTP API 服务
  worker/       抓取调度进程（可 -once 手动跑一轮）
  seed/         初始化角色、策略、管理员
configs/        config.yaml 与环境变量样例
deployments/    docker-compose（PG 55432 / Redis 56379）
internal/
  bootstrap/    依赖装配（三个入口共用）
  config/       配置加载与校验（默认值 < YAML < 环境变量）
  auth/         密码哈希、JWT、会话/刷新令牌、邮箱验证码
  casbin/       授权模型、PostgreSQL 策略适配器、角色管理
  crawler/      抓取引擎：身份头、robots、入口发现、字段抽取、范围过滤、编排
  ent/          ent schema 与生成代码（含 migrate）
  mail/         邮件抽象（console / SMTP）与邮件模板
  server/       gin 路由、中间件、handler（含 system/setup 安装向导入口）
  service/      业务逻辑（认证、用户、公司、岗位、审核、配置、抓取落库、安装向导 setup.go）
  scheduler/    cron 调度（周期抓取 + 过期判定）
  store/        PostgreSQL 与 Redis 连接
docs/           架构设计与 API 文档
```

---

## 8. 开发

```bash
# 重新生成 ent 代码（修改 internal/ent/schema/*.go 后）
go generate ./internal/ent/...

# 构建与静态检查
go build ./...
go vet ./...
gofmt -l .

# 单元测试（Redis 集成用例在 Redis 不可用时自动跳过，不会伪装通过）
go test ./...
```

### 端到端冒烟测试

`tests/smoke.ps1` 会启动一条完整的真实链路（170+ 项断言），并在结尾清理自己产生的数据。

> ⚠️ **该脚本必须保留 UTF-8 BOM**。Windows PowerShell 5.1 会把无 BOM 的 UTF-8 文件按 ANSI 解析，
> 脚本里的中文断言名会让引号错配、直接语法报错。用编辑器保存时请选择「UTF-8 with BOM」；
> 若脚本报出一连串「缺少表达式 / 意外的标记」之类的解析错误，先检查 BOM 是否被工具链去掉了。

```bash
# 1) 启动夹具企业官网（任意静态服务器均可，示例用 Python）
python -m http.server 8899 --directory tests/fixtures/site

# 2) 以联调模式启动 API（回显验证码，便于自动化跑注册/找回密码）
#    PowerShell:
$env:JOBSROBOT_AUTH_EXPOSE_CODE_IN_RESPONSE='true'; go run ./cmd/api

# 3) 运行
powershell -File tests/smoke.ps1
```

覆盖范围：未鉴权拦截 → 注册/登录/刷新/找回密码 → 公司录入 → 自动定位《加入我们》入口 →
真实抓取 → 字段抽取校验（任职要求/联系方式/工作地址/发布时间/薪资）→ 白名单过滤 →
幂等重抓 → 审核发布 → 公开可见性 → 人工编辑与退回审核 → RBAC 越权拦截 →
收集范围热更新 → 审计留痕 → 登出失效 → 数据清理。

夹具站点（`tests/fixtures/site`）同时提供 **JSON-LD 结构化**与**纯 DOM** 两类岗位页，
并含一个命中「外包」排除词的岗位，用于分别验证两条抽取路径与过滤逻辑。

---

## 9. 已知边界与后续

- **SPA 招聘站点**：默认策略是静态抓取优先、内容过短时自动切换无头浏览器兜底。
  若某站点必须渲染，可在公司配置里开 `force_headless`；无头浏览器缺失时服务仍可运行，
  只是该站点会抓不到内容（启动日志会给出明确告警）。
- **登录表单类站点**：暂不支持需要登录才能查看的招聘后台。
- **多副本部署**：策略每 60 秒自动从数据库重载；抓取由 Redis 锁保证互斥。
  若需要更严格的实时性，可在变更后调用 `/api/v1/admin/authz/policies` 相关接口触发重载。
- **前端对接**：接口契约已冻结在 `internal/service/view.go` 的视图模型里，
  设计稿到位后按需增删字段即可，不影响领域模型。
