# -*- coding: utf-8 -*-
"""批量下载 AI 光标图标 -> 去白底 -> 裁剪 -> 缩放到 64x64 PNG"""
import os, urllib.request
from PIL import Image
from collections import deque

OUT = r'D:\code\github\jobs-robot\apps\web\public\cursors'
TMP = r'D:\code\github\jobs-robot\design\cursor-system\ai'
os.makedirs(TMP, exist_ok=True)

ICONS = {
    'arrow':        'https://aka.doubaocdn.com/s/7lfFrGO7xV',
    'pointer':      'https://aka.doubaocdn.com/s/bd4Tzcu0TJ',
    'text':         'https://aka.doubaocdn.com/s/C4TFMC3yrF',
    'copy':         'https://aka.doubaocdn.com/s/VoUZEiXq5N',
    'move':         'https://aka.doubaocdn.com/s/Zqlavw70nQ',
    'not-allowed':  'https://aka.doubaocdn.com/s/JO5wJ07u8s',
    'grab':         'https://aka.doubaocdn.com/s/bd4Tzcu0TJ',
    'grabbing':     'https://aka.doubaocdn.com/s/56N6Aw7nQm',
    'progress':     'https://aka.doubaocdn.com/s/9bmw476tjE',
    'wait':         'https://aka.doubaocdn.com/s/z066hxoR7j',
    'resize-ew':    'https://aka.doubaocdn.com/s/yvNZjc0YRf',
    'resize-ns':    'https://aka.doubaocdn.com/s/UeZPO4BPLV',
    'resize-nesw':  'https://aka.doubaocdn.com/s/rg4Kv6afr4',
    'resize-nwse':  'https://aka.doubaocdn.com/s/Kxu3b4LzXh',
    'crosshair':    'https://aka.doubaocdn.com/s/nokcJEcGL0',
    'zoom-in':      'https://aka.doubaocdn.com/s/UJBPgAbAGq',
    'zoom-out':     'https://aka.doubaocdn.com/s/oZiFPfVFyN',
}

def remove_white_bg(im, thresh=222):
    im = im.convert('RGBA')
    w, h = im.size
    px = im.load()
    visited = [[False]*w for _ in range(h)]
    q = deque()
    for x in range(w):
        for y in (0, h-1):
            r,g,b,a = px[x,y]
            if r>=thresh and g>=thresh and b>=thresh:
                q.append((x,y)); visited[y][x]=True
    for y in range(h):
        for x in (0, w-1):
            r,g,b,a = px[x,y]
            if r>=thresh and g>=thresh and b>=thresh:
                q.append((x,y)); visited[y][x]=True
    while q:
        x,y = q.popleft()
        px[x,y] = (255,255,255,0)
        for dx,dy in ((1,0),(-1,0),(0,1),(0,-1)):
            nx,ny = x+dx, y+dy
            if 0<=nx<w and 0<=ny<h and not visited[ny][nx]:
                r,g,b,a = px[nx,ny]
                if r>=thresh and g>=thresh and b>=thresh:
                    visited[ny][nx]=True
                    q.append((nx,ny))
    return im

for name, url in ICONS.items():
    raw = os.path.join(TMP, name + '.png')
    if not os.path.exists(raw):
        urllib.request.urlretrieve(url, raw)
    im = Image.open(raw)
    cut = remove_white_bg(im)
    bbox = cut.getbbox()
    if bbox:
        cut = cut.crop(bbox)
    size = 32
    cut.thumbnail((size, size), Image.LANCZOS)
    canvas = Image.new('RGBA', (size, size), (0,0,0,0))
    canvas.paste(cut, ((size-cut.width)//2, (size-cut.height)//2), cut)
    canvas.save(os.path.join(OUT, name + '.png'))
    print(name, 'ok', canvas.size)
print('ALL DONE')
