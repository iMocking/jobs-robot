# JobHub 光标系统 v3（科技·游戏风）— 设计 Brief

## 背景与变更
- v2（画布 `AxkTsY0y-4LSn3reeUk-0201`）被用户判定需进一步："科技风，偏游戏风格"。
- v3 目标：电竞 HUD / 赛博霓虹审美，明显区别于 v1 系统风、v2 现代风。

## 设计语言（核心变化）
1. **霓虹辉光工艺**：viewBox 24→**32**，主体居中 24 区域（偏移 +4），辉光以「宽半透明亮青描边堆叠」外溢到 24–32 边界内（晕外 3.4px@0.32 + 晕中 2px@0.5），不依赖 filter（规避 cursor 渲染兼容风险）。
2. **电光渐变**：`#A5F3FC → #22D3EE → #3B82F6 → #4F46E5`（青→蓝→紫四停）；禁用态霓虹红 `#FDA4AF → #F43F5E`。
3. **深核 + 亮芯**：主体 = 辉光晕 → 渐变填充 → 深核描边（#0A1535 1.9px）→ 白亮芯（0.7px @0.95）——高对比电子感。
4. **几何锐化**：箭头更长更尖（尾翼锐角）；crosshair 改为**游戏缺口准星**（十字线与中心断开 + 光点）；move 中心**菱形能量核** + 四向箭头；copy 霓虹圆角方块 + 白加号。
5. 线条/圆环同工艺（晕 4.6/2.8 + 渐变芯 1.6 + 白芯 0.5）。

## 热点（24 系 +4）
- arrow/copy/progress：尖 (5,5)
- pointer/grab：指尖 (18,7)
- text/move/not-allowed/grabbing/wait/resize-*/crosshair：中心 (16,16)
- zoom-in/out：镜心 (15,15)
→ style.css 全部 `--cur-*` hotspot 需整体更新。

## 风格板（index.html）游戏 HUD 化
- 深空背景 + CSS 网格线 + 星点 + 青紫霓虹光斑；卡片**斜切角**（clip-path）+ 霓虹描边 + hover 辉光；HUD 风格标题（大 letter-spacing 霓虹渐变文字）。

## 验收
- 17 枚新 SVG：霓虹辉光外溢可见、电光渐变、深核白芯、缺口准星等游戏细节；
- computed cursor 全部生效（热点更新）；build 通过；
- 新风格板整页 → 上传 → 画布 `+create`（source_ids 引用 v2 `AxkTsY0y-4LSn3reeUk-0201`）→ `+get` → present_files。
