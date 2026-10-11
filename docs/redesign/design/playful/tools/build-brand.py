#!/usr/bin/env python3
"""Builds the momoshtrem logo set (SVG) into brand/logo and brand/explorations.

Run from docs/redesign/design/playful:  python3 tools/build-brand.py
PNG exports are rendered afterwards by tools/render-brand.cjs.
"""
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
LOGO = ROOT / "brand" / "logo"
EXP = ROOT / "brand" / "explorations"

C = dict(
    peach="#FFA48D", peach_lo="#FF8B6E", coral="#F2795A",
    matcha="#CFD866", matcha_rim="#B7C83A", olive="#7A900F", olive_hi="#9DB021",
    butter="#FFE58A", fur="#F4F0E6", pink="#FFB3B5", line="#9C5548",
    ink="#2A1C16", cream="#FFF8F0", terracotta="#B8482C",
)


def svg(w, h, body, title="momoshtrem", extra=""):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" width="{w}" height="{h}" '
            f'role="img" aria-label="{title}"{extra}>\n<title>{title}</title>\n{body}\n</svg>\n')


# ---------------------------------------------------------------- the cat (Momo), front view
def cat(cx=205, top=206, s=1.0, fur=None, pink=None, line=None, shadow=None, bottom=440, face=True):
    """Momo sitting, cropped at `bottom`. Geometry is authored around cx=205, top=206 at s=1."""
    fur, pink, line = fur or C["fur"], pink or C["pink"], line or C["line"]

    def P(x, y):
        return f"{cx + (x - 205) * s:.1f} {top + (y - 206) * s:.1f}"

    b = bottom
    body = (f"M {P(140,272)} C {P(139,250)} {P(140,230)} {P(143,212)} Q {P(145,195)} {P(158,203)} "
            f"L {P(184,229)} Q {P(205,223)} {P(226,229)} L {P(252,203)} Q {P(265,195)} {P(267,212)} "
            f"C {P(270,230)} {P(271,250)} {P(270,272)} C {P(271,292)} {P(274,302)} {P(280,314)} "
            f"C {P(294,350)} {P(304,396)} {cx + 103 * s:.1f} {b} L {cx - 103 * s:.1f} {b} "
            f"C {P(106,396)} {P(116,350)} {P(130,314)} C {P(136,302)} {P(139,292)} {P(140,272)} Z")
    out = []
    if shadow:
        out.append(f'<path d="{body}" fill="{shadow}" transform="translate({10 * s:.1f} 0)"/>')
    out.append(f'<path d="{body}" fill="{fur}"/>')
    # inner ears
    out.append(f'<path d="M {P(149,219)} Q {P(150,207)} {P(158,212)} L {P(177,231)} Q {P(162,233)} {P(149,241)} Z" fill="{pink}"/>')
    out.append(f'<path d="M {P(261,219)} Q {P(260,207)} {P(252,212)} L {P(233,231)} Q {P(248,233)} {P(261,241)} Z" fill="{pink}"/>')
    if face:
        sw = 4.6 * s
        out.append(f'<ellipse cx="{P(166,295).split()[0]}" cy="{P(166,295).split()[1]}" rx="{13 * s:.1f}" ry="{8.5 * s:.1f}" fill="{pink}"/>')
        out.append(f'<ellipse cx="{P(244,295).split()[0]}" cy="{P(244,295).split()[1]}" rx="{13 * s:.1f}" ry="{8.5 * s:.1f}" fill="{pink}"/>')
        out.append(f'<path d="M {P(171,281)} Q {P(180,270)} {P(189,281)} M {P(221,281)} Q {P(230,270)} {P(239,281)}" '
                   f'fill="none" stroke="{line}" stroke-width="{sw:.1f}" stroke-linecap="round"/>')
        out.append(f'<path d="M {P(199.5,288)} L {P(210.5,288)} L {P(205,294.5)} Z" fill="{line}" stroke="{line}" '
                   f'stroke-width="{2 * s:.1f}" stroke-linejoin="round"/>')
        out.append(f'<path d="M {P(205,294)} L {P(205,298)} M {P(198.5,300.5)} Q {P(205,303)} {P(205,298)} Q {P(205,303)} {P(211.5,300.5)}" '
                   f'fill="none" stroke="{line}" stroke-width="{3.2 * s:.1f}" stroke-linecap="round" stroke-linejoin="round"/>')
    return "\n".join(out)


# ---------------------------------------------------------------- the refined mark: "Channel Momo"
def tv_mark(mono=None, shadow=True, detail="full", uid="m"):
    """512×512 mark: matcha TV, antennas, Momo on screen. mono = single colour (str) for monochrome."""
    if mono:
        ink = mono
        knock = "var(--knock, #fff)"  # unused; mono uses mask
    body_c = mono or C["matcha"]
    rim_c = mono or C["matcha_rim"]
    olive = mono or C["olive"]
    knob = mono or C["matcha_rim"]
    screen_c = C["peach"]
    out = []
    # antennas
    out.append(f'<g stroke="{olive}" stroke-width="17" stroke-linecap="round">'
               f'<path d="M 252 160 L 214 62"/><path d="M 268 160 L 306 50"/></g>')
    out.append(f'<circle cx="211" cy="56" r="19" fill="{knob}"/><circle cx="309" cy="44" r="21" fill="{knob}"/>')
    if shadow and not mono:
        out.append(f'<rect x="54" y="168" width="430" height="300" rx="66" fill="{C["peach_lo"]}"/>')
    out.append(f'<rect x="38" y="152" width="430" height="300" rx="66" fill="{body_c}"/>')
    if mono:
        # monochrome: screen and panel details are knocked out with a mask
        m = (f'<mask id="{uid}-k"><rect width="512" height="512" fill="#fff"/>'
             f'<rect x="76" y="186" width="278" height="232" rx="54" fill="#000"/>'
             f'<g fill="#000"><rect x="384" y="198" width="54" height="104" rx="10"/></g>'
             f'<circle cx="411" cy="343" r="15" fill="#000"/><circle cx="411" cy="396" r="22" fill="#000"/></mask>')
        out = [o.replace(f'fill="{body_c}"', f'fill="{body_c}" mask="url(#{uid}-k)"') for o in out]
        out.insert(0, f"<defs>{m}</defs>")
        out.append(f'<g fill="none" stroke="{olive}" stroke-width="8" stroke-linecap="round">'
                   f'<path d="M 396 214 L 426 250 M 426 214 L 396 250 M 396 252 L 426 288 M 426 252 L 396 288"/></g>')
        out.append(f'<circle cx="411" cy="343" r="7" fill="{olive}"/><circle cx="411" cy="396" r="12" fill="{olive}"/>')
        # cat silhouette in the screen, face knocked out
        out.append(f'<clipPath id="{uid}-scr"><rect x="76" y="186" width="278" height="232" rx="54"/></clipPath>')
        out.append(f'<g clip-path="url(#{uid}-scr)">{cat(cx=215, top=214, s=1.08, fur=ink, pink=ink, line=ink, face=False, bottom=430)}</g>')
        return "\n".join(out)
    # screen rim + screen
    out.append(f'<rect x="68" y="178" width="294" height="248" rx="60" fill="{rim_c}"/>')
    out.append(f'<rect x="76" y="186" width="278" height="232" rx="54" fill="{screen_c}"/>')
    # side panel: speaker + knobs
    out.append(f'<rect x="384" y="198" width="54" height="104" rx="10" fill="{rim_c}"/>')
    if detail == "full":
        out.append(f'<g fill="none" stroke="{olive}" stroke-width="7" stroke-linecap="round">'
                   f'<path d="M 396 214 L 426 250 M 426 214 L 396 250 M 396 252 L 426 288 M 426 252 L 396 288"/></g>')
    out.append(f'<clipPath id="{uid}-kn"><circle cx="411" cy="343" r="15"/><circle cx="411" cy="396" r="22"/></clipPath>')
    out.append(f'<circle cx="411" cy="343" r="15" fill="{C["olive_hi"]}"/><circle cx="411" cy="396" r="22" fill="{C["olive_hi"]}"/>')
    out.append(f'<g clip-path="url(#{uid}-kn)" fill="{olive}"><circle cx="418" cy="350" r="15"/><circle cx="421" cy="406" r="22"/></g>')
    # Momo on screen
    out.append(f'<clipPath id="{uid}-scr"><rect x="76" y="186" width="278" height="232" rx="54"/></clipPath>')
    out.append(f'<g clip-path="url(#{uid}-scr)">'
               f'{cat(cx=215, top=214, s=1.08, shadow=C["peach_lo"], bottom=430)}</g>')
    return "\n".join(out)


def stitch(x, y, w, h, r, sw, color, shadow, dash, gap, off=6):
    d = f'M {x + r} {y} H {x + w - r} A {r} {r} 0 0 1 {x + w} {y + r} V {y + h - r} A {r} {r} 0 0 1 {x + w - r} {y + h} H {x + r} A {r} {r} 0 0 1 {x} {y + h - r} V {y + r} A {r} {r} 0 0 1 {x + r} {y} Z'
    common = f'fill="none" stroke-width="{sw}" stroke-linecap="round" stroke-dasharray="{dash} {gap}" stroke-dashoffset="{dash / 2}"'
    return (f'<path d="{d}" stroke="{shadow}" {common} transform="translate({off} {off})"/>'
            f'<path d="{d}" stroke="{color}" {common}/>')


def app_icon(size=1024, with_stitch=True, uid="ai"):
    s = size / 1024
    body = [f'<rect width="1024" height="1024" fill="{C["peach"]}"/>',
            f'<radialGradient id="{uid}-g" cx="50%" cy="38%" r="70%"><stop offset="0" stop-color="#FFB49E"/>'
            f'<stop offset="1" stop-color="{C["peach"]}"/></radialGradient>',
            f'<rect width="1024" height="1024" fill="url(#{uid}-g)"/>']
    if with_stitch:
        body.append(stitch(70, 70, 884, 884, 200, 30, C["butter"], C["peach_lo"], 92, 78, off=7))
    body.append(f'<g transform="translate(141 178) scale(1.42)">{tv_mark(uid=uid + "m")}</g>')
    return svg(size, size, f'<g transform="scale({s})">' + "\n".join(body) + "</g>", "momoshtrem app icon")


def maskable(size=512, uid="mk"):
    # Safe zone: circle of radius 40 % centred. The mark sits inside a 300 px box.
    s = size / 512
    body = [f'<rect width="512" height="512" fill="{C["peach"]}"/>',
            f'<g transform="translate(89 98) scale(0.64)">{tv_mark(uid=uid)}</g>']
    return svg(size, size, f'<g transform="scale({s})">' + "\n".join(body) + "</g>", "momoshtrem")


# ---------------------------------------------------------------- favicon, drawn on its own small grid
def favicon(px):
    if px <= 16:
        b = (f'<rect width="16" height="16" rx="3.5" fill="{C["peach"]}"/>'
             f'<path d="M7 5.2 L5.6 2.2 M9 5.2 L10.6 1.8" stroke="{C["olive"]}" stroke-width="1.3" stroke-linecap="round"/>'
             f'<rect x="1.5" y="5" width="13" height="9.5" rx="2.6" fill="{C["matcha"]}"/>'
             f'<rect x="3" y="6.5" width="8" height="6.5" rx="1.8" fill="{C["peach"]}"/>'
             f'<path d="M4.3 13 V9.2 L4.9 7.6 L6.2 8.7 H7.8 L9.1 7.6 L9.7 9.2 V13 Z" fill="{C["fur"]}"/>'
             f'<rect x="12" y="7.2" width="1.4" height="1.4" rx=".7" fill="{C["olive"]}"/>'
             f'<rect x="12" y="10.4" width="1.4" height="1.4" rx=".7" fill="{C["olive"]}"/>')
        return svg(16, 16, b, "momoshtrem")
    b = (f'<rect width="32" height="32" rx="7" fill="{C["peach"]}"/>'
         f'<path d="M14.6 10 L11.6 4 M17.4 10 L20.6 3.4" stroke="{C["olive"]}" stroke-width="2.2" stroke-linecap="round"/>'
         f'<circle cx="11.4" cy="3.8" r="1.9" fill="{C["matcha_rim"]}"/><circle cx="20.8" cy="3.2" r="2" fill="{C["matcha_rim"]}"/>'
         f'<rect x="3" y="10" width="26" height="19" rx="5" fill="{C["matcha"]}"/>'
         f'<rect x="5.5" y="12.5" width="16" height="14" rx="3.6" fill="{C["peach"]}"/>'
         f'<path d="M8.4 26.5 V19 Q8.4 17.6 8.8 16.4 L9.6 14.6 L12 16.6 Q13.5 16.2 15 16.6 L17.4 14.6 L18.2 16.4 Q18.6 17.6 18.6 19 V26.5 Z" fill="{C["fur"]}"/>'
         f'<path d="M10.6 20.2 q.9 -1 1.8 0 M14.6 20.2 q.9 -1 1.8 0" stroke="{C["line"]}" stroke-width=".9" fill="none" stroke-linecap="round"/>'
         f'<rect x="23.6" y="13.4" width="3.4" height="6" rx="1" fill="{C["matcha_rim"]}"/>'
         f'<circle cx="25.3" cy="22.4" r="1.3" fill="{C["olive"]}"/><circle cx="25.3" cy="25.7" r="1.6" fill="{C["olive"]}"/>')
    return svg(32, 32, b, "momoshtrem")


# ---------------------------------------------------------------- explorations
def exp_a():
    """A · Channel Momo: your drawing, rebuilt on geometry."""
    return svg(512, 512, f'<rect width="512" height="512" rx="112" fill="{C["peach"]}"/>'
               + stitch(26, 26, 460, 460, 96, 14, C["butter"], C["peach_lo"], 44, 38, off=4)
               + f'<g transform="translate(70 76) scale(.74)">{tv_mark(uid="ea")}</g>', "Exploration A: Channel Momo")


def exp_b():
    """B · Ear-tenna: the TV is the cat. Antennas become ears, the screen becomes the face."""
    f = C
    body = (f'<rect width="512" height="512" rx="112" fill="{f["butter"]}"/>'
            # shadow + body with ears
            f'<g transform="translate(14 14)"><path d="M 96 214 L 104 104 Q 106 84 124 94 L 206 150 H 306 L 388 94 Q 406 84 408 104 L 416 214 '
            f'Q 440 214 440 250 V 384 Q 440 432 392 432 H 120 Q 72 432 72 384 V 250 Q 72 214 96 214 Z" fill="{f["peach_lo"]}"/></g>'
            f'<path d="M 96 214 L 104 104 Q 106 84 124 94 L 206 150 H 306 L 388 94 Q 406 84 408 104 L 416 214 '
            f'Q 440 214 440 250 V 384 Q 440 432 392 432 H 120 Q 72 432 72 384 V 250 Q 72 214 96 214 Z" fill="{f["matcha"]}"/>'
            f'<path d="M 120 128 L 126 196 L 176 160 Z M 392 128 L 386 196 L 336 160 Z" fill="{f["pink"]}"/>'
            f'<rect x="108" y="196" width="296" height="200" rx="58" fill="{f["fur"]}"/>'
            f'<path d="M 176 284 Q 196 262 216 284 M 296 284 Q 316 262 336 284" stroke="{f["line"]}" stroke-width="11" fill="none" stroke-linecap="round"/>'
            f'<ellipse cx="166" cy="318" rx="24" ry="15" fill="{f["pink"]}"/><ellipse cx="346" cy="318" rx="24" ry="15" fill="{f["pink"]}"/>'
            f'<path d="M 244 304 H 268 L 256 318 Z" fill="{f["line"]}" stroke="{f["line"]}" stroke-width="5" stroke-linejoin="round"/>'
            f'<path d="M 256 318 V 326 M 242 334 Q 256 340 256 326 Q 256 340 270 334" stroke="{f["line"]}" stroke-width="7" fill="none" stroke-linecap="round"/>'
            f'<circle cx="200" cy="466" r="0" fill="none"/>')
    return svg(512, 512, body, "Exploration B: Ear-tenna")


def exp_c():
    """C · Momo face: just the head, a sticker. The most compact, but the TV story is gone."""
    f = C
    head = ("M 112 300 C 108 250 116 214 128 190 L 140 102 Q 144 82 162 92 L 222 140 Q 256 132 290 140 L 350 92 "
            "Q 368 82 372 102 L 384 190 C 396 214 404 250 400 300 C 396 380 334 420 256 420 C 178 420 116 380 112 300 Z")
    body = (f'<rect width="512" height="512" rx="112" fill="{f["terracotta"]}"/>'
            f'<path d="{head}" fill="#8A321D" transform="translate(12 12)"/>'
            f'<path d="{head}" fill="{f["fur"]}"/>'
            f'<path d="M 150 132 Q 154 112 168 122 L 210 156 Q 178 162 156 184 Z M 362 132 Q 358 112 344 122 L 302 156 Q 334 162 356 184 Z" fill="{f["pink"]}"/>'
            f'<path d="M 182 284 Q 202 262 222 284 M 290 284 Q 310 262 330 284" stroke="{f["line"]}" stroke-width="12" fill="none" stroke-linecap="round"/>'
            f'<ellipse cx="168" cy="322" rx="26" ry="16" fill="{f["pink"]}"/><ellipse cx="344" cy="322" rx="26" ry="16" fill="{f["pink"]}"/>'
            f'<path d="M 243 306 H 269 L 256 321 Z" fill="{f["line"]}" stroke="{f["line"]}" stroke-width="6" stroke-linejoin="round"/>'
            f'<path d="M 256 321 V 330 M 240 338 Q 256 345 256 330 Q 256 345 272 338" stroke="{f["line"]}" stroke-width="8" fill="none" stroke-linecap="round"/>')
    return svg(512, 512, body, "Exploration C: Momo face")


def main():
    LOGO.mkdir(parents=True, exist_ok=True)
    EXP.mkdir(parents=True, exist_ok=True)
    (EXP / "a-channel-momo.svg").write_text(exp_a())
    (EXP / "b-ear-tenna.svg").write_text(exp_b())
    (EXP / "c-momo-face.svg").write_text(exp_c())
    (LOGO / "mark.svg").write_text(svg(512, 512, tv_mark(uid="mk1"), "momoshtrem"))
    (LOGO / "mark-flat.svg").write_text(svg(512, 512, tv_mark(shadow=False, uid="mk2"), "momoshtrem"))
    (LOGO / "mark-mono.svg").write_text(svg(512, 512, tv_mark(mono="currentColor", uid="mk3"), "momoshtrem").replace(
        'role="img"', 'role="img" style="color:#2A1C16"'))
    (LOGO / "app-icon.svg").write_text(app_icon())
    (LOGO / "maskable.svg").write_text(maskable())
    (LOGO / "favicon-16.svg").write_text(favicon(16))
    (LOGO / "favicon.svg").write_text(favicon(32))
    print("brand svgs written")




# ---------------------------------------------------------------- wordmark (Fredoka SemiBold, outlined)
def wordmark_glyphs(text="momoshtrem", wght=600, wdth=100, tracking=-0.012):
    from fontTools.ttLib import TTFont
    from fontTools.varLib import instancer
    from fontTools.pens.svgPathPen import SVGPathPen
    from fontTools.pens.transformPen import TransformPen
    f = TTFont(ROOT / "assets" / "fonts" / "fredoka-latin.woff2")
    f = instancer.instantiateVariableFont(f, {"wght": wght, "wdth": wdth})
    upm = f["head"].unitsPerEm
    cmap = f.getBestCmap()
    gs = f.getGlyphSet()
    x = 0
    paths = []
    for ch in text:
        g = cmap[ord(ch)]
        pen = SVGPathPen(gs)
        tp = TransformPen(pen, (1, 0, 0, -1, x, 0))
        gs[g].draw(tp)
        paths.append((ch, x, pen.getCommands()))
        x += f["hmtx"][g][0] + tracking * upm
    asc = f["OS/2"].sTypoAscender
    xh = f["OS/2"].sxHeight
    return paths, x - tracking * upm, upm, asc, xh


def wordmark_svg(color=C["ink"], title="momoshtrem"):
    paths, width, upm, asc, xh = wordmark_glyphs()
    top = -int(xh * 1.62)  # room for ascenders (h, t)
    h = int(-top + 0.26 * upm)
    body = f'<g fill="{color}">' + "".join(f'<path d="{d}"/>' for _, _, d in paths) + "</g>"
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 {top} {int(width)} {h}" width="{int(width / 4)}" '
            f'height="{int(h / 4)}" role="img" aria-label="{title}"><title>{title}</title>{body}</svg>\n'), (0, top, width, h)


def lockup_svg(color=C["ink"], mono=False, title="momoshtrem"):
    paths, width, upm, asc, xh = wordmark_glyphs()
    # mark height = 2.3 × x-height, wordmark baseline aligned with the TV's bottom edge
    mark_h = xh * 3.1
    s = mark_h / 512
    gap = xh * 0.38
    mark = tv_mark(mono=color if mono else None, uid="lk" + ("m" if mono else "c"))
    mx = 0
    my = -0.89 * mark_h
    wx = 512 * s * 0.95 + gap
    body = (f'<g transform="translate({mx:.1f} {my:.1f}) scale({s:.4f})">{mark}</g>'
            f'<g fill="{color}" transform="translate({wx:.1f} 0)">' + "".join(f'<path d="{d}"/>' for _, _, d in paths) + "</g>")
    W = wx + width
    top = my
    H = -top + 0.24 * upm
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 {top:.0f} {W:.0f} {H:.0f}" width="{W / 5:.0f}" height="{H / 5:.0f}" '
            f'role="img" aria-label="{title}"><title>{title}</title>{body}</svg>\n')


def write_wordmarks():
    (LOGO / "wordmark.svg").write_text(wordmark_svg()[0])
    (LOGO / "wordmark-cream.svg").write_text(wordmark_svg(C["cream"])[0])
    (LOGO / "lockup.svg").write_text(lockup_svg())
    (LOGO / "lockup-dark.svg").write_text(lockup_svg(C["cream"]))
    (LOGO / "lockup-mono.svg").write_text(lockup_svg(C["ink"], mono=True))
    print("wordmarks written")


if __name__ == "__main__":
    main()
    write_wordmarks()
