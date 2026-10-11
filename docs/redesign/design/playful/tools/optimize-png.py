#!/usr/bin/env python3
"""Quantise PNG screenshots to 256 colours (visually identical for UI, about a third of the size).
Usage: python3 tools/optimize-png.py exports"""
import sys
from pathlib import Path
from PIL import Image

total_in = total_out = 0
for p in sorted(Path(sys.argv[1]).glob("*.png")):
    before = p.stat().st_size
    im = Image.open(p).convert("RGB")
    im.quantize(colors=256, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.FLOYDSTEINBERG).save(p, optimize=True)
    total_in += before
    total_out += p.stat().st_size
print(f"optimised: {total_in / 1e6:.1f} MB -> {total_out / 1e6:.1f} MB")
