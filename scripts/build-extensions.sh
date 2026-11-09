#!/bin/sh
# Builds the browser extensions: build/extensions/avpnd-{firefox,chrome}-<version>.zip
# Usage: scripts/build-extensions.sh [version]   (a leading "v" and any "-suffix" are stripped)
set -eu

cd "$(dirname "$0")/.."
version="${1:-${VERSION:-0.0.0}}"
version="${version#v}"
version="${version%%-*}"
case "$version" in
  *[!0-9.]* | "") echo "invalid version: $version" >&2; exit 1 ;;
esac

out=build/extensions
rm -rf "$out"
mkdir -p "$out"

for browser in firefox chrome; do
  dir="$out/$browser"
  mkdir -p "$dir"
  cp -R extension/src extension/icons "$dir/"
  sed "s/__VERSION__/$version/" "extension/manifest.$browser.json" > "$dir/manifest.json"
  (cd "$dir" && zip -qr -X "../avpnd-$browser-$version.zip" .)
  echo "built $out/avpnd-$browser-$version.zip"
done
