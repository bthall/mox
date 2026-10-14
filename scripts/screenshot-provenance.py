#!/usr/bin/env python3
"""Record the actual model frame and renderer in a generated PNG."""

import argparse
import hashlib
import json
from pathlib import Path
import struct
import subprocess
import zlib


def chunk(kind, data):
    return (struct.pack(">I", len(data)) + kind + data
            + struct.pack(">I", zlib.crc32(kind + data)))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("png", type=Path)
    parser.add_argument("frame", type=Path)
    parser.add_argument("--source-commit")
    args = parser.parse_args()
    commit = args.source_commit or subprocess.check_output(
        ["git", "rev-parse", "HEAD"], text=True).strip()
    metadata = {
        "Title": "Mox terminal capture: " + args.png.stem,
        "Description": "Deterministic demo data rendered by the actual Mox Go UI models.",
        "Source": json.dumps({
            "repository": "https://github.com/bthall/mox",
            "commit": commit,
            "generator": "internal/cli/screenshot_gen_test.go",
            "frame": str(args.frame),
            "frame_sha256": hashlib.sha256(args.frame.read_bytes()).hexdigest(),
            "renderer": "freeze v0.2.2 (SVG), rsvg-convert (PNG)",
        }, sort_keys=True),
    }
    raw = args.png.read_bytes()
    signature = b"\x89PNG\r\n\x1a\n"
    if not raw.startswith(signature):
        raise ValueError("Expected a PNG: " + str(args.png))
    output = bytearray(signature)
    offset = len(signature)
    while offset < len(raw):
        length = struct.unpack(">I", raw[offset:offset + 4])[0]
        end = offset + length + 12
        if end > len(raw):
            raise ValueError("Truncated PNG: " + str(args.png))
        kind = raw[offset + 4:offset + 8]
        data = raw[offset + 8:end - 4]
        if kind == b"IEND":
            for key, value in metadata.items():
                output.extend(chunk(b"tEXt", (key + "\0" + value).encode("latin-1")))
        if kind != b"tEXt" or data.split(b"\0", 1)[0].decode("latin-1") not in metadata:
            output.extend(raw[offset:end])
        offset = end
    args.png.write_bytes(output)


if __name__ == "__main__":
    main()
