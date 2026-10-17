# -*- coding: utf-8 -*-
"""生成 JobHub 光标系统 17 枚 SVG（24x24，蓝渐变+白描边家族语言）"""
import os

OUT = r'D:\code\github\jobs-robot\apps\web\public\cursors'
os.makedirs(OUT, exist_ok=True)

DEF = ('<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">'
       '<stop offset="0" stop-color="#7DD3FC"/><stop offset="1" stop-color="#2563EB"/>'
       '</linearGradient><linearGradient id="gr" x1="0" y1="0" x2="1" y2="1">'
       '<stop offset="0" stop-color="#F87171"/><stop offset="1" stop-color="#DC2626"/>'
       '</linearGradient></defs>')

ARROW = ('<path d="M1,1 L1,22 L6.5,16.5 L9.5,24 L13.5,22.5 L10.6,15.4 L17,15.4 Z" '
         'fill="url(#g)" stroke="#FFFFFF" stroke-width="1.4" stroke-linejoin="round"/>')

HAND = ('<path d="M7.6,12.7 C7.1,13.2 6.2,13.1 5.9,12.4 C5.6,11.7 6.1,10.9 6.9,10.8 '
        'C7.3,10.7 7.7,10.8 8.1,11 L8.1,7.4 C8.1,6.6 8.8,5.9 9.6,5.9 C10.4,5.9 11.1,6.6 '
        '11.1,7.4 L11.1,10.8 L12.2,10.8 L12.2,3.6 C12.2,2.8 12.9,2.1 13.7,2.1 C14.5,2.1 '
        '15.2,2.8 15.2,3.6 L15.2,10.8 L16.3,10.8 L16.3,5.8 C16.3,5 17,4.3 17.8,4.3 '
        'C18.6,4.3 19.3,5 19.3,5.8 L19.3,12.6 C19.3,16.4 16.1,19.5 12.4,19.5 '
        'C9.5,19.5 7.3,17.4 6.9,14.7 C6.8,14 7.1,13.3 7.6,12.7 Z" '
        'fill="url(#g)" stroke="#FFFFFF" stroke-width="1.3" stroke-linejoin="round"/>')

CURSORS = {
    'arrow': ARROW,
    'pointer': HAND,
    'grab': HAND,
    'text': (
        '<path d="M11.1,4.4 L12.9,4.4 L12.9,6.6 L11.1,6.6 Z '
        'M11.1,17.4 L12.9,17.4 L12.9,19.6 L11.1,19.6 Z '
        'M11.5,6 L12.5,6 L12.5,18 L11.5,18 Z" fill="#FFFFFF"/>'
        '<path d="M11.6,4.9 L12.4,4.9 L12.4,6.1 L11.6,6.1 Z '
        'M11.6,17.9 L12.4,17.9 L12.4,19.1 L11.6,19.1 Z '
        'M11.8,6.2 L12.2,6.2 L12.2,17.8 L11.8,17.8 Z" fill="url(#g)"/>'
    ),
    'copy': ARROW + (
        '<path d="M15.2,17.2 L19.4,17.2 M17.3,15.1 L17.3,19.3" stroke="#FFFFFF" '
        'stroke-width="3" stroke-linecap="round"/>'
        '<path d="M15.2,17.2 L19.4,17.2 M17.3,15.1 L17.3,19.3" stroke="#2563EB" '
        'stroke-width="1.5" stroke-linecap="round"/>'
    ),
    'move': (
        '<path d="M12,2.5 L12,7.5 M12,16.5 L12,21.5 M2.5,12 L7.5,12 M16.5,12 L21.5,12" '
        'stroke="#FFFFFF" stroke-width="3.2" stroke-linecap="round"/>'
        '<path d="M12,2.5 L12,7.5 M12,16.5 L12,21.5 M2.5,12 L7.5,12 M16.5,12 L21.5,12" '
        'stroke="url(#g)" stroke-width="1.6" stroke-linecap="round"/>'
        '<path d="M12,1.8 L9.6,5.6 L14.4,5.6 Z M12,22.2 L9.6,18.4 L14.4,18.4 Z '
        'M1.8,12 L5.6,9.6 L5.6,14.4 Z M22.2,12 L18.4,9.6 L18.4,14.4 Z" '
        'fill="url(#g)" stroke="#FFFFFF" stroke-width="1" stroke-linejoin="round"/>'
    ),
    'not-allowed': (
        '<circle cx="12" cy="12" r="9.5" fill="url(#gr)" stroke="#FFFFFF" stroke-width="1.4"/>'
        '<path d="M6.2,17.8 L17.8,6.2" stroke="#FFFFFF" stroke-width="3.2" stroke-linecap="round"/>'
        '<path d="M6.2,17.8 L17.8,6.2" stroke="#FECACA" stroke-width="1.6" stroke-linecap="round"/>'
    ),
    'grabbing': (
        '<rect x="7.5" y="7" width="9" height="10.5" rx="4.8" '
        'fill="url(#g)" stroke="#FFFFFF" stroke-width="1.3"/>'
        '<path d="M7.5,10.5 C5.8,10.8 4.8,12 5.1,13.6 C5.4,15.2 7,15.9 8.6,15.2" '
        'fill="url(#g)" stroke="#FFFFFF" stroke-width="1.3" stroke-linejoin="round"/>'
    ),
    'progress': ARROW + (
        '<path d="M17.5,13.2 A4.8,4.8 0 1 1 13,17.8" fill="none" stroke="#FFFFFF" '
        'stroke-width="3.6" stroke-linecap="round"/>'
        '<path d="M17.5,13.2 A4.8,4.8 0 1 1 13,17.8" fill="none" stroke="url(#g)" '
        'stroke-width="1.9" stroke-linecap="round"/>'
    ),
    'wait': (
        '<circle cx="12" cy="12" r="8.5" fill="none" stroke="#FFFFFF" stroke-width="3.8"/>'
        '<circle cx="12" cy="12" r="8.5" fill="none" stroke="url(#g)" stroke-width="2"/>'
        '<circle cx="12" cy="12" r="1.7" fill="url(#g)" stroke="#FFFFFF" stroke-width="1"/>'
    ),
    'resize-ew': (
        '<path d="M3,12 L21,12" stroke="#FFFFFF" stroke-width="3.2" stroke-linecap="round"/>'
        '<path d="M3,12 L21,12" stroke="url(#g)" stroke-width="1.6" stroke-linecap="round"/>'
        '<path d="M3,12 L7.4,8.6 L7.4,15.4 Z M21,12 L16.6,8.6 L16.6,15.4 Z" '
        'fill="url(#g)" stroke="#FFFFFF" stroke-width="1" stroke-linejoin="round"/>'
    ),
    'resize-ns': (
        '<path d="M12,3 L12,21" stroke="#FFFFFF" stroke-width="3.2" stroke-linecap="round"/>'
        '<path d="M12,3 L12,21" stroke="url(#g)" stroke-width="1.6" stroke-linecap="round"/>'
        '<path d="M12,3 L8.6,7.4 L15.4,7.4 Z M12,21 L8.6,16.6 L15.4,16.6 Z" '
        'fill="url(#g)" stroke="#FFFFFF" stroke-width="1" stroke-linejoin="round"/>'
    ),
    'resize-nesw': (
        '<path d="M4.5,19.5 L19.5,4.5" stroke="#FFFFFF" stroke-width="3.2" stroke-linecap="round"/>'
        '<path d="M4.5,19.5 L19.5,4.5" stroke="url(#g)" stroke-width="1.6" stroke-linecap="round"/>'
        '<path d="M4.5,19.5 L7.3,16.3 L7.7,19.7 Z M19.5,4.5 L16.3,7.3 L19.7,7.7 Z" '
        'fill="url(#g)" stroke="#FFFFFF" stroke-width="1" stroke-linejoin="round"/>'
    ),
    'resize-nwse': (
        '<path d="M4.5,4.5 L19.5,19.5" stroke="#FFFFFF" stroke-width="3.2" stroke-linecap="round"/>'
        '<path d="M4.5,4.5 L19.5,19.5" stroke="url(#g)" stroke-width="1.6" stroke-linecap="round"/>'
        '<path d="M4.5,4.5 L7.3,7.7 L3.7,7.7 Z M19.5,19.5 L16.7,16.3 L20.3,16.3 Z" '
        'fill="url(#g)" stroke="#FFFFFF" stroke-width="1" stroke-linejoin="round"/>'
    ),
    'crosshair': (
        '<path d="M12,3 L12,8.5 M12,15.5 L12,21 M3,12 L8.5,12 M15.5,12 L21,12" '
        'stroke="#FFFFFF" stroke-width="3.2" stroke-linecap="round"/>'
        '<path d="M12,3 L12,8.5 M12,15.5 L12,21 M3,12 L8.5,12 M15.5,12 L21,12" '
        'stroke="url(#g)" stroke-width="1.6" stroke-linecap="round"/>'
        '<circle cx="12" cy="12" r="2.2" fill="url(#g)" stroke="#FFFFFF" stroke-width="1"/>'
    ),
    'zoom-in': (
        '<circle cx="10.5" cy="10.5" r="5.8" fill="url(#g)" stroke="#FFFFFF" stroke-width="1.4"/>'
        '<path d="M14.8,14.8 L20.2,20.2" stroke="#FFFFFF" stroke-width="3.2" stroke-linecap="round"/>'
        '<path d="M14.8,14.8 L20.2,20.2" stroke="url(#g)" stroke-width="1.6" stroke-linecap="round"/>'
        '<path d="M10.5,7.7 L10.5,13.3 M7.7,10.5 L13.3,10.5" stroke="#FFFFFF" '
        'stroke-width="2.6" stroke-linecap="round"/>'
        '<path d="M10.5,7.7 L10.5,13.3 M7.7,10.5 L13.3,10.5" stroke="#2563EB" '
        'stroke-width="1.2" stroke-linecap="round"/>'
    ),
    'zoom-out': (
        '<circle cx="10.5" cy="10.5" r="5.8" fill="url(#g)" stroke="#FFFFFF" stroke-width="1.4"/>'
        '<path d="M14.8,14.8 L20.2,20.2" stroke="#FFFFFF" stroke-width="3.2" stroke-linecap="round"/>'
        '<path d="M14.8,14.8 L20.2,20.2" stroke="url(#g)" stroke-width="1.6" stroke-linecap="round"/>'
        '<path d="M7.7,10.5 L13.3,10.5" stroke="#FFFFFF" stroke-width="2.6" stroke-linecap="round"/>'
        '<path d="M7.7,10.5 L13.3,10.5" stroke="#2563EB" stroke-width="1.2" stroke-linecap="round"/>'
    ),
}

for name, body in CURSORS.items():
    svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">'
           + DEF + body + '</svg>')
    with open(os.path.join(OUT, name + '.svg'), 'w', encoding='utf-8') as f:
        f.write(svg)
    print(name, 'ok', len(svg))
print('TOTAL', len(CURSORS))
