# JobHub 光标系统 v2（现代风格化）— 设计 Brief

## 背景与变更
- v1（已交付画布 `cMUYgQqW-4LSn3reBG6-0201`）被用户判定"太普通、太 Windows 风格"。
- v2 目标：**现代、风格化的审美**，摆脱系统默认光标轮廓的既视感，建立品牌化设计语言。
- 仍为 pure-coding 路线：17 枚 SVG（public/cursors/*.svg）+ style.css 变量引用；风格板同步现代化。

## 现代风格设计语言（核心变化）
1. **修长几何造型**：箭头不再使用 Windows 粗轮廓，改为细长锐利的现代指针（长尾、小角度、圆角尾端）；手型更纤瘦流畅；所有图标更紧凑、几何化。
2. **双层描边工艺**：主体 = 渐变填充 + 深蓝外描边（#1E3A8A，宽约 2）+ 白色内描边（宽约 1.1）→ 任意背景上"描边+轮廓"双保险，产生悬浮质感（替代滤镜投影，规避 cursor 渲染兼容风险）。
3. **线条三层工艺**：细线元素 = 深蓝外 4.6 + 白 3.4 + 渐变芯 1.8（圆头），比 v1 的白粗+渐变细更精致。
4. **渐变**：保持品牌蓝家族但更通透：`#7DD3FC → #3B82F6 → #2563EB` 三停渐变；禁用态柔和红 `#FDA4AF → #E11D48`。
5. **端头圆润**：linecap/linejoin round；I-beam 用纤细竖线 + 上下圆点收尾；copy 用圆角方块 + 白加号；move/crosshair 中心圆点。

## 状态清单（17 枚，与 v1 相同覆盖）
arrow / pointer / text / copy / move / not-allowed / grab / grabbing / progress / wait / resize-ew / resize-ns / resize-nesw / resize-nwse / crosshair / zoom-in / zoom-out

## 热点
- arrow / copy / progress：箭头尖 (1,1)（新箭头路径尖保留在 (1,1)，CSS 无需改）
- pointer / grab：指尖 (14,3)（手型最高指尖约 (14.3,2.8)）→ style.css 中 `--cur-pointer`/`--cur-grab` hotspot 更新
- 其余：中心 (12,12) 或 (11,11)，与 v1 一致

## 风格板（index.html）现代化
- 深色带紫调渐变背景（#0B1220 → #1E1B4B → #312E81 系）+ 玻璃拟态卡片（backdrop-blur、白 8% 底、圆角 20px、hover 上浮）
- 更大字号标题、letter-spacing；保持"悬停卡片体验对应光标"交互

## 验收
- 17 枚新 SVG 渲染：细长箭头、双层描边工艺、端头圆润、风格统一现代；
- style.css hotspot 更新后 computed cursor 全部生效；build 通过；
- 新风格板整页截图 → 上传 → 画布 `+create`（source_ids 引用 v1 母版 cMUYgQqW-4LSn3reBG6-0201）→ `+get` 回读 → present_files。
