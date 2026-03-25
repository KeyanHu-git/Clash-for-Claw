from __future__ import annotations

from pathlib import Path
import textwrap
from PIL import Image, ImageDraw, ImageFilter, ImageFont


ROOT = Path(__file__).resolve().parents[1]
DOCS = ROOT / "docs"
ASSETS = ROOT / "Assets"

OUTPUT = DOCS / "github-social-preview.png"
SCREENSHOT = DOCS / "readme-hero.png"
ICON = ASSETS / "ClashForClaw-icon-preview.png"

WIDTH = 1280
HEIGHT = 640


def hex_rgba(value: str, alpha: int = 255) -> tuple[int, int, int, int]:
    value = value.lstrip("#")
    return tuple(int(value[i : i + 2], 16) for i in (0, 2, 4)) + (alpha,)


def load_font(size: int, bold: bool = False) -> ImageFont.FreeTypeFont:
    font_name = "segoeuib.ttf" if bold else "segoeui.ttf"
    return ImageFont.truetype(str(Path("C:/Windows/Fonts") / font_name), size=size)


def vertical_gradient(size: tuple[int, int], top: tuple[int, int, int], bottom: tuple[int, int, int]) -> Image.Image:
    width, height = size
    image = Image.new("RGB", size, top)
    draw = ImageDraw.Draw(image)
    for y in range(height):
        ratio = y / max(1, height - 1)
        color = tuple(int(top[i] + (bottom[i] - top[i]) * ratio) for i in range(3))
        draw.line((0, y, width, y), fill=color)
    return image


def draw_glow(draw: ImageDraw.ImageDraw, box: tuple[int, int, int, int], color: tuple[int, int, int], alpha: int) -> Image.Image:
    layer = Image.new("RGBA", (WIDTH, HEIGHT), (0, 0, 0, 0))
    layer_draw = ImageDraw.Draw(layer)
    layer_draw.rounded_rectangle(box, radius=28, fill=color + (alpha,))
    return layer.filter(ImageFilter.GaussianBlur(36))


def fit_image(image: Image.Image, target: tuple[int, int]) -> Image.Image:
    target_w, target_h = target
    ratio = max(target_w / image.width, target_h / image.height)
    resized = image.resize((int(image.width * ratio), int(image.height * ratio)), Image.Resampling.LANCZOS)
    left = (resized.width - target_w) // 2
    top = (resized.height - target_h) // 2
    return resized.crop((left, top, left + target_w, top + target_h))


def add_chip(draw: ImageDraw.ImageDraw, xy: tuple[int, int], text: str) -> int:
    x, y = xy
    font = load_font(20, bold=True)
    bbox = draw.textbbox((0, 0), text, font=font)
    chip_w = bbox[2] - bbox[0] + 32
    chip_h = 42
    draw.rounded_rectangle((x, y, x + chip_w, y + chip_h), radius=16, fill=hex_rgba("#0F1F34", 255), outline=hex_rgba("#1CC8F3", 90), width=1)
    draw.text((x + 16, y + 9), text, font=font, fill=hex_rgba("#D7F6FF", 255))
    return chip_w


def draw_wrapped_text(
    draw: ImageDraw.ImageDraw,
    xy: tuple[int, int],
    text: str,
    font: ImageFont.FreeTypeFont,
    fill: tuple[int, int, int, int],
    max_width: int,
    line_spacing: int = 8,
) -> None:
    words = text.split()
    lines: list[str] = []
    current = ""
    for word in words:
        candidate = word if not current else current + " " + word
        bbox = draw.textbbox((0, 0), candidate, font=font)
        if bbox[2] - bbox[0] <= max_width:
            current = candidate
        else:
            if current:
                lines.append(current)
            current = word
    if current:
        lines.append(current)

    x, y = xy
    for line in lines:
        draw.text((x, y), line, font=font, fill=fill)
        bbox = draw.textbbox((0, 0), line, font=font)
        y += (bbox[3] - bbox[1]) + line_spacing


def main() -> None:
    DOCS.mkdir(parents=True, exist_ok=True)

    background = vertical_gradient((WIDTH, HEIGHT), (8, 18, 30), (12, 28, 46)).convert("RGBA")
    overlay = Image.new("RGBA", (WIDTH, HEIGHT), (0, 0, 0, 0))
    overlay_draw = ImageDraw.Draw(overlay)

    # Soft structural panels
    overlay_draw.rounded_rectangle((44, 40, WIDTH - 44, HEIGHT - 40), radius=36, outline=hex_rgba("#5CE1FF", 28), width=1)
    overlay_draw.rounded_rectangle((54, 52, 610, HEIGHT - 52), radius=28, fill=hex_rgba("#0C1624", 205))
    overlay_draw.rounded_rectangle((640, 78, WIDTH - 54, HEIGHT - 78), radius=30, fill=hex_rgba("#101C2D", 220))

    # Ambient graphic lines
    for offset in range(0, 320, 28):
        overlay_draw.arc((760 - offset, 40 + offset, 1280 - offset, 560 + offset), 210, 320, fill=hex_rgba("#1CC8F3", max(10, 42 - offset // 10)), width=2)
    for x in range(90, 560, 48):
        overlay_draw.line((x, 420, x + 120, 580), fill=hex_rgba("#1CC8F3", 12), width=1)
    for y in range(110, 570, 38):
        overlay_draw.line((80, y, 550, y), fill=hex_rgba("#85E9FF", 8), width=1)

    canvas = Image.alpha_composite(background, overlay)

    # Glows
    canvas = Image.alpha_composite(canvas, draw_glow(ImageDraw.Draw(canvas), (675, 120, 1215, 500), (23, 205, 255), 42))
    canvas = Image.alpha_composite(canvas, draw_glow(ImageDraw.Draw(canvas), (86, 88, 530, 280), (23, 205, 255), 28))

    draw = ImageDraw.Draw(canvas)

    # Icon
    icon = Image.open(ICON).convert("RGBA")
    icon = fit_image(icon, (66, 66))
    icon_bg = Image.new("RGBA", (82, 82), (0, 0, 0, 0))
    ImageDraw.Draw(icon_bg).rounded_rectangle((0, 0, 82, 82), radius=22, fill=hex_rgba("#0D2034", 255), outline=hex_rgba("#1CC8F3", 80), width=1)
    icon_bg.alpha_composite(icon, (8, 8))
    canvas.alpha_composite(icon_bg, (86, 84))

    # Header copy
    small = load_font(18, bold=True)
    title = load_font(54, bold=True)
    subtitle = load_font(24, bold=False)
    body = load_font(22, bold=False)

    draw.text((182, 92), "OPENCLAW / CLAW", font=small, fill=hex_rgba("#39D7FF", 255))
    draw.text((84, 182), "Clash for Claw", font=title, fill=hex_rgba("#F4FBFF", 255))
    draw.text((86, 255), "Windows desktop proxy client", font=subtitle, fill=hex_rgba("#CDE8F5", 255))
    draw_wrapped_text(
        draw,
        (86, 298),
        "mihomo subscriptions, local ports, Windows service mode, and seamless failover in one control panel.",
        body,
        hex_rgba("#9DB6C7", 255),
        max_width=470,
        line_spacing=10,
    )

    chip_x = 86
    chip_y = 392
    chip_x += add_chip(draw, (chip_x, chip_y), "mihomo / Clash") + 12
    chip_x += add_chip(draw, (chip_x, chip_y), "Windows service") + 12
    add_chip(draw, (86, chip_y + 56), "OpenClaw gateway")
    add_chip(draw, (298, chip_y + 56), "Seamless failover")

    # Screenshot card
    screenshot_box = (680, 118, 1210, 500)
    card = Image.new("RGBA", (screenshot_box[2] - screenshot_box[0], screenshot_box[3] - screenshot_box[1]), (0, 0, 0, 0))
    card_draw = ImageDraw.Draw(card)
    card_draw.rounded_rectangle((0, 0, card.width, card.height), radius=28, fill=hex_rgba("#0A1420", 255), outline=hex_rgba("#1CC8F3", 60), width=1)

    screenshot = Image.open(SCREENSHOT).convert("RGBA")
    screenshot = screenshot.crop((44, 54, screenshot.width - 44, screenshot.height - 90))
    screenshot = fit_image(screenshot, (card.width - 24, card.height - 24))
    mask = Image.new("L", (card.width - 24, card.height - 24), 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, mask.width, mask.height), radius=22, fill=255)
    card.alpha_composite(screenshot, (12, 12))
    card.putalpha(ImageChops.screen(card.getchannel("A"), Image.new("L", card.size, 255)))
    # Apply rounded crop mask to screenshot area only
    rounded_card = Image.new("RGBA", card.size, (0, 0, 0, 0))
    rounded_card_draw = ImageDraw.Draw(rounded_card)
    rounded_card_draw.rounded_rectangle((0, 0, card.width, card.height), radius=28, fill=(255, 255, 255, 255))
    card = Image.composite(card, Image.new("RGBA", card.size, (0, 0, 0, 0)), rounded_card.getchannel("A"))
    canvas.alpha_composite(card, (screenshot_box[0], screenshot_box[1]))

    # Footer line
    footer_font = load_font(18, bold=False)
    draw.text((86, 560), "Subscription mode  •  Local port mode  •  Windows service mode  •  OpenClaw / Claw", font=footer_font, fill=hex_rgba("#6E8EA3", 255))

    canvas.convert("RGB").save(OUTPUT, quality=95)


if __name__ == "__main__":
    from PIL import ImageChops

    main()
