# -*- coding: utf-8 -*-
"""生成 JobHub 光标系统 v6 —— 参考图风格：锐利几何 + 蓝渐变填充 + 纯白描边 + 扁平
参考：ref-v6.png（锐利 miter、#A5F3FC→#3B82F6→#2563EB 蓝渐变、白描边、无辉光）
"""
import os
import re

OUT = r'D:\code\github\jobs-robot\apps\web\public\cursors'
os.makedirs(OUT, exist_ok=True)

DEF = ('<defs>'
       '<linearGradient id="g" x1="0" y1="0" x2="1" y2="1">'
       '<stop offset="0" stop-color="#A5F3FC"/><stop offset="0.5" stop-color="#3B82F6"/><stop offset="1" stop-color="#2563EB"/>'
       '</linearGradient>'
       '<linearGradient id="rg" x1="0" y1="0" x2="1" y2="1">'
       '<stop offset="0" stop-color="#FDA4AF"/><stop offset="1" stop-color="#E11D48"/>'
       '</linearGradient>'
       '</defs>')

OUTLINE = '#0F1B3D'

def blue_fill(d):
    """蓝渐变填充 + 白描边 + 深蓝细外轮廓（锐利 miter）"""
    return (
        '<path d="%s" fill="url(#g)" stroke="%s" stroke-width="2.2" stroke-linejoin="miter"/>'
        '<path d="%s" fill="url(#g)" stroke="#FFFFFF" stroke-width="1.2" stroke-linejoin="miter"/>'
        % (d, OUTLINE, d)
    )

def blue_line(d, w=1.3):
    """蓝芯线 + 白描边 + 深蓝外（锐利）"""
    return (
        '<path d="%s" fill="none" stroke="%s" stroke-width="%s" stroke-linecap="butt"/>'
        '<path d="%s" fill="none" stroke="#FFFFFF" stroke-width="%s" stroke-linecap="butt"/>'
        '<path d="%s" fill="none" stroke="url(#g)" stroke-width="%s" stroke-linecap="butt"/>'
        % (d, OUTLINE, w + 1.6, d, w + 0.7, d, w)
    )

def blue_circle(cx, cy, r, w=1.3):
    return (
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="%s"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="#FFFFFF" stroke-width="%s"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="url(#g)" stroke-width="%s"/>'
        % (cx, cy, r, OUTLINE, w + 1.6, cx, cy, r, w + 0.7, cx, cy, r, w)
    )

def blue_dot(cx, cy, r):
    return (
        '<circle cx="%s" cy="%s" r="%s" fill="url(#g)"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="#FFFFFF" stroke-width="1.1"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="0.5"/>'
        % (cx, cy, r, cx, cy, r, cx, cy, r, OUTLINE)
    )

def shift_path(p, dx=4, dy=4):
    nums = re.findall(r'[-+]?\d*\.?\d+', p)
    out = []
    for i, n in enumerate(nums):
        v = float(n) + (dx if i % 2 == 0 else dy)
        out.append(('%g' % v) if v == int(v) else ('%.1f' % v))
    it = iter(out)
    return re.sub(r'[-+]?\d*\.?\d+', lambda m: next(it), p)

# 细长锐利箭头（参考 pointer：尖+竖边+小尾翼）24 系
ARROW24 = 'M1,1 L1,18.5 L6.8,15.2 L9,20.8 L12.6,19.6 L10.4,14.6 L15.8,14.6 Z'
ARROW = blue_fill(shift_path(ARROW24))

# 食指点手型（参考 hand）24 系
HAND24 = (
    'M9,12 L9,6.5 C9,5.5 9.7,5 10.5,5 C11.3,5 12,5.5 12,6.5 L12,10.5 '
    'L13,10.5 L13,4.5 C13,3.5 13.7,3 14.5,3 C15.3,3 16,3.5 16,4.5 L16,10.5 '
    'L17,10.5 L17,6.5 C17,5.5 17.7,5 18.5,5 C19.3,5 20,5.5 20,6.5 L20,12.5 '
    'C20,15.5 17.5,18 14,18 C11,18 9,16.5 8.3,14 C8.1,13.3 8.3,12.5 9,12 Z'
)
HAND = blue_fill(shift_path(HAND24))

CURSORS = {
    'arrow': ARROW,
    'pointer': HAND,
    'grab': HAND,
    'text': (
        blue_line('M16,7 L16,25')
    ),
    'copy': (ARROW
             + '<rect x="20" y="20" width="7" height="7" fill="url(#g)"/>'
             + '<rect x="20" y="20" width="7" height="7" fill="none" stroke="%s" stroke-width="1.6"/>' % OUTLINE
             + '<rect x="20" y="20" width="7" height="7" fill="none" stroke="#FFFFFF" stroke-width="1"/>'
             + '<path d="M23.5,21.6 L23.5,25.4 M21.6,23.5 L25.4,23.5" stroke="#FFFFFF" stroke-width="1.4"/>'
             + '<path d="M23.5,21.6 L23.5,25.4 M21.6,23.5 L25.4,23.5" stroke="%s" stroke-width="0.5"/>' % OUTLINE
             ),
    'move': (
        blue_line('M16,5.5 L16,9 M16,23 L16,26.5 M5.5,16 L9,16 M23,16 L26.5,16')
        + blue_fill('M16,4 L13.8,8 L18.2,8 Z M16,28 L13.8,24 L18.2,24 Z '
                    'M4,16 L8,13.8 L8,18.2 Z M28,16 L24,13.8 L24,18.2 Z')
        + blue_fill('M16,13.2 L18.8,16 L16,18.8 L13.2,16 Z')
    ),
    'not-allowed': (
        blue_circle(16, 16, 11)
        + '<path d="M10.2,21.8 L21.8,10.2" stroke="%s" stroke-width="2.4"/>' % OUTLINE
        + '<path d="M10.2,21.8 L21.8,10.2" stroke="#FFFFFF" stroke-width="1.6"/>'
        + '<path d="M10.2,21.8 L21.8,10.2" stroke="url(#rg)" stroke-width="0.9"/>'
    ),
    'grabbing': (
        '<rect x="12.2" y="11.5" width="7.6" height="9.8" fill="url(#g)"/>'
        + ('<rect x="12.2" y="11.5" width="7.6" height="9.8" fill="none" stroke="%s" stroke-width="1.6"/>' % OUTLINE)
        + '<rect x="12.2" y="11.5" width="7.6" height="9.8" fill="none" stroke="#FFFFFF" stroke-width="1"/>'
    ),
    'progress': (ARROW + blue_circle(21.5, 17.5, 4.2)),
    'wait': (blue_circle(16, 16, 9.5, w=1.1) + blue_dot(16, 16, 1.8)),
    'resize-ew': (blue_line('M8,16 L24,16')
                  + blue_fill('M8,16 L11.5,13.2 L11.5,18.8 Z M24,16 L20.5,13.2 L20.5,18.8 Z')),
    'resize-ns': (blue_line('M16,8 L16,24')
                  + blue_fill('M16,8 L13.2,11.5 L18.8,11.5 Z M16,24 L13.2,20.5 L18.8,20.5 Z')),
    'resize-nesw': (blue_line('M9.8,22.2 L22.2,9.8')
                    + blue_fill('M9.8,22.2 L12.4,19.8 L13.2,22.4 Z M22.2,9.8 L19.8,12.4 L22.4,13.2 Z')),
    'resize-nwse': (blue_line('M9.8,9.8 L22.2,22.2')
                    + blue_fill('M9.8,9.8 L12.4,12.4 L9.8,13.2 Z M22.2,22.2 L19.6,19.6 L22.2,18.8 Z')),
    'crosshair': (blue_line('M16,7.5 L16,11.5 M16,20.5 L16,24.5 M7.5,16 L11.5,16 M20.5,16 L24.5,16')
                  + blue_dot(16, 16, 2)),
    'zoom-in': (blue_circle(15, 15, 6.8)
                + blue_line('M20.4,20.4 L25,25', w=1.2)
                + '<path d="M15,11.8 L15,18.2 M11.8,15 L18.2,15" stroke="%s" stroke-width="2"/>' % OUTLINE
                + '<path d="M15,11.8 L15,18.2 M11.8,15 L18.2,15" stroke="#FFFFFF" stroke-width="1.4"/>'
                + '<path d="M15,11.8 L15,18.2 M11.8,15 L18.2,15" stroke="url(#g)" stroke-width="0.8"/>'
                ),
    'zoom-out': (blue_circle(15, 15, 6.8)
                 + blue_line('M20.4,20.4 L25,25', w=1.2)
                 + '<path d="M11.8,15 L18.2,15" stroke="%s" stroke-width="2"/>' % OUTLINE
                 + '<path d="M11.8,15 L18.2,15" stroke="#FFFFFF" stroke-width="1.4"/>'
                 + '<path d="M11.8,15 L18.2,15" stroke="url(#g)" stroke-width="0.8"/>'
                 ),
}

for name, body in CURSORS.items():
    svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">'
           + DEF + body + '</svg>')
    with open(os.path.join(OUT, name + '.svg'), 'w', encoding='utf-8') as f:
        f.write(svg)
print('TOTAL', len(CURSORS))
