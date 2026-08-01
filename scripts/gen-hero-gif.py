"""Render .github/assets/hero.gif — the README/social animation.

A terminal types `punch 3000`, the share link prints, and a browser slides
in with the shared site. Palette matches the landing page.

    uv run --with pillow python scripts/gen-hero-gif.py
"""
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

S = 3
OUT = 1440, 663
W, H = 960 * S, 442 * S

BG = (255, 255, 255)
WASH = (246, 242, 254)
INK = (11, 11, 12)
BODY = (63, 63, 70)
FAINT = (113, 113, 122)
LINE = (230, 230, 233)
ACCENT = (109, 40, 217)

TERM_BG = (23, 22, 28)
TERM_BAR = (31, 30, 38)
TERM_TEXT = (242, 241, 246)
TERM_MUTED = (150, 148, 162)
TERM_ACCENT = (196, 181, 253)

F = "/System/Library/Fonts/Helvetica.ttc"
M = "/System/Library/Fonts/Menlo.ttc"
f_mono = ImageFont.truetype(M, 20 * S)
f_mono_sm = ImageFont.truetype(M, 16 * S)
f_ui = ImageFont.truetype(F, 19 * S)
f_ui_b = ImageFont.truetype(F, 24 * S, index=1)
f_big = ImageFont.truetype(F, 31 * S, index=1)
f_sm = ImageFont.truetype(F, 15 * S)

TERM = (30 * S, 92 * S, 500 * S, 400 * S)
BROW = (520 * S, 92 * S, 930 * S, 400 * S)

OUTPUT = Path(__file__).resolve().parent.parent / ".github/assets/hero.gif"
CMD = "punch 3000"
LINK = "punchpage.pages.dev/#r=8fK2…"


def base():
    im = Image.new("RGB", (W, H), BG)
    d = ImageDraw.Draw(im)
    for i in range(int(H * 0.7)):
        t = 1 - i / (H * 0.7)
        c = tuple(round(BG[k] + (WASH[k] - BG[k]) * (t ** 1.5)) for k in range(3))
        d.line([(0, i), (W, i)], fill=c)
    return im, d


def panel(d, box, fill, radius=12):
    x0, y0, x1, y1 = box
    d.rounded_rectangle([x0 + 3 * S, y0 + 5 * S, x1 + 3 * S, y1 + 5 * S], radius * S, fill=(238, 236, 243))
    d.rounded_rectangle(box, radius * S, fill=fill, outline=LINE if fill == BG else None, width=S)


def dots(d, x, y, dark):
    for i, c in enumerate([(255, 95, 87), (255, 189, 46), (39, 201, 63)] if not dark else
                          [(90, 88, 100), (90, 88, 100), (90, 88, 100)]):
        d.ellipse([x + i * 15 * S, y, x + i * 15 * S + 8 * S, y + 8 * S], fill=c)


def terminal(d, typed, lines):
    x0, y0, x1, y1 = TERM
    panel(d, TERM, TERM_BG)
    d.rounded_rectangle([x0, y0, x1, y0 + 36 * S], 12 * S, fill=TERM_BAR)
    d.rectangle([x0, y0 + 24 * S, x1, y0 + 36 * S], fill=TERM_BAR)
    dots(d, x0 + 16 * S, y0 + 14 * S, True)
    d.text((x0 + (x1 - x0) / 2 - d.textlength("terminal", font=f_sm) / 2, y0 + 11 * S),
           "terminal", font=f_sm, fill=TERM_MUTED)

    tx, ty = x0 + 24 * S, y0 + 60 * S
    d.text((tx, ty), "$", font=f_mono, fill=TERM_ACCENT)
    d.text((tx + 16 * S, ty), typed, font=f_mono, fill=TERM_TEXT)
    if len(typed) < len(CMD):
        cw = d.textlength(typed, font=f_mono)
        d.rectangle([tx + 16 * S + cw + 2 * S, ty, tx + 16 * S + cw + 9 * S, ty + 16 * S], fill=TERM_TEXT)

    ly = ty + 44 * S
    for text, color, font in lines:
        d.text((tx, ly), text, font=font, fill=color)
        ly += 34 * S


def browser(d, progress, show_page):
    x0, y0, x1, y1 = BROW
    # slide in from the right
    off = int((1 - progress) * 40 * S)
    box = (x0 + off, y0, x1 + off, y1)
    panel(d, box, BG)
    bx0, by0, bx1, by1 = box
    d.rounded_rectangle([bx0, by0, bx1, by0 + 40 * S], 12 * S, fill=(248, 248, 250))
    d.rectangle([bx0, by0 + 28 * S, bx1, by0 + 40 * S], fill=(248, 248, 250))
    d.line([bx0, by0 + 40 * S, bx1, by0 + 40 * S], fill=LINE)
    dots(d, bx0 + 16 * S, by0 + 16 * S, False)
    # url pill
    ux0, ux1 = bx0 + 72 * S, bx1 - 18 * S
    d.rounded_rectangle([ux0, by0 + 9 * S, ux1, by0 + 31 * S], 11 * S, fill=(238, 238, 241))
    d.text((ux0 + 12 * S, by0 + 13 * S), LINK, font=f_mono_sm, fill=FAINT)

    if not show_page:
        return
    # the shared site
    px = bx0 + 26 * S
    py = by0 + 62 * S
    d.text((px, py), "Your local app", font=f_big, fill=INK)
    d.text((px, py + 48 * S), "Running on your machine,", font=f_ui, fill=BODY)
    d.text((px, py + 74 * S), "open on someone else's.", font=f_ui, fill=BODY)
    # a couple of faux content rows
    ry = py + 104 * S
    for w in (0.82, 0.62, 0.72):
        d.rounded_rectangle([px, ry, px + int((bx1 - bx0 - 52 * S) * w), ry + 13 * S], 6 * S, fill=(237, 237, 241))
        ry += 24 * S
    d.rounded_rectangle([px, ry + 8 * S, px + 128 * S, ry + 44 * S], 9 * S, fill=ACCENT)
    label = "Shared live"
    d.text((px + 64 * S - d.textlength(label, font=f_sm) / 2, ry + 18 * S), label, font=f_sm, fill=(255, 255, 255))


def caption(d, text):
    d.text((W / 2 - d.textlength(text, font=f_ui_b) / 2, 30 * S), text, font=f_ui_b, fill=INK)


frames, durations = [], []

# 1. typing the command
for i in range(1, len(CMD) + 1):
    im, d = base()
    caption(d, "One command shares your local app")
    terminal(d, CMD[:i], [])
    frames.append(im)
    durations.append(70)

# 2. output: sharing + link
sharing = [("PunchPage is sharing http://127.0.0.1:3000", TERM_MUTED, f_mono_sm)]
link = sharing + [("", TERM_TEXT, f_mono_sm), (LINK, TERM_ACCENT, f_mono_sm)]
for lines, hold in ((sharing, 420), (link, 900)):
    im, d = base()
    caption(d, "One command shares your local app")
    terminal(d, CMD, lines)
    frames.append(im)
    durations.append(hold)

# 3. browser slides in, page loads
for p in (0.35, 0.7, 1.0):
    im, d = base()
    caption(d, "Anyone opens the link — no server in between")
    terminal(d, CMD, link)
    browser(d, p, False)
    frames.append(im)
    durations.append(110)
for hold in (140, 2600):
    im, d = base()
    caption(d, "Anyone opens the link — no server in between")
    terminal(d, CMD, link)
    browser(d, 1.0, True)
    frames.append(im)
    durations.append(hold)

small = [f.resize(OUT, Image.LANCZOS) for f in frames]
pal = [f.quantize(colors=128, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE) for f in small]
pal[0].save(
    OUTPUT,
    save_all=True, append_images=pal[1:], duration=durations, loop=0, disposal=2, optimize=False,
)
print(f"wrote {OUTPUT} {small[0].size} {len(pal)} frames")
