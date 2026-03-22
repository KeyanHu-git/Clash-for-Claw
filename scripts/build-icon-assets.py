from pathlib import Path

from PIL import Image, ImageChops, ImageDraw, ImageFilter


REPO_ROOT = Path(__file__).resolve().parent.parent
ASSETS = REPO_ROOT / "Assets"


def rounded_bar_mask(size: tuple[int, int], radius: int) -> Image.Image:
    mask = Image.new("L", size, 0)
    draw = ImageDraw.Draw(mask)
    draw.rounded_rectangle((0, 0, size[0], size[1]), radius=radius, fill=255)
    return mask


def linear_gradient(size: tuple[int, int], start: tuple[int, int, int], end: tuple[int, int, int], horizontal: bool = False) -> Image.Image:
    width, height = size
    gradient = Image.new("RGBA", size)
    px = gradient.load()
    for y in range(height):
        for x in range(width):
            t = (x / max(1, width - 1)) if horizontal else (y / max(1, height - 1))
            px[x, y] = (
                round(start[0] + (end[0] - start[0]) * t),
                round(start[1] + (end[1] - start[1]) * t),
                round(start[2] + (end[2] - start[2]) * t),
                255,
            )
    return gradient


def alpha_blur(canvas: Image.Image, bbox: tuple[int, int, int, int], color: tuple[int, int, int, int], blur: int) -> Image.Image:
    layer = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)
    draw.ellipse(bbox, fill=color)
    canvas.alpha_composite(layer.filter(ImageFilter.GaussianBlur(blur)))
    return canvas


def apply_alpha(img: Image.Image, factor: float) -> Image.Image:
    alpha = img.getchannel("A").point(lambda a: int(a * factor))
    img.putalpha(alpha)
    return img


def render_icon_master(size: int = 1024) -> Image.Image:
    canvas = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    canvas = alpha_blur(canvas, (150, 150, 900, 900), (44, 220, 255, 28), 52)
    canvas = alpha_blur(canvas, (265, 265, 760, 760), (36, 203, 191, 16), 32)

    ring_layer = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
    ring_draw = ImageDraw.Draw(ring_layer)
    ring_draw.arc((126, 126, 898, 898), start=44, end=316, fill=(44, 212, 255, 255), width=126)
    ring_draw.arc((210, 210, 814, 814), start=52, end=308, fill=(17, 55, 84, 232), width=26)
    canvas.alpha_composite(ring_layer)

    fill_ring = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
    fill_draw = ImageDraw.Draw(fill_ring)
    fill_draw.arc((214, 214, 810, 810), start=42, end=316, fill=(176, 247, 255, 184), width=92)
    fill_ring = fill_ring.filter(ImageFilter.GaussianBlur(8))
    canvas.alpha_composite(fill_ring)

    core = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
    core_draw = ImageDraw.Draw(core)
    core_draw.ellipse((298, 298, 726, 726), fill=(20, 34, 48, 248))
    core_draw.ellipse((334, 334, 690, 690), fill=(24, 40, 57, 236))
    core_draw.ellipse((290, 290, 734, 734), outline=(255, 255, 255, 32), width=16)
    canvas.alpha_composite(core)

    shine = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
    shine_draw = ImageDraw.Draw(shine)
    shine_draw.pieslice((210, 180, 618, 636), start=145, end=233, fill=(255, 255, 255, 88))
    shine_draw.pieslice((228, 196, 602, 620), start=148, end=230, fill=(255, 255, 255, 0))
    shine = shine.filter(ImageFilter.GaussianBlur(10))
    canvas.alpha_composite(shine)

    bars_layer = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
    bar_gradient = linear_gradient((84, 374), (102, 246, 245), (32, 214, 184))
    bar_mask = rounded_bar_mask((84, 374), 36)
    bar_gradient.putalpha(bar_mask)

    for index, x in enumerate([566, 636, 706, 776]):
        bar = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
        bar.paste(bar_gradient, (x, 282 + index * 4), bar_gradient)
        bar = bar.rotate(27, center=(x + 42, 468), resample=Image.Resampling.BICUBIC)
        opacity = 0.82 - (index * 0.09)
        bar = apply_alpha(bar, opacity)
        bars_layer.alpha_composite(bar)

    bars_shadow = bars_layer.copy().filter(ImageFilter.GaussianBlur(12))
    bars_shadow = apply_alpha(bars_shadow, 0.22)
    bars_shadow = ImageChops.offset(bars_shadow, -14, 14)
    canvas.alpha_composite(bars_shadow)
    canvas.alpha_composite(bars_layer)

    bars_glow = bars_layer.copy().filter(ImageFilter.GaussianBlur(20))
    bars_glow = apply_alpha(bars_glow, 0.24)
    canvas.alpha_composite(bars_glow)

    claw_cuts = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
    claw_draw = ImageDraw.Draw(claw_cuts)
    for points in [
        [(772, 262), (854, 302), (818, 372), (736, 332)],
        [(804, 654), (886, 694), (850, 764), (768, 724)],
    ]:
        claw_draw.polygon(points, fill=(44, 212, 255, 255))
    claw_cuts = claw_cuts.filter(ImageFilter.GaussianBlur(1))
    canvas.alpha_composite(claw_cuts)

    return canvas


def centered_icon(master: Image.Image, canvas_size: tuple[int, int], icon_scale: float, glow: bool = False) -> Image.Image:
    canvas = Image.new("RGBA", canvas_size, (0, 0, 0, 0))
    target = round(min(canvas_size) * icon_scale)
    icon = master.resize((target, target), Image.Resampling.LANCZOS)
    x = (canvas_size[0] - target) // 2
    y = (canvas_size[1] - target) // 2
    if glow:
        glow_layer = icon.filter(ImageFilter.GaussianBlur(max(6, target // 18)))
        glow_alpha = glow_layer.getchannel("A").point(lambda a: int(a * 0.22))
        glow_layer.putalpha(glow_alpha)
        canvas.alpha_composite(glow_layer, (x, y))
    canvas.alpha_composite(icon, (x, y))
    return canvas


def save_png(img: Image.Image, path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    img.save(path)


def main() -> None:
    master = render_icon_master(1024)

    preview = master.resize((512, 512), Image.Resampling.LANCZOS)
    save_png(preview, ASSETS / "ClashForClaw-icon-preview.png")

    ico_sizes = [(256, 256), (128, 128), (96, 96), (64, 64), (48, 48), (32, 32), (24, 24), (16, 16)]
    master.save(ASSETS / "ClashForClaw.ico", sizes=ico_sizes)

    save_png(centered_icon(master, (300, 300), 0.86, glow=True), ASSETS / "Square150x150Logo.scale-200.png")
    save_png(centered_icon(master, (88, 88), 0.84, glow=False), ASSETS / "Square44x44Logo.scale-200.png")
    save_png(centered_icon(master, (24, 24), 0.86, glow=False), ASSETS / "Square44x44Logo.targetsize-24_altform-unplated.png")
    save_png(centered_icon(master, (50, 50), 0.84, glow=False), ASSETS / "StoreLogo.png")
    save_png(centered_icon(master, (150, 150), 0.86, glow=True), ASSETS / "Square150x150Logo.png")
    save_png(centered_icon(master, (44, 44), 0.84, glow=False), ASSETS / "Square44x44Logo.png")

    wide = Image.new("RGBA", (620, 300), (15, 22, 34, 255))
    wide = alpha_blur(wide, (40, 10, 370, 290), (44, 215, 255, 34), 46)
    wide = alpha_blur(wide, (270, -10, 610, 260), (32, 214, 184, 24), 54)
    wide_icon = centered_icon(master, (220, 220), 0.92, glow=False)
    wide.alpha_composite(wide_icon, (36, 40))
    wide_text = ImageDraw.Draw(wide)
    wide_text.rounded_rectangle((250, 68, 574, 232), radius=30, fill=(22, 32, 48, 168))
    save_png(wide, ASSETS / "Wide310x150Logo.scale-200.png")

    splash = Image.new("RGBA", (1240, 600), (9, 16, 27, 255))
    splash = alpha_blur(splash, (60, 80, 640, 640), (44, 215, 255, 32), 80)
    splash = alpha_blur(splash, (580, -20, 1220, 520), (32, 214, 184, 20), 90)
    splash_icon = centered_icon(master, (280, 280), 0.94, glow=False)
    splash.alpha_composite(splash_icon, (138, 160))
    save_png(splash, ASSETS / "SplashScreen.scale-200.png")
    save_png(splash.resize((620, 300), Image.Resampling.LANCZOS), ASSETS / "SplashScreen.png")

    lock = centered_icon(master, (48, 48), 0.80, glow=False)
    save_png(lock, ASSETS / "LockScreenLogo.scale-200.png")

    print("Icon assets regenerated.")


if __name__ == "__main__":
    main()
