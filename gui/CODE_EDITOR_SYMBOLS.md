# Editor and SDK declaration model

`CodeEditor.ParseSymbols()` and `gui.OutlineSymbols(source)` use the same tolerant
Go declaration model. It distinguishes code from comments/raw strings, handles
inline comments, grouped declarations, multiple var/const names, generic receiver
methods and package-less snippets. Valid declarations recovered by `go/parser`
remain available while a function is incomplete. This is not a promise to recover
all declarations after every possible syntax error.

Locations address the actual unsaved buffer: lines are zero-based; SDK columns
are UTF-8 **byte** offsets, not UTF-16 LSP positions. Compiler `//line` directives
must not redirect editor jumps. Definition/reference fallback uses the recovered
AST and never invents declarations through a regexp search in comments or strings.
These local helpers are name-based fallbacks, not a replacement for gopls scope,
type or workspace resolution.

Only top-level declarations belong in the outline. Function body interiors are
lexically blanked before outline parsing, preserving original byte lengths and
newlines; this avoids statement AST allocation without treating braces in strings
as syntax. Source/details and returned snapshots have separate ownership. Display
signatures are bounded; actual symbol names and source locations are not clipped.
The editor keeps its one-parse-per-text-revision cache. Unchanged revisions perform
no parsing. Parsing changed Go source is more expensive than the former incorrect
line scanner; very large buffers still need background analysis, not a claim of
constant-time edits.

## `ged.CodeOutlinePanel`

All panel operations are UI-thread operations.

- Switching editors, editing/undoing, or explicit host revision invalidation
  retires semantic symbols belonging to the previous buffer state.
- `SetSymbols` is the synchronous compatibility API. It deep-copies the tree;
  nil/empty preserves the documented local-parser fallback behavior.
- Async hosts should capture the editor pointer and `editor.TextRevision()` before
  requesting symbols, marshal back to the UI thread, then call
  `SetSymbolsForEditor(editor, revision, symbols)`. `false` means stale/unbound:
  no view mutation occurred. Hosts still own their request/server lifecycle.
- Go generic receiver methods group under their base type. Semantic `macro` kinds
  use the `M` label. Position/alphabetic sort does not change caret source order.
- Up/Down, Home/End and PageUp/PageDown select; Left/Right collapse/expand or select
  a parent/child; Enter navigates once (key-repeat is ignored). Arrows retain panel
  focus. The existing navigation callback remains line-based for compatibility;
  exact-column outline activation is not yet provided. SilkIDE activates the target
  through `CodeEditor.NavigateToLineCol`: this explicit-jump SDK API reveals folds,
  clears old selections/secondary carets/completion and returns input focus without
  modifying text or undo history. Existing scroll-only APIs retain their semantics.
- The header/body are clipped separately. Labels use UTF-8-safe ellipsis, font
  metrics and theme colors. Font changes also change hit geometry and row height.
  Only visible rows are drawn. Fonts, visible labels and unchanged caret lookup
  are cached; resize/filter/content changes clamp scrolling before interaction.

## Regression evidence

```sh
go test -race -count=1 ./gui ./ged -run 'TestGoDeclarations|TestNavigation|TestOutline|TestEditLayoutClamps'
go test -run '^$' -bench BenchmarkOutlineIdleDraw -benchmem ./ged
```

The presentation tests render real Cairo surfaces in light/dark themes with
14/24-point fonts and 5,000 symbols. They assert clipping, widths, theme usage,
painter-state balance and bounded visible draw calls. These are offscreen tests,
not native Win10/macOS input acceptance. Native UI results must be tied to their
own source manifest and executable hash.
