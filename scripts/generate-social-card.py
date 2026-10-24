"""Generate the static 1200 x 630 social preview for the nodemap."""

from math import cos, pi, sin
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont


ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "web" / "og.png"
SCALE = 2
SIZE = (1200, 630)
W, H = (n * SCALE for n in SIZE)
GREEN = (166, 255, 0)
FONT = Path("/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf")
BOLD = Path("/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf")


def pt(x, y):
    return (round(x * SCALE), round(y * SCALE))


def font(size, bold=False):
    return ImageFont.truetype(str(BOLD if bold else FONT), round(size * SCALE))


def project(lat, lon, center=(900, 312), radius=221):
    lat, lon = lat * pi / 180, lon * pi / 180
    x = cos(lat) * sin(lon)
    y = sin(lat)
    z = cos(lat) * cos(lon)
    return pt(center[0] + radius * x, center[1] - radius * y), z


def curve(draw, coords, fill, width):
    segment = []
    for lat, lon in coords:
        p, depth = project(lat, lon)
        if depth > 0.015:
            segment.append(p)
        elif len(segment) > 1:
            draw.line(segment, fill=fill, width=width * SCALE, joint="curve")
            segment = []
        else:
            segment = []
    if len(segment) > 1:
        draw.line(segment, fill=fill, width=width * SCALE, joint="curve")


def main():
    image = Image.new("RGB", (W, H), (0, 0, 0))
    # A subtle green halo echoes the Blocknet site's luminous sphere.
    halo = Image.new("RGBA", (W, H))
    hd = ImageDraw.Draw(halo)
    hd.ellipse((*pt(663, 71), *pt(1137, 553)), fill=(*GREEN, 76))
    halo = halo.filter(ImageFilter.GaussianBlur(78 * SCALE))
    image = Image.alpha_composite(image.convert("RGBA"), halo)

    globe = Image.new("RGBA", (W, H))
    gd = ImageDraw.Draw(globe)
    gd.ellipse((*pt(679, 91), *pt(1121, 533)), fill=(7, 20, 2, 255), outline=(80, 125, 4, 255), width=2 * SCALE)
    for lat in range(-60, 61, 15):
        curve(gd, [(lat, lon) for lon in range(-90, 91, 2)], (72, 112, 7, 190), 1)
    for lon in range(-75, 76, 15):
        curve(gd, [(lat, lon) for lat in range(-90, 91, 2)], (72, 112, 7, 190), 1)

    # Static network routes suggest the map without implying live node counts.
    routes = [
        ((37, -57), (54, 17), 43),
        ((54, 17), (16, 66), 48),
        ((16, 66), (-27, 35), 34),
        ((-27, 35), (-8, -30), 36),
        ((-8, -30), (37, -57), 42),
        ((54, 17), (-27, 35), 45),
    ]
    route_glow = Image.new("RGBA", (W, H))
    rg = ImageDraw.Draw(route_glow)
    for (a_lat, a_lon), (b_lat, b_lon), lift in routes:
        points = []
        for i in range(101):
            t = i / 100
            lat = a_lat * (1 - t) + b_lat * t + lift * sin(pi * t)
            lon = a_lon * (1 - t) + b_lon * t
            p, depth = project(lat, lon)
            if depth > 0:
                points.append(p)
        if len(points) > 1:
            rg.line(points, fill=(*GREEN, 180), width=2 * SCALE, joint="curve")
    image = Image.alpha_composite(image, globe)
    image = Image.alpha_composite(image, route_glow.filter(ImageFilter.GaussianBlur(7 * SCALE)))
    image = Image.alpha_composite(image, route_glow)

    draw = ImageDraw.Draw(image)
    for lat, lon in [(37, -57), (54, 17), (16, 66), (-27, 35), (-8, -30)]:
        p, _ = project(lat, lon)
        x, y = p
        draw.ellipse((x - 7*SCALE, y - 7*SCALE, x + 7*SCALE, y + 7*SCALE), fill=(*GREEN, 75))
        draw.ellipse((x - 3*SCALE, y - 3*SCALE, x + 3*SCALE, y + 3*SCALE), fill=(*GREEN, 255))

    draw.text(pt(70, 183), "blocknet", font=font(76), fill=(242, 242, 242))
    draw.text(pt(430, 183), "_", font=font(76, bold=True), fill=(*GREEN, 255))
    draw.text(pt(73, 290), "live network nodemap", font=font(25), fill=(147, 147, 147))
    draw.rectangle((*pt(72, 369), *pt(401, 416)), outline=(84, 130, 0), width=SCALE)
    draw.text(pt(92, 380), "EXPLORE THE NETWORK", font=font(18, bold=True), fill=(*GREEN, 255))
    draw.text(pt(73, 565), "blocknetnodes.toxicrainbows.com", font=font(15), fill=(85, 85, 85))

    image.convert("RGB").resize(SIZE, Image.Resampling.LANCZOS).save(OUT, optimize=True)


if __name__ == "__main__":
    main()
