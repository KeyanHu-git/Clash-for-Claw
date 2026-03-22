from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter


REPO_ROOT = Path(__file__).resolve().parent.parent
SOURCE = REPO_ROOT / "artifacts" / "readme-window-raw.png"
TARGET = REPO_ROOT / "docs" / "readme-hero.png"
WINDOW_RADIUS = 28
SIDE_MARGIN = 36
TOP_MARGIN = 24
BOTTOM_MARGIN = 34


def rounded_mask(size: tuple[int, int], radius: int, scale: int = 4) -> Image.Image:
    width, height = size
    mask = Image.new("L", (width * scale, height * scale), 0)
    draw = ImageDraw.Draw(mask)
    draw.rounded_rectangle((0, 0, mask.width - 1, mask.height - 1), radius=radius * scale, fill=255)
    return mask.resize(size, Image.Resampling.LANCZOS)


def build_window_card(window: Image.Image) -> Image.Image:
    mask = rounded_mask(window.size, WINDOW_RADIUS)
    clipped = Image.new("RGBA", window.size, (0, 0, 0, 0))
    clipped.paste(window, (0, 0), mask)

    frame = Image.new("RGBA", window.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(frame)
    draw.rounded_rectangle(
        (0, 0, window.size[0] - 1, window.size[1] - 1),
        radius=WINDOW_RADIUS,
        outline=(255, 255, 255, 40),
        width=1,
    )
    return Image.alpha_composite(clipped, frame)


def build_shadow(window_size: tuple[int, int], radius: int) -> Image.Image:
    shadow_canvas = Image.new("RGBA", (window_size[0] + 72, window_size[1] + 72), (0, 0, 0, 0))

    for offset, color, blur, spread in [
        ((16, 18), (8, 12, 20, 18), 14, 10),
        ((16, 18), (8, 12, 20, 8), 24, 24),
    ]:
        mask = rounded_mask((window_size[0] + spread, window_size[1] + spread), radius + 2)
        layer = Image.new("RGBA", mask.size, color)
        pasted = Image.new("RGBA", shadow_canvas.size, (0, 0, 0, 0))
        pasted.paste(layer, offset, mask)
        shadow_canvas = Image.alpha_composite(shadow_canvas, pasted.filter(ImageFilter.GaussianBlur(blur)))

    return shadow_canvas


def main() -> None:
    if not SOURCE.exists():
        raise FileNotFoundError(f"raw capture not found: {SOURCE}")

    window = Image.open(SOURCE).convert("RGBA")
    canvas_size = (
        window.size[0] + (SIDE_MARGIN * 2),
        window.size[1] + TOP_MARGIN + BOTTOM_MARGIN,
    )
    canvas = Image.new("RGBA", canvas_size, (247, 249, 252, 255))

    card = build_window_card(window)
    shadow = build_shadow(window.size, WINDOW_RADIUS)

    x = SIDE_MARGIN
    y = TOP_MARGIN

    canvas.alpha_composite(shadow, (x - 18, y - 10))

    floor_shadow = Image.new("RGBA", canvas_size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(floor_shadow)
    draw.ellipse(
        (x + 110, y + window.size[1] - 2, x + window.size[0] - 110, y + window.size[1] + 28),
        fill=(10, 14, 22, 10),
    )
    floor_shadow = floor_shadow.filter(ImageFilter.GaussianBlur(14))
    canvas = Image.alpha_composite(canvas, floor_shadow)

    canvas.alpha_composite(card, (x, y))

    TARGET.parent.mkdir(parents=True, exist_ok=True)
    canvas.convert("RGB").save(TARGET, quality=96)
    print(f"saved {TARGET}")


if __name__ == "__main__":
    main()
