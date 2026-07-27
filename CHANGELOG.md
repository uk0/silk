# Changelog

## v2.5.0 (2026-07-27)

### IDE — Qt Creator-class depth

Engines and panels closing the gap to Qt Creator. Editor engines and models are
pure and GL-free; wiring them into the editor/designer/IDE hotspots is the next
step.

- **Editor engines** — multi-cursor with per-cursor selections, rectangular
  (column) blocks, sticky columns and bottom-up edit ordering; a visual-line
  wrap layout with lossless source↔visual mapping; a `go/scanner`-based fold
  scanner that no longer counts braces inside strings or comments and folds
  import and comment blocks; a find/replace model with regex, case, whole-word,
  in-selection, wrap-around and preserve-case replace
- **Shared documents** — a `Document` with revisions, dirty tracking and
  edit-remapped anchors, an editor workspace with real split groups over one
  buffer (previously split cloned the text), a versioned session (ordered tabs,
  cursors, folds, splits, unsaved buffers) and a bookmark store that re-anchors
  by line context
- **LSP** — the client now advertises capabilities and workspace folders and
  **answers server-to-client requests**; previously `workspace/configuration`
  and `workspace/applyEdit` were dropped, which silently broke organize-imports
  and every command-based refactor. Completion keeps `textEdit` /
  `additionalTextEdits` so auto-import works, workspace edits decode
  `documentChanges` with versions and resource operations, and call hierarchy,
  type hierarchy, implementation, prepare-rename, inlay hints, semantic tokens
  and code lens are added. Edits apply transactionally: preflight, in-memory
  computation, all-or-nothing write with rollback, and a no-touch preview
- **Panels** — call hierarchy, type hierarchy, code insights (semantic tokens /
  inlay hints / code lens), grouped and filterable Find References, an outline
  driven by semantic symbols, a Go analyzer panel and a hierarchical test
  explorer
- **Build & run** — kits (toolchain, GOOS/GOARCH, tags, race/coverage, output,
  local or SSH deploy) persisted per project, multiple named run/debug
  configurations, and a task runner executing a dependency graph with streaming
  events, history and process-group cancellation, replacing hardcoded and
  uncancellable `go build` / `test` / `vet`
- **Testing** — results now come from the `go test -json` event stream, so
  output is attributed to the owning test and subtests nest under their parent,
  fixing misattribution of interleaved multi-package console output
- **Debugger** — clear/amend/toggle breakpoints with conditions, hit counts and
  logpoints; goroutine- and frame-scoped locals **and arguments**; lazily
  expanded variable children; goroutine pagination; debuggee output capture; and
  a debug panel with a breakpoint table, explicit goroutine/frame selection
  (watches previously always evaluated in goroutine -1 / frame 0) and a console
- **Version control** — branches, remotes, fetch/pull/push, stash, rebase,
  cherry-pick and conflict status behind non-interactive typed APIs with pure
  parsers (NUL-delimited porcelain, C-style quoted paths); a workspace panel
  grouping conflict/staged/unstaged/untracked against the real index; a
  multi-file patch model with hunk staging that no longer drops the unchanged
  lines between hunks or everything past the first file; and a diff3 merge
  engine with a conflict editor
- **Terminal & search** — a PTY-backed shell session with a full ANSI/SGR/cursor
  state machine replacing the one-shot line-oriented command runner; project
  search exposes regex/case/whole-word/include-exclude and replaces
  transactionally with preview and rollback; the TODO scanner becomes a
  configurable multi-language incremental index that no longer matches inside Go
  string literals

### 组态 / SCADA runtime

- `scada.Services` — one shared `TagDB`, `AlarmDB`, historian, event log, recipe
  book and stats collector with `New`/`Start`/`Stop` and alarm→event/notify
  wiring; `BindScreen` walks a widget tree and connects industrial widgets and
  panels through their plain-data setters (`scada` imports `gui`, never the
  reverse)
- Code generation emits `Services`-backed applications (`BindServices`, one
  shared runtime, no private `TagDB` per app) and silkide runs a live preview
  against an in-memory Services with drivers off
- Operator panels — recipe, report, event log, trend playback, statistics and
  calc — plus a device point-list editor and an end-to-end HMI example
- Redundant driver and gateway expose failover/error callbacks

### Designer & UI fixes

- Property IDs accept unicode labels — every Chinese property id (液位, 协议,
  点表 …) was previously lowercased, rejected and **silently dropped** from the
  designer
- `Table`/`HeaderView` no longer dereference an uninitialised theme face (drawing
  either one panicked)
- `SearchBox`/`NumberInput` key and text handlers had signatures matching neither
  dispatched interface, so typing was discarded; `Button` gains Enter/Space
- Designed widget properties now round-trip through `Save`/`LoadDesign`
- silkide shows a live `PropertySheet` for the selected widget, and worker
  build/test/vet output is marshalled onto the UI thread

### Icons

- 18 new icons at 16/22/32/48 in the existing flat style: `silkide`, `sandbox`,
  `platform` (silkide had **no application icon** — it is looked up by executable
  name), `diagram`, `map`, `project`, and distinct icons for the call-hierarchy,
  type-hierarchy, test-explorer, bookmarks, outline, todo, references, packages,
  keymap, help, log and merge panels (13 panels previously shared `edit`, 9
  shared `tree-view`)

### Build

- Windows CI/release and the README move from **MSYS2 MINGW64 to UCRT64**
  (`mingw-w64-ucrt-x86_64-*`)
- `win32.HRESULT` widened to `uintptr` — `syscall.NewCallback` requires a
  uintptr-sized result, so the COM drag-drop vtables panicked at startup on
  Go 1.25 (`compileCallback: expected function with one uintptr-sized result`)
- SQLite switches to the go-gettable `mattn/go-sqlite3`; the vendored fork was
  gitignored, so a clean clone could not build

## v2.4.0 (2026-07-09)

### 组态 / SCADA & Industrial Automation
- Real-time tag database (`TagDB`) with value-driven bindings, animation and alarms
- Field-bus drivers: **Modbus TCP, Siemens S7, OPC-UA, MQTT** — all PLC data types, all four register/byte orders (ABCD/DCBA/BADC/CDAB), read-only / read-write
- Driver **redundancy** (primary/backup failover), protocol **gateway** (data forwarding), **simulator** driver (hardware-free testing)
- `DeviceComponent` — widget-form device configuration placeable in the designer
- Structured-config **device templates** (batch tag instantiation)
- **Historian** (SQLite tag history), **reports** (interval aggregation, CSV/HTML), **trend playback**
- **Recipe** management, **calc/formula** tags, **event log**, live tag **statistics**
- Alarm engine + notification/event bridge; industrial widgets (Tank, Gauge, Valve, Indicator, Pipe, …)
- Runtime **Go scripting** (yaegi); **user auth** + login sessions

### IDE (silkide) — Qt Creator parity
- LSP (gopls): completion / hover / definition / references / rename / format / code actions
- Delve debugger integration
- **Locator** (fuzzy quick-open), **find-in-files**, **snippets**, **build-issue** parsing

### Build
- Go-gettable SQLite (`mattn/go-sqlite3`) — the tree now builds from a clean clone
- CI covers the full SCADA + IDE package set

## v2.3.0 (2026-04-12)

### New Widgets (20 new)
- ToggleSwitch, SearchBox, NumberInput, DatePicker, ColorPicker, Rating
- DropdownButton, SwitchGroup, Avatar, Badge, Breadcrumb, Tag
- Card, Accordion, Link, LabelSeparator, Placeholder, Timeline
- NotificationPanel, ImageView

### Visual Designer
- Smart alignment guides during drag
- Ctrl+Scroll zoom, Space+drag pan
- Object inspector, undo history panel
- Tab order editor, widget locking
- Form size presets (device profiles)
- Welcome screen, custom template saving
- Mode selector (Design / Code)
- File explorer, multi-tab editor
- Build output panel with error navigation
- Toolbar with common actions
- Theme preview panel
- Keyboard shortcut reference

### Code Editor
- Text selection, undo/redo, find/replace
- Auto-completion, code snippets
- Cmd/Ctrl+Click cross-file navigation
- Symbol navigation, Go to line
- Bracket matching, minimap, status bar
- Bookmarks, rename refactoring
- Code formatting (gofmt), error markers
- Split editor view, word wrap

### Framework
- Layout engine: stretch weights, min/max sizes, alignment
- Animation system: 12 easing functions, groups
- Style system: 5 color schemes, widget presets
- Context menu system, right-click support
- Modern theme (Tailwind-inspired colors)
- Performance: pixmap caching, backbuffer reuse
- Menu popup positioning fix (macOS)

### Testing
- 280+ automated tests
- Widget factory, layout, animation, style, codegen, persistence tests
- Benchmarks for layout and widget creation

## v2.2.0 (2026-04-10)
- Initial open source release
- 40 widgets, visual designer, code generation
- Cairo 2D rendering, GLFW/Win32 windowing
- TDoc persistence, signal-slot events
- 6 project templates, dark/light theme
