#!/usr/bin/env python3
"""Momo, the momoshtrem cat: six poses plus an avatar head.

Writes brand/mascot/*.svg (static files with fixed colours) and assets/js/mascot.js
(the same drawings with CSS custom properties, so they follow the theme and can animate).
Run from docs/redesign/design/playful:  python3 tools/build-mascot.py
"""
from pathlib import Path
import json

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "brand" / "mascot"

# CSS variable → fallback. Inline copies pick up the theme; <img> copies use the fallback.
V = {
    "fur": ("--momo-fur", "#F4F0E6"),
    "fur2": ("--momo-fur-shade", "#E6DCCB"),
    "pink": ("--momo-pink", "#FFB3B5"),
    "line": ("--momo-line", "#9C5548"),
    "spot": ("--momo-spot", "#FFD9CB"),
    "shadow": ("--momo-shadow", "#FF8B6E"),
    "ground": ("--momo-ground", "rgb(156 85 72 / 0.16)"),
    "matcha": ("--momo-matcha", "#CFD866"),
    "rim": ("--momo-matcha-rim", "#B7C83A"),
    "olive": ("--momo-olive", "#7A900F"),
    "peach": ("--momo-peach", "#FFA48D"),
    "butter": ("--momo-butter", "#FFE58A"),
    "terra": ("--momo-terracotta", "#D45A3A"),
    "terra2": ("--momo-terracotta-deep", "#A93F25"),
    "mouth": ("--momo-mouth", "#7A3B33"),
    "lens": ("--momo-lens", "rgb(221 238 250 / 0.55)"),
    "coffee": ("--momo-coffee", "#8A5A44"),
    "white": ("--momo-white", "#FFFFFF"),
}


def c(name, inline):
    var, fb = V[name]
    return f"var({var}, {fb})" if inline else fb


# ---------------------------------------------------------------- shared head (local coords, centre 0,0)
HEAD = ("M -144 40 C -148 -10 -140 -46 -128 -70 L -116 -158 Q -112 -178 -94 -168 L -34 -120 Q 0 -128 34 -120 "
        "L 94 -168 Q 112 -178 116 -158 L 128 -70 C 140 -46 148 -10 144 40 C 140 120 78 160 0 160 C -78 160 -140 120 -144 40 Z")
EAR_L = "M -106 -128 Q -102 -150 -88 -139 L -48 -106 Q -80 -100 -101 -78 Z"
EAR_R = "M 106 -128 Q 102 -150 88 -139 L 48 -106 Q 80 -100 101 -78 Z"


def head(k, x, y, s, tilt=0, eyes="happy", mouth="smile", look=(0, 0), shadow=True, blush=True, extra=""):
    L = k("line")
    sw = 9
    parts = []
    if shadow:
        parts.append(f'<path d="{HEAD}" fill="{k("shadow")}" transform="translate(16 12)"/>')
    parts.append(f'<path d="{HEAD}" fill="{k("fur")}"/>')
    parts.append(f'<path d="{EAR_L} {EAR_R}" fill="{k("pink")}"/>')
    if blush:
        parts.append(f'<ellipse cx="-90" cy="64" rx="27" ry="16" fill="{k("pink")}"/><ellipse cx="90" cy="64" rx="27" ry="16" fill="{k("pink")}"/>')
    lx, ly = look
    if eyes == "happy":
        parts.append(f'<path d="M -76 26 Q -55 2 -34 26 M 34 26 Q 55 2 76 26" fill="none" stroke="{L}" stroke-width="{sw}" stroke-linecap="round"/>')
    elif eyes == "sleep":
        parts.append(f'<path d="M -76 18 Q -55 38 -34 18 M 34 18 Q 55 38 76 18" fill="none" stroke="{L}" stroke-width="{sw}" stroke-linecap="round"/>')
    elif eyes == "squeeze":
        parts.append(f'<path d="M -74 8 L -42 22 L -74 36 M 74 8 L 42 22 L 74 36" fill="none" stroke="{L}" stroke-width="{sw}" stroke-linecap="round" stroke-linejoin="round"/>')
    elif eyes == "open":
        parts.append(f'<g class="momo-eyes"><circle cx="{-55 + lx}" cy="{20 + ly}" r="16" fill="{L}"/><circle cx="{55 + lx}" cy="{20 + ly}" r="16" fill="{L}"/>'
                     f'<circle cx="{-49 + lx}" cy="{13 + ly}" r="5.5" fill="{k("white")}"/><circle cx="{61 + lx}" cy="{13 + ly}" r="5.5" fill="{k("white")}"/></g>')
    elif eyes == "detective":
        parts.append(f'<path d="M -78 20 Q -55 12 -32 22" fill="none" stroke="{L}" stroke-width="{sw}" stroke-linecap="round"/>')
        parts.append(f'<circle cx="55" cy="20" r="16" fill="{L}"/><circle cx="61" cy="13" r="5.5" fill="{k("white")}"/>')
    # nose
    parts.append(f'<path d="M -14 46 H 14 L 0 62 Z" fill="{L}" stroke="{L}" stroke-width="6" stroke-linejoin="round"/>')
    if mouth == "smile":
        parts.append(f'<path d="M 0 62 V 72 M -18 80 Q 0 88 0 72 Q 0 88 18 80" fill="none" stroke="{L}" stroke-width="7" stroke-linecap="round" stroke-linejoin="round"/>')
    elif mouth == "yawn":
        parts.append(f'<path d="M 0 62 V 70" stroke="{L}" stroke-width="7" stroke-linecap="round"/>'
                     f'<path d="M -26 82 Q -26 70 0 70 Q 26 70 26 82 Q 26 128 0 128 Q -26 128 -26 82 Z" fill="{k("mouth")}"/>'
                     f'<path d="M -16 116 Q 0 100 16 116 Q 10 126 0 126 Q -10 126 -16 116 Z" fill="{k("pink")}"/>')
    elif mouth == "o":
        parts.append(f'<path d="M 0 62 V 70" stroke="{L}" stroke-width="7" stroke-linecap="round"/><ellipse cx="0" cy="84" rx="9" ry="11" fill="{k("mouth")}"/>')
    elif mouth == "flat":
        parts.append(f'<path d="M 0 62 V 72 M -14 80 Q 0 84 14 80" fill="none" stroke="{L}" stroke-width="7" stroke-linecap="round"/>')
    parts.append(extra)
    return f'<g class="momo-head" transform="translate({x} {y}) rotate({tilt}) scale({s})">' + "".join(parts) + "</g>"


def paw_print(k, x, y, s=1.0, rot=0, color=None):
    col = color or k("peach")
    return (f'<g transform="translate({x} {y}) rotate({rot}) scale({s})" fill="{col}">'
            f'<ellipse cx="0" cy="6" rx="9" ry="7.5"/><circle cx="-10" cy="-6" r="4"/><circle cx="-3.5" cy="-11" r="4"/>'
            f'<circle cx="3.5" cy="-11" r="4"/><circle cx="10" cy="-6" r="4"/></g>')


def sparkle(x, y, r, fill, cls="momo-sparkle"):
    return (f'<path class="{cls}" d="M {x} {y - r} Q {x + r * .18} {y - r * .18} {x + r} {y} Q {x + r * .18} {y + r * .18} {x} {y + r} '
            f'Q {x - r * .18} {y + r * .18} {x - r} {y} Q {x - r * .18} {y - r * .18} {x} {y - r} Z" fill="{fill}"/>')


def zz(x, y, size, color, cls):
    h = size
    return (f'<path class="{cls}" d="M {x} {y} H {x + h} L {x} {y + h} H {x + h}" fill="none" stroke="{color}" stroke-width="{max(4, size * .22):.1f}" '
            f'stroke-linecap="round" stroke-linejoin="round"/>')


def spot(k, cx=200, cy=212, r=158):
    return f'<circle class="momo-spot" cx="{cx}" cy="{cy}" r="{r}" fill="{k("spot")}"/>'


def ground(k, cx=200, cy=356, rx=118):
    return f'<ellipse cx="{cx}" cy="{cy}" rx="{rx}" ry="11" fill="{k("ground")}"/>'


def sitting_body(k, cx=200, base=340, w=1.0, chest=True):
    """Front-facing sitting body (under the head), paws at the base."""
    def X(x):
        return cx + (x - 200) * w
    b = (f"M {X(138)} {base - 6} C {X(124)} {base - 44} {X(130)} {base - 108} {X(152)} {base - 140} L {X(248)} {base - 140} "
         f"C {X(270)} {base - 108} {X(276)} {base - 44} {X(262)} {base - 6} Q {X(200)} {base + 10} {X(138)} {base - 6} Z")
    out = [f'<path d="{b}" fill="{k("shadow")}" transform="translate(12 6)"/>', f'<path d="{b}" fill="{k("fur")}"/>']
    if chest:
        out.append(f'<path d="M {X(174)} {base - 100} Q {X(200)} {base - 70} {X(226)} {base - 100} Q {X(222)} {base - 62} {X(200)} {base - 56} '
                   f'Q {X(178)} {base - 62} {X(174)} {base - 100} Z" fill="{k("white")}" opacity=".7"/>')
    for px in (176, 224):
        out.append(f'<ellipse cx="{X(px)}" cy="{base - 6}" rx="{23 * w:.1f}" ry="14" fill="{k("fur")}"/>'
                   f'<path d="M {X(px) - 7} {base - 12} V {base - 4} M {X(px) + 7} {base - 12} V {base - 4}" stroke="{k("fur2")}" stroke-width="3.5" stroke-linecap="round"/>')
    return "".join(out)


def tail(k, d, width=24, cls="momo-tail"):
    return (f'<g class="{cls}"><path d="{d}" fill="none" stroke="{k("shadow")}" stroke-width="{width}" stroke-linecap="round" transform="translate(8 6)"/>'
            f'<path d="{d}" fill="none" stroke="{k("fur")}" stroke-width="{width}" stroke-linecap="round"/></g>')


# ---------------------------------------------------------------- poses
def pose_sleeping(k):
    tv = (f'<rect x="108" y="268" width="200" height="104" rx="26" fill="{k("shadow")}"/>'
          f'<rect x="98" y="258" width="200" height="104" rx="26" fill="{k("matcha")}"/>'
          f'<rect x="110" y="270" width="134" height="80" rx="20" fill="{k("rim")}"/>'
          f'<rect x="115" y="275" width="124" height="70" rx="17" fill="{k("peach")}"/>'
          f'<path d="M 128 290 Q 134 282 144 282" stroke="{k("white")}" stroke-width="5" fill="none" stroke-linecap="round" opacity=".55"/>'
          f'<rect x="256" y="274" width="28" height="34" rx="6" fill="{k("rim")}"/>'
          f'<circle cx="270" cy="322" r="7" fill="{k("olive")}"/><circle cx="270" cy="340" r="9" fill="{k("olive")}"/>')
    body = "M 112 262 C 102 214 146 186 204 184 C 262 182 306 208 300 262 Z"
    loaf = (f'<path d="{body}" fill="{k("shadow")}" transform="translate(10 0)"/><path d="{body}" fill="{k("fur")}"/>'
            f'<path d="M 214 196 Q 246 194 270 210" stroke="{k("fur2")}" stroke-width="5" fill="none" stroke-linecap="round"/>')
    t = tail(k, "M 290 250 C 322 262 318 300 300 318", 20)
    paws = f'<ellipse cx="176" cy="258" rx="22" ry="10" fill="{k("fur")}"/><ellipse cx="128" cy="259" rx="16" ry="8" fill="{k("fur")}"/>'
    h = head(k, 146, 214, 0.36, tilt=-8, eyes="sleep", mouth="flat", shadow=False)
    zs = (zz(196, 150, 16, k("terra"), "momo-z momo-z1") + zz(222, 112, 22, k("terra"), "momo-z momo-z2")
          + zz(256, 66, 30, k("terra"), "momo-z momo-z3"))
    return spot(k) + ground(k, 205, 372, 112) + tv + t + loaf + paws + h + zs


def pose_peeking(k):
    ledge = (f'<rect x="44" y="276" width="320" height="104" rx="30" fill="{k("shadow")}" transform="translate(10 8)"/>'
             f'<rect x="44" y="276" width="320" height="104" rx="30" fill="{k("matcha")}"/>'
             f'<rect x="44" y="276" width="320" height="16" rx="8" fill="{k("rim")}"/>')
    h = head(k, 204, 236, 0.5, tilt=9, eyes="open", mouth="o", look=(-6, -8))
    paws = ""
    for px in (148, 262):
        paws += (f'<ellipse cx="{px}" cy="282" rx="26" ry="17" fill="{k("fur")}"/>'
                 f'<path d="M {px - 8} 276 V 290 M {px + 8} 276 V 290" stroke="{k("fur2")}" stroke-width="4" stroke-linecap="round"/>')
    dots = "".join(f'<circle class="momo-dot momo-dot{i + 1}" cx="{300 + i * 22}" cy="124" r="7" fill="{k("terra")}"/>' for i in range(3))
    return spot(k) + h + ledge + paws + dots


def pose_peeking_shy(k):
    """Peeking, eyes closed: used on sign-in while the password field has focus."""
    ledge = (f'<rect x="44" y="276" width="320" height="104" rx="30" fill="{k("shadow")}" transform="translate(10 8)"/>'
             f'<rect x="44" y="276" width="320" height="104" rx="30" fill="{k("matcha")}"/>'
             f'<rect x="44" y="276" width="320" height="16" rx="8" fill="{k("rim")}"/>')
    h = head(k, 204, 236, 0.5, tilt=-4, eyes="happy", mouth="smile")
    paws = ""
    for px in (148, 262):
        paws += (f'<ellipse cx="{px}" cy="282" rx="26" ry="17" fill="{k("fur")}"/>'
                 f'<path d="M {px - 8} 276 V 290 M {px + 8} 276 V 290" stroke="{k("fur2")}" stroke-width="4" stroke-linecap="round"/>')
    return spot(k) + h + ledge + paws


def pose_knocking(k):
    table = (f'<rect x="26" y="290" width="276" height="18" rx="9" fill="{k("terra2")}"/>'
             f'<rect x="60" y="300" width="16" height="72" rx="6" fill="{k("terra2")}"/>'
             f'<rect x="250" y="300" width="16" height="72" rx="6" fill="{k("terra2")}"/>')
    t = tail(k, "M 108 286 C 92 300 92 336 112 352", 18)
    body = sitting_body(k, cx=158, base=294, w=0.82)
    arm = (f'<path d="M 196 214 Q 240 214 262 236" fill="none" stroke="{k("shadow")}" stroke-width="22" stroke-linecap="round" transform="translate(6 6)"/>'
           f'<path d="M 196 214 Q 240 214 262 236" fill="none" stroke="{k("fur")}" stroke-width="22" stroke-linecap="round"/>'
           f'<ellipse cx="266" cy="240" rx="15" ry="12" fill="{k("fur")}"/>')
    h = head(k, 158, 152, 0.42, tilt=-6, eyes="open", mouth="smile", look=(4, 2))
    mug = (f'<g class="momo-cup" transform="translate(300 262) rotate(32)">'
           f'<path d="M 26 -12 Q 44 -12 44 4 Q 44 20 24 20" fill="none" stroke="{k("butter")}" stroke-width="9"/>'
           f'<rect x="-24" y="-30" width="50" height="58" rx="10" fill="{k("butter")}"/>'
           f'<rect x="-24" y="-8" width="50" height="10" fill="{k("terra")}"/>'
           f'<ellipse cx="1" cy="-30" rx="25" ry="6" fill="{k("coffee")}"/></g>')
    drops = (f'<g class="momo-drops" fill="{k("coffee")}">'
             f'<path d="M 344 230 Q 352 242 344 248 Q 336 242 344 230 Z"/>'
             f'<path d="M 360 262 Q 368 274 360 280 Q 352 274 360 262 Z"/>'
             f'<circle cx="336" cy="300" r="5"/></g>')
    swoosh = f'<path d="M 318 196 Q 340 204 350 222 M 300 186 Q 322 186 340 200" fill="none" stroke="{k("line")}" stroke-width="4" stroke-linecap="round" opacity=".45"/>'
    return spot(k) + ground(k, 190, 376, 150) + table + t + body + h + arm + mug + drops + swoosh


def pose_proud(k):
    t = tail(k, "M 252 334 C 318 344 342 286 318 236 Q 310 218 296 224", 24)
    body = sitting_body(k, cx=200, base=342)
    h = head(k, 200, 160, 0.44, tilt=-5, eyes="happy", mouth="smile")
    # rosette with a check
    pts = []
    import math
    for i in range(24):
        r = 30 if i % 2 == 0 else 24
        a = math.pi * 2 * i / 24
        pts.append(f"{176 + r * math.cos(a):.1f} {262 + r * math.sin(a):.1f}")
    ros = (f'<g class="momo-rosette"><path d="M 164 282 L 154 324 L 166 316 L 174 328 L 180 286 Z M 190 282 L 200 322 L 188 316 L 182 328 L 176 286 Z" fill="{k("terra")}"/>'
           f'<path d="M {" L ".join(pts)} Z" fill="{k("matcha")}"/><circle cx="176" cy="262" r="18" fill="{k("white")}"/>'
           f'<path d="M 167 262 L 174 270 L 186 255" fill="none" stroke="{k("olive")}" stroke-width="5" stroke-linecap="round" stroke-linejoin="round"/></g>')
    sp = sparkle(86, 118, 18, k("butter")) + sparkle(318, 94, 14, k("terra")) + sparkle(96, 252, 10, k("terra")) + sparkle(330, 176, 20, k("butter"))
    return spot(k) + ground(k) + t + body + ros + h + sp


def pose_detective(k):
    trail = paw_print(k, 70, 330, 0.9, -20) + paw_print(k, 96, 296, 0.9, -10) + paw_print(k, 62, 262, 0.8, -25) + paw_print(k, 92, 224, 0.75, -15)
    t = tail(k, "M 152 334 C 104 344 88 300 104 262", 22)
    body = sitting_body(k, cx=196, base=342)
    hat = (f'<path d="M -98 -86 Q -84 -186 0 -190 Q 84 -186 98 -86 Z" fill="{k("terra")}"/>'
           f'<path d="M -66 -170 L -48 -88 M -20 -188 L -14 -88 M 24 -188 L 18 -88 M 66 -170 L 50 -88 M -92 -120 H 92 M -86 -150 H 86" '
           f'stroke="{k("terra2")}" stroke-width="7" opacity=".55"/>'
           f'<path d="M -110 -86 Q 0 -66 110 -86 Q 116 -70 100 -64 Q 0 -46 -100 -64 Q -116 -70 -110 -86 Z" fill="{k("terra2")}"/>'
           f'<circle cx="0" cy="-192" r="12" fill="{k("terra2")}"/>')
    h = head(k, 196, 162, 0.45, tilt=-4, eyes="detective", mouth="flat", extra=hat)
    # magnifying glass over the right eye; the eye looks bigger through the lens
    ex, ey = 196 + 55 * 0.45, 162 + 20 * 0.45
    glass = (f'<g class="momo-glass"><path d="M {ex + 26} {ey + 30} L {ex + 64} {ey + 104}" stroke="{k("olive")}" stroke-width="15" stroke-linecap="round"/>'
             f'<circle cx="{ex}" cy="{ey}" r="31" fill="{k("lens")}"/>'
             f'<circle cx="{ex}" cy="{ey}" r="12" fill="{k("line")}"/><circle cx="{ex + 4}" cy="{ey - 5}" r="4.5" fill="{k("white")}"/>'
             f'<circle cx="{ex}" cy="{ey}" r="31" fill="none" stroke="{k("rim")}" stroke-width="9"/>'
             f'<path d="M {ex - 16} {ey - 14} Q {ex - 8} {ey - 22} {ex + 2} {ey - 22}" stroke="{k("white")}" stroke-width="4" fill="none" stroke-linecap="round" opacity=".8"/>'
             f'<ellipse cx="{ex + 58}" cy="{ey + 96}" rx="16" ry="13" fill="{k("fur")}"/></g>')
    sp = sparkle(320, 110, 16, k("butter"), "momo-sparkle momo-s1") + sparkle(346, 168, 10, k("terra"), "momo-sparkle momo-s2") + sparkle(300, 64, 9, k("terra"), "momo-sparkle momo-s3")
    return spot(k) + ground(k) + trail + t + body + h + glass + sp


def pose_yawn(k):
    moon = (f'<path d="M 318 52 A 40 40 0 1 0 352 118 A 32 32 0 1 1 318 52 Z" fill="{k("butter")}"/>'
            + sparkle(256, 64, 9, k("butter")) + sparkle(372, 166, 7, k("butter")) + sparkle(64, 120, 8, k("terra")))
    body = ("M 70 344 Q 58 332 74 326 L 126 322 C 150 300 196 266 238 238 C 266 216 304 214 318 242 "
            "C 330 270 322 312 314 344 Q 302 352 290 344 L 282 306 C 250 312 204 326 172 344 Z")
    b = f'<path d="{body}" fill="{k("shadow")}" transform="translate(10 6)"/><path d="{body}" fill="{k("fur")}"/>'
    legs = (f'<ellipse cx="88" cy="340" rx="24" ry="11" fill="{k("fur")}"/><ellipse cx="124" cy="342" rx="22" ry="10" fill="{k("fur")}"/>'
            f'<path d="M 82 334 V 344 M 94 334 V 344" stroke="{k("fur2")}" stroke-width="3.5" stroke-linecap="round"/>')
    t = tail(k, "M 314 246 C 344 216 334 170 360 150", 20)
    h = head(k, 130, 262, 0.42, tilt=-14, eyes="squeeze", mouth="yawn")
    return spot(k) + ground(k, 200, 356, 140) + moon + t + b + legs + h


def avatar(k):
    return (f'<circle cx="200" cy="200" r="200" fill="{k("spot")}"/>'
            + head(k, 200, 222, 0.84, eyes="happy", mouth="smile", shadow=False))


POSES = {
    "sleeping": ("Momo asleep on the TV", pose_sleeping),
    "peeking": ("Momo peeking over the edge", pose_peeking),
    "peeking-shy": ("Momo looking away", pose_peeking_shy),
    "knocking": ("Momo knocking a cup off the table", pose_knocking),
    "proud": ("Momo, proud, with a rosette", pose_proud),
    "detective": ("Momo the detective with a magnifying glass", pose_detective),
    "yawn": ("Momo stretching and yawning", pose_yawn),
    "avatar": ("Momo", avatar),
}


def build(name, inline):
    title, fn = POSES[name]
    k = lambda n: c(n, inline)  # noqa: E731
    cls = f' class="momo momo-{name}"' if inline else ""
    aria = f'role="img" aria-label="{title}"'
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 400"{cls} {aria}>'
            + ("" if inline else f"<title>{title}</title>") + fn(k) + "</svg>")


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    js = {}
    for name in POSES:
        (OUT / f"{name}.svg").write_text(build(name, False) + "\n")
        js[name] = build(name, True)
    (ROOT / "assets" / "js" / "mascot.js").write_text(
        "/* Momo the cat. Generated by tools/build-mascot.py. Colours follow --momo-* custom properties. */\n"
        f"window.MOMO = {json.dumps(js, indent=0)};\n")
    print("mascot written:", ", ".join(POSES))


if __name__ == "__main__":
    main()
