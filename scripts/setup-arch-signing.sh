#!/usr/bin/env bash
set -euo pipefail
umask 077
state=${1:-${XDG_DATA_HOME:-$HOME/.local/share}/mox-arch-signing}
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
fingerprint=$(cat "$repo_root/packaging/arch/key-fingerprint.txt")
[[ $fingerprint =~ ^[A-F0-9]{40}$ ]]
mkdir -p "$state/gnupg"
chmod 700 "$state" "$state/gnupg"
export GNUPGHOME="$state/gnupg"
if ! gpg --batch --list-secret-keys "$fingerprint" >/dev/null 2>&1; then
  printf 'Restore signing key %s into %s before configuring Actions.\n' "$fingerprint" "$GNUPGHOME" >&2
  printf 'Import the existing private-key backup with: gpg --homedir %q --import /path/to/signing-key.asc\n' "$GNUPGHOME" >&2
  exit 1
fi
printf '%s\n' "$fingerprint" > "$state/fingerprint"
gpg --batch --armor --export "$fingerprint" > "$state/public-key.asc"
gpg --batch --pinentry-mode loopback --passphrase '' --armor --export-secret-keys "$fingerprint" > "$state/signing-key.asc"
chmod 600 "$state/signing-key.asc"
printf 'Fingerprint: %s\nPublic key: %s/public-key.asc\nPrivate key: %s/signing-key.asc\n' "$fingerprint" "$state" "$state"
printf 'Configure the repository secret without displaying the private key:\n'
printf '  gh secret set ARCH_REPO_SIGNING_KEY --repo bthall/mox < %q\n' "$state/signing-key.asc"
