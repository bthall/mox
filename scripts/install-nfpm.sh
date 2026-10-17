#!/usr/bin/env bash
set -euo pipefail
destination=${1:?Usage: install-nfpm.sh DIRECTORY}
mkdir -p "$destination"
archive="$destination/nfpm.tar.gz"
curl --fail --location --silent --show-error --retry 3 \
  https://github.com/goreleaser/nfpm/releases/download/v2.48.0/nfpm_2.48.0_Linux_x86_64.tar.gz \
  --output "$archive"
printf '%s  %s\n' 6d7f9a28b2f397934403950fcbba83f598b08d02edbe79a0815b3b4897341750 "$archive" | sha256sum --check
tar -xzf "$archive" -C "$destination" nfpm
rm "$archive"
