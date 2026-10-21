# ListWidget as a resizable results viewport

By default, `ListWidget.SizeHints()` describes its **content**: preferred width
plus padding and row height times item count. This preserves legacy content-sized
lists. It is not appropriate for an initially empty asynchronous results pane.

For a file/symbol finder, command palette or other resizable list:

```go
box := gui.NewVBox()
query := gui.NewEdit()
results := gui.NewListWidget()
results.SetFillAvailable(true)
box.AddWidget(query)
box.AddWidget(results)
```

`SetFillAvailable(true)` opts into a stretch weight of one and horizontal/vertical
growth. Preferred height is six rows; the actual viewport follows the parent
layout's available space, not the number of results. Refilling an empty list or
thousands of results therefore does not collapse it or push over the dialog's
footer. The existing scrolling/selection APIs are unchanged. The setter
invalidates the parent layout and is exposed as a designer property.

Call the setter and mutate rows on the UI thread. Set it back to false to restore
content-sized hints without clearing the selection or item data. Fill mode does
not size a native window for you; the host still owns the dialog size and layout.

Regression tests cover the empty asynchronous state, result counts 0/1/20/1000,
parent resize, opt-out, and the actual IDE command-palette content/footer bounds.

For modal hosts, configure `dialog.SetInitialFocus(query)` instead of calling
`query.SetFocus()` before `ShowModal()`. Focus is assigned after native activation,
so the owning editor's focus is still available when the dialog is cancelled.
Run selected commands after `ShowModal` returns, not inside a submit callback.

## Failed modal initialization

Modal rollback is registered before native activation or the caller's show
callback. A recovered panic produces `SILK-GUI001` and a stack trace, releases
the modal/owner lock, and cannot return an acceptance result. Check
`dialog.LastError()` after `ShowModal()` to distinguish a failure from an ordinary
Cancel. Low-level hosts can use `window.ModalError()`. Both are UI-thread APIs;
the stored error is reset on the next modal invocation. Existing return types
are unchanged. This recovery does not promise rollback of arbitrary application
mutations performed inside a faulty callback; preserve unsaved work and report
the error code and stack.
