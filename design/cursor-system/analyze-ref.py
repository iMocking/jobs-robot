# -*- coding: utf-8 -*-
from PIL import Image
im = Image.open(r'D:\code\github\jobs-robot\design\cursor-system\ref-v6.png').convert('RGBA')
print('orig', im.size)
# 放大 3.5 倍
big = im.resize((im.width*3, im.height*3), Image.LANCZOS)
big.save(r'D:\code\github\jobs-robot\design\cursor-system\ref-v6-big.png')
# 单独裁 pointer（行1列1，约 x 10-70, y 10-70）
w, h = im.size
col_w = w / 5
row_h = h / 3
pointer = im.crop((0, 0, int(col_w), int(row_h))).resize((int(col_w)*5, int(row_h)*5), Image.LANCZOS)
pointer.save(r'D:\code\github\jobs-robot\design\cursor-system\ref-pointer.png')
hand = im.crop((int(col_w), int(row_h), int(col_w*2), int(row_h*2))).resize((int(col_w)*5, int(row_h)*5), Image.LANCZOS)
hand.save(r'D:\code\github\jobs-robot\design\cursor-system\ref-hand.png')
move = im.crop((int(col_w*2), int(row_h*2), int(col_w*3), int(row_h*3))).resize((int(col_w)*5, int(row_h)*5), Image.LANCZOS)
move.save(r'D:\code\github\jobs-robot\design\cursor-system\ref-move.png')
print('done')
