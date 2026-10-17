import hashlib
import io
import os
from pathlib import Path
import json
import shutil
import struct
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch

import arch_packages
import arch_publish
import arch_repository


EPOCH = 1700000000


def archives(directory, version="0.8.0"):
    directory.mkdir(parents=True, exist_ok=True)
    lines = []
    for goarch, (_, machine) in arch_packages.ARCHES.items():
        binary = bytearray(32)
        binary[:6] = b"\x7fELF\x02\x01"
        binary[18:20] = struct.pack("<H", machine)
        archive = directory / f"mox_{version}_linux_{goarch}.tar.gz"
        with tarfile.open(archive, "w:gz") as tar:
            for name, data in (("mox", bytes(binary)), ("LICENSE", b"MIT fixture license\n")):
                member = tarfile.TarInfo(name)
                member.size = len(data)
                member.mtime = EPOCH
                tar.addfile(member, io.BytesIO(data))
        lines.append(f"{arch_packages.sha256(archive)}  {archive.name}\n")
    (directory / "checksums.txt").write_text("".join(lines))


class DistributionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix="mox-arch-test-")
        cls.root = Path(cls.temporary.name)
        cls.original_home = os.environ.get("GNUPGHOME")
        os.environ["GNUPGHOME"] = str(cls.root / "gnupg")
        Path(os.environ["GNUPGHOME"]).mkdir(mode=0o700)
        subprocess.run(["gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase", "", "--quick-generate-key",
                        "Fictional Arch test key", "ed25519", "sign", "1d"], check=True)
        keys = arch_repository.run("gpg", "--batch", "--with-colons", "--list-secret-keys")
        cls.fingerprint = next(line.split(":")[9] for line in keys.splitlines() if line.startswith("fpr:"))
        archives(cls.root / "source")
        arch_packages.build("0.8.0", cls.root / "source", cls.root / "packages", EPOCH)

    @classmethod
    def tearDownClass(cls):
        subprocess.run(["gpgconf", "--kill", "gpg-agent"], check=True)
        if cls.original_home is None:
            os.environ.pop("GNUPGHOME", None)
        else:
            os.environ["GNUPGHOME"] = cls.original_home
        cls.temporary.cleanup()

    def test_native_package_metadata_and_reproducibility(self):
        with tempfile.TemporaryDirectory() as work:
            other = Path(work) / "packages"
            arch_packages.build("0.8.0", self.root / "source", other, EPOCH)
            for arch in ("x86_64", "aarch64"):
                name = f"mox-tmux-0.8.0-1-{arch}.pkg.tar.zst"
                package = self.root / "packages" / name
                info = arch_repository.run("bsdtar", "-xOf", str(package), ".PKGINFO")
                for field in (f"arch = {arch}", "pkgname = mox-tmux", "pkgver = 0.8.0-1", "depend = tmux>=3.1"):
                    self.assertIn(field, info)
                self.assertEqual(arch_packages.sha256(package), arch_packages.sha256(other / name))
                contents = arch_repository.run("bsdtar", "-tf", str(package))
                self.assertIn("usr/bin/mox", contents)
                self.assertIn("usr/share/licenses/mox-tmux/LICENSE", contents)

    def test_rejects_modified_release(self):
        with tempfile.TemporaryDirectory() as work:
            source = Path(work) / "source"
            archives(source)
            archive = source / "mox_0.8.0_linux_amd64.tar.gz"
            archive.write_bytes(archive.read_bytes() + b"tampered")
            with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                arch_packages.build("0.8.0", source, Path(work) / "packages", EPOCH)

    def test_rejects_unsafe_version(self):
        with self.assertRaises(ValueError):
            arch_packages.build("../../other", self.root / "source", self.root / "bad", EPOCH)

    def test_signed_repository_and_tamper_detection(self):
        with tempfile.TemporaryDirectory() as work:
            output = Path(work) / "repo"
            arch_repository.build(self.root / "packages", output, self.fingerprint)
            for arch in ("x86_64", "aarch64"):
                database = output / arch / "bthall.db"
                arch_repository.verify(database, self.fingerprint)
                self.assertFalse(database.is_symlink())
                entries = arch_repository.previous_versions(database)
                self.assertEqual(entries["mox-tmux"]["VERSION"], "0.8.0-1")
                package = next((output / arch).glob("*.pkg.tar.zst"))
                self.assertEqual(entries["mox-tmux"]["SHA256SUM"], arch_packages.sha256(package))
                package.write_bytes(package.read_bytes() + b"tampered")
                with self.assertRaises(subprocess.CalledProcessError):
                    arch_repository.verify(package, self.fingerprint)

    def test_refuses_downgrade_and_same_version_replacement(self):
        with tempfile.TemporaryDirectory() as work:
            root = Path(work)
            arch_repository.build(self.root / "packages", root / "previous", self.fingerprint)
            for version, epoch in (("0.7.0", EPOCH), ("0.8.0", EPOCH + 1)):
                source, packages = root / version / "source", root / version / "packages"
                archives(source, version)
                arch_packages.build(version, source, packages, epoch)
                with self.assertRaisesRegex(ValueError, "refusing downgrade or replacement"):
                    arch_repository.build(packages, root / version / "repo", self.fingerprint, root / "previous")

    def test_upgrade_preserves_other_packages_and_file_database(self):
        with tempfile.TemporaryDirectory() as work:
            root = Path(work)
            shutil.copytree(self.root / "packages", root / "old-packages")
            config = {
                "name": "fixture-helper", "arch": "amd64", "version": "1.0.0", "release": "1",
                "contents": [{"src": str(self.root / "source" / "checksums.txt"), "dst": "/usr/share/fixture-helper/readme"}],
            }
            (root / "nfpm.json").write_text(json.dumps(config))
            subprocess.run(["nfpm", "package", "--packager", "archlinux", "--config", str(root / "nfpm.json"),
                            "--target", str(root / "old-packages" / "fixture-helper.pkg.tar.zst")], check=True)
            arch_repository.build(root / "old-packages", root / "previous", self.fingerprint)
            archives(root / "source", "0.8.1")
            arch_packages.build("0.8.1", root / "source", root / "new-packages", EPOCH)
            arch_repository.build(root / "new-packages", root / "updated", self.fingerprint, root / "previous")
            database = root / "updated" / "x86_64" / "bthall.db"
            entries = arch_repository.previous_versions(database)
            self.assertEqual(entries["mox-tmux"]["VERSION"], "0.8.1-1")
            self.assertEqual(entries["fixture-helper"]["VERSION"], "1.0.0-1")
            with tarfile.open(root / "updated" / "x86_64" / "bthall.files") as tar:
                member = next(m for m in tar.getmembers() if m.name.startswith("fixture-helper-") and m.name.endswith("/files"))
                self.assertIn(b"usr/share/fixture-helper/readme", tar.extractfile(member).read())

    def test_rejects_previous_repository_from_wrong_key(self):
        with tempfile.TemporaryDirectory() as work:
            root = Path(work)
            arch_repository.build(self.root / "packages", root / "previous", self.fingerprint)
            with self.assertRaisesRegex(ValueError, "unexpected signing key"):
                arch_repository.build(self.root / "packages", root / "updated", "A" * 40, root / "previous")

    def test_immutable_release_asset(self):
        path = self.root / "packages" / "arch-checksums.txt"
        digest = "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()
        with patch("arch_publish.gh") as gh:
            arch_publish.immutable("fixture/repo", "v0.8.0", path, [{"name": path.name, "digest": digest}])
            gh.assert_not_called()
            with self.assertRaisesRegex(ValueError, "refusing to replace"):
                arch_publish.immutable("fixture/repo", "v0.8.0", path, [{"name": path.name, "digest": "sha256:wrong"}])
            gh.assert_not_called()

    def test_signing_setup_requires_the_committed_key(self):
        with tempfile.TemporaryDirectory() as work:
            root = Path(work)
            script = root / "scripts" / "setup-arch-signing.sh"
            script.parent.mkdir()
            shutil.copyfile(Path(__file__).with_name(script.name), script)
            fingerprint_file = root / "packaging" / "arch" / "key-fingerprint.txt"
            fingerprint_file.parent.mkdir(parents=True)
            fingerprint_file.write_text(self.fingerprint + "\n")
            state = root / "state"

            def setup():
                return subprocess.run(["bash", str(script), str(state)], capture_output=True, text=True)

            self.assertNotEqual(setup().returncode, 0)
            self.assertFalse((state / "signing-key.asc").exists())
            keys = subprocess.check_output(["gpg", "--homedir", str(state / "gnupg"), "--batch",
                                            "--with-colons", "--list-secret-keys"], text=True)
            self.assertNotIn("sec:", keys)
            backup = subprocess.check_output(["gpg", "--batch", "--export-secret-keys", self.fingerprint])
            subprocess.run(["gpg", "--homedir", str(state / "gnupg"), "--batch", "--import"],
                           input=backup, check=True, capture_output=True)
            try:
                # A restored but unrelated key must not be exported for Actions.
                fingerprint_file.write_text("A" * 40 + "\n")
                result = setup()
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Restore signing key", result.stderr)
                self.assertFalse((state / "signing-key.asc").exists())
                fingerprint_file.write_text(self.fingerprint + "\n")
                result = setup()
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual((state / "fingerprint").read_text().strip(), self.fingerprint)
                self.assertIn("BEGIN PGP PRIVATE KEY BLOCK", (state / "signing-key.asc").read_text())
                self.assertEqual((state / "signing-key.asc").stat().st_mode & 0o777, 0o600)
            finally:
                subprocess.run(["gpgconf", "--homedir", str(state / "gnupg"), "--kill", "gpg-agent"], check=True)


if __name__ == "__main__":
    unittest.main()
