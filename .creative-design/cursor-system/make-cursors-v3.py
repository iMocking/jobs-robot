# -*- coding: utf-8 -*-
"""生成 JobHub 光标系统 v3 —— 科技·游戏风 17 枚 SVG
viewBox 32x32，主体居中 24（偏移 +4），霓虹辉光外溢 + 电光渐变 + 深核白芯
"""
import os
import re

OUT = r'D:\code\github\jobs-robot\apps\web\public\cursors'
os.makedirs(OUT, exist_ok=True)

DEF = ('<defs>'
       '<linearGradient id="g" x1="0" y1="0" x2="1" y2="1">'
       '<stop offset="0" stop-color="#A5F3FC"/><stop offset="0.35" stop-color="#22D3EE"/>'
       '<stop offset="0.72" stop-color="#3B82F6"/><stop offset="1" stop-color="#4F46E5"/></linearGradient>'
       '<linearGradient id="gr" x1="0" y1="0" x2="1" y2="1">'
       '<stop offset="0" stop-color="#FDA4AF"/><stop offset="1" stop-color="#F43F5E"/></linearGradient>'
       '</defs>')

GLOW = '#67E8F9'   # 霓虹亮青
CORE = '#0A1535'   # 深核

def glow_fill(d, grad='url(#g)'):
    """辉光外晕(2层) + 渐变填充 + 深核描边 + 白亮芯"""
    return (
        '<path d="%s" fill="none" stroke="%s" stroke-width="3.4" opacity="0.32" stroke-linejoin="round"/>'
        '<path d="%s" fill="none" stroke="%s" stroke-width="2" opacity="0.5" stroke-linejoin="round"/>'
        '<path d="%s" fill="%s" stroke-linejoin="round"/>'
        '<path d="%s" fill="none" stroke="%s" stroke-width="1.9" stroke-linejoin="round"/>'
        '<path d="%s" fill="none" stroke="#FFFFFF" stroke-width="0.7" opacity="0.95" stroke-linejoin="round"/>'
        % (d, GLOW, d, GLOW, d, grad, d, CORE, d)
    )

def glow_line(d):
    return (
        '<path d="%s" fill="none" stroke="%s" stroke-width="4.6" opacity="0.3" stroke-linecap="round"/>'
        '<path d="%s" fill="none" stroke="%s" stroke-width="2.8" opacity="0.5" stroke-linecap="round"/>'
        '<path d="%s" fill="none" stroke="url(#g)" stroke-width="1.6" stroke-linecap="round"/>'
        '<path d="%s" fill="none" stroke="#FFFFFF" stroke-width="0.5" opacity="0.9" stroke-linecap="round"/>'
        % (d, GLOW, d, GLOW, d, d)
    )

def glow_circle(cx, cy, r):
    return (
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="4" opacity="0.3"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="2.6" opacity="0.5"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="url(#g)" stroke-width="1.4"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="#FFFFFF" stroke-width="0.5" opacity="0.9"/>'
        % (cx, cy, r, GLOW, cx, cy, r, GLOW, cx, cy, r, cx, cy, r)
    )

def glow_dot(cx, cy, r):
    return (
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="3" opacity="0.3"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="url(#g)"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s" stroke-width="1.5"/>'
        '<circle cx="%s" cy="%s" r="%s" fill="none" stroke="#FFFFFF" stroke-width="0.6" opacity="0.95"/>'
        % (cx, cy, r, GLOW, cx, cy, r, cx, cy, r, CORE, cx, cy, r)
    )

def glow_rect(x, y, w, h, rx):
    return (
        '<rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="none" stroke="%s" stroke-width="3.2" opacity="0.3"/>'
        '<rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="none" stroke="%s" stroke-width="1.9" opacity="0.5"/>'
        '<rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="url(#g)"/>'
        '<rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="none" stroke="%s" stroke-width="1.6"/>'
        '<rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="none" stroke="#FFFFFF" stroke-width="0.6" opacity="0.95"/>'
        % (x, y, w, h, rx, GLOW, x, y, w, h, rx, GLOW, x, y, w, h, rx,
           x, y, w, h, rx, CORE, x, y, w, h, rx)
    )

def shift_path(p, dx=4, dy=4):
    """路径全部数字成对 +dx/+dy（坐标平移）"""
    nums = re.findall(r'[-+]?\d*\.?\d+', p)
    out = []
    for i, n in enumerate(nums):
        v = float(n) + (dx if i % 2 == 0 else dy)
        out.append(('%g' % v) if v == int(v) else ('%.1f' % v))
    it = iter(out)
    return re.sub(r'[-+]?\d*\.?\d+', lambda m: next(it), p)

# 细长锐利能量箭头（24 系坐标，+4 后主体落 5..29）
ARROW24 = 'M1,1 L1,19 L6.4,15.6 L9.2,21.8 L12.9,20.5 L10.4,15 L17,15 Z'
ARROW = glow_fill(shift_path(ARROW24))

# 手型（24 系，平移 +4）
HAND24 = (
    'M8.4,11.4 L8.4,7.8 C8.4,6.9 9.1,6.2 10,6.2 C10.9,6.2 11.6,6.9 11.6,7.8 '
    'L11.6,10.8 L12.6,10.8 L12.6,4.6 C12.6,3.7 13.3,3 14.2,3 C15.1,3 15.8,3.7 '
    '15.8,4.6 L15.8,10.8 L16.8,10.8 L16.8,6.8 C16.8,5.9 17.5,5.2 18.4,5.2 '
    'C19.3,5.2 20,5.9 20,6.8 L20,12.6 C20,16.2 17.2,18.9 13.6,18.9 '
    'C10.5,18.9 8.4,17.1 7.8,14.3 C7.7,13.7 7.9,13.1 8.2,12.5 Z'
)
THUMB24 = 'M7.4,13.6 C6.2,14 5.2,13.3 5.2,12.3 C5.2,11.3 6.1,10.6 7.1,10.9 L8.2,11.3'
HAND = glow_fill(shift_path(HAND24)) + glow_fill(shift_path(THUMB24))

CURSORS = {
    'arrow': ARROW,
    'pointer': HAND,
    'grab': HAND,
    'text': (glow_line('M16,7.6 L16,24.4')
             + glow_dot(16, 7.6, 2.1) + glow_dot(16, 24.4, 2.1)),
    'copy': (ARROW + glow_rect(19.6, 19.6, 7, 7, 2)
             + '<path d="M23.1,21.2 L23.1,25 M20.8,23.1 L25.4,23.1" '
               'stroke="#FFFFFF" stroke-width="1.6" stroke-linecap="round"/>'),
    'move': (glow_line('M16,5.6 L16,8.6 M16,23.4 L16,26.4 M5.6,16 L8.6,16 M23.4,16 L26.4,16')
             + glow_fill('M16,3.8 L13.9,7.3 L18.1,7.3 Z M16,28.2 L13.9,24.7 L18.1,24.7 Z '
                         'M3.8,16 L7.3,13.9 L7.3,18.1 Z M28.2,16 L24.7,13.9 L24.7,18.1 Z')
             + glow_fill('M16,12.6 L19.4,16 L16,19.4 L12.6,16 Z')),
    'not-allowed': (
        '<circle cx="16" cy="16" r="11" fill="none" stroke="#F43F5E" stroke-width="4.6" opacity="0.28"/>'
        '<circle cx="16" cy="16" r="11" fill="none" stroke="#F43F5E" stroke-width="2.8" opacity="0.5"/>'
        '<circle cx="16" cy="16" r="11" fill="none" stroke="url(#gr)" stroke-width="1.5"/>'
        '<circle cx="16" cy="16" r="11" fill="none" stroke="#FFFFFF" stroke-width="0.5" opacity="0.9"/>'
        '<path d="M10.2,21.8 L21.8,10.2" fill="none" stroke="#F43F5E" stroke-width="4" opacity="0.3" stroke-linecap="round"/>'
        '<path d="M10.2,21.8 L21.8,10.2" fill="none" stroke="url(#gr)" stroke-width="2.2" stroke-linecap="round"/>'
        '<path d="M10.2,21.8 L21.8,10.2" fill="none" stroke="#FFFFFF" stroke-width="0.8" opacity="0.9" stroke-linecap="round"/>'
    ),
    'grabbing': (
        glow_rect(11.9, 10.9, 8.4, 10.6, 4.6)
        + '<path d="M14.1,12 L14.1,20.6" stroke="#FFFFFF" stroke-width="0.7" opacity="0.95"/>'
        + '<path d="M17,12 L17,20.6" stroke="#FFFFFF" stroke-width="0.7" opacity="0.95"/>'
        + glow_fill(shift_path('M7.9,10.2 C6.1,10.5 5.1,11.8 5.4,13.4 C5.7,15 7.3,15.7 8.8,15'))
    ),
    'progress': (ARROW +
        '<path d="M21.3,17.2 A4.5,4.5 0 1 1 17.4,21.5" fill="none" stroke="%s" '
        'stroke-width="4.2" opacity="0.3" stroke-linecap="round"/>'
        '<path d="M21.3,17.2 A4.5,4.5 0 1 1 17.4,21.5" fill="none" stroke="%s" '
        'stroke-width="2.6" opacity="0.5" stroke-linecap="round"/>'
        '<path d="M21.3,17.2 A4.5,4.5 0 1 1 17.4,21.5" fill="none" stroke="url(#g)" '
        'stroke-width="1.5" stroke-linecap="round"/>'
        '<path d="M21.3,17.2 A4.5,4.5 0 1 1 17.4,21.5" fill="none" stroke="#FFFFFF" '
        'stroke-width="0.5" opacity="0.9" stroke-linecap="round"/>' % (GLOW, GLOW)
    ),
    'wait': glow_circle(16, 16, 9.5) + glow_dot(16, 16, 1.8),
    'resize-ew': (glow_line('M8,16 L24,16')
                  + glow_fill('M8,16 L11.6,13.2 L11.6,18.8 Z M24,16 L20.4,13.2 L20.4,18.8 Z')),
    'resize-ns': (glow_line('M16,8 L16,24')
                  + glow_fill('M16,8 L13.2,11.6 L18.8,11.6 Z M16,24 L13.2,20.4 L18.8,20.4 Z')),
    'resize-nesw': (glow_line('M9.6,22.4 L22.4,9.6')
                    + glow_fill('M9.6,22.4 L12.2,19.8 L13,22.6 Z M22.4,9.6 L19.8,12.2 L22.6,13 Z')),
    'resize-nwse': (glow_line('M9.6,9.6 L22.4,22.4')
                    + glow_fill('M9.6,9.6 L12.2,12.2 L9.4,13 Z M22.4,22.4 L19.8,19.8 L22.6,19 Z')),
    'crosshair': (glow_line('M16,7 L16,11.4 M16,20.6 L16,25 M7,16 L11.4,16 M20.6,16 L25,16')
                  + glow_dot(16, 16, 2)),
    'zoom-in': (glow_circle(15, 15, 6.6)
                + glow_line('M19.8,19.8 L24.6,24.6')
                + '<path d="M15,11.6 L15,18.4 M11.6,15 L18.4,15" '
                  'stroke="#FFFFFF" stroke-width="2.2" stroke-linecap="round"/>'),
    'zoom-out': (glow_circle(15, 15, 6.6)
                 + glow_line('M19.8,19.8 L24.6,24.6')
                 + '<path d="M11.6,15 L18.4,15" '
                   'stroke="#FFFFFF" stroke-width="2.2" stroke-linecap="round"/>'),
}

for name, body in CURSORS.items():
    svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">'
           + DEF + body + '</svg>')
    with open(os.path.join(OUT, name + '.svg'), 'w', encoding='utf-8') as f:
        f.write(svg)
print('TOTAL', len(CURSORS))
