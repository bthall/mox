#!/usr/bin/env python3
"""Build signed pacman repositories, preserving existing package entries."""

import argparse
import hashlib
from pathlib import Path
import re
import shutil
import subprocess
import tarfile


def run(*args):
    return subprocess.check_output(args, text=True)


def fields(text, separator):
    result = {}
    for line in text.splitlines():
        if separator in line:
            key, value = line.split(separator, 1)
            result.setdefault(key.strip(), []).append(value.strip())
    return result


def verify(path, fingerprint):
    result = run("gpg", "--batch", "--status-fd=1", "--verify", str(path) + ".sig", str(path))
    if f"[GNUPG:] VALIDSIG {fingerprint} " not in result:
        raise ValueError(f"unexpected signing key: {path}")


def previous_versions(database):
    result = {}
    with tarfile.open(database) as tar:
        for member in tar.getmembers():
            if member.isfile() and member.name.endswith("/desc"):
                text = tar.extractfile(member).read().decode()
                entries = dict(re.findall(r"%([A-Z0-9]+)%\n([^\n]+)", text))
                result[entries["NAME"]] = entries
    return result


def build(packages, output, fingerprint, previous=None):
    if not re.fullmatch(r"[A-F0-9]{40}", fingerprint):
        raise ValueError("expected a full signing fingerprint")
    output.mkdir(parents=True, exist_ok=True)
    if any(output.iterdir()):
        raise ValueError("repository output directory must be empty")
    grouped = {}
    for package in sorted(packages.glob("*.pkg.tar.zst")):
        info = fields(run("bsdtar", "-xOf", str(package), ".PKGINFO"), "=")
        arch = info["arch"][0]
        if arch not in ("x86_64", "aarch64"):
            raise ValueError(f"unsupported architecture: {arch}")
        grouped.setdefault(arch, []).append((package, info))
    if not grouped:
        raise ValueError("no packages found")
    for arch, entries in grouped.items():
        dest = output / arch
        dest.mkdir()
        database = dest / "bthall.db.tar.gz"
        old = {}
        if previous and (previous / arch / database.name).exists():
            prior = previous / arch / database.name
            verify(prior, fingerprint)
            old = previous_versions(prior)
            for name in ("bthall.db.tar.gz", "bthall.files.tar.gz"):
                source = previous / arch / name
                verify(source, fingerprint)
                shutil.copy2(source, dest / name)
        files = []
        for package, info in entries:
            name, version = info["pkgname"][0], info["pkgver"][0]
            if name in old:
                comparison = int(run("vercmp", old[name]["VERSION"], version).strip())
                digest = hashlib.sha256(package.read_bytes()).hexdigest()
                if comparison > 0 or (comparison == 0 and old[name]["SHA256SUM"] != digest):
                    raise ValueError(f"refusing downgrade or replacement of {name} {version}")
            target = dest / package.name
            shutil.copy2(package, target)
            subprocess.run(["gpg", "--batch", "--yes", "--local-user", fingerprint + "!", "--detach-sign", str(target)], check=True)
            verify(target, fingerprint)
            files.append(str(target))
        subprocess.run(["repo-add", "--sign", "--key", fingerprint, str(database), *files], check=True)
        verify(database, fingerprint)
        # Upload regular files: GitHub artifacts do not preserve symlinks.
        for short, full in (("bthall.db", "bthall.db.tar.gz"), ("bthall.files", "bthall.files.tar.gz")):
            for suffix in ("", ".sig"):
                alias = dest / (short + suffix)
                alias.unlink(missing_ok=True)
                shutil.copyfile(dest / (full + suffix), alias)
        (dest / "bthall-arch.asc").write_text(run("gpg", "--armor", "--export", fingerprint))
        for stale in dest.glob("*.old*"):
            stale.unlink()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("packages", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("fingerprint")
    parser.add_argument("--previous", type=Path)
    args = parser.parse_args()
    build(args.packages.resolve(), args.output.resolve(), args.fingerprint, args.previous)
