#!/usr/bin/env python3
"""Genereert client/assets/wallpaparr.ico (32x32, 32-bpp) voor het tray-icoon.

Geen dependencies: het ICO-bestand wordt met de hand opgebouwd.
Motief: donker vierkant met blauwe berg en gele zon (zoals het favicon van de web-UI).

Gebruik:  python3 tools/make_icon.py
"""
import os
import struct

W = H = 32
BG = (0x12, 0x15, 0x1d)       # donker (web-UI --bg-2)
BLUE = (0x7a, 0xa2, 0xf7)     # accent
YELLOW = (0xf7, 0xc9, 0x48)   # accent-2


def pixel(x, y):
    # afgeronde hoeken: buiten de radius transparant
    r = 6
    for cx, cy in ((r, r), (W - 1 - r, r), (r, H - 1 - r), (W - 1 - r, H - 1 - r)):
        if (x < r and y < r and (x, y) == (0, 0)) or False:
            pass
    # eenvoudige radius-check per hoek
    if x < r and y < r and (x - r) ** 2 + (y - r) ** 2 > r * r:
        return (0, 0, 0, 0)
    if x > W - 1 - r and y < r and (x - (W - 1 - r)) ** 2 + (y - r) ** 2 > r * r:
        return (0, 0, 0, 0)
    if x < r and y > H - 1 - r and (x - r) ** 2 + (y - (H - 1 - r)) ** 2 > r * r:
        return (0, 0, 0, 0)
    if x > W - 1 - r and y > H - 1 - r and (x - (W - 1 - r)) ** 2 + (y - (H - 1 - r)) ** 2 > r * r:
        return (0, 0, 0, 0)

    # zon
    sx, sy, sr = 21.5, 9.5, 3.4
    if (x - sx) ** 2 + (y - sy) ** 2 <= sr * sr:
        return YELLOW + (255,)

    # bergen: twee driehoeken
    def berg(px, top_x, top_y, half, kleur):
        # driehoek met top op (top_x, top_y)
        if px < top_x - half or px > top_x + half:
            return None
        # hoogte van de rand op deze x
        rand = top_y + int((abs(px - top_x) / half) * (H - top_y))
        if y >= rand:
            return kleur + (255,)
        return None

    for kleur, tx, ty, half in ((BLUE, 12, 13, 11),):
        c = berg(x, tx, ty, half, kleur)
        if c:
            return c
    return BG + (255,)


def build_ico():
    # ICONIMAGE: BITMAPINFOHEADER + BGRA-pixels (bottom-up) + AND-mask
    pixels = []
    for y in range(H - 1, -1, -1):        # bottom-up
        for x in range(W):
            r, g, b, a = pixel(x, y)
            pixels += [b, g, r, a]        # BGRA
    xor = bytes(pixels)

    and_mask = bytearray()
    for y in range(H - 1, -1, -1):
        row = 0
        bits = 0
        for x in range(W):
            r, g, b, a = pixel(x, y)
            bits |= (1 if a == 0 else 0) << (7 - (x % 8))
            if x % 8 == 7:
                and_mask.append(bits)
                bits = 0
        and_mask.append(bits)
    # rijen uitlijnen op 4 bytes
    while len(and_mask) % 4:
        and_mask.append(0)

    bih = struct.pack("<IiiHHIIiiII", 40, W, H * 2, 1, 32, 0, len(xor), 0, 0, 0, 0)
    image = bih + xor + bytes(and_mask)

    header = struct.pack("<HHH", 0, 1, 1)
    entry = struct.pack("<BBBBHHII", W, H, 0, 0, 1, 32, len(image), 22)
    return header + entry + image


if __name__ == "__main__":
    hier = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    uit = os.path.join(hier, "client", "assets", "wallpaparr.ico")
    os.makedirs(os.path.dirname(uit), exist_ok=True)
    data = build_ico()
    with open(uit, "wb") as f:
        f.write(data)
    print(f"{uit}  {len(data)} bytes")
