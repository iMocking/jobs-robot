# 前端强约束开发行为规范 v2.1

## 0. 目的、关键词与优先级

### 0.1 目的

本规范用于约束 AI Agent 在前端项目中的技术选型、权限、路由、状态、API 契约、UI、配置管理页面、测试与交付行为，确保与后端商业级契约一致。

### 0.2 关键词

- **MUST / MUST NOT**：必须 / 禁止。
- **SHOULD / SHOULD NOT**：应该 / 不应，偏离必须说明理由。
- **MAY**：可以。

### 0.3 优先级

1. 用户当前任务明确指定；
2. 仓库现有 `package.json`、lock 文件、OpenAPI、现有组件与路由；
3. 本规范默认栈；
4. AI Agent 自行判断。

“明确指定”指当前任务文本写明“使用 X 替代默认 Y”。安全、类型检查、权限校验、禁止泄露敏感信息、自测停服务为不可覆盖 MUST。

---

## 1. AI Agent 工作流强制约束

1. 改动前 MUST 阅读 `package.json`、lock 文件、路由、stores、API 层、权限配置、OpenAPI 和现有测试。
2. MUST 使用默认技术栈；MUST NOT 擅自更换框架、路由、状态管理、样式方案或引入组件库。
3. MUST NOT 擅自升级依赖；新增依赖 MUST 说明理由、体积/兼容影响和替代方案。
4. 新增接口调用 MUST 与后端 OpenAPI 和统一返回结构一致；MUST NOT 自行臆造字段。
5. 改动后 MUST 执行类型检查、lint、构建；失败 MUST NOT 声称完成。
6. 前端自测 dev server MUST 默认使用 5173 端口，并代理后端 8080；自测后 MUST 停止自己启动的服务。
7. MUST NOT 占用 8080 或 kill 非自己启动的进程。
8. MUST 输出交付报告：变更摘要、影响页面、测试命令与结果、启动/停止说明、未决问题。
9. MUST NOT 删除或跳过失败测试来通过验收。
10. MUST NOT 在控制台、日志、代码、测试中输出 token、密码、SMTP/OSS 密钥等敏感信息。

---

## 2. 默认强制技术栈

| 领域 | 强制默认 |
|---|---|
| 语言 | TypeScript，`strict` 开启 |
| 框架 | Vue 3 |
| 样式 | TailwindCSS |
| 路由 | vue-router |
| 状态 | pinia |
| 权限 | vue-router 路由守卫 + 动态路由 + `v-permission` 指令 |
| 请求 | 统一 axios 封装 |
| 开发端口 | 5173，代理后端 8080 |

说明：

- TailwindCSS 是样式工具，MUST NOT 将其表述为 UI 组件框架。
- `vue-admin-template` 仅可作为后台模板参考，MUST NOT 当作权限框架；使用前 MUST 确认 Vue3 兼容性。
- 默认 MUST NOT 擅自引入 Element Plus/Ant Design Vue 等组件库；需要时 MUST 由用户明确指定。
- 版本 MUST 跟随 `package.json` 与 lock 文件。

---

## 3. 工程结构强制规范

MUST 按职责组织，至少包含：

```text
src/
  api/          # 接口函数与 DTO 类型
  components/   # 通用组件
  composables/  # 组合式逻辑
  layouts/      # 布局
  router/       # 路由与守卫
  stores/       # pinia
  views/        # 页面
  permission/   # 权限指令、菜单过滤、动态路由
  utils/        # 请求、时间、校验
  types/        # 全局类型
```

强制要求：

- API 调用 MUST 集中在 `api/`；MUST NOT 在页面中散落 axios。
- 全局状态 MUST 放 pinia；页面局部状态 MUST NOT 强行塞入全局 store。
- 权限逻辑 MUST 集中在 `permission/` 与路由守卫；MUST NOT 每个页面重复手写。
- DTO 类型 MUST 与后端 OpenAPI 对齐；MUST NOT 使用 `any` 逃避类型。

---

## 4. 跨端契约强制规范

1. 统一返回 MUST 按后端结构解析：

   ```json
   { "message": "xxx", "body": {}, "request_id": "01H...", "code": 0 }
   ```

2. `code === 0` 表示成功；非 0 MUST 按统一错误码提示，MUST NOT 只看 HTTP 状态。
3. 请求 MUST 透传或生成 `X-Request-ID`；错误提示 SHOULD 展示 `request_id` 便于排查。
4. ID MUST 按 string 处理；MUST NOT 用 number 解析 ULID。
5. 时间字段 MUST 按 int64 Unix 秒 UTC 接收；展示时 MUST 转换为用户本地时区。
6. 上传、配置、初始化等接口 MUST 以后端 OpenAPI 为准；MUST NOT 臆造字段。
7. 写操作 MUST 防重复提交；MUST NOT 擅自自动重试非幂等写请求。

---

## 5. 路由与权限强制规范

- MUST 使用 `vue-router` 路由守卫 + 动态路由实现权限。
- 登录后 MUST 根据后端返回的用户、角色、权限生成可访问路由与菜单。
- 无权限路由 MUST 跳转 403；不存在路由 MUST 跳转 404。
- 按钮级权限 MUST 使用 `v-permission` 指令或统一权限函数；MUST NOT 仅靠隐藏菜单做安全控制。
- 前端权限仅用于体验；MUST 明确后端仍必须做权限校验。
- 动态路由 MUST 处理刷新恢复；MUST NOT 刷新后丢失权限路由。
- 菜单 MUST 由权限路由生成；MUST NOT 硬编码与权限不一致的菜单。

---

## 6. 状态管理强制规范

- 用户会话、权限、菜单、系统配置摘要 MUST 使用 pinia 管理。
- token 存储 MUST 遵循项目安全方案；SHOULD 优先 httpOnly cookie 存刷新令牌，访问令牌避免长期落 localStorage。
- MUST NOT 将 SMTP 密码、OSS secret 等敏感配置存入前端持久化状态。
- store MUST 提供清晰 action；MUST NOT 在组件中直接改 store state 绕过 action。
- 退出登录 MUST 清理会话、权限路由、缓存和敏感状态。

---

## 7. API 层与错误处理强制规范

- MUST 统一 axios 实例：baseURL、超时、`X-Request-ID`、认证头、统一响应解析。
- MUST 统一处理 401、403、404、5xx 和业务 `code`。
- 401 MUST 清理会话并跳转登录；403 MUST 跳转无权限页。
- 错误提示 MUST 使用后端 `message`，并保留 `request_id`。
- MUST NOT 在页面中重复实现拦截器逻辑。
- 上传 MUST 前端校验大小/MIME 作为体验；MUST 明确后端仍必须校验。
- MUST NOT 在控制台输出完整响应中的敏感字段。

---

## 8. UI、表单与配置管理页面强制规范

- UI MUST 使用 TailwindCSS 完成；默认不引入组件库。
- 表单 MUST 有校验、loading、禁用重复提交、错误提示和成功反馈。
- 列表 MUST 有 loading、空状态、错误状态和分页。
- 认证页面 MUST 默认支持邮箱注册、邮箱验证、登录、忘记密码、重置密码、修改密码。
- 系统配置页面 MUST 支持：
  - SMTP 配置与“测试连接”；
  - OSS 配置与“测试连接”；
  - 本地图片存储配置；
  - Casbin 身份配置；
  - 初次启动初始化页面。
- 敏感配置 MUST 脱敏显示；留空表示不修改；MUST NOT 在页面回显明文密钥。
- 初始化页面 MUST 仅在未初始化时可访问；初始化后 MUST 隐藏/拒绝；MUST 支持初始化令牌输入。
- Casbin 配置变更 MUST 提示审计与影响范围；MUST NOT 提供无确认的通配符权限快捷操作。
- 上传组件 MUST 展示大小/MIME 限制；MUST NOT 信任客户端文件名。

---

## 9. 类型与质量强制规范

- TypeScript MUST 开启 `strict`；MUST NOT 滥用 `any`。
- API DTO、统一返回、分页、错误码 MUST 有类型。
- 时间转换 MUST 封装统一工具；MUST NOT 每个页面手写时区转换。
- 权限判断 MUST 封装统一函数/指令；MUST NOT 散落字符串比较。
- 组件 MUST 拆分职责；MUST NOT 单文件堆积无关业务。
- MUST NOT 在模板中写复杂业务逻辑；移动到 composable/store。

---

## 10. 测试与验收强制标准

改动后 MUST 通过（包管理器跟随 lock 文件）：

```bash
pnpm typecheck
pnpm lint
pnpm build
```

如项目无对应脚本，MUST 补充或报告。

关键页面 SHOULD 有组件/交互测试，至少覆盖：

- 登录、注册、忘记密码、重置密码表单校验；
- 401/403/404 路由行为；
- 动态路由刷新恢复；
- 按钮权限；
- SMTP/OSS 配置脱敏与留空不修改；
- 初始化页面仅未初始化可访问；
- 统一返回与 request_id 错误提示。

自测 dev server MUST 使用 5173，结束后 MUST 停止并确认端口释放。

---

## 11. 交付报告强制格式

MUST 包含：

1. 变更摘要与影响页面；
2. 新增/修改 API 调用及契约对齐说明；
3. 权限路由、菜单、按钮权限变更；
4. 配置页面与脱敏说明；
5. 测试命令与结果；
6. 启动/停止说明；
7. 未决问题与风险。

---

## 12. 禁止事项

- 禁止擅自更换 Vue3、TypeScript、TailwindCSS、vue-router、pinia。
- 禁止擅自引入组件库、升级依赖、删除测试、跳过类型检查。
- 禁止将 `vue-admin-template` 当作权限框架。
- 禁止只做菜单隐藏而不做路由/按钮权限。
- 禁止在页面散落 axios、重复实现拦截器。
- 禁止用 number 解析 ULID、返回本地时间字符串。
- 禁止回显 SMTP/OSS 明文密钥，禁止将敏感配置写入持久化状态或控制台。
- 禁止占用 8080、kill 非自己启动的进程。
- 禁止臆造后端字段或绕过统一返回结构。