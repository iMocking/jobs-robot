# 后端强约束开发行为规范 v2.1

## 0. 目的、关键词与优先级

### 0.1 目的

本规范用于约束 AI Agent 在后端项目中的技术选型、架构设计、身份权限、系统配置、初始化、安全、测试与交付行为，确保产出达到商业级可落地标准。

### 0.2 关键词

- **MUST / MUST NOT**：必须 / 禁止，不可协商。
- **SHOULD / SHOULD NOT**：应该 / 不应，偏离必须说明理由。
- **MAY**：可以，按需选择。

### 0.3 优先级

冲突时 MUST 按以下优先级执行：

1. 用户当前任务文本中的明确指定；
2. 仓库现有 README、`go.mod`、lock 文件、迁移文件、OpenAPI 等既有契约；
3. 本规范默认栈与强制约束；
4. AI Agent 自行判断。

“明确指定”指用户在当前任务文本中写明“使用 X 替代默认 Y”。仅凭“项目可能适合”“未反对”“行业常见”不得覆盖默认栈。

安全、密钥保护、初始化幂等、DDD 依赖方向、测试验收、禁止硬编码密钥为**不可覆盖 MUST**。

---

## 1. AI Agent 工作流强制约束

1. 改动前 MUST 阅读 README、`go.mod`、lock 文件、目录结构、迁移文件、OpenAPI、现有测试和配置样例。
2. MUST 使用默认技术栈；需替换默认栈时，必须由用户当前任务明确指定，否则 MUST NOT 擅自更换。
3. MUST NOT 擅自升级依赖、引入新依赖、替换 ORM/日志/缓存/调度/监控方案。新增依赖 MUST 先说明理由、影响和替代方案。
4. 新增接口 MUST 同步更新 OpenAPI/接口文档、错误码枚举、权限策略、审计要求和测试。
5. 改动后 MUST 执行格式化、静态检查、测试和构建；失败 MUST NOT 声称完成。
6. 自测启动服务 MUST 使用 8080 端口；自测完成后 MUST 停止自己启动的服务并确认端口释放。
7. MUST NOT kill 非自己启动的进程；端口被占用 MUST 先报告，不得强行占用或改端口绕过。
8. MUST 输出交付报告：变更摘要、影响范围、测试命令与结果、启动/停止说明、未决问题和风险。
9. MUST NOT 删除或跳过失败测试来“通过验收”；修复代码或明确报告。
10. MUST NOT 在日志、代码、测试、文档中写入真实密码、SMTP 密码、OSS secret、token 明文。

---

## 2. 默认强制技术栈

在没有用户当前任务明确替代指定时，MUST 使用：

| 领域 | 强制默认 |
|---|---|
| 语言 | Go |
| Web 框架 | Gin |
| 数据库 | PostgreSQL |
| ORM/持久化 | ent |
| 权限 | Casbin |
| 日志 | zap |
| 缓存 | Redis |
| 任务调度 | cron |
| 监控 | Prometheus client |
| ID | ULID |
| 时间 | int64 Unix 秒，UTC |
| 服务端口 | 8080 |

版本 MUST 跟随 `go.mod` 与 lock 文件；禁止擅自升级。新项目 MUST 选择稳定版本并锁定。

生产迁移 MUST NOT 只依赖 ent 自动迁移；MUST 有可审查、可回滚的迁移方案。ent schema MUST 定义必要索引、唯一约束和外键策略。

---

## 3. DDD 架构强制规范

后端 MUST 以 DDD 分层落地，禁止 controller/service/mapper 三层贫血堆代码。

### 3.1 分层与依赖方向

- **interfaces**：HTTP/CLI/事件入口；只做协议转换、鉴权、参数校验、DTO 映射；MUST NOT 写业务规则和 SQL。
- **application**：用例编排、事务边界、权限编排、领域事件发布；MUST NOT 直接依赖 ent、SMTP SDK、OSS SDK。
- **domain**：聚合根、实体、值对象、领域服务、仓储接口、领域事件；MUST 纯 Go，MUST NOT import Gin/ent/zap 等基础设施框架。
- **infrastructure**：ent 持久化、SMTP、OSS/本地存储、Casbin adapter、Redis、Prometheus 实现；MUST 实现 domain/application 定义的端口。

依赖方向 MUST 为 `interfaces -> application -> domain`；infrastructure 通过依赖倒置实现端口。

### 3.2 建模与事务

- 身份权限限界上下文 MUST 至少包含 User、Credential、EmailVerificationToken、PasswordResetToken、Role/Permission 策略等模型。
- 系统配置 SHOULD 作为独立限界上下文或平台子域，禁止把配置读写塞进认证应用服务。
- 事务边界 MUST 在 application 层；跨聚合一致性 MUST 用领域事件/最终一致性；domain MUST NOT 直接开事务。
- DTO 与领域模型 MUST 分离；MUST NOT 将 ent 实体直接作为 API 返回。
- 关键用例 MUST 有单元测试和集成测试；domain 规则 MUST 可脱离数据库测试。

---

## 4. ID、时间、API 返回与错误码

### 4.1 ID

- 所有业务实体主键 MUST 使用 ULID 字符串。
- API 中 ID MUST 为 string；数据库 MUST 为 `char(26)` 或 `varchar(26)`。
- MUST NOT 使用自增 ID、UUID 替代 ULID，除非用户明确指定。

### 4.2 时间

- `CreatedAt`、`UpdatedAt`、`DeletedAt` MUST 使用 `int64` Unix 秒，始终 UTC。
- JSON 字段 MUST 为 `created_at`、`updated_at`、`deleted_at`。
- MUST NOT 在 API 中返回本地时间字符串；前端按用户时区自行转换。

### 4.3 统一返回结构

所有接口成功/错误响应 MUST 使用：

```json
{
  "message": "xxx",
  "body": {},
  "request_id": "01H...",
  "code": 0
}
```

强制要求：

- `request_id` MUST 从 `X-Request-ID` 透传；不存在时生成 ULID。
- `code` MUST 使用统一枚举；`0` 表示成功，非 0 表示业务错误。
- HTTP 状态码 MUST 正确；MUST NOT 全部返回 200。
- 错误响应 MUST 同结构；MUST NOT 泄露堆栈、SQL、密钥、内部路径。
- 分页、列表、空 body MUST 保持结构一致。

---

## 5. 身份认证与权限默认规范

### 5.1 邮箱 + SMTP 默认认证

包含注册/登录/修改密码的身份权限项目，MUST 默认使用邮箱 + SMTP 完成身份闭环；MUST NOT 默认使用手机短信、微信扫码等未指定方式。

- 注册 MUST 使用邮箱 + 密码；注册后 MUST 发送邮箱验证邮件；未验证邮箱 MUST 限制敏感操作。
- 登录 MUST 默认使用邮箱 + 密码；邮箱验证码登录/二次验证 MAY 支持，但 MUST 由配置显式开启。
- 修改密码 MUST 支持“旧密码 + 新密码”或“邮箱验证码/重置链接 + 新密码”。
- 忘记密码 MUST 仅通过邮箱重置链接或邮箱验证码完成。
- 邮箱验证 token、重置 token MUST 一次性、带 TTL；数据库 MUST 仅存哈希；使用后立即失效。
- 修改密码成功后 MUST 撤销该用户全部刷新令牌/会话，并记录审计日志。
- 邮件发送 MUST 抽象为 `MailerPort`，由 infrastructure 实现 SMTP；MUST NOT 在 domain/application 直接调用 SMTP SDK。
- 邮件发送 SHOULD 异步化并支持重试、失败记录；验证邮件/重置邮件 MUST 支持重发和限流。
- 认证接口 MUST 防用户枚举：登录失败、邮箱不存在、发送验证码等响应不得泄露账号是否存在。
- 认证接口 MUST 限流并记录审计日志；日志 MUST NOT 输出密码、SMTP 密码、token 明文。

### 5.2 默认认证接口

MUST 至少提供：

- `POST /auth/register`
- `POST /auth/email/verify`
- `POST /auth/email/resend`
- `POST /auth/login`
- `POST /auth/password/forgot`
- `POST /auth/password/reset`
- `POST /auth/password/change`
- `POST /auth/logout`
- `GET  /auth/me`

### 5.3 Casbin 权限

- MUST 使用 Casbin 做权限控制；MUST 定义 model 与 policy 存储方式。
- 默认角色、超级管理员权限、策略变更 MUST 有审计。
- Casbin model/policy 变更 MUST 校验合法性、清理缓存、记录操作人。
- MUST NOT 无审计地开放通配符权限或全量权限。
- 权限中间件 MUST 在 application/interfaces 边界统一执行；MUST NOT 在 domain 依赖 Casbin SDK。

---

## 6. 系统自定义配置规范

包含权限的系统 MUST 支持以下可配置项，并通过管理接口/管理台维护，禁止硬编码：

1. **SMTP**：host、port、username、password、from_email、from_name、encryption(tls/ssl/none)、timeout。
2. **OSS 对象存储**：provider、endpoint、region、bucket、access_key、secret_key、public_base_url、path_prefix。
3. **本地图片存储**：root_dir、public_base_url、max_size、allowed_mime、path_prefix。
4. **Casbin 身份配置**：model 文本/文件、policy 存储方式、默认角色、策略变更审计。
5. **初次启动初始化**：系统是否已初始化、超级管理员、默认角色/权限、必要系统参数。

强制约束：

- 配置 MUST 按领域建模为 `SystemSetting` 聚合；MUST NOT 以散落环境变量读取代替可管理配置。环境变量仅用于引导启动和密钥。
- SMTP 密码、OSS `secret_key`、加密密钥等敏感配置 MUST 加密存储；API 返回 MUST 脱敏；MUST NOT 写入日志。
- 配置变更 MUST 校验、审计、记录操作人；SMTP/OSS MUST 提供“测试连接”接口。
- 配置读取 MUST 通过 `ConfigPort`/`SettingRepository`；业务代码 MUST NOT 直接读全局变量。
- 对象存储 MUST 抽象为 `StoragePort`，OSS 与本地存储为可切换实现。
- 上传 MUST 校验大小、MIME、扩展名；MUST NOT 信任客户端文件名。
- 本地存储 MUST 防路径穿越；生产环境 SHOULD 默认 OSS，本地存储仅作为显式配置。

### 6.1 首次启动初始化

- MUST 提供幂等初始化流程：bootstrap 命令或仅未初始化状态可调用的初始化接口。
- 初始化 MUST 创建超级管理员、默认角色/Casbin policy、必要系统配置，并写入初始化完成标记。
- 初始化 MUST 使用数据库锁/唯一约束保证并发幂等；已初始化后 MUST 拒绝重复初始化。
- 超级管理员初始密码 MUST 通过一次性随机密码/环境变量/初始化令牌提供；MUST NOT 硬编码默认密码。
- 初始化接口 MUST 仅在系统未初始化时开放，并做来源限制/初始化令牌校验。

建议接口：

- `GET /system/bootstrap/status`
- `POST /system/bootstrap`
- `GET/PUT /system/settings/smtp`
- `POST /system/settings/smtp/test`
- `GET/PUT /system/settings/oss`
- `POST /system/settings/oss/test`
- `GET/PUT /system/settings/storage/local`
- `GET/PUT /system/settings/casbin`

---

## 7. 数据、缓存、调度、日志、监控与审计

- PostgreSQL MUST 使用必要索引、唯一约束、外键策略；迁移 MUST 可审查、可回滚。
- 事务边界 MUST 在 application 层；仓储实现 MUST 在 infrastructure。
- Redis key MUST 带应用前缀和 TTL；MUST NOT 存无过期敏感数据。
- cron 任务 MUST 使用 UTC、幂等、防重叠；关键任务 MUST 有失败记录和告警。
- zap MUST 输出 JSON；至少包含 `request_id`、`method`、`path`、`status`、`latency`。
- Prometheus MUST 暴露 `/metrics`；关键接口、认证失败、邮件发送、配置变更 SHOULD 有指标。
- 认证、密码修改、权限变更、配置变更、初始化 MUST 记录审计日志。
- 审计日志 MUST 包含操作人、动作、对象、结果、request_id、时间；MUST NOT 包含敏感明文。

---

## 8. 安全强制要求

- MUST NOT 硬编码密码、SMTP 密码、OSS secret、JWT 密钥、加密密钥。
- 密码 MUST 使用强哈希（如 Argon2id/bcrypt），MUST NOT 明文或可逆加密存储。
- 认证接口 MUST 限流、防暴力破解、防用户枚举。
- token MUST 有 TTL；刷新令牌 SHOULD 轮换；修改密码后 MUST 撤销会话。
- 上传 MUST 做服务端校验；本地存储 MUST 防路径穿越；OSS 配置 MUST 最小权限。
- 配置密钥 API 返回 MUST 脱敏；日志 MUST NOT 输出敏感信息。
- 错误响应 MUST NOT 泄露堆栈、SQL、内部路径。

---

## 9. 测试与验收强制标准

改动后 MUST 通过：

```bash
gofmt -l .
go vet ./...
go test ./...
go build ./...
```

`gofmt -l .` 应无输出。

集成测试 MUST 至少覆盖：

- 注册 -> 邮箱验证 -> 登录；
- 忘记密码 -> 重置密码 -> 旧会话失效；
- 修改密码 -> 会话撤销；
- 初始化幂等、重复初始化拒绝；
- SMTP/OSS 配置测试连接；
- Casbin 权限拒绝/放行；
- 统一返回结构与 request_id 透传。

未通过验收 MUST NOT 声称完成。自测服务 MUST 使用 8080 并在结束后停止。

---

## 10. 交付报告强制格式

MUST 包含：

1. 变更摘要与影响范围；
2. 新增/修改接口及 OpenAPI 更新说明；
3. 数据库迁移与回滚说明；
4. 权限策略与审计变更；
5. 测试命令与结果；
6. 服务启动/停止说明；
7. 未决问题、风险与需用户确认项。

---

## 11. 禁止事项

- 禁止擅自更换语言、框架、数据库、ORM、日志、缓存、调度、监控方案。
- 禁止擅自升级依赖、删除测试、跳过测试、硬编码密钥、修改默认端口。
- 禁止在未说明情况下引入新依赖或组件库。
- 禁止在 domain/application 直接调用 ent、SMTP SDK、OSS SDK。
- 禁止将 ent 实体直接作为 API 返回。
- 禁止全 200 返回、错误响应结构不一致、`request_id` 缺失。
- 禁止初始化接口可重复调用、默认硬编码超级管理员密码。
- 禁止配置密钥明文存储、API 明文返回、日志输出。
- 禁止 kill 非自己启动的进程或占用用户手动启动端口。