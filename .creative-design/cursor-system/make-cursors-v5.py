# -*- coding: utf-8 -*-
"""生成 JobHub 光标系统 v5 —— 鸣潮式 v4 基础上升级：菱形圆润箭头
v4 相同工艺（白+深轮廓+浅蓝亮线+克制辉光），箭头改为菱形轮廓（无尾翼）+ 全系圆角化
"""
import os
import re

OUT = r'D:\code\github\jobs-robot\apps\web\public\cursors'
os.makedirs(OUT, exist_ok=True)

DEF = ('<defs>'
       '<linearGradient id="b" x1="0" y1="0" x2="1" y2="1">'
       '<stop offset="0" stop-color="#93C5FD"/><stop offset="1" stop-color="#3B82F6"/></linearGradient>'
       '<linearGradient id="r" x1="0" y1="0" x2="1" y2="1">'
       '<stop offset="0" stop-color="#FCA5A5"/><stop offset="1" stop-color="#EF4444"/></linearGradient>'
       '</defs>')

OUTLINE = '#0F1B3D'
ACCENT = '#38BDF8'

def white_fill(d):
    """白主体 + 深外轮廓 + 浅蓝内亮线 + 克制辉光（圆角 join）"""
    return (
        '<path d="%s" fill="none" stroke="%s" stroke-width="1.8" opacity="0.22" stroke-linejoin="round"/>'
        '<path d="%s" fill="#FFFFFF" stroke-linejoin="round"/>'
        '<path d="%s" fill="none" stroke="%s" stroke-width="1.3" stroke-linejoin="round"/>'
        '<path d="%s" fill="none" stroke="%s" stroke-width="0.55" stroke-linejoin="round"/>'
        % (d, ACCENT, d, d, OUTLINE, d, ACCENT)
    )

def white_line(d, w=1.4, accent=0.5):
    return (
        '<path d="%s" fill="none" stroke="%s" stroke-width="%s" opacity="0.2" stroke-linecap="round"/>'
        '<path d="%s" fill="none" stroke="#FFFFFF" stroke-width="%s" stroke-linecap="round"/>'
        '<path d="%s" fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round"/>'
        '<path d="%s" fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round"/>'
        % (d, ACCENT, w + 1.6, d, w + 1.0, d, OUTLINE, w, d, ACCENT, accent)
    )

def white_circle(cx, cy, r, w=1.4):
    return (
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="%s" opacity="0.2"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="#FFFFFF" stroke-width="%s"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="%s"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="%s"/>'
        % (cx, cy, r, ACCENT, w + 1.8, cx, cy, r, w + 0.9,
           cx, cy, r, OUTLINE, w, cx, cy, r, ACCENT, 0.5)
    )

def white_dot(cx, cy, r):
    return (
        '<circle cx="%s" cy="%s" r="%s" fill="#FFFFFF"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="1.1"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="0.45"/>'
        % (cx, cy, r, cx, cy, r, OUTLINE, cx, cy, r, ACCENT)
    )

def shift_path(p, dx=4, dy=4):
    nums = re.findall(r'[-+]?\d*\.?\d+', p)
    out = []
    for i, n in enumerate(nums):
        v = float(n) + (dx if i % 2 == 0 else dy)
        out.append(('%g' % v) if v == int(v) else ('%.1f' % v))
    it = iter(out)
    return re.sub(r'[-+]?\d*\.?\d+', lambda m: next(it), p)

# 菱形圆润箭头（24 系 4 点菱形：尖 1,1 · 左翼 4.8,15.8 · 尾点 11.5,20.2 · 右翼 18.2,15.8）
ARROW24 = 'M1,1 L4.8,15.8 L11.5,20.2 L18.2,15.8 Z'
ARROW = white_fill(shift_path(ARROW24))

# 纤细手型（24 系）
HAND24 = (
    'M8.4,11.4 L8.4,7.9 C8.4,7 9.1,6.3 10,6.3 C10.9,6.3 11.6,7 11.6,7.9 '
    'L11.6,10.9 L12.6,10.9 L12.6,4.8 C12.6,3.9 13.3,3.2 14.2,3.2 C15.1,3.2 15.8,3.9 '
    '15.8,4.8 L15.8,10.9 L16.8,10.9 L16.8,7 C16.8,6.1 17.5,5.4 18.4,5.4 '
    'C19.3,5.4 20,6.1 20,7 L20,12.7 C20,16.1 17.3,18.8 13.7,18.8 '
    'C10.6,18.8 8.5,17 7.9,14.3 C7.8,13.7 8,13.1 8.3,12.5 Z'
)
THUMB24 = 'M7.4,13.7 C6.2,14.1 5.2,13.4 5.2,12.4 C5.2,11.4 6.1,10.7 7.1,11 L8.2,11.4'
HAND = white_fill(shift_path(HAND24)) + white_fill(shift_path(THUMB24))

CURSORS = {
    'arrow': ARROW,
    'pointer': HAND,
    'grab': HAND,
    'text': (
        white_line('M16,7.2 L16,24.8')
        + '<path d="M12.6,5.6 L19.4,5.6 M12.6,26.4 L19.4,26.4" fill="none" '
          'stroke="#FFFFFF" stroke-width="1.6" stroke-linecap="round"/>'
        + '<path d="M12.6,5.6 L19.4,5.6 M12.6,26.4 L19.4,26.4" fill="none" '
          'stroke="%s" stroke-width="0.6" stroke-linecap="round"/>' % ACCENT
    ),
    'copy': (ARROW
             + '<rect x="19.8" y="19.8" width="7.2" height="7.2" rx="1.6" fill="#FFFFFF"/>'
             + '<rect x="19.8" y="19.8" width="7.2" height="7.2" rx="1.6" fill="none" '
               'stroke="%s" stroke-width="1.2"/>' % OUTLINE
             + '<rect x="19.8" y="19.8" width="7.2" height="7.2" rx="1.6" fill="none" '
               'stroke="%s" stroke-width="0.5"/>' % ACCENT
             + '<path d="M23.4,21.4 L23.4,25.4 M21.4,23.4 L25.4,23.4" '
               'stroke="%s" stroke-width="1.5" stroke-linecap="round"/>' % ACCENT
             + '<path d="M23.4,21.4 L23.4,25.4 M21.4,23.4 L25.4,23.4" '
               'stroke="#0F1B3D" stroke-width="0.45" stroke-linecap="round"/>'),
    'move': (
        white_line('M16,5.8 L16,8.8 M16,23.2 L16,26.2 M5.8,16 L8.8,16 M23.2,16 L26.2,16')
        + white_fill('M16,4 L14.2,7.4 L17.8,7.4 Z M16,28 L14.2,24.6 L17.8,24.6 Z '
                     'M4,16 L7.4,14.2 L7.4,17.8 Z M28,16 L24.6,14.2 L24.6,17.8 Z')
        + white_fill('M16,13 L19,16 L16,19 L13,16 Z')
    ),
    'not-allowed': (
        white_circle(16, 16, 11)
        + '<path d="M10.2,21.8 L21.8,10.2" fill="none" stroke="#FFFFFF" stroke-width="2.2" stroke-linecap="round"/>'
        + '<path d="M10.2,21.8 L21.8,10.2" fill="none" stroke="url(#r)" stroke-width="1.1" stroke-linecap="round"/>'
        + '<path d="M10.2,21.8 L21.8,10.2" fill="none" stroke="%s" stroke-width="0.4" stroke-linecap="round"/>' % OUTLINE
    ),
    'grabbing': (
        '<rect x="12.2" y="11.2" width="8" height="10.2" rx="2.8" fill="#FFFFFF"/>'
        + '<rect x="12.2" y="11.2" width="8" height="10.2" rx="2.8" fill="none" '
          'stroke="%s" stroke-width="1.2"/>' % OUTLINE
        + '<rect x="12.2" y="11.2" width="8" height="10.2" rx="2.8" fill="none" '
          'stroke="%s" stroke-width="0.5"/>' % ACCENT
        + '<path d="M14.6,12.4 L14.6,20.2 M17.2,12.4 L17.2,20.2" stroke="%s" '
          'stroke-width="0.5" stroke-linecap="round"/>' % ACCENT
        + white_fill(shift_path('M7.9,10.4 C6.1,10.7 5.1,12 5.4,13.6 C5.7,15.2 7.3,15.9 8.8,15.2'))
    ),
    'progress': (ARROW + white_circle(21.3, 17.2, 4.5)),
    'wait': (white_circle(16, 16, 9.5) + white_dot(16, 16, 1.9)),
    'resize-ew': (white_line('M8,16 L24,16')
                  + white_fill('M8,16 L11.4,13.4 L11.4,18.6 Z M24,16 L20.6,13.4 L20.6,18.6 Z')),
    'resize-ns': (white_line('M16,8 L16,24')
                  + white_fill('M16,8 L13.4,11.4 L18.6,11.4 Z M16,24 L13.4,20.6 L18.6,20.6 Z')),
    'resize-nesw': (white_line('M9.8,22.2 L22.2,9.8')
                    + white_fill('M9.8,22.2 L12.2,19.7 L13,22.4 Z M22.2,9.8 L19.7,12.2 L22.4,13 Z')),
    'resize-nwse': (white_line('M9.8,9.8 L22.2,22.2')
                    + white_fill('M9.8,9.8 L12.2,12.2 L9.5,13 Z M22.2,22.2 L19.7,19.7 L22.4,19 Z')),
    'crosshair': (white_line('M16,7.4 L16,11.6 M16,20.4 L16,24.6 M7.4,16 L11.6,16 M20.4,16 L24.6,16')
                  + white_dot(16, 16, 2)),
    'zoom-in': (white_circle(15, 15, 6.8)
                + white_line('M20.4,20.4 L25,25', w=1.3)
                + '<path d="M15,11.8 L15,18.2 M11.8,15 L18.2,15" stroke="#FFFFFF" '
                  'stroke-width="1.8" stroke-linecap="round"/>'
                + '<path d="M15,11.8 L15,18.2 M11.8,15 L18.2,15" stroke="%s" '
                  'stroke-width="0.6" stroke-linecap="round"/>' % ACCENT
                + '<path d="M15,11.8 L15,18.2 M11.8,15 L18.2,15" stroke="%s" '
                  'stroke-width="1.6" opacity="0.2" stroke-linecap="round"/>' % ACCENT),
    'zoom-out': (white_circle(15, 15, 6.8)
                 + white_line('M20.4,20.4 L25,25', w=1.3)
                 + '<path d="M11.8,15 L18.2,15" stroke="#FFFFFF" stroke-width="1.8" stroke-linecap="round"/>'
                 + '<path d="M11.8,15 L18.2,15" stroke="%s" stroke-width="0.6" stroke-linecap="round"/>' % ACCENT),
}

for name, body in CURSORS.items():
    svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">'
           + DEF + body + '</svg>')
    with open(os.path.join(OUT, name + '.svg'), 'w', encoding='utf-8') as f:
        f.write(svg)
print('TOTAL', len(CURSORS))
