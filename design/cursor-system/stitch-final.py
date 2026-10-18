# -*- coding: utf-8 -*-
"""拼接最终版风格板整页"""
from PIL import Image

base = r'D:\code\github\jobs-robot\apps\web'
w = 1027
total_h = 1573
seg = [
    (0, 'sb-f-0.png', 0, 780),
    (760, 'sb-f-1.png', 760, 1540),
    (1520, 'sb-f-2.png', 1520, 1573),
]
full = Image.new('RGB', (w, total_h), (15, 31, 77))
for off, fname, start, end in seg:
    im = Image.open(base + '\\' + fname).convert('RGB')
    crop_top = start - off
    im = im.crop((0, crop_top, w, crop_top + (end - start)))
    full.paste(im, (0, start))
full.save(base + '\\cursor-styleboard-final.png')
print('saved', full.size)
