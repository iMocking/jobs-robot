# -*- coding: utf-8 -*-
"""拼接 v4 风格板整页"""
from PIL import Image

base = r'D:\code\github\jobs-robot\apps\web'
w = 1027
total_h = 2036
seg = [(0, 'v4-f-0.png', 740), (740, 'v4-f-1.png', 1480), (1480, 'v4-f-2.png', 2036)]
full = Image.new('RGB', (w, total_h), (6, 11, 26))
for off, fname, end in seg:
    im = Image.open(base + '\\' + fname).convert('RGB')
    im = im.crop((0, 0, w, end - off))
    full.paste(im, (0, off))
full.save(base + '\\cursor-styleboard-v4.png')
print('saved', full.size)
