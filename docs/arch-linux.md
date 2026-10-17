# Arch Linux distribution

Mox's native Arch packages are hosted on GitHub Releases. No AUR account or
AUR helper is needed. Packages are available for x86_64 and aarch64 and
contain the same binaries as the corresponding Linux release archives.
They depend on tmux 3.1 or newer and install as `mox-tmux`, matching the
source PKGBUILD's package name.

## Install a release package

Download the `.pkg.tar.zst` for your architecture and `arch-checksums.txt`
from the [release](https://github.com/bthall/mox/releases/latest). Verify and
install it from the download directory:

```sh
sha256sum --check --ignore-missing arch-checksums.txt
sudo pacman -U ./mox-tmux-0.8.0-1-x86_64.pkg.tar.zst
```

Use the actual downloaded version and architecture in the filename.

## Signed repository and automatic updates

The `archlinux-x86_64` and `archlinux-aarch64` release channels host the
signed repositories for automatic updates through pacman.

Download the repository's public key:

```sh
curl --fail --location --output bthall-arch.asc \
  https://raw.githubusercontent.com/bthall/mox/main/packaging/arch/bthall-arch.asc
gpg --show-keys --with-fingerprint bthall-arch.asc
```

Its full fingerprint must be:

```text
B51791ADF3189C33B3EA5F32EABA3DBA7C3D4400
```

Add and locally trust this key:

```sh
sudo pacman-key --add bthall-arch.asc
sudo pacman-key --lsign-key B51791ADF3189C33B3EA5F32EABA3DBA7C3D4400
```

Append this section to `/etc/pacman.conf`, after the official repositories:

```ini
[bthall]
SigLevel = Required DatabaseRequired
Server = https://github.com/bthall/mox/releases/download/archlinux-$arch
```

Then install or update normally:

```sh
sudo pacman -Syu mox-tmux
```

Both packages and repository databases are signed. Repository releases retain
old package files for clients with cached databases. These repository channels
are separate from the latest application release.

## Maintainer setup

The dedicated key's public half and fingerprint are checked in under
`packaging/arch/`. Its private half is kept outside the checkout.

On a new machine, restore the existing private-key backup into the isolated
key directory first:

```sh
mkdir -p "$HOME/.local/share/mox-arch-signing/gnupg"
chmod 700 "$HOME/.local/share/mox-arch-signing" "$HOME/.local/share/mox-arch-signing/gnupg"
gpg --homedir "$HOME/.local/share/mox-arch-signing/gnupg" \
  --import /path/to/backup/signing-key.asc
```

`bash scripts/setup-arch-signing.sh` validates that the restored secret key
matches the checked-in fingerprint, exports it locally, and prints a command
to set `ARCH_REPO_SIGNING_KEY` as a GitHub Actions repository secret:

```sh
gh secret set ARCH_REPO_SIGNING_KEY --repo bthall/mox \
  < "$HOME/.local/share/mox-arch-signing/signing-key.asc"
```

The GitHub settings interface can also set the secret. Its value is the full
ASCII-armored private key. Keep the private key and revocation certificate
backed up outside Git. The setup script refuses to generate a replacement key.
Key rotation requires a deliberate public-key update and client key rotation.
The workflow requires the checked-in fingerprint.

The normal Release workflow calls **Arch packages** after archive verification.
It verifies the existing release checksums, wraps the Linux binaries with
pinned nFPM, and attaches native packages and `arch-checksums.txt` to the
versioned release. Existing assets must be byte-identical on a retry.

If the signing secret is configured, the second job signs packages, verifies
the existing repository before modifying it, rejects downgrades or changed
bytes for the same package version, and publishes the two signed repository
channels. Without the secret, package publication succeeds and the workflow
reports that repository signing is pending.

To backfill a published stable release, run **Arch packages** with its version,
or commit `.github/arch-release-request.json` on main:

```json
{"version": "v0.8.0"}
```

The workflow never moves the application's release tag or rebuilds its binaries.
Tests use temporary signing keys and repositories:

```sh
bash scripts/install-nfpm.sh /tmp/mox-nfpm
PATH=/tmp/mox-nfpm:$PATH python3 -m unittest discover -s scripts -p test_arch_distribution.py
```
