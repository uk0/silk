# Live-alarm binding ownership — verified combined increment

Baseline: `f7a191707e748a5f9133ea2498f7ee7c9959b195`. The AlarmPanel correction is
verified together with startup restoration and watcher changes. This combined
increment supersedes the earlier startup-only candidate; old Windows tasks/source
remain untouched. Their results are not verification of the combined source.

## Reproduced behavior

The original public flow is Bind → Update → cancel/rebind → UI drain. Three
regressions fail on the original SDK:

- An already queued refresh still updates a panel after unsubscribe.
- An old queued database refresh overwrites a newly bound database's rows.
- A burst of 1,000 transitions queues 1,000 UI refresh callbacks.

Two later prototype regressions also expose retained source/owner references
through long-lived cancellation handles. These are separate from the original
SDK counterexamples. Preserve the distinction between the original failures and
prototype hardening; neither is inferred from comments alone.

## Correction and contract

- One per-panel delivery gate owns the current subscription and at most one
  queued UI refresh. Rebinding retires the old owner without requiring callers
  to remember its cancel handle. Old handles cannot cancel a replacement.
- Subscribe before the initial snapshot so a transition cannot disappear between
  the first read and subscription. All widget reads/writes stay on the UI thread.
- The returned cancel function is thread-safe and idempotent. It revokes future
  UI admission; an already admitted refresh may finish. It does not join UI or
  clear the last displayed rows. `BindAlarmDB(nil)` on UI detaches and clears.
- Retirement drops source/subscription references under the delivery mutex and
  atomically detaches a held handle's owner. An admitted refresh borrows its
  source under that same mutex; cancellation cannot race its source pointer.
- No polling timer or per-event goroutine is added. This is not a typed alarm
  engine, durable alarm history, permissions system or PLC actuation.

## Isolated evidence before merge

| Overlay test scope | Ordinary named pass / skip | Race named pass / skip |
| --- | ---: | ---: |
| macOS arm64 complete GUI package | 3,372 / 9 | 3,372 / 9 |
| Linux arm64 / Xvfb complete GUI package | 3,340 / 11 | 3,340 / 11 |

Both platform runners exit zero. Linux checks the frozen underlying source
before/after; only the two explicit overlay files change. Thirteen alarm cases
also pass 100 race repetitions: 1,300 named passes, zero skip/fail. A new-test
cleanup error (`defer` evaluated Bind(nil) immediately) was corrected separately;
its failed logs are retained rather than attributed to the SDK.

These are package/overlay results, not whole-repository or Windows approval.
The new combined freeze passes complete macOS/Linux/Windows ordinary/race suites
and 12 strict bundled-gopls probes; see the exact counts and source identity
in [startup verification](STARTUP_IO_QA_20261115.md#publication-identity-update).
All platform runners and source/publication-input checks exit zero; none are
inherited from the earlier startup-only candidate. A fresh macOS V3 CUA replay captures
and passes basic restored input/save, design reload, settings readback and normal
exit. Earlier -3811 failures remain retained; IME and alarm-specific macOS native
UI remain untested rather than inferred from that basic replay.

## Combined-source Windows native replay

The separately built native SDK fixture uses the unmodified merged GUI package
and runs in interactive Session 1. Its five screenshots were inspected against
independent JSON snapshots, all recorded from the UI thread:

- Start with OLD-A. Emit 1,000 transitions, switch to CURRENT-B and invoke the
  old cancellation handle: the visible row remains CURRENT-B, value 10.
- Acknowledge CURRENT-B: both visible state and readback show Acked true.
- Cancel, then mutate both databases to 999: the panel keeps B's last value 10
  and acknowledgment, rather than admitting the canceled subscription.
- Explicit UI-thread Bind(nil), then mutate both sources: the panel stays empty.
- Normal Alt+F4 exit is independently read as code zero through a held native
  process handle. The known bare SDK default-icon gap remains; this replay does
  not claim a new alarm engine, history store or all-widget visual approval.

This native lifecycle evidence and the completed whole-repository Windows
race/strict gates refer to the same combined source. The first strict harness's
wrong staging path and corrected replay are documented, not silently discarded.

```sh
go test -race -count=100 ./gui -run '^TestAlarmPanel'
go test -race -count=1 ./gui ./ged ./cmd/silkide
```

Raw isolated failures, overlays, source hashes and proposal history:
`bin/qa-artifacts/alarm-lifecycle-20261115/`.
