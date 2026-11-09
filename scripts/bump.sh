#!/bin/sh
# Tags the next semantic version: scripts/bump.sh [patch|minor|major]  (default: patch)
# Creates an annotated vX.Y.Z tag on HEAD; it does not push.
set -eu

cd "$(dirname "$0")/.."
kind="${1:-patch}"

if [ -n "$(git status --porcelain)" ]; then
  echo "working tree is not clean; commit or stash first" >&2
  exit 1
fi

# Latest vX.Y.Z tag by version order; none yet means v0.0.0.
last=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -n1 || true)
last="${last:-v0.0.0}"
IFS=. read -r major minor patch <<EOT
${last#v}
EOT

case "$kind" in
  major) major=$((major + 1)); minor=0; patch=0 ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  patch) patch=$((patch + 1)) ;;
  *) echo "usage: $0 [patch|minor|major]" >&2; exit 1 ;;
esac

next="v$major.$minor.$patch"
if [ "$(git rev-parse -q --verify "$last^{commit}" 2>/dev/null || true)" = "$(git rev-parse HEAD)" ]; then
  echo "HEAD is already tagged $last" >&2
  exit 1
fi

git tag -a "$next" -m "$next"
echo "$last -> $next (tagged HEAD)"
echo "publish with: git push origin $next"
