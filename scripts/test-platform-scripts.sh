#!/usr/bin/env bash
# Filesystem-only regression tests; no apt, Windows processes or GUI required.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/runtime/bin" "$tmp/out"
touch "$tmp/out/silkide.exe" "$tmp/out/silkgen.exe"
printf 'cairo-runtime\n' > "$tmp/runtime/bin/libcairo-2.dll"
printf 'font-runtime\n' > "$tmp/runtime/bin/libfont-1.dll"

cat > "$tmp/bin/ldd" <<'EOF'
#!/usr/bin/env bash
case ${LDD_MODE:-ok} in
  failure) echo 'ldd unavailable' >&2; exit 1 ;;
  missing) echo 'libmissing.dll => not found'; exit 0 ;;
  vanished) printf 'gone.dll => %s/bin/gone.dll (0x100)\n' "$TEST_DLL_ROOT"; exit 0 ;;
  foreign) echo 'foreign.dll => /unexpected/foreign.dll (0x100)'; exit 0 ;;
  staged) printf 'cairo.dll => %s/libcairo-2.dll (0x100)\nfont.dll => %s/libfont-1.dll (0x200)\n' "$TEST_DLL_OUT" "$TEST_DLL_OUT"; exit 0 ;;
esac
printf 'cairo.dll => %s/bin/libcairo-2.dll (0x100)\n' "$TEST_DLL_ROOT"
printf 'font.dll => %s/bin/libfont-1.dll (0x200)\n' "$TEST_DLL_ROOT"
echo 'KERNEL32.dll => /c/Windows/SYSTEM32/KERNEL32.dll (0x300)'
EOF
chmod +x "$tmp/bin/ldd"
cat > "$tmp/bin/cp" <<'EOF'
#!/usr/bin/env bash
if [[ ${FAIL_DLL_COPY:-0} == 1 ]]; then
  echo 'simulated copy failure' >&2
  exit 42
fi
exec /bin/cp "$@"
EOF
chmod +x "$tmp/bin/cp"
# MSYS2 minimal installs omit diffutils; inherited Git Bash cmp can also
# interpret /ucrt64 against the wrong runtime root. Never call it in bundling.
cat > "$tmp/bin/cmp" <<'EOF'
#!/usr/bin/env bash
printf 'unexpected cmp call\n' >> "$CMP_PROBE_LOG"
exit 127
EOF
chmod +x "$tmp/bin/cmp"
export CMP_PROBE_LOG="$tmp/cmp-called"
export PATH="$tmp/bin:$PATH"
export TEST_DLL_ROOT="$tmp/runtime"
export TEST_DLL_OUT="$tmp/out"
export SILK_DLL_ROOTS="$tmp/runtime"

bash "$root/scripts/bundle-windows-dlls.sh" "$tmp/out" > "$tmp/result"
[[ $(cat "$tmp/out/libcairo-2.dll") == cairo-runtime ]]
[[ $(cat "$tmp/out/libfont-1.dll") == font-runtime ]]
[[ ! -e $tmp/out/KERNEL32.dll ]]
# Repeated dependencies/calls are safe, but corrupt or conflicting copies fail.
bash "$root/scripts/bundle-windows-dlls.sh" "$tmp/out" > "$tmp/result"
LDD_MODE=staged bash "$root/scripts/bundle-windows-dlls.sh" "$tmp/out" > "$tmp/result"
printf 'wrong-version\n' > "$tmp/out/libfont-1.dll"
if LDD_MODE=staged bash "$root/scripts/bundle-windows-dlls.sh" "$tmp/out" > "$tmp/result" 2>&1; then
  echo 'conflicting staged DLL was silently accepted' >&2; exit 1
fi
if bash "$root/scripts/bundle-windows-dlls.sh" "$tmp/out" > "$tmp/result" 2>&1; then
  echo 'conflicting DLL was silently accepted' >&2; exit 1
fi
cp "$tmp/runtime/bin/libfont-1.dll" "$tmp/out/libfont-1.dll"
rm "$tmp/out/libfont-1.dll"
if FAIL_DLL_COPY=1 bash "$root/scripts/bundle-windows-dlls.sh" "$tmp/out" > "$tmp/result" 2>&1; then
  echo 'DLL copy failure was silently accepted' >&2; exit 1
fi
cp "$tmp/runtime/bin/libfont-1.dll" "$tmp/out/libfont-1.dll"
for mode in failure missing vanished foreign; do
  if LDD_MODE=$mode bash "$root/scripts/bundle-windows-dlls.sh" "$tmp/out" > "$tmp/result" 2>&1; then
    echo "bad ldd result was silently accepted: $mode" >&2; exit 1
  fi
done

[[ ! -e "$CMP_PROBE_LOG" ]] || { echo 'bundling relied on foreign/missing cmp' >&2; exit 1; }

deps=$(bash "$root/scripts/install-linux-deps.sh" --print)
for required in build-essential pkg-config libcairo2-dev libx11-dev libxcursor-dev libxi-dev libxinerama-dev libxrandr-dev libxxf86vm-dev libgl1-mesa-dev zenity xdg-utils; do
  grep -qx "$required" <<< "$deps"
done
echo 'PASS: Linux dependencies; DLL copy/failure, duplicates, conflicts, missing and foreign dependencies'
bash "$root/scripts/test-branding-packaging.sh"
