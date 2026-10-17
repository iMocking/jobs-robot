# JobHub 光标系统 v1 — 设计 Brief

## 主题与对象
- 主题：为 JobHub 直招 Web 前端（apps/web，Vue3 + Tailwind v4）设计一套完整、美观、统一视觉语言的自定义光标系统，覆盖常规光标的所有状态。
- 核心对象：无实体商品；核心视觉资产为"鼠标光标图标家族"（17 个状态）。视觉语言继承用户参考图与已验收光标：**浅蓝渐变（#7DD3FC → #2563EB）填充 + 白色 1.4 描边 + 圆润 linejoin**，禁用态使用品牌红渐变（#F87171 → #DC2626）。

## 受众与场景
- 使用者在 JobHub 全站（首页/职位/个人中心/admin）移动鼠标时的系统反馈；深色 hero 与浅色内容区均需清晰。

## 覆盖状态清单（17 个）
1. arrow（常规默认）— 经典鼠标箭头，渐变填充白描边，热点(1,1)
2. pointer（可点击）— 品牌蓝手型，热点指尖(13,2)
3. text（文本输入）— 品牌蓝 I-beam（白描边双层），热点(12,12)
4. copy（复制）— 箭头 + 右下白描边加号，热点(1,1)
5. move（移动）— 四向箭头，热点(12,12)
6. not-allowed（禁止）— 红渐变圆环 + 白色斜线，热点(12,12)
7. grab（抓取）— 张开手，热点(13,2)
8. grabbing（握紧）— 握拳 + 拇指，热点(12,12)
9. progress（进行中）— 箭头 + 开口圆环，热点(1,1)
10. wait（等待）— 渐变圆环 + 中心点，热点(12,12)
11. resize-ew / 12. resize-ns / 13. resize-nesw / 14. resize-nwse（四向缩放）— 双向箭头
15. crosshair（精确选择）— 十字准星，热点(12,12)
16. zoom-in（放大）— 渐变放大镜 + 白描边加号
17. zoom-out（缩小）— 渐变放大镜 + 白描边减号

## 视觉意图
- 统一"渐变蓝 + 白描边"家族语言（与用户已验收的箭头光标一致）；形状辨识优先（手型=可点击、加号=复制、红圈=禁止、四向=移动/缩放）。
- 每枚光标 24×24 viewBox，热点精确落位；fallback 关键字齐全（auto/pointer/text/copy/move/not-allowed/grab/grabbing/progress/wait/ew-resize 等），SVG 加载失败时回退系统光标。

## 落地方式
- 光标 SVG 以独立文件存放 `apps/web/public/cursors/*.svg`，`src/style.css` 用 CSS 变量 `--cur-*` + 工具类 `.cursor-*` 引用；默认态/可点击/文本/禁用四类自动生效，其余提供工具类按需使用。

## 验收重点
- 17 枚 SVG 渲染形状正确、风格统一（渐变+白描边）；
- 浏览器实测：默认=渐变箭头、按钮/链接=手型、输入框=I-beam、禁用=红圈、工具类（copy/move 等）逐一生效；
- 构建通过；设计风格板渲染并写入 Canvas。

## 备注
- 过程文件：本项目 `.creative-design/cursor-system/`；设计风格板截图作为 IMAGE 资产写入 Canvas `/deliverables/jobhub-ui/cursor-system`。
