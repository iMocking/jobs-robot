# -*- coding: utf-8 -*-
"""从 AI 生成的白底图标去除背景 -> 透明 PNG，并缩放到 cursor 尺寸"""
from PIL import Image
from collections import deque

def remove_white_bg(im, thresh=245):
    """从四角 floodfill 去白色背景"""
    im = im.convert('RGBA')
    w, h = im.size
    px = im.load()
    visited = [[False]*w for _ in range(h)]
    q = deque()
    # 从四角 + 边缘所有白点入队
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

im = Image.open(r'D:\code\github\jobs-robot\design\cursor-system\ai-arrow.png')
cut = remove_white_bg(im)
# 裁剪到内容边界
bbox = cut.getbbox()
cut = cut.crop(bbox)
# 缩放到 64x64（保持比例，居中）
size = 64
cut.thumbnail((size,size), Image.LANCZOS)
canvas = Image.new('RGBA', (size,size), (0,0,0,0))
canvas.paste(cut, ((size-cut.width)//2, (size-cut.height)//2), cut)
canvas.save(r'D:\code\github\jobs-robot\apps\web\public\cursors\arrow.png')
print('arrow.png saved', canvas.size, 'bbox', bbox)
