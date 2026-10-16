import pathlib
import struct

size = 32
pixels = bytearray()
for y in reversed(range(size)):
    for x in range(size):
        cream = 7 <= y <= 24 and (8 <= x <= 10 or 21 <= x <= 23 or abs(x - (9 + (y-7)*13/17)) <= 1.6)
        r, g, b = (250, 248, 239) if cream else (53, 60, 49)
        cx = min(max(x, 7), 24)
        cy = min(max(y, 7), 24)
        alpha = 255 if (x-cx)**2 + (y-cy)**2 <= 49 else 0
        pixels.extend((b, g, r, alpha))
mask = b'\0' * (size * 4)
bitmap = struct.pack('<IiiHHIIiiII', 40, size, size*2, 1, 32, 0, len(pixels), 0, 0, 0, 0) + pixels + mask
icon = struct.pack('<HHH', 0, 1, 1) + struct.pack('<BBBBHHII', size, size, 0, 0, 1, 32, len(bitmap), 22) + bitmap
destination = pathlib.Path(__file__).resolve().parent.parent / 'desktop' / 'icons' / 'icon.ico'
destination.parent.mkdir(parents=True, exist_ok=True)
destination.write_bytes(icon)
print('Original Neko icon generated')
