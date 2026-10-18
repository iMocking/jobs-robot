# -*- coding: utf-8 -*-
"""生成 JobHub 光标系统 v2 —— 现代风格化 17 枚 SVG
设计语言：修长几何造型 / 渐变填充 + 深蓝外描边 + 白内描边（双层工艺）
线条三层：深蓝外 + 白 + 渐变芯；端头圆润；通透三停渐变
"""
import os

OUT = r'D:\code\github\jobs-robot\apps\web\public\cursors'
os.makedirs(OUT, exist_ok=True)

DEF = ('<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">'
       '<stop offset="0" stop-color="#7DD3FC"/><stop offset="0.55" stop-color="#3B82F6"/>'
       '<stop offset="1" stop-color="#2563EB"/></linearGradient>'
       '<linearGradient id="gr" x1="0" y1="0" x2="1" y2="1">'
       '<stop offset="0" stop-color="#FDA4AF"/><stop offset="1" stop-color="#E11D48"/>'
       '</linearGradient></defs>')

# 细长现代箭头（尖在 1,1，双层描边）
ARROW = (
    '<path d="M1,1 L1,19 L6.2,15.4 L9,21.6 L12.8,20.3 L10.2,14.9 L16.8,14.9 Z" '
    'fill="url(#g)" stroke="#1E3A8A" stroke-width="2.2" stroke-linejoin="round"/>'
    '<path d="M1,1 L1,19 L6.2,15.4 L9,21.6 L12.8,20.3 L10.2,14.9 L16.8,14.9 Z" '
    'fill="url(#g)" stroke="#FFFFFF" stroke-width="1.1" stroke-linejoin="round"/>'
)

# 现代纤瘦手型（四指圆润 + 拇指）
HAND = (
    '<path d="M8.4,11.4 L8.4,7.8 C8.4,6.9 9.1,6.2 10,6.2 C10.9,6.2 11.6,6.9 11.6,7.8 '
    'L11.6,10.8 L12.6,10.8 L12.6,4.6 C12.6,3.7 13.3,3 14.2,3 C15.1,3 15.8,3.7 '
    '15.8,4.6 L15.8,10.8 L16.8,10.8 L16.8,6.8 C16.8,5.9 17.5,5.2 18.4,5.2 '
    'C19.3,5.2 20,5.9 20,6.8 L20,12.6 C20,16.2 17.2,18.9 13.6,18.9 '
    'C10.5,18.9 8.4,17.1 7.8,14.3 C7.7,13.7 7.9,13.1 8.2,12.5 Z" '
    'fill="url(#g)" stroke="#1E3A8A" stroke-width="2" stroke-linejoin="round"/>'
    '<path d="M8.4,11.4 L8.4,7.8 C8.4,6.9 9.1,6.2 10,6.2 C10.9,6.2 11.6,6.9 11.6,7.8 '
    'L11.6,10.8 L12.6,10.8 L12.6,4.6 C12.6,3.7 13.3,3 14.2,3 C15.1,3 15.8,3.7 '
    '15.8,4.6 L15.8,10.8 L16.8,10.8 L16.8,6.8 C16.8,5.9 17.5,5.2 18.4,5.2 '
    'C19.3,5.2 20,5.9 20,6.8 L20,12.6 C20,16.2 17.2,18.9 13.6,18.9 '
    'C10.5,18.9 8.4,17.1 7.8,14.3 C7.7,13.7 7.9,13.1 8.2,12.5 Z" '
    'fill="url(#g)" stroke="#FFFFFF" stroke-width="1" stroke-linejoin="round"/>'
    '<path d="M7.4,13.6 C6.2,14 5.2,13.3 5.2,12.3 C5.2,11.3 6.1,10.6 7.1,10.9 L8.2,11.3" '
    'fill="url(#g)" stroke="#1E3A8A" stroke-width="2" stroke-linejoin="round"/>'
    '<path d="M7.4,13.6 C6.2,14 5.2,13.3 5.2,12.3 C5.2,11.3 6.1,10.6 7.1,10.9 L8.2,11.3" '
    'fill="url(#g)" stroke="#FFFFFF" stroke-width="1" stroke-linejoin="round"/>'
)

def line3(d):
    """三层描边线：深蓝外 + 白 + 渐变芯"""
    return (
        '<path d="%s" fill="none" stroke="#1E3A8A" stroke-width="4.6" stroke-linecap="round"/>'
        '<path d="%s" fill="none" stroke="#FFFFFF" stroke-width="3.4" stroke-linecap="round"/>'
        '<path d="%s" fill="none" stroke="url(#g)" stroke-width="1.8" stroke-linecap="round"/>' % (d, d, d)
    )

def fill2(d):
    """双层描边填充：深蓝外 + 白内"""
    return (
        '<path d="%s" fill="url(#g)" stroke="#1E3A8A" stroke-width="1.4" stroke-linejoin="round"/>'
        '<path d="%s" fill="url(#g)" stroke="#FFFFFF" stroke-width="0.7" stroke-linejoin="round"/>' % (d, d)
    )

CURSORS = {
    'arrow': ARROW,
    'pointer': HAND,
    'grab': HAND,
    'text': (
        line3('M12,3.6 L12,20.4') +
        '<circle cx="12" cy="4" r="1.9" fill="url(#g)" stroke="#1E3A8A" stroke-width="1.4"/>'
        '<circle cx="12" cy="4" r="1.9" fill="url(#g)" stroke="#FFFFFF" stroke-width="0.7"/>'
        '<circle cx="12" cy="20" r="1.9" fill="url(#g)" stroke="#1E3A8A" stroke-width="1.4"/>'
        '<circle cx="12" cy="20" r="1.9" fill="url(#g)" stroke="#FFFFFF" stroke-width="0.7"/>'
    ),
    'copy': ARROW + (
        '<rect x="14.9" y="14.9" width="6" height="6" rx="1.8" '
        'fill="url(#g)" stroke="#1E3A8A" stroke-width="1.4"/>'
        '<rect x="14.9" y="14.9" width="6" height="6" rx="1.8" '
        'fill="url(#g)" stroke="#FFFFFF" stroke-width="0.7"/>'
        '<path d="M17.9,16.4 L17.9,19.4 M16.4,17.9 L19.4,17.9" stroke="#FFFFFF" '
        'stroke-width="1.3" stroke-linecap="round"/>'
    ),
    'move': (
        '<circle cx="12" cy="12" r="1.5" fill="url(#g)" stroke="#1E3A8A" stroke-width="1.4"/>'
        '<circle cx="12" cy="12" r="1.5" fill="url(#g)" stroke="#FFFFFF" stroke-width="0.7"/>'
        + line3('M12,4 L12,7.5 M12,16.5 L12,20 M4,12 L7.5,12 M16.5,12 L20,12') +
        fill2('M12,3 L10.4,6.2 L13.6,6.2 Z M12,21 L10.4,17.8 L13.6,17.8 Z '
              'M3,12 L6.2,10.4 L6.2,13.6 Z M21,12 L17.8,10.4 L17.8,13.6 Z')
    ),
    'not-allowed': (
        '<circle cx="12" cy="12" r="8.6" fill="none" stroke="#FDA4AF" stroke-width="6"/>'
        '<circle cx="12" cy="12" r="8.6" fill="none" stroke="url(#gr)" stroke-width="3.2"/>'
        '<path d="M6.6,17.4 L17.4,6.6" fill="none" stroke="#1E3A8A" stroke-width="3.6" stroke-linecap="round"/>'
        '<path d="M6.6,17.4 L17.4,6.6" fill="none" stroke="#FFFFFF" stroke-width="2.4" stroke-linecap="round"/>'
        '<path d="M6.6,17.4 L17.4,6.6" fill="none" stroke="url(#gr)" stroke-width="1.1" stroke-linecap="round"/>'
    ),
    'grabbing': (
        '<rect x="7.9" y="6.9" width="8.4" height="10.6" rx="4.6" '
        'fill="url(#g)" stroke="#1E3A8A" stroke-width="1.8"/>'
        '<rect x="7.9" y="6.9" width="8.4" height="10.6" rx="4.6" '
        'fill="url(#g)" stroke="#FFFFFF" stroke-width="0.9"/>'
        '<path d="M10.1,8 L10.1,16.6" stroke="#FFFFFF" stroke-width="0.7" opacity="0.95"/>'
        '<path d="M13,8 L13,16.6" stroke="#FFFFFF" stroke-width="0.7" opacity="0.95"/>'
        '<path d="M7.9,10.2 C6.1,10.5 5.1,11.8 5.4,13.4 C5.7,15 7.3,15.7 8.8,15" '
        'fill="url(#g)" stroke="#1E3A8A" stroke-width="1.8" stroke-linejoin="round"/>'
        '<path d="M7.9,10.2 C6.1,10.5 5.1,11.8 5.4,13.4 C5.7,15 7.3,15.7 8.8,15" '
        'fill="url(#g)" stroke="#FFFFFF" stroke-width="0.9" stroke-linejoin="round"/>'
    ),
    'progress': ARROW + (
        '<path d="M17.3,13.2 A4.5,4.5 0 1 1 13.4,17.5" fill="none" stroke="#1E3A8A" '
        'stroke-width="4.4" stroke-linecap="round"/>'
        '<path d="M17.3,13.2 A4.5,4.5 0 1 1 13.4,17.5" fill="none" stroke="#FFFFFF" '
        'stroke-width="3.2" stroke-linecap="round"/>'
        '<path d="M17.3,13.2 A4.5,4.5 0 1 1 13.4,17.5" fill="none" stroke="url(#g)" '
        'stroke-width="1.7" stroke-linecap="round"/>'
    ),
    'wait': (
        '<circle cx="12" cy="12" r="8" fill="none" stroke="#1E3A8A" stroke-width="4.8"/>'
        '<circle cx="12" cy="12" r="8" fill="none" stroke="#FFFFFF" stroke-width="3.4"/>'
        '<circle cx="12" cy="12" r="8" fill="none" stroke="url(#g)" stroke-width="1.8"/>'
        '<circle cx="12" cy="12" r="1.4" fill="url(#g)" stroke="#1E3A8A" stroke-width="1.2"/>'
        '<circle cx="12" cy="12" r="1.4" fill="url(#g)" stroke="#FFFFFF" stroke-width="0.6"/>'
    ),
    'resize-ew': (
        line3('M3.6,12 L20.4,12') +
        fill2('M3.6,12 L6.8,9.5 L6.8,14.5 Z M20.4,12 L17.2,9.5 L17.2,14.5 Z')
    ),
    'resize-ns': (
        line3('M12,3.6 L12,20.4') +
        fill2('M12,3.6 L9.5,6.8 L14.5,6.8 Z M12,20.4 L9.5,17.2 L14.5,17.2 Z')
    ),
    'resize-nesw': (
        line3('M4.8,19.2 L19.2,4.8') +
        fill2('M4.8,19.2 L7.2,16.6 L7.8,19.4 Z M19.2,4.8 L16.6,7.2 L19.4,7.8 Z')
    ),
    'resize-nwse': (
        line3('M4.8,4.8 L19.2,19.2') +
        fill2('M4.8,4.8 L7.2,7.4 L4.4,7.8 Z M19.2,19.2 L16.6,16.8 L19.4,16.2 Z')
    ),
    'crosshair': (
        line3('M12,3.6 L12,9 M12,15 L12,20.4 M3.6,12 L9,12 M15,12 L20.4,12') +
        '<circle cx="12" cy="12" r="1.7" fill="url(#g)" stroke="#1E3A8A" stroke-width="1.4"/>'
        '<circle cx="12" cy="12" r="1.7" fill="url(#g)" stroke="#FFFFFF" stroke-width="0.7"/>'
    ),
    'zoom-in': (
        '<circle cx="10.6" cy="10.6" r="5.3" fill="url(#g)" stroke="#1E3A8A" stroke-width="1.8"/>'
        '<circle cx="10.6" cy="10.6" r="5.3" fill="url(#g)" stroke="#FFFFFF" stroke-width="0.9"/>'
        + line3('M14.5,14.5 L19.6,19.6') +
        '<path d="M10.6,8.1 L10.6,13.1 M8.1,10.6 L13.1,10.6" stroke="#FFFFFF" '
        'stroke-width="2.6" stroke-linecap="round"/>'
        '<path d="M10.6,8.1 L10.6,13.1 M8.1,10.6 L13.1,10.6" stroke="#2563EB" '
        'stroke-width="1.2" stroke-linecap="round"/>'
    ),
    'zoom-out': (
        '<circle cx="10.6" cy="10.6" r="5.3" fill="url(#g)" stroke="#1E3A8A" stroke-width="1.8"/>'
        '<circle cx="10.6" cy="10.6" r="5.3" fill="url(#g)" stroke="#FFFFFF" stroke-width="0.9"/>'
        + line3('M14.5,14.5 L19.6,19.6') +
        '<path d="M8.1,10.6 L13.1,10.6" stroke="#FFFFFF" stroke-width="2.6" stroke-linecap="round"/>'
        '<path d="M8.1,10.6 L13.1,10.6" stroke="#2563EB" stroke-width="1.2" stroke-linecap="round"/>'
    ),
}

for name, body in CURSORS.items():
    svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">'
           + DEF + body + '</svg>')
    with open(os.path.join(OUT, name + '.svg'), 'w', encoding='utf-8') as f:
        f.write(svg)
print('TOTAL', len(CURSORS))
