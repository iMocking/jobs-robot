# -*- coding: utf-8 -*-
"""拼接风格板整页截图（正确裁剪重叠）"""
from PIL import Image

base = r'D:\code\github\jobs-robot\apps\web'
w = 1027
# (滚动偏移, 文件, 文档内容起点, 文档内容终点)
seg = [
    (0, 'sb-part-0.png', 0, 780),
    (760, 'sb-part-1.png', 760, 1540),
    (1520, 'sb-part-2.png', 1520, 1573),
]
total_h = 1573
full = Image.new('RGB', (w, total_h), (15, 31, 77))
for off, fname, start, end in seg:
    im = Image.open(base + '\\' + fname).convert('RGB')
    crop_top = start - off          # 文档 start 在视口内的 y
    crop_bot = crop_top + (end - start)
    im = im.crop((0, crop_top, w, crop_bot))
    full.paste(im, (0, start))
full.save(base + '\\cursor-styleboard-full.png')
print('saved', full.size)
