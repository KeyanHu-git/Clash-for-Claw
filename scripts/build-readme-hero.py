from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter


REPO_ROOT = Path(__file__).resolve().parent.parent
SOURCE = REPO_ROOT / "artifacts" / "readme-window-printwindow.png"
TARGET = REPO_ROOT / "docs" / "readme-hero.png"


def rounded_mask(size: tuple[int, int], radius: int, scale: int = 4) -> Image.Image:
    width, height = size
    mask = Image.new("L", (width * scale, height * scale), 0)
    draw = ImageDraw.Draw(mask)
    draw.rounded_rectangle((0, 0, mask.width - 1, mask.height - 1), radius=radius * scale, fill=255)
    return mask.resize(size, Image.Resampling.LANCZOS)


def ellipse_glow(canvas_size: tuple[int, int], bbox: tuple[int, int, int, int], color: tuple[int, int, int, int], blur: int) -> Image.Image:
    layer = Image.new("RGBA", canvas_size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)
    draw.ellipse(bbox, fill=color)
    return layer.filter(ImageFilter.GaussianBlur(blur))


def main() -> None:
    canvas_size = (1660, 1080)
    canvas = Image.new("RGBA", canvas_size, "#fcfdff")

    # Light background atmosphere, kept subtle so the window stays primary.
    for bbox, color, blur in [
        ((-180, 600, 720, 1380), (39, 195, 245, 18), 100),
        ((1080, -160, 1780, 520), (20, 184, 166, 16), 90),
        ((420, 80, 1450, 910), (15, 23, 42, 8), 110),
    ]:
        canvas = Image.alpha_composite(canvas, ellipse_glow(canvas_size, bbox, color, blur))

    window = Image.open(SOURCE).convert("RGBA")
    window_size = window.size

    # Keep the original capture size so the README cover does not distort the UI.
    x = (canvas_size[0] - window_size[0]) // 2
    y = 96
    radius = 24
    mask = rounded_mask(window_size, radius)

    shadow = Image.new("RGBA", (window_size[0] + 120, window_size[1] + 140), (0, 0, 0, 0))
    shadow_mask = rounded_mask((window_size[0] + 24, window_size[1] + 24), radius + 10)
    shadow_card = Image.new("RGBA", shadow_mask.size, (12, 18, 28, 52))
    shadow.paste(shadow_card, (48, 40), shadow_mask)
    shadow = shadow.filter(ImageFilter.GaussianBlur(28))
    canvas.alpha_composite(shadow, (x - 56, y - 14))

    floor_shadow = Image.new("RGBA", canvas_size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(floor_shadow)
    draw.ellipse((x + 120, y + window_size[1] - 8, x + window_size[0] - 120, y + window_size[1] + 92), fill=(15, 23, 42, 18))
    floor_shadow = floor_shadow.filter(ImageFilter.GaussianBlur(32))
    canvas = Image.alpha_composite(canvas, floor_shadow)

    clipped_window = Image.new("RGBA", window_size, (0, 0, 0, 0))
    clipped_window.paste(window, (0, 0), mask)
    canvas.alpha_composite(clipped_window, (x, y))

    TARGET.parent.mkdir(parents=True, exist_ok=True)
    canvas.convert("RGB").save(TARGET, quality=95)
    print(f"saved {TARGET}")


if __name__ == "__main__":
    main()
