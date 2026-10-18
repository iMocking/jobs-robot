# -*- coding: utf-8 -*-
"""拼接 v5 风格板整页"""
from PIL import Image

base = r'D:\code\github\jobs-robot\apps\web'
w = 1027
total_h = 2009
seg = [(0, 'v5-f-0.png', 740), (740, 'v5-f-1.png', 1480), (1480, 'v5-f-2.png', 2009)]
full = Image.new('RGB', (w, total_h), (6, 11, 26))
for off, fname, end in seg:
    im = Image.open(base + '\\' + fname).convert('RGB')
    im = im.crop((0, 0, w, end - off))
    full.paste(im, (0, off))
full.save(base + '\\cursor-styleboard-v5.png')
print('saved', full.size)
