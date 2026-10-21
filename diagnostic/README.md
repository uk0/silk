# Structured Silk diagnostics

`github.com/uk0/silk/diagnostic` is a dependency-light SDK package. Use stable
codes rather than matching translated messages. An error contains `code`,
`severity`, `message`, a remediation `hint`, source `location` and optional
underlying `cause`. `errors.Is`/`errors.As` work on the original wrapped error;
JSON preserves the cause text, not its executable Go type.

Locations use **1-based lines and UTF-8 byte columns**, with a zero-based byte
offset. A zero line means a file-level error. UI integrations must translate
byte columns to their rune/UTF-16 coordinate system explicitly.

```go
err := diagnostic.New(diagnostic.FileRead, "cannot read source", cause).
    At(diagnostic.Location{File: path})
if diagnostic.CodeOf(err) == diagnostic.FileRead { /* show recovery UI */ }
json.NewEncoder(log).Encode(err)
```

`Lookup` / `Catalog` return copies. Existing code meanings must not be reused.
Unknown codes passed to `New` fall back to `SILK-0001`. `At` returns a copy.

## Integration status

Macro expansion, analysis and generated-output protection now return these
errors. The macro CLI supports `-diagnostics-json` on stderr, with a nonzero
exit status; successful generated Go stays on stdout. Interactive macro errors
carry codes and remediation in the editor/problem pane. Explorer scan/create/rename/delete errors use the file codes. Explicit IDE
language actions (definition, references, code actions, formatting and workspace
symbols) use `SILK-LSP001` when unavailable and `SILK-LSP002` for request failures,
with underlying cause and recovery hints in the Log pane. Interrupted/failed native
LSP transport uses `SILK-LSP003`, preserves its cancellation/I/O cause, and advises
a service restart; it is not suppressed as normal supersession. Superseded request
cancellation does not produce error banners. SDK `LSPClient.Err()` exposes the terminal cause even when the initiating request
was superseded. IDE session retirement reports it once; intentional shutdown and
old-session callbacks stay silent. Startup failures retain `SILK-LSP001` and cause.
Modal initialization/event-loop panics use `SILK-GUI001` with a logged stack;
`Dialog.LastError()` / `Window.ModalError()` distinguish these failures from
normal cancellation. The command palette and file/symbol finders surface them.
Not every historical IDE/SDK
failure has been migrated yet. See the active iteration checklist.

Editor Git refresh reports `SILK-GIT001/002/003` through `CodeEditor.GitStatusError()`
and the IDE Log pane with cause/recovery hint. Automatic repeats are deduplicated
per editor; the palette action **Refresh Editor Git Markers** provides an explicit
success/error banner. Non-repository files are a normal empty result, not errors.

## Catalog

| Code | Meaning | Recovery |
| --- | --- | --- |
| `SILK-0001` | Internal operation failed | Keep the error code and underlying cause when reporting this failure. |
| `SILK-DOC001` | Invalid TDoc syntax | Inspect the decoded UTF-8 location, indentation and key escapes; retain the original file. |
| `SILK-DOC002` | Cannot read TDoc | Check the source, path, permissions and original I/O cause. |
| `SILK-DOC003` | Cannot decode compressed TDoc | Restore a complete gzip member and validate with a full load, not a limited prefix read. |
| `SILK-GIT001` | Git is unavailable | Install Git or add it to the application PATH, then refresh editor Git markers. |
| `SILK-GIT002` | Cannot refresh Git markers | Check repository permissions and the underlying cause, then retry. Editing remains available. |
| `SILK-GIT003` | Git diff resource limit reached | Reduce the diff or generated file size; partial markers are not displayed as complete. |
| `SILK-GUI001` | Modal operation failed | Save current work, reopen the dialog, and include the code and logged stack when reporting. |
| `SILK-FS001` | Cannot read file | Check the file still exists and is readable. |
| `SILK-FS002` | Cannot write file | Check directory permissions and available disk space; keep the unsaved buffer. |
| `SILK-FS003` | Invalid filename | Use a single portable filename without separators or reserved device names. |
| `SILK-FS004` | Destination already exists | Choose another name; existing files are not overwritten. |
| `SILK-FS005` | Cannot scan directory | Check directory permissions and retry Refresh. |
| `SILK-FS006` | File has unsaved edits | Save or close the affected editors before changing the filesystem. |
| `SILK-FS007` | Project scan limit reached | Choose a smaller root or adjust scan limits; this index is incomplete. |
| `SILK-FS008` | File operation is busy | Wait for the pending operation or stop the active task; the editor buffer is retained. |
| `SILK-FS009` | Background file work did not stop | Wait for Git work to finish and retry rename; no filesystem change was made, and buffers/cause are retained. |
| `SILK-LSP001` | Language server unavailable | Install/configure gopls and restart the language service. |
| `SILK-LSP002` | Language request failed | Check the underlying server error and retry after the workspace finishes loading. |
| `SILK-LSP003` | Language-server transport interrupted | Save work and restart the language service; a partial or stalled pipe was closed. |
| `SILK-MAC001` | Macro resource limit exceeded | Split the source or reduce expansion count/output size. |
| `SILK-MAC002` | Invalid macro directive | Use standalone //silk:macro, //silk:end or //silk:expand lines. |
| `SILK-MAC003` | Invalid macro declaration or invocation | Use Name(arguments); parameters must be distinct ASCII identifiers. |
| `SILK-MAC004` | Nested or recursive macro | Close the current definition; macro bodies cannot contain expansions. |
| `SILK-MAC005` | Duplicate macro definition | Rename or remove the duplicate definition in this file. |
| `SILK-MAC006` | Unknown macro parameter | Declare the ${Name} parameter in the macro signature. |
| `SILK-MAC007` | Unclosed macro definition | Add a standalone //silk:end after the macro body. |
| `SILK-MAC008` | Unknown macro | Define the macro in this file; cross-file imports are not supported. |
| `SILK-MAC009` | Wrong macro argument count | Supply one argument for each declared parameter. |
| `SILK-MAC010` | Expanded Go is invalid | Check the macro body and arguments at the expansion site; inspect the underlying Go parser error. |
| `SILK-MAC011` | Generated output is protected | Choose a regular .go output carrying silkgen's exact generated marker; never overwrite handwritten or symlink output. |
| `SILK-MAC012` | Cannot install macro output | Check output-directory permissions and disk space; previous output is preserved if installation fails. |
| `SILK-MAC013` | Macro document is unsaved | Save the .silkgo source and any modified generated output before expanding. |
| `SILK-MAC014` | Macro source changed during generation | Expand again using the current source. |

### Settings persistence

`SILK-SET001` protects a file that failed to load; `SILK-SET002` reports actual
write/replace failure without clearing dirty state; `SILK-SET003` means a flush
wait ended while filesystem work may still be running. See
[`settings/PERSISTENCE.md`](../settings/PERSISTENCE.md) for the scheduler and
shutdown contract. IDE preference callbacks now enqueue coalesced background
saves, report current errors on the UI thread, and join the final save after the
event loop exits. See [TDoc I/O](../core/TDOC_IO.md) for the generic reader's
`SILK-DOC001/002/003` codes, framing and validation boundaries.
