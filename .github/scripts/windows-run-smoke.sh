#!/usr/bin/env bash
# Start a silk GUI binary and require it to survive.
#
# Windows regressions that a build cannot catch all look the same at runtime:
# the process dies within a second of starting.
#   - no event loop registered -> "main loop mechanism unavailable"
#   - a resource ordinal faked as a pointer -> checkptr abort during init
#   - a missing DLL / bad cgo link -> exit before main
# So: launch, wait, and fail if it is gone or if its output carries a known
# fatal marker. A healthy GUI app just sits in its message pump.
#
# Usage: windows-run-smoke.sh ./design.exe [seconds]
set -uo pipefail

BIN="${1:?usage: windows-run-smoke.sh <binary> [seconds]}"
WAIT="${2:-12}"
LOG="$(basename "$BIN").smoke.log"

if [ ! -f "$BIN" ]; then
  echo "::error::$BIN was not built"
  exit 1
fi

echo "starting $BIN (will check after ${WAIT}s)"
"$BIN" > "$LOG" 2>&1 &
PID=$!

# Poll instead of one long sleep so an immediate crash is reported promptly.
for _ in $(seq 1 "$WAIT"); do
  sleep 1
  if ! kill -0 "$PID" 2>/dev/null; then
    wait "$PID"
    CODE=$?
    echo "::error::$BIN exited after less than ${WAIT}s (exit $CODE) — it should have stayed in its event loop"
    echo "----- $LOG -----"
    cat "$LOG" || true
    exit 1
  fi
done

echo "$BIN still running after ${WAIT}s"
kill "$PID" 2>/dev/null || true
wait "$PID" 2>/dev/null || true

# A process can stay alive and still have printed something fatal (a recovered
# panic in a worker, a checkptr report on another goroutine).
if grep -qiE "panic:|fatal error|checkptr|main loop mechanism unavailable" "$LOG"; then
  echo "::error::$BIN logged a fatal marker while running"
  echo "----- $LOG -----"
  cat "$LOG"
  exit 1
fi

echo "----- $LOG (tail) -----"
tail -n 30 "$LOG" || true
echo "OK: $BIN started, stayed alive and logged nothing fatal"
