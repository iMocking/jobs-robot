# -*- coding: utf-8 -*-
from PIL import Image
img = Image.open(r'D:\code\github\jobs-robot\apps\web\v6-board.png').convert('RGB')
crop = img.crop((70, 320, 300, 520)).resize((460, 400), Image.LANCZOS)
crop.save(r'D:\code\github\jobs-robot\design\cursor-system\z-arrow6.png')
print('ok', img.size)
