#!/usr/bin/env bash
set -euo pipefail
umask 077
state=${1:-${XDG_DATA_HOME:-$HOME/.local/share}/mox-arch-signing}
mkdir -p "$state/gnupg"
chmod 700 "$state" "$state/gnupg"
export GNUPGHOME="$state/gnupg"
if [[ ! -f "$state/fingerprint" ]]; then
  gpg --batch --pinentry-mode loopback --passphrase '' \
    --quick-generate-key 'bthall Arch packages' ed25519 sign 2y
  gpg --batch --with-colons --list-secret-keys | awk -F: '$1=="fpr" {print $10; exit}' > "$state/fingerprint"
fi
fingerprint=$(cat "$state/fingerprint")
gpg --batch --armor --export "$fingerprint" > "$state/public-key.asc"
gpg --batch --pinentry-mode loopback --passphrase '' --armor --export-secret-keys "$fingerprint" > "$state/signing-key.asc"
chmod 600 "$state/signing-key.asc"
printf 'Fingerprint: %s\nPublic key: %s/public-key.asc\nPrivate key: %s/signing-key.asc\n' "$fingerprint" "$state" "$state"
printf 'Configure the repository secret without displaying the private key:\n'
printf '  gh secret set ARCH_REPO_SIGNING_KEY --repo bthall/mox < %q\n' "$state/signing-key.asc"
