# -*- coding: utf-8 -*-
from PIL import Image
base = r'D:\code\github\jobs-robot\apps\web'
w, total_h = 814, 2028
seg = [(0, 'v7-f-0.png', 740), (740, 'v7-f-1.png', 1480), (1480, 'v7-f-2.png', 2028)]
full = Image.new('RGB', (w, total_h), (184, 230, 214))
for off, fname, end in seg:
    im = Image.open(base + '\\' + fname).convert('RGB')
    im = im.crop((0, 0, w, end - off))
    full.paste(im, (0, off))
full.save(base + '\\cursor-styleboard-v7.png')
print('saved', full.size)
