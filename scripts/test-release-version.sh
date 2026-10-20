#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/cmd/silkide"
printf 'module github.com/uk0/silk\n' > "$fixture/go.mod"
printf 'const ideVersion = "0.1.4"\n' > "$fixture/cmd/silkide/menu_actions.go"
check() { bash "$root/scripts/validate-release-version.sh" "$1" "$fixture"; }
check v0.1.4
check v0.1.4-rc.1
for tag in v2.5.1 v0.1.3 garbage v0.1.4/unsafe; do
  if check "$tag"; then echo "unexpectedly accepted $tag" >&2; exit 1; fi
done
printf 'const ideVersion = "2.5.1"\n' > "$fixture/cmd/silkide/menu_actions.go"
if check v2.5.1; then echo 'major suffix was not enforced' >&2; exit 1; fi
printf 'module github.com/uk0/silk/v2\n' > "$fixture/go.mod"
check v2.5.1
echo 'PASS: release tag / IDE / Go module version alignment'
