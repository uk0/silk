# Unicode field completion

Run `go run ./examples/edit-completion` on a machine with the Silk GUI dependencies.

- **Complete** filters the current prefix and accepts the first match. The seed
  `中🙂 ap 后缀` becomes `中🙂 apple🍎 后缀`, preserving text after the caret.
- **Undo / Redo**, keyboard shortcuts and the field context menu share the same
  bounded Edit history. Right-click or Shift+F10 keeps the target selection visible.
- **Read-only** prevents completion and editing; **Reset** restores a clean seed.
- **Snapshot** writes `SNAPSHOT { ... }` to stdout, including text, prefix, rune
  caret, selection, history availability, read-only state and user-edit count.

`SetCompletionPrefixStart` uses a **UTF-8 byte offset** for compatibility.
`SetCaretPos`, selection and the returned caret use **rune offsets**. An interior
byte offset snaps backward to the rune start. Invalid/out-of-caret anchors mean
an empty prefix at the active caret; completion must never slice UTF-8 bytes.

Completion is one user edit, including one changed/edited notification pair;
it obeys read-only, disabled, maximum-length and validator gates. Undo restores
the prior selection and prefix anchor. Programmatic `SetText` starts a new
history and resets the prefix anchor to zero.

This example deliberately uses a visible Complete button, not an automatically
displayed suggestion popup. `Completer` is the matching model; hosts still own
candidate presentation. No timer, background scan or network service is used.
