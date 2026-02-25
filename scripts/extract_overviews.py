#!/usr/bin/env python3
"""Extract CS2 map radar PNGs from the game's VPK archive.

Usage:
    python3 extract_overviews.py [--game-dir <path>] [--out <dir>]

Defaults:
    game-dir: /mnt/c/Program Files (x86)/Steam/steamapps/common/Counter-Strike Global Offensive
    out:      ../internal/maps/overviews/

Run this when Valve updates a map's radar image. After running, rebuild the binary
so the new PNGs are embedded via go:embed.
"""

import argparse
import re
import struct
import sys
from pathlib import Path

try:
    import lz4.block
    from PIL import Image
except ImportError:
    print("Required packages missing. Run: pip3 install lz4 Pillow")
    sys.exit(1)

PRO_MAPS = [
    "de_ancient",
    "de_anubis",
    "de_dust2",
    "de_inferno",
    "de_mirage",
    "de_nuke",
    "de_overpass",
    "de_train",
    "de_vertigo",
]

DEFAULT_GAME_DIR = "/mnt/c/Program Files (x86)/Steam/steamapps/common/Counter-Strike Global Offensive"
DEFAULT_OUT_DIR  = Path(__file__).parent.parent / "internal" / "maps" / "overviews"


def read_cstring(f):
    b = b""
    while True:
        c = f.read(1)
        if c in (b"\x00", b""):
            return b.decode("utf-8", errors="replace")
        b += c


def extract_vpk_file(vpk_dir_path: str, archive_index: int, offset: int, length: int) -> bytes:
    pak = vpk_dir_path.replace("_dir.vpk", f"_{archive_index:03d}.vpk")
    with open(pak, "rb") as f:
        f.seek(offset)
        return f.read(length)


def scan_vpk(vpk_dir_path: str, wanted: dict) -> dict:
    """Scan the VPK directory and return {key: (archive_index, offset, length)} for wanted paths."""
    found = {}
    with open(vpk_dir_path, "rb") as f:
        f.read(28)  # skip header
        while True:
            ext = read_cstring(f)
            if not ext:
                break
            while True:
                path = read_cstring(f)
                if not path:
                    break
                while True:
                    name = read_cstring(f)
                    if not name:
                        break
                    crc            = struct.unpack("<I", f.read(4))[0]
                    preload_bytes  = struct.unpack("<H", f.read(2))[0]
                    archive_index  = struct.unpack("<H", f.read(2))[0]
                    entry_offset   = struct.unpack("<I", f.read(4))[0]
                    entry_length   = struct.unpack("<I", f.read(4))[0]
                    term           = struct.unpack("<H", f.read(2))[0]
                    preload        = f.read(preload_bytes)
                    full_path      = f"{path}/{name}.{ext}"
                    if full_path in wanted:
                        found[wanted[full_path]] = (archive_index, entry_offset, entry_length)
    return found


def decode_vtex(data: bytes) -> Image.Image:
    """Decode a Source 2 compiled texture (vtex_c) to a PIL Image.

    The texture data is LZ4-block-compressed RGBA8888 at 1024×1024.
    """
    file_size = struct.unpack_from("<I", data, 0)[0]
    pixel_data = data[file_size:]
    raw = lz4.block.decompress(pixel_data, uncompressed_size=4 * 1024 * 1024)
    return Image.frombytes("RGBA", (1024, 1024), raw, "raw", "RGBA")


def parse_overview_meta(txt_bytes: bytes) -> dict:
    text = txt_bytes.decode("utf-8", errors="replace")
    meta = {}
    for key in ("pos_x", "pos_y", "scale"):
        m = re.search(rf'"{key}"\s+"([^"]+)"', text)
        if m:
            meta[key] = float(m.group(1))
    return meta


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--game-dir", default=DEFAULT_GAME_DIR)
    parser.add_argument("--out", default=str(DEFAULT_OUT_DIR))
    args = parser.parse_args()

    game_dir = Path(args.game_dir)
    out_dir  = Path(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)

    vpk_path = str(game_dir / "game" / "csgo" / "pak01_dir.vpk")

    wanted = {}
    for m in PRO_MAPS:
        wanted[f"panorama/images/overheadmaps/{m}_radar_psd.vtex_c"]       = m
        wanted[f"panorama/images/overheadmaps/{m}_lower_radar_psd.vtex_c"] = f"{m}_lower"
        wanted[f"resource/overviews/{m}.txt"]                               = f"_txt_{m}"

    print(f"Scanning {vpk_path} ...")
    found = scan_vpk(vpk_path, wanted)

    for key, info in sorted(found.items()):
        data = extract_vpk_file(vpk_path, *info)

        if key.startswith("_txt_"):
            mapname = key[5:]
            meta = parse_overview_meta(data)
            print(f"  {mapname}: {meta}")
        else:
            img = decode_vtex(data)
            out_path = out_dir / f"{key}.png"
            img.save(out_path)
            print(f"  saved {out_path.name}  ({img.size[0]}×{img.size[1]})")

    print("\nDone. Rebuild demoview to embed updated images:")
    print("  go build -o demoview ./cmd/demoview/")


if __name__ == "__main__":
    main()
