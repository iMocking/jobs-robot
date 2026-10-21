# -*- coding: utf-8 -*-
from PIL import Image
base = r'D:\code\github\jobs-robot\apps\web'
w = 814
total_h = 2044
seg = [(0, 'v6-f-0.png', 740), (740, 'v6-f-1.png', 1480), (1480, 'v6-f-2.png', 2044)]
full = Image.new('RGB', (w, total_h), (184, 230, 214))
for off, fname, end in seg:
    im = Image.open(base + '\\' + fname).convert('RGB')
    im = im.crop((0, 0, w, end - off))
    full.paste(im, (0, off))
full.save(base + '\\cursor-styleboard-v6.png')
print('saved', full.size)
