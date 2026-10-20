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
