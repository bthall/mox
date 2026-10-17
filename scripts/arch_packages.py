#!/usr/bin/env python3
"""Package verified Mox release archives without rebuilding their binaries."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import struct
import subprocess
import tarfile
import tempfile


ARCHES = {"amd64": ("x86_64", 62), "arm64": ("aarch64", 183)}


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def build(version, source, output, epoch):
    version = version.removeprefix("v")
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        raise ValueError("expected a stable X.Y.Z version")
    checksums = {}
    for line in (source / "checksums.txt").read_text().splitlines():
        digest, name = line.split(maxsplit=1)
        if name in checksums:
            raise ValueError("duplicate checksum entry")
        checksums[name] = digest
    output.mkdir(parents=True, exist_ok=True)
    packages = []
    for goarch, (arch, machine) in ARCHES.items():
        name = f"mox_{version}_linux_{goarch}.tar.gz"
        archive = source / name
        if checksums.get(name) != sha256(archive):
            raise ValueError(f"release checksum mismatch: {name}")
        with tempfile.TemporaryDirectory(prefix="mox-arch-") as directory:
            work = Path(directory)
            with tarfile.open(archive) as tar:
                for filename in ("mox", "LICENSE"):
                    member = tar.getmember(filename)
                    if not member.isfile() or member.size > 64 * 1024 * 1024:
                        raise ValueError(f"invalid release member: {filename}")
                    (work / filename).write_bytes(tar.extractfile(member).read())
            binary = (work / "mox").read_bytes()
            if len(binary) < 20 or binary[:6] != b"\x7fELF\x02\x01" or struct.unpack("<H", binary[18:20])[0] != machine:
                raise ValueError(f"unexpected binary architecture: {name}")
            config = {
                "name": "mox-tmux", "arch": goarch, "platform": "linux",
                "version": version, "release": "1", "homepage": "https://github.com/bthall/mox",
                "maintainer": "Brandon Hall <brandon@brandonhall.us>", "license": "MIT",
                "archlinux": {"packager": "Brandon Hall <brandon@brandonhall.us>"},
                "description": "Declarative tmux session manager",
                "depends": ["tmux>=3.1"], "provides": ["mox"], "conflicts": ["mox-tmux-bin"],
                "contents": [
                    {"src": str(work / "mox"), "dst": "/usr/bin/mox", "file_info": {"mode": 0o755}},
                    {"src": str(work / "LICENSE"), "dst": "/usr/share/licenses/mox-tmux/LICENSE", "file_info": {"mode": 0o644}},
                ],
            }
            config_path = work / "nfpm.json"
            config_path.write_text(json.dumps(config))
            package = output / f"mox-tmux-{version}-1-{arch}.pkg.tar.zst"
            subprocess.run(["nfpm", "package", "--packager", "archlinux", "--config", str(config_path), "--target", str(package)],
                           env={**os.environ, "SOURCE_DATE_EPOCH": str(epoch)}, check=True)
            packages.append(package)
    (output / "arch-checksums.txt").write_text("".join(f"{sha256(p)}  {p.name}\n" for p in packages))
    return packages


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version")
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--epoch", type=int, required=True)
    args = parser.parse_args()
    build(args.version, args.source.resolve(), args.output.resolve(), args.epoch)
