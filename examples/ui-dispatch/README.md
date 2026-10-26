# Worker-to-UI dispatch

Run `go run ./examples/ui-dispatch` with the native Silk dependencies installed.

- **Inline typed call**: `InvokeValue` executes immediately on the UI thread.
- **Worker typed call**: a worker awaits a typed result without touching controls.
- **Cancel queued update**: cancellation suppresses a `PostContext` callback.
- **Publish 100000 snapshots**: `NewLatestUI` coalesces progress, not event history.
- **Reject worker window creation**: `TryAttachWindow` returns `ErrUIThread` before
  touching a widget from the wrong thread.
- **Open modal through worker**: `Invoke` creates and runs an owned modal on the
  UI thread. Close it to return a result to the worker.
- **Snapshot** prints JSON for repeatable native UI verification.

Always set a deadline for worker-side synchronous calls. Cancellation cannot
interrupt an already-started UI callback; it may finish after the caller returns.
Callbacks must be short, return data instead of writing caller stack variables,
and must not wait for a worker which is waiting for the UI. Use `RunAsync` for HTTP,
file IO or expensive computation, then marshal only the small UI update.

`NewLatestUI` has one pending/running delivery per instance and no timer. Close it
on the UI thread before disposing the receiver; do not use it for events that
must all be preserved. These helpers do not make arbitrary widgets thread-safe.
