#!/bin/sh
# Releases the changes since the last version tag: scripts/release.sh [patch|minor|major]
#
# If main has commits after the latest vX.Y.Z tag, this runs vet and tests, tags
# the next version (patch by default, see bump.sh) and pushes the commits and the
# tag together, which triggers the release workflow. With nothing new it does nothing.
# DRY_RUN=1 only reports what would happen.
set -eu

cd "$(dirname "$0")/.."
kind="${1:-patch}"
branch=main
remote=origin

fail() { echo "release: $*" >&2; exit 1; }

[ "$(git rev-parse --abbrev-ref HEAD)" = "$branch" ] || fail "not on $branch"
[ -z "$(git status --porcelain)" ] || fail "working tree is not clean; commit or stash first"
git remote get-url "$remote" >/dev/null 2>&1 || fail "no '$remote' remote configured"

git fetch -q --tags "$remote"
if git rev-parse -q --verify "refs/remotes/$remote/$branch" >/dev/null; then
  [ "$(git rev-list --count "HEAD..$remote/$branch")" = 0 ] || fail "$branch is behind $remote/$branch; pull first"
fi

last=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -n1 || true)
if [ -n "$last" ]; then
  range="$last..HEAD"
else
  range=HEAD
fi

if [ "$(git rev-list --count "$range")" = 0 ]; then
  echo "No changes since ${last:-the start}; nothing to release."
  exit 0
fi

echo "Changes since ${last:-the start}:"
git log --oneline "$range"
echo

if [ -n "${DRY_RUN:-}" ]; then
  echo "DRY_RUN: would run vet and tests, bump ($kind) from ${last:-v0.0.0}, and push $branch and the new tag to $remote."
  exit 0
fi

go vet ./...
go test -race ./...

scripts/bump.sh "$kind"
tag=$(git tag --points-at HEAD --list 'v[0-9]*' | head -n1)

# One atomic push: either both the commits and the tag land, or neither.
if ! git push --atomic "$remote" "$branch" "refs/tags/$tag"; then
  git tag -d "$tag" >/dev/null
  fail "push failed; removed local tag $tag"
fi
echo "Released $tag"
