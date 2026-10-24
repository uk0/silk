# Bounded language-server work progress

`LSPClient.WorkProgressUpdates()` and `WorkProgressSnapshot()` add an opt-in,
read-only view of server work-done progress. They do not measure CPU, outstanding
RPCs, or unreported cache work. An empty active list is **not** proof of idle.

1. Call `WorkProgressUpdates()` before `Initialize`.
2. Merge `window.workDoneProgress: true` into your initialize capabilities.
   `DefaultClientCapabilities()` intentionally leaves this disabled for hosts
   that do not implement a progress UI.
3. On each channel wake, read the latest `WorkProgressSnapshot()`. Route GUI
   changes through the host's UI queue, coalescing callbacks rather than posting
   one callback for each server event.
4. Drain ordinary `Notifications()` separately. Work-done `begin/report/end`
   messages no longer compete with diagnostics after opt-in; partial-result
   progress and unknown progress shapes retain their normal notification route.
5. The progress channel closes with the transport. Retire the UI state when the
   client/session changes; never let old-session callbacks update a new session.

Snapshots own their token bytes and percentage pointers. Numeric and string
progress tokens remain distinct. Omitted report fields preserve prior values;
explicit empty messages and `cancellable: false` replace them. Duplicate reports
produce no extra wake. A completed operation is removed even if its optional
final message exceeds the display payload budget.

Retention limits are deliberate: 64 active operations, a single pending wake,
1,024-byte encoded tokens, 16 KiB work-update payloads, 256-byte titles and
512-byte messages (UTF-8 boundaries preserved). `Limited` stays true for the
session if an update/capacity limit could hide work. Do not display "all done"
for an incomplete view. `LastCompleted` is only the most recent reported title.
Cancellation flags are metadata; this API does not automatically cancel work.

The IDE opts in before initialization, presents a keyboard-accessible status
button, and includes the snapshot in Toolchain Status. It has no progress polling
timer or spinner. Repeated tooltip-only changes do not invalidate layout/paint.
The status action reports paths/recovery and never installs tools automatically.
