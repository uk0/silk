# Cancellable workspace-symbol requests

`LSPClient.WorkspaceSymbolContext(ctx, query)` has the same result shape as
`WorkspaceSymbol(query)`. The old method remains source-compatible and uses a
background context. Results retain zero-based **UTF-16** wire coordinates; use
`gui.CodeEditor.ScrollToUTF16Position` only after the destination buffer is open.

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
symbols, err := client.WorkspaceSymbolContext(ctx, "Target")
// Cancel from the owner when a newer query supersedes this one.
// errors.Is(err, context.Canceled) is an expected retirement, not a user error.
```

## Transport deadlines and stream integrity

The shared transport has a ten-second default budget covering write-gate waiting,
native pipe I/O and response waiting. Earlier caller cancellation/deadlines win.
`SendNotificationContext(ctx, method, params)` adds the same control to outgoing
notifications; the original `SendNotification` remains synchronous and compatible.
A successful return means a complete frame was written, **not** that the server
processed it. Sequential successful didOpen/didChange/didClose calls retain order.

Cancellation has three distinct outcomes:

- **Before writing / waiting for the gate:** no bytes from that call are written;
  the session remains usable. Pre-cancelled requests allocate no request ID.
- **After a complete request write:** the pending ID is removed immediately;
  a single bounded worker sends best-effort `$/cancelRequest` without keeping the
  canceled caller alive. Its queue holds 64 IDs; saturation drops cancellation
  advice, never document changes. Each cancellation frame has a 500 ms budget.
  Late responses are ignored and cannot resolve a different request.
- **During an active request write:** the caller returns cancellation immediately.
  One serialized writer gives the frame up to 500 ms to finish. This matters when
  the peer has read the full frame but Write has not returned its acknowledgement.
  If the write completes, send exactly one protocol cancellation and keep the
  healthy session. If it remains stuck, close the stream with `SILK-LSP003`,
  preserve the cause, release waiters and reap the owned server. Hosts observe
  that terminal failure through `Err()` / Notifications closure.
- **During a notification/reply write:** there is no per-request cancellation
  protocol. An interrupted active write retires the stream with `SILK-LSP003`
  rather than continuing with unknown document state. Waiting-to-write calls
  still cancel without harming the session. Short writes fail, never claim success.

A single writer owns stdin and joins its cancellation callback before accepting
another frame. There is no goroutine per queued write or canceled request. The
caller deadline bounds its wait, while canceled request I/O has the bounded drain
grace above. Explicit Close is idempotent, with short best-effort shutdown followed
by pipe/process cleanup. Response timeout alone does not kill a healthy session.

The interruption guarantee applies to the native process pipes created by
`LaunchLSPClient` (and tested `os.Pipe`/`io.Pipe`). It is not a promise to preempt
arbitrary user `MarshalJSON` code, CPU serialization or a custom `io.WriteCloser`
whose Close does not unblock Write. Server-side errors and malformed responses
remain errors, not empty-result success.

## SilkIDE search scheduling

The symbol finder owns one worker, one replaceable pending query and at most
one queued UI delivery. A 90 ms quiet period debounces typing; updates immediately
cancel the previous context and invalidate its rows. Even a non-cooperative
request cannot create a new goroutine/RPC per keystroke. It must finish before
the next request starts. Closing the dialog is non-blocking, cancels active work
and retires queued results/errors. The SDK transport now releases a canceled
native-pipe request using the stream-integrity rules above. An injected custom
request that ignores context still cannot create additional per-dialog workers.

Client identity and query generations are validated before display/selection.
A language-server replacement re-queries the current text instead of opening
an old-session result. The view explicitly distinguishes searching, empty,
failed and cancelled states. It shows at most 500 matches with a visible
refinement notice; the SDK result is not truncated. Return in the query field
uses the currently selected row. `SILK-LSP001` / `SILK-LSP002` / `SILK-LSP003`, the cause and the
recovery hint remain available in the banner, session Log pane and error tooltip.

## Terminal failure reporting

`LSPClient.Err()` is a thread-safe snapshot of the first unexpected terminal
transport failure. It returns nil for a live session or intentional Close,
including a graceful shutdown deadline. Call it when Notifications closes to
recover the cause even if a request owner canceled or invalidated its delivery.
The returned diagnostic is copied so UI annotation does not mutate client state.
SilkIDE session retirement checks both client identity and generation, clears
stale completion state without discarding text, and surfaces the code/cause/hint
once. Launch/initialize failures use SILK-LSP001; the initialize callback captures
external cancellation before canceling its own cleanup context, so real failures
no longer disappear before the GUI sees them.
