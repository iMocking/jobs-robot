# -*- coding: utf-8 -*-
"""裁剪参考图 5列3行 = 15 个图标，放大 4 倍保存"""
from PIL import Image
import os

im = Image.open(r'D:\code\github\jobs-robot\design\cursor-system\ref-v6.png').convert('RGBA')
w, h = im.size
cols, rows = 5, 3
cw, rh = w/cols, h/rows
names = ['pointer','help','work','busy','cross',
         'text','hand','unavailable','vert','horz',
         'dgn1','dgn2','move','alternate','link']
outdir = r'D:\code\github\jobs-robot\design\cursor-system\refs'
os.makedirs(outdir, exist_ok=True)
for i, name in enumerate(names):
    r, c = divmod(i, cols)
    box = (int(c*cw), int(r*rh), int((c+1)*cw), int((r+1)*rh))
    cell = im.crop(box)
    # 裁掉标签文字（下半部分约 40% 是文字）
    iw, ih = cell.size
    cell = cell.crop((0, 0, iw, int(ih*0.62)))
    cell = cell.resize((cell.width*4, cell.height*4), Image.LANCZOS)
    cell.save(os.path.join(outdir, name + '.png'))
print('done', len(names))
