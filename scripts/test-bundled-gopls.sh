#!/usr/bin/env bash
# Run real editor/LSP tests beside the staged tools, through the same resolver
# as the installed IDE. Neither a developer PATH gopls nor a skipped test counts.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
out=${1:?usage: test-bundled-gopls.sh STAGING_DIRECTORY}
mkdir -p "$out"
out=$(cd "$out" && pwd)
suffix=''; [[ $(go env GOOS) != windows ]] || suffix='.exe'
probe="$out/.gopls-integration$suffix"
[[ ! -e "$probe" ]] || { echo "Probe already exists: $probe" >&2; exit 1; }
trap 'rm -f "$probe"' EXIT
cd "$root"
go run ./cmd/silktools -verify -out "$out/tools"
go test -c -o "$probe" ./cmd/silkide
export SILKIDE_GOPLS=''
export SILK_TEST_REQUIRE_GOPLS=1
export SILK_TEST_REQUIRE_BUNDLED_GOPLS=1
"$probe" -test.v -test.timeout=4m -test.run='^Test(GoplsUnsavedUnicodeCompletionAndFormatting|EditorRenameRealGoplsKeepsUnsavedDefinition|RealGoplsNavigationUnsavedTypeImplementationAndBack)$'
