#!/usr/bin/env bash
set -euo pipefail

# Run from the repository root. Do not use -short or accept a SQLite skip:
# the hosted Windows lane must exercise the same data path as the workbench.
log=${1:-hmi-runtime-test.log}
CGO_ENABLED=1 go test -race -p 1 -count=1 -timeout 12m -v \
  ./hmi/ ./hmiapp/ ./point/... 2>&1 | tee "$log"

required=(
  TestDiagnosticObserversOnlyNotifyOnTransitionsAndRetire
  TestDiagnosticCancellationRetiresAlreadyQueuedFanout
  TestWorkbenchSnapshotSeparatesMaintenanceAndUsesRunningPoints
  TestRuntimeViewShowsLiveHTTPDiagnosticsWithoutSnapshot
  TestRuntimeDiagnosticStatusPrioritizesFaultsInNarrowPanes
  TestHTTPSourceKeepsHealthyMappingsAtConfiguredCadence
  TestSQLiteRejectsLosingTextBeforeTransactionCommit
)
for name in "${required[@]}"; do
  if ! grep -Eq "^--- PASS: $name \\(" "$log"; then
    printf 'Required HMI regression did not pass: %s\n' "$name" >&2
    exit 1
  fi
done
