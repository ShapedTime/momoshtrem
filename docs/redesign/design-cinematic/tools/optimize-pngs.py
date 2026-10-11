"""Quantise exported screens to 256 colours (≈7× smaller). Run after export-screens.mjs.
Usage: python3 tools/optimize-pngs.py   (needs Pillow)"""
from pathlib import Path
from PIL import Image

root = Path(__file__).resolve().parent.parent / 'screens'
before = after = 0
for p in sorted(root.rglob('*.png')):
    before += p.stat().st_size
    im = Image.open(p)
    if im.mode == 'P':
        after += p.stat().st_size
        continue
    im.convert('RGB').quantize(256, method=Image.Quantize.FASTOCTREE, dither=Image.Dither.FLOYDSTEINBERG).save(p, optimize=True)
    after += p.stat().st_size
print(f'{before / 1e6:.1f} MB -> {after / 1e6:.1f} MB')
