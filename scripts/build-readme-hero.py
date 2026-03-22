from pathlib import Path

from PIL import Image, ImageChops, ImageDraw, ImageFilter


REPO_ROOT = Path(__file__).resolve().parent.parent
SOURCE = REPO_ROOT / "artifacts" / "readme-window-raw.png"
TARGET = REPO_ROOT / "docs" / "readme-hero.png"
CANVAS_SIZE = (1680, 1080)
WINDOW_RADIUS = 28


def rounded_mask(size: tuple[int, int], radius: int, scale: int = 4) -> Image.Image:
    width, height = size
    mask = Image.new("L", (width * scale, height * scale), 0)
    draw = ImageDraw.Draw(mask)
    draw.rounded_rectangle((0, 0, mask.width - 1, mask.height - 1), radius=radius * scale, fill=255)
    return mask.resize(size, Image.Resampling.LANCZOS)


def gradient_background(size: tuple[int, int]) -> Image.Image:
    width, height = size
    base = Image.new("RGBA", size, "#f4f7fb")

    top = Image.new("RGBA", size, (0, 0, 0, 0))
    top_pixels = top.load()
    for y in range(height):
        ratio = y / max(height - 1, 1)
        r = int(244 - (ratio * 18))
        g = int(247 - (ratio * 14))
        b = int(251 - (ratio * 10))
        for x in range(width):
            top_pixels[x, y] = (r, g, b, 255)

    base = Image.alpha_composite(base, top)

    for bbox, color, blur in [
        ((-120, -40, 640, 520), (16, 24, 40, 32), 120),
        ((1040, 40, 1760, 760), (24, 144, 255, 20), 140),
        ((240, 700, 1340, 1260), (15, 23, 42, 16), 150),
    ]:
        layer = Image.new("RGBA", size, (0, 0, 0, 0))
        draw = ImageDraw.Draw(layer)
        draw.ellipse(bbox, fill=color)
        base = Image.alpha_composite(base, layer.filter(ImageFilter.GaussianBlur(blur)))

    vignette = Image.new("L", size, 0)
    draw = ImageDraw.Draw(vignette)
    draw.ellipse((-200, -160, width + 200, height + 220), fill=220)
    vignette = ImageChops.invert(vignette).filter(ImageFilter.GaussianBlur(100))
    shade = Image.new("RGBA", size, (8, 12, 22, 34))
    base = Image.composite(shade, base, vignette)
    return base


def layered_shadow(window_size: tuple[int, int], radius: int) -> Image.Image:
    shadow_canvas = Image.new("RGBA", (window_size[0] + 220, window_size[1] + 220), (0, 0, 0, 0))

    for offset, color, blur, spread in [
        ((80, 96), (9, 15, 26, 44), 48, 46),
        ((86, 104), (9, 15, 26, 26), 82, 78),
        ((84, 136), (9, 15, 26, 14), 120, 132),
    ]:
        mask = rounded_mask((window_size[0] + spread, window_size[1] + spread), radius + (spread // 8))
        layer = Image.new("RGBA", mask.size, color)
        pasted = Image.new("RGBA", shadow_canvas.size, (0, 0, 0, 0))
        pasted.paste(layer, offset, mask)
        shadow_canvas = Image.alpha_composite(shadow_canvas, pasted.filter(ImageFilter.GaussianBlur(blur)))

    return shadow_canvas


def build_window_card(window: Image.Image) -> Image.Image:
    mask = rounded_mask(window.size, WINDOW_RADIUS)
    clipped = Image.new("RGBA", window.size, (0, 0, 0, 0))
    clipped.paste(window, (0, 0), mask)

    frame = Image.new("RGBA", window.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(frame)
    draw.rounded_rectangle(
        (0, 0, window.size[0] - 1, window.size[1] - 1),
        radius=WINDOW_RADIUS,
        outline=(255, 255, 255, 78),
        width=1,
    )

    return Image.alpha_composite(clipped, frame)


def main() -> None:
    if not SOURCE.exists():
        raise FileNotFoundError(f"raw capture not found: {SOURCE}")

    canvas = gradient_background(CANVAS_SIZE)
    window = Image.open(SOURCE).convert("RGBA")
    card = build_window_card(window)

    x = (CANVAS_SIZE[0] - window.size[0]) // 2
    y = 108

    shadow = layered_shadow(window.size, WINDOW_RADIUS)
    canvas.alpha_composite(shadow, (x - 84, y - 70))

    floor_shadow = Image.new("RGBA", CANVAS_SIZE, (0, 0, 0, 0))
    draw = ImageDraw.Draw(floor_shadow)
    draw.ellipse(
        (x + 120, y + window.size[1] - 10, x + window.size[0] - 120, y + window.size[1] + 104),
        fill=(12, 18, 28, 22),
    )
    floor_shadow = floor_shadow.filter(ImageFilter.GaussianBlur(34))
    canvas = Image.alpha_composite(canvas, floor_shadow)

    canvas.alpha_composite(card, (x, y))

    TARGET.parent.mkdir(parents=True, exist_ok=True)
    canvas.convert("RGB").save(TARGET, quality=96)
    print(f"saved {TARGET}")


if __name__ == "__main__":
    main()
