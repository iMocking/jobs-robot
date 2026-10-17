# -*- coding: utf-8 -*-
"""拼接 v3 风格板整页"""
from PIL import Image

base = r'D:\code\github\jobs-robot\apps\web'
w = 1027
total_h = 2044
seg = [(0, 'v3-f-0.png', 740), (740, 'v3-f-1.png', 1480), (1480, 'v3-f-2.png', 2044)]
full = Image.new('RGB', (w, total_h), (4, 7, 15))
for off, fname, end in seg:
    im = Image.open(base + '\\' + fname).convert('RGB')
    im = im.crop((0, 0, w, end - off))
    full.paste(im, (0, off))
full.save(base + '\\cursor-styleboard-v3.png')
print('saved', full.size)
