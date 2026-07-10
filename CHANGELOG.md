# Changelog

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
