# JobHub 光标系统 v6（参考图风格：锐利蓝填充白描边）— 设计 Brief

## 参考图分析（用户上传 ref-v6.png，350×208，5列×3行）
- **视觉语言**：锐利几何（miter join，棱角分明）、蓝色渐变填充、纯白描边、扁平干净无辉光、形状纤细简洁
- **关键图标造型**：pointer=细长锐利箭头（小尾翼）；busy=双层环；cross=细线十字+中心圆点；text=细 I-beam 竖线；hand=食指点手型；unavailable=四瓣花；vert/horz=竖向菱形/眼睛形；move=四向箭头+中心菱形；dgn=斜向双向箭头；link=手型
- **底色**：浅薄荷绿（#B8E6D0 系）——风格板底色对齐浅色

## v6 设计规格
1. **主体**：蓝色渐变填充 `#A5F3FC → #3B82F6 → #2563EB`（参考图浅蓝→深蓝）
2. **描边**：纯白描边 1.2px（miter join 锐利）+ 深蓝极细外轮廓 0.5px（#0F1B3D，保证浅背景可见）
3. **无辉光、无阴影**（扁平）
4. **形状**：保持标准可识别（箭头/手/I-beam/双向箭头/准星/放大镜），但全部锐利化、对齐参考图比例；箭头=细长锐利+小尾翼
5. **热点沿用**（viewBox 32，主体居中 24）：arrow/copy/progress 5,5；pointer/grab 18,7；中心 16,16；zoom 15,15 —— style.css 无需改
6. **风格板**：浅薄荷绿底 + 白色半透明卡片 + 深蓝文字（反转深色 HUD）

## 映射（参考图造型 → 17 标准状态）
- arrow=参考 pointer；progress=arrow+小环；wait=双层环；crosshair=细线十字+中心；
- text=细 I-beam；pointer/grab=手型；not-allowed=红圈斜线（锐利）；
- move=四向箭头+中心菱形；resize=锐利双向箭头；zoom=放大镜（锐利）

## 验收
- 17 枚 SVG：锐利 miter、蓝填充白描边、扁平无辉光；
- computed cursor 生效；build 通过；
- 新风格板 → 上传 → 画布 create → get → present_files。过程文件在 design/cursor-system/。
