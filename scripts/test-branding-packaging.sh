#!/usr/bin/env bash
# Filesystem-only packaging regression; never installs into the caller's home,
# starts an application, runs a real resource compiler or rewrites brand assets.
set -euo pipefail
# Fixture files are tiny synthetic text, not production DLLs. POSIX cksum
# compares their exact bytes/length without requiring MSYS2 diffutils.
assert_copy() {
  local expected actual
  expected=$(cksum < "$1") || return 1
  actual=$(cksum < "$2") || return 1
  [[ $expected == "$actual" ]]
}
root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
out="$tmp/release with space \$cash %bonus \"quote\""
mkdir -p "$out/icon" "$tmp/assets" "$tmp/bin" "$tmp/empty"
printf '#!/bin/sh\nexit 0\n' > "$out/silkide"
chmod +x "$out/silkide"
cp "$out/silkide" "$out/silkide.exe"
mkdir -p "$out/tools/licenses/gopls"
printf '#!/bin/sh\nexit 0\n' > "$out/tools/gopls"
chmod +x "$out/tools/gopls"
printf 'license\n' > "$out/tools/licenses/gopls/LICENSE"
printf 'manifest fixture\n' > "$out/tools/gopls.json"
printf 'SDK icon resource\n' > "$out/icon/existing.png"
for name in silkide-256.png silkide.ico silkide.icns; do printf 'test-%s\n' "$name" > "$tmp/assets/$name"; done
export SILKIDE_BRANDING_DIR="$tmp/assets"
bash "$root/scripts/package-silkide-branding.sh" darwin "$out" 0.1.3
app="$out/SilkIDE.app/Contents"
assert_copy "$out/silkide" "$app/MacOS/silkide"
assert_copy "$tmp/assets/silkide.icns" "$app/Resources/silkide.icns"
assert_copy "$out/icon/existing.png" "$app/Resources/icon/existing.png"
grep -q '<key>CFBundleIconFile</key><string>silkide.icns</string>' "$app/Info.plist"
grep -q '<key>CFBundleShortVersionString</key><string>0.1.3</string>' "$app/Info.plist"
[[ -x "$app/MacOS/silkide" ]]
assert_copy "$out/tools/gopls" "$app/MacOS/tools/gopls"
assert_copy "$out/tools/gopls.json" "$app/MacOS/tools/gopls.json"
assert_copy "$out/tools/licenses/gopls/LICENSE" "$app/MacOS/tools/licenses/gopls/LICENSE"
[[ -x "$app/MacOS/tools/gopls" ]]
# A repeat build must replace executable inodes, not truncate signed/running
# Mach-O files in place. Hard links make that observable on every test host.
ln "$app/MacOS/silkide" "$tmp/previous-ide"
ln "$app/MacOS/tools/gopls" "$tmp/previous-gopls"
mkdir -p "$app/_CodeSignature"
printf 'obsolete signature\n' > "$app/_CodeSignature/CodeResources"
printf 'obsolete tool\n' > "$app/MacOS/tools/removed-tool"
printf '#!/bin/sh\nexit 2\n' > "$out/silkide"
printf '#!/bin/sh\nexit 3\n' > "$out/tools/gopls"
bash "$root/scripts/package-silkide-branding.sh" darwin "$out" 0.1.4
assert_copy "$out/silkide" "$app/MacOS/silkide"
assert_copy "$out/tools/gopls" "$app/MacOS/tools/gopls"
[[ $(cat "$tmp/previous-ide") == $'#!/bin/sh\nexit 0' ]]
[[ $(cat "$tmp/previous-gopls") == $'#!/bin/sh\nexit 0' ]]
[[ ! -e "$app/_CodeSignature" && ! -e "$app/MacOS/tools/removed-tool" ]]
# A copy failure must leave the previous complete package usable.
mkdir -p "$tmp/fail-bin"
real_cp=$(command -v cp)
printf '#!/bin/bash\ncase "$*" in *gopls*|*tools*) exit 42;; esac\nexec "%s" "$@"\n' "$real_cp" > "$tmp/fail-bin/cp"
chmod +x "$tmp/fail-bin/cp"
if PATH="$tmp/fail-bin:$PATH" bash "$root/scripts/package-silkide-branding.sh" darwin "$out" 0.1.5 > "$tmp/copy-failure" 2>&1; then echo 'Failed bundle copy silently succeeded' >&2; exit 1; fi
assert_copy "$out/silkide" "$app/MacOS/silkide"
assert_copy "$out/tools/gopls" "$app/MacOS/tools/gopls"
grep -q '<key>CFBundleShortVersionString</key><string>0.1.4</string>' "$app/Info.plist"
# If publishing the staged directory fails, put the old bundle back.
mkdir -p "$tmp/fail-move"
real_mv=$(command -v mv)
printf '#!/bin/bash\ncase "$1" in */.silkide-bundle.*/SilkIDE.app) exit 42;; esac\nexec "%s" "$@"\n' "$real_mv" > "$tmp/fail-move/mv"
chmod +x "$tmp/fail-move/mv"
if PATH="$tmp/fail-move:$PATH" bash "$root/scripts/package-silkide-branding.sh" darwin "$out" 0.1.5 > "$tmp/move-failure" 2>&1; then echo 'Failed bundle publish silently succeeded' >&2; exit 1; fi
assert_copy "$out/silkide" "$app/MacOS/silkide"
assert_copy "$out/tools/gopls" "$app/MacOS/tools/gopls"
grep -q '<key>CFBundleShortVersionString</key><string>0.1.4</string>' "$app/Info.plist"
bash "$root/scripts/package-silkide-branding.sh" linux "$out"
assert_copy "$tmp/assets/silkide-256.png" "$out/share/icons/hicolor/256x256/apps/silkide.png"
XDG_DATA_HOME="$tmp/desktop-data" bash "$out/install-desktop-entry.sh" "$out" > "$tmp/install.log"
entry="$tmp/desktop-data/applications/silkide.desktop"
grep -Fx 'Icon=silkide' "$entry"
grep -F '\\$cash %%bonus \\"quote\\"/silkide"' "$entry"
if grep -q '%[fFuU]' "$entry"; then echo 'Launcher falsely advertises file-open arguments' >&2; exit 1; fi
assert_copy "$tmp/assets/silkide-256.png" "$tmp/desktop-data/icons/hicolor/256x256/apps/silkide.png"
bash "$root/scripts/package-silkide-branding.sh" windows "$out"
assert_copy "$tmp/assets/silkide.ico" "$out/silkide.ico"
for bad in unknown; do
  if bash "$root/scripts/package-silkide-branding.sh" "$bad" "$out" > "$tmp/failure" 2>&1; then echo 'Unsupported platform accepted' >&2; exit 1; fi
done
if bash "$root/scripts/package-silkide-branding.sh" darwin "$out" '1<script>' > "$tmp/failure" 2>&1; then echo 'Unsafe plist version accepted' >&2; exit 1; fi
if SILKIDE_BRANDING_DIR="$tmp/empty" bash "$root/scripts/package-silkide-branding.sh" linux "$out" > "$tmp/failure" 2>&1; then echo 'Missing icon assets silently packaged' >&2; exit 1; fi
cat > "$tmp/bin/windres" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ ${FAIL_WINDRES:-0} != 1 ]] || exit 42
out=''; previous=''
for arg in "$@"; do
  if [[ "$previous" == --output ]]; then out=$arg; fi
  previous=$arg
done
[[ -n "$out" ]]
printf 'COFF-test-resource\n' > "$out"
EOF
chmod +x "$tmp/bin/windres"
WINDRES="$tmp/bin/windres" bash "$root/scripts/build-windows-icon.sh" "$tmp/test_windows_amd64.syso"
[[ -s "$tmp/test_windows_amd64.syso" ]]
if FAIL_WINDRES=1 WINDRES="$tmp/bin/windres" bash "$root/scripts/build-windows-icon.sh" "$tmp/failed.syso" > "$tmp/failure" 2>&1; then echo 'Failed resource compiler silently accepted' >&2; exit 1; fi
echo 'PASS: macOS bundle/plist/resources, Windows ICO/compiler failure, Linux desktop identity/quoted paths'
