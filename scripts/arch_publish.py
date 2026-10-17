#!/usr/bin/env python3
"""Publish Arch assets with gh, refusing to replace existing package bytes."""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile


def gh(*args):
    return subprocess.check_output(["gh", *args], text=True)


def release(repo, tag):
    return json.loads(gh("api", f"repos/{repo}/releases/tags/{tag}"))


def releases(repo):
    pages = json.loads(gh("api", "--paginate", "--slurp", f"repos/{repo}/releases?per_page=100"))
    return [item for page in pages for item in page]


def immutable(repo, tag, path, assets):
    existing = next((a for a in assets if a["name"] == path.name), None)
    digest = "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()
    if existing:
        remote_digest = existing.get("digest")
        if not remote_digest:
            with tempfile.TemporaryDirectory() as temporary:
                gh("release", "download", tag, "--repo", repo, "--pattern", path.name, "--dir", temporary)
                remote_digest = "sha256:" + hashlib.sha256((Path(temporary) / path.name).read_bytes()).hexdigest()
        if remote_digest != digest:
            raise ValueError(f"refusing to replace published asset: {path.name}")
    else:
        gh("release", "upload", tag, str(path), "--repo", repo)


def publish_packages(repo, version, directory):
    info = release(repo, version)
    if info["draft"] or info["prerelease"]:
        raise ValueError("Arch publication requires an already-published stable release")
    # Publish the manifest after its package files are available.
    for path in sorted(directory.iterdir(), key=lambda p: (p.name == "arch-checksums.txt", p.name)):
        if path.name.endswith(".pkg.tar.zst") or path.name == "arch-checksums.txt":
            immutable(repo, version, path, info["assets"])


def download_previous(repo, directory):
    for arch in ("x86_64", "aarch64"):
        tag = "archlinux-" + arch
        # Query the release list so authentication/network errors never look like
        # an empty repository and silently drop its previous package entries.
        items = releases(repo)
        if not any(item["tag_name"] == tag for item in items):
            continue
        dest = directory / arch
        dest.mkdir(parents=True, exist_ok=True)
        for name in ("bthall.db.tar.gz", "bthall.db.tar.gz.sig", "bthall.files.tar.gz", "bthall.files.tar.gz.sig"):
            gh("release", "download", tag, "--repo", repo, "--pattern", name, "--dir", str(dest))


def publish_repository(repo, version, commit, directory):
    for arch in ("x86_64", "aarch64"):
        source = directory / arch
        if not source.is_dir():
            continue
        tag = "archlinux-" + arch
        items = releases(repo)
        if not any(item["tag_name"] == tag for item in items):
            gh("release", "create", tag, "--repo", repo, "--target", commit, "--latest=false",
               "--title", f"bthall Arch repository ({arch})",
               "--notes", "Rolling signed pacman repository. Setup: https://github.com/bthall/mox/blob/main/docs/arch-linux.md")
        info = release(repo, tag)
        # Keep all historical package assets for clients with cached databases.
        for path in sorted(source.glob("*.pkg.tar.zst")):
            immutable(repo, tag, path, info["assets"])
        # Package signatures and the public key precede the database update.
        metadata = sorted(p for p in source.iterdir() if not p.name.endswith(".pkg.tar.zst"))
        metadata.sort(key=lambda p: p.name.startswith("bthall.db"))
        for path in metadata:
            gh("release", "upload", tag, str(path), "--repo", repo, "--clobber")
        stable = release(repo, version)
        for signature in source.glob("*.pkg.tar.zst.sig"):
            if not any(a["name"] == signature.name for a in stable["assets"]):
                gh("release", "upload", version, str(signature), "--repo", repo)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=("packages", "previous", "repository"))
    parser.add_argument("repo")
    parser.add_argument("version")
    parser.add_argument("directory", type=Path)
    parser.add_argument("--commit")
    args = parser.parse_args()
    if args.operation == "packages":
        publish_packages(args.repo, args.version, args.directory)
    elif args.operation == "previous":
        download_previous(args.repo, args.directory)
    else:
        if not args.commit:
            parser.error("repository publication requires --commit")
        publish_repository(args.repo, args.version, args.commit, args.directory)
