#!/usr/bin/env bash
# Reject tags that Go cannot consume without changing the module import path.
set -euo pipefail
tag="${1:?usage: validate-release-version.sh <tag> [repo-root]}"
root="${2:-$(cd "$(dirname "$0")/.." && pwd)}"
if [[ ! "$tag" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid release tag: $tag" >&2; exit 1
fi
major="${BASH_REMATCH[1]}"
module="$(awk '$1 == "module" {print $2; exit}' "$root/go.mod")"
if [[ "$major" != 0 && "$major" != 1 && "$module" != */v"$major" ]]; then
  echo "$tag requires module path suffix /v$major; refusing to break SDK consumers of $module" >&2; exit 1
fi
if [[ "$module" =~ /v([0-9]+)$ && "${BASH_REMATCH[1]}" != "$major" ]]; then
  echo "$tag does not match module major suffix: $module" >&2; exit 1
fi
ide="$(awk -F '"' '/^const ideVersion = / {print $2; exit}' "$root/cmd/silkide/menu_actions.go")"
base="${tag#v}"; base="${base%%-*}"
if [[ -z "$ide" || "$base" != "$ide" ]]; then
  echo "release $tag does not match SilkIDE version $ide" >&2; exit 1
fi
echo "release $tag matches IDE $ide and module $module"
