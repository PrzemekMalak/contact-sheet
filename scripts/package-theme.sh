#!/usr/bin/env bash
# Package the theme (this repo's root theme files) as the archive that
# `photo init` and `photo theme update` install into a site's themes/.
#
#   scripts/package-theme.sh <version> <output.tar.gz>
set -euo pipefail

version="$1"
out="$(cd "$(dirname "$2")" && pwd)/$(basename "$2")"
root="$(cd "$(dirname "$0")/.." && pwd)"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

for p in layouts assets content static hugo.toml theme.toml LICENSE; do
  cp -R "$root/$p" "$stage/"
done
echo "$version" > "$stage/VERSION"
find "$stage" -name .DS_Store -delete
COPYFILE_DISABLE=1 tar -czf "$out" -C "$stage" .
