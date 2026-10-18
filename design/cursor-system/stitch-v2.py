# -*- coding: utf-8 -*-
"""拼接 v2 风格板整页"""
from PIL import Image

base = r'D:\code\github\jobs-robot\apps\web'
w = 1027
total_h = 1975
seg = [(0, 'v2-f-0.png', 740), (740, 'v2-f-1.png', 1480), (1480, 'v2-f-2.png', 1975)]
full = Image.new('RGB', (w, total_h), (11, 18, 32))
for off, fname, end in seg:
    im = Image.open(base + '\\' + fname).convert('RGB')
    im = im.crop((0, off - off, w, end - off))
    full.paste(im, (0, off))
full.save(base + '\\cursor-styleboard-v2.png')
print('saved', full.size)
