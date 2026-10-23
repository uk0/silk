<p align="center">
  <img src="assets/branding/silkide-256.png" width="128" height="128" alt="SilkIDE folded silk S logo">
</p>

# Silk — Cross-Platform Go UI Framework & Visual Designer

A Go-native GUI framework with a **visual form designer** and **integrated Go IDE**. Drag widgets onto a canvas, arrange nested layouts, generate Go code, then build and run from the same workbench. The project is working toward a Qt Creator-style workflow; it does not claim full feature parity.

The SilkIDE mark is a cyan-to-blue folded silk **S**. Transparent PNGs and
Windows/macOS application icons live in [assets/branding](assets/branding/README.md).

**Release:** [SilkIDE v0.1.5 / SDK 2.5.1](https://github.com/uk0/silk/releases/tag/v0.1.5)
provides Windows amd64, Linux amd64 and macOS arm64 / amd64 archives with SHA-256 checksums.
The four-platform release gates and actual Windows package desktop checks passed;
see [release notes](RELEASE_NOTES_v0.1.5.md) and [package QA](RACE_RENDER_QA_20261020.md).
Windows system/per-monitor DPI scaling remains incomplete and may still blur the
whole window. macOS packages require external Cairo and are not Developer ID
signed/notarized standalone apps.

---

## Editor and language-service fixes (development)

### Semantic navigation and coordinated panels

- **Edit** menu and the command palette expose Go to Definition (`F12`),
  Go to Type Definition (`Cmd/Ctrl+F12`), Go to Implementation (`Alt+F12`),
  and Find References (`Shift+F12`). Multiple destinations stay selectable
  in the References panel rather than silently choosing the first result.
- **Back/Forward** (`Alt+Left/Right`, or `Cmd/Ctrl+[/]` in the editor) share
  a bounded cross-file location history. Diagnostics, outline, references and
  macro jumps join that history; closed targets can reopen and successful
  file renames migrate the stored paths. Navigation preserves unsaved buffers,
  clears stale selections, reveals folds and handles UTF-16/rune conversion.
- Navigation requests are single-flight/latest-wins, with cancellation and
  document/caret/session guards. SDK hosts can reuse precise navigation hooks,
  `NavigationStack`, and cancellable definition/type/implementation APIs.
- The designer event-code panel retains drafts on repeated selection,
  unsubscribes from old designs, and saves handler changes into design Undo/Redo.
  Narrow panels wrap their actions instead of overlapping the editor tabs.
- Status-bar cells stay inside narrow windows, elide long text and keep readable
  gaps. Placeholder branch/caret/version values are removed; the displayed runtime
  version comes from the running IDE, not an assumed workspace toolchain.

Verification and remaining gaps: [navigation/SDK iteration report](NAVIGATION_SDK_QA_20261022.md).

### Designer event handlers and draft safety

- Double-click a supported control to create or locate its default event handler.
  **Design → Event Handlers** and the control context menu expose its registered
  events with their actual Go signatures. Reopening an existing handler does not
  duplicate it; creation and binding share one design Undo/Redo operation.
- The embedded handler editor's unsaved text now participates in the design's
  dirty state, Save/Save All, recovery snapshots and Save/Discard/Cancel guard.
  Saving the design no longer requires clicking the code panel's separate Save
  button, and successful companion generation does not steal panel focus or erase
  unrelated build diagnostics.
- SDK strong-typed export supports multiple methods per control, preserves user
  handler files on regeneration and rebinds method receivers to the exported UI
  type. Hosts can use `SaveDesignWithPendingEdits()` for a checked draft-inclusive
  snapshot; `SaveDesign()` remains a committed-model-only snapshot.
- Scope: this navigation edits **embedded design handlers**, not external `.go`
  files. The IDE's automatic `.silk.go` tree generator is not yet unified with the
  strong-typed event exporter; default preview event execution is not claimed.

Verification, usage and remaining gaps:
[designer event workflow report](DESIGNER_EVENTS_QA_20261023.md).

Open **Tools → Appearance & Editor** (or the command palette) to select English
or 简体中文, dark/light theme, UI/editor font families and sizes, tab width,
accent color, and editor background/text/keyword/comment/string colors. Empty
colors restore theme defaults. Font/color changes apply immediately; the UI
language is saved and applied on restart. `SILKIDE_LOCALE` overrides the saved
language when explicitly set.

- Source tabs render as real tab stops, not missing-glyph boxes; caret, selection
  and mouse positioning use the same widths without rewriting the source.
- Windows declares **system DPI awareness before creating windows** and uses
  grayscale, fully hinted text. This prevents primary/system-DPI bitmap
  virtualization; mixed-DPI per-monitor scaling is still not implemented.
- gopls document open/change/close are ordered. A 90 ms quiet-period worker keeps
  only the latest edit, cancels obsolete completion requests, and rejects stale
  document/session results. Explicit commands flush unsaved text first.
- Completion respects UTF-16 text edits and auto-import edits as one undoable
  change with correct redo caret restoration. Semantic candidates rank before
  local guesses; long signatures are clipped/elided without overlapping labels.
  The IDE requests plain-text completions, not LSP snippets.
  Ctrl+Space requests fresh candidates without inserting a space on Windows;
  informational diagnostics no longer paint red error markers. `gopls` remains an
  external tool, not bundled by this source change.

See [verification and remaining limits](EDITOR_LSP_QA_20261020.md).

![Native Windows editor: Chinese text, Unicode and consistent tab stops](screenshots/silkide-editor-lsp-windows.png)

*Windows 10 native desktop, 2026-10-21 PDT; persisted custom fonts/colors.
[Appearance settings](screenshots/silkide-appearance-windows.png),
[completion](screenshots/silkide-completion-windows.png), and
[English workbench with gopls hover](screenshots/silkide-hover-windows.png).
Win11 and mixed-DPI monitor transitions remain unverified.*

## Current development iteration: satin sprites, HMI and macros

This work is **not part of the published v0.1.5 package**.

- Image-model-generated [16-icon satin atlas](paint/assets/README.md), embedded
  unchanged with calibrated crops, bounded device-resolution caches and neutral
  disabled rendering. Common IDE actions use it by default; set
  `SILK_ICON_STYLE=vector` before launch to retain the vector action theme.
  Structural glyphs stay vector. Idle buttons gain a cached, subtle material
  gradient and border rather than a per-frame image effect.
- Designer **SpriteView** and **ProcessValue** components: Tag binding,
  properties, drag/drop, persistence, preview and Go generation. Gauge also
  exposes `TagName`. Try the [hardware-free HMI example](examples/satin-dashboard).
- Visual Tag updates coalesce bursts to the latest UI value; unbinding retires
  queued updates and running animation. This is not a lossless event pipeline.
- Experimental [Silk macros](macro/README.md): `.silkgo` → ordinary Go via
  `silkgen -macro` or **Tools → Expand Silk Macros**. Explicit generation,
  bounded expansion and overwrite guards; no Go compiler fork or runtime code
  execution. See the [runnable accessor example](examples/silk-macros).

See [iteration QA and remaining limits](SPRITE_MACRO_QA_20261020.md).

![Actual Windows satin HMI dashboard: active valve, pressure card, theme-aware gauge and labeled buttons](screenshots/silk-satin-dashboard-windows.png)

*Native Windows 10 window, local simulated Tags, 2026-10-20 PDT. Button-driven
42.5/95 pressure and dark/light transitions were exercised; this is not a mockup
or a hardware/PLC certification. [Light-theme alarm state](screenshots/silk-satin-dashboard-windows-light.png).*

---

## Table of Contents

- [Integrated Go Designer / IDE](#integrated-go-designer--ide) — screenshot and workflow
- [Validation Status](#validation-status) — tested platforms and remaining gaps
- [Quick Start](#quick-start) — build and launch
- [Development Setup](#development-setup) — prerequisites by OS
- [Project Structure](#project-structure) — what lives where
- [Your First App](#your-first-app) — hello world with the SDK
- [Using the Designer](#using-the-designer) — visual form design
- [Building & Running](#building--running) — all the common commands
- [Features](#features) — what's included
- [Troubleshooting](#troubleshooting) — common issues

---

## Integrated Go Designer / IDE

![Silk IDE on macOS: widget palette, nested design canvas, selected control properties and build output](screenshots/silkide-designer-macos.jpg)

*Actual macOS arm64 window captured on 2026-09-11, using the dark theme and an isolated demo project. This is a running application, not a mockup.*

![SilkIDE on Windows 10: grouped menus, searchable widget palette and nested GridLayout](screenshots/silkide-designer-windows.png)

*Actual Windows 10 interactive desktop capture from the 2026-10-20 iteration (PDT), limited to the application frame. See [Windows rendering and SDK QA](WINDOWS_RENDERING_QA_20261020.md) for reproducible steps and remaining issues.*

[Actual Windows menu capture](screenshots/silkide-menu-windows.png).
[Latest Windows font / menu QA capture](screenshots/silkide-windows-visual.png) (debug outlines enabled).
[v0.1.5 actual Windows release-package capture](screenshots/silkide-v0.1.5-windows.png) (debug outlines enabled).
[Windows IME positioning and Unicode input follow-up](WINDOWS_INPUT_QA_20261020.md).

```bash
make run-ide     # Qt Creator-style Go editing + visual design workbench
make test-ide    # IDE, designer, widgets and tool-engine regression tests
```

- **Welcome / Edit / Design** modes: `Cmd+1/2/3` on macOS, `Ctrl+1/2/3` elsewhere.
- Save and undo/redo always target the **visible document**, from the toolbar,
  command palette or keyboard. `Cmd/Ctrl+Shift+S` saves all changed documents.
- Real file tabs only: edits show `*`; undoing back to the saved text clears it.
- Restart restores tab order, active file, cursor, vertical scroll and mode.
  Missing files are skipped; older path-only sessions still load. Unsaved code
  buffers are **not** crash backups: save/discard/cancel remains the quit guard.
- Saving `.silkui` regenerates a companion `.silk.go`: nested widgets, logical
  geometry, empty captions, scalar properties and exact RGBA colors are preserved.
  Declarative output describes structure; wire event handlers with `BuildWithIndex`
  in host code, or use the imperative exporter for handler bodies.
- Live preview: `Cmd/Ctrl+Shift+R`, or the eye button; export is a separate action.
- Built-in satin action sprites and fallback vector icons require no external
  directory. Vector glyphs use exact sRGB
  theme/semantic palettes and rasterize at the device transform for Retina and
  fractional scaling; disabled icons have their own neutral palette.
- Build / Run infer the nearest `go.mod` for forms stored in subdirectories;
  explicit project and run-directory settings take precedence.
- Drag-and-drop respects nested container ownership and form bounds. HBox, VBox,
  Grid and Form layouts share selection/locking rules with the context menu.
- File / Edit / View / Design / Build / Tools / Help menus group related actions,
  show shortcuts in a separate column, and support Alt accelerators and arrows.
  The searchable widget palette retains factory identity and drag/drop behavior.
  Search fields support selection replacement and clipboard editing, with long
  queries clipped away from the search/clear icons.
- Menus support keyboard navigation, disabled actions and nested popup dismissal.
  Properties and View Code activate the corresponding dock instead of silently
  doing nothing; export reports the actual output path or an error.
- Stop terminates the active Run command and its child processes as well as the
  debugger; independently opened terminal sessions are left alone.
- Project settings use the selected project root and persist Kit changes.
  About separates IDE and SDK versions, shows runtime/build details, and wraps or
  scrolls long content without overlapping its right-aligned Close button.
- Fit canvas: `Cmd/Ctrl+Shift+0` (plain `F` remains available for code input).

For desktop verification, use a disposable project and an isolated settings
home. `gopls` and `dlv` are optional external tools; installing or building the
IDE alone does not establish language-server or debugger availability.

### Validation Status

The 2026-10-20 race/render follow-up bounds UI callback turns, prevents native
Windows input/paint/timer starvation during callback bursts, joins outstanding
GLFW wakeups before shutdown, and serializes build-progress delivery. The
optional pure-Go backend no longer shares mutable glyph buffers across separate
canvases, and its dash, exact-color, disabled-icon and high-DPI stroke regressions
are covered. Raster/custom icons now select device-resolution sources, Windows offscreen
text uses neutral grayscale antialiasing, and the GL blit respects premultiplied
alpha instead of darkening translucent edges. These fixes do not add Windows
per-monitor DPI awareness. See [race and rendering QA](RACE_RENDER_QA_20261020.md) for actual
commands, evidence and remaining backend limits.


The 2026-10-11 core iteration adds zero-allocation RGBA/NRGBA image uploads,
damage-sized GLFW texture transfers, draw-time invalidation retention, and
in-place activation of an existing free layout container. Real macOS GL texture
readback covers PBO and direct-upload paths. See [Rendering QA](RENDERING_QA.md)
for measurements, reproduction commands, and platform/UI verification limits.
The Windows UI iteration adds owned native Open/Save dialogs, theme-aware object
inspection, process-local Go tool discovery, and deterministic retirement of
native paint resources. It also removes translation-only scaled-font cache keys.
Integer properties now use an exact, registered editor; generated companions
follow the neighboring Go package. Background Windows tool commands no longer
open an intrusive console window.
The 2026-10-20 native audit fixes `EndPaint` accidentally calling `BeginPaint`,
owned window-icon retirement, and two additional misrouted SDK API wrappers.
The reproduced repeated-menu GDI growth now plateaus in the bounded native run.
The editor also supports document-boundary Ctrl/Cmd+Home/End navigation and
Shift selection, including Unicode, folded EOF and viewport following.
The input follow-up anchors Windows IME composition/candidates to the focused
editor, handles UTF-16 surrogate pairs and `WM_UNICHAR`, and verifies actual
Pinyin commit/cancel plus supplementary Unicode save/readback. The text follow-up
adds coverage-checked Windows CJK/emoji font runs with matching drawing/caret
metrics, while retaining ASCII monospace rendering. Native file chooser waits
no longer produce false UI-stall dumps. This is not full IME or complex-script
shaping certification; see [Windows text QA](WINDOWS_TEXT_QA_20261020.md).

| Environment | Latest local evidence | Not established by this check |
| --- | --- | --- |
| macOS arm64 / Go 1.26.5 | 2026-10-20 18-package race regression (4,885 pass / 9 skip), pure-Go rendering and strict real GL readback; later GUI/task-runner deltas retested; earlier desktop QA remains dated evidence | New seven-group menu desktop QA on macOS, every secondary panel action, real gopls/dlv workflows, mixed-DPI multi-monitor behavior |
| Linux arm64 / Go 1.26.0 | Earlier Debian 12 build, four-package tests, vet and non-root Xvfb window-startup smoke; not rerun this iteration | Interactive desktop/IME/font coverage; a portable dependency-free binary |
| Windows 10 22H2 amd64 / Go 1.26.5 / Cairo 1.18.4 | Thirteen-package native and race regression (4,638 pass / 39 skip), pure-Go rendering, race/normal GUI input/save/resize and background-output checks; later native task-tree cancellation regressions; earlier bounded GDI plateau, paint pairing, accelerator dispatch, icon lifetime, layout persistence/resize and F6 build; real Pinyin commit/cancel, Unicode save/readback, visible CJK/emoji/Ext-B glyphs, caret geometry and scaled pixel comparisons | Startup/input-latency certification, complex shaping and raw-glyph rich text fallback, full IME, mixed-DPI/multi-monitor, all widget/menu combinations, real gopls/dlv, physical printing and unbounded resource stability |

Run/Build currently use **saved files on disk**; save edits before running.
IDE symbol actions require `gopls` and report when it is unavailable. Some panels
still have incomplete localization/font fallback. Passing tests or installing
external tools alone is not a full desktop compatibility claim.
The earlier `silk_pure_go` dash, icon color/path and allocation failures are
fixed and its 192-case rendering run passes on macOS and Windows. This does not
certify complete native Cairo parity, complex shaping or every desktop path.

2026-10-22 review/security follow-up: [15-finding fixes and CVE audit](REVIEW_FIXES_20261022.md).
The module now requires **Go 1.26.10**; use a security-patched toolchain
(1.26.10, or at least 1.27.3 on the 1.27 branch). Upgraded Go networking/image/system
modules and a pinned `govulncheck` workflow check for the audited vulnerable versions.
Existing binaries must be rebuilt; the historical platform evidence above is not automatically upgraded.

Detailed, dated verification records:
[workbench](WORKBENCH_QA.md) · [menus and About](MENU_QA.md) ·
[SDK/platform compatibility](SDK_COMPATIBILITY.md) ·
[popup ownership](POPUP_AUDIT.md) · [designer gaps](DESIGNER_GAPS.md) ·
[Windows desktop iteration](WINDOWS_UI_QA.md) ·
[Windows rendering/SDK verification](WINDOWS_RENDERING_QA_20261020.md) ·
[Windows input follow-up](WINDOWS_INPUT_QA_20261020.md).
Local `bin/qa-artifacts/` evidence mentioned in those records is not shipped in Git.

### Rendering performance

Static UI no longer requests frames just to maintain a refresh rate. Ongoing
quiet animation is capped at 25 FPS; input briefly raises the ceiling to 60 FPS,
with extra recovery time after expensive frames. Windows rendering uses a
coalesced deadline timer rather than a paint-to-animation feedback loop;
custom modal dialogs block waiting for messages instead of polling every 1 ms.
Hidden busy widgets stop their animation. Text/modified-state/outline caches
avoid repeated full-document work while idle. These are code-level fixes;
Windows native idle/resource samples are recorded in [Windows UI QA](WINDOWS_UI_QA.md);
**a full before/after CPU reduction or input-latency improvement is not established**.

Windows previously requested macOS's Menlo font and resolved a proportional
fallback, disabling the editor's ASCII measurement fast path. Platform defaults
are now Consolas / Menlo / monospace. On the Windows QA machine, the isolated
48-character measurement benchmark drops from 4.96–5.55 µs and 2 allocations
to 77–97 ns and zero allocations. This is **one hot path**, not whole-IDE FPS;
non-ASCII text and non-monospaced overrides retain native shaping.

See [bounded CPU capture and Windows verification](PERFORMANCE-CAPTURE.md).
Profiling is opt-in, local-only, and stops automatically after 30 seconds.

## Quick Start

```bash
# 1. Clone
git clone https://github.com/uk0/silk.git
cd silk

# 2. Install system dependencies (macOS)
brew install cairo pkg-config

# 3. Build and run the integrated Go designer / IDE
make run-ide

# 4. Or run the widget gallery demo
CGO_CFLAGS="-I/opt/homebrew/include" go run demo.go
```

Drag widgets from the left panel onto the canvas, press **F5** to compile & run.

---

## Development Setup

### Prerequisites

| Dependency | Minimum Version | Purpose |
|------------|----------------|---------|
| **Go** | 1.26+ | Compiler |
| **CGO** | enabled | Required for Cairo |
| **Cairo** | 1.16+ | 2D rendering backend |
| **pkg-config** | any | Find Cairo headers |
| **GLFW 3.3** | auto-downloaded | Window management (macOS/Linux) |

### macOS

```bash
# Install Homebrew (if not installed)
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Install Cairo and pkg-config
brew install cairo pkg-config

# Verify
pkg-config --cflags --libs cairo
# Should output: -I/opt/homebrew/Cellar/cairo/.../include -L/opt/homebrew/Cellar/cairo/.../lib -lcairo
```

If `go build` can't find `cairo/cairo.h`, export the include path:

```bash
export C_INCLUDE_PATH=/opt/homebrew/include
export CGO_CFLAGS="-I/opt/homebrew/include"
export CGO_LDFLAGS="-L/opt/homebrew/lib -lcairo"
```

Add these to your `~/.zshrc` or `~/.bashrc` for persistent use.

### Windows

silk's Cairo bindings need `gcc + pkg-config + cairo` from MSYS2's UCRT64
toolchain. Once those are on `PATH`, `go build` / `go run` / `go test` work
from any of the three common Windows shells. Pick the one you prefer; the
build flow is identical, only the env-var syntax differs.

#### One-time setup

1. Install **MSYS2** from <https://www.msys2.org/>.
2. Open the **MSYS2 UCRT64** shell (Start → "MSYS2 UCRT64").
3. Install the toolchain:
   ```bash
   pacman -S mingw-w64-ucrt-x86_64-gcc \
             mingw-w64-ucrt-x86_64-pkgconf \
             mingw-w64-ucrt-x86_64-cairo
   ```

#### Build & run from each shell

**MSYS2 UCRT64** (recommended — `PATH` is already set):
```bash
cd /c/path/to/silk
go build -o silkide.exe ./cmd/silkide/
./silkide.exe
go test ./gui/ ./ged/ ./paint/ ./graph/ ./geom/
```

**PowerShell**:
```powershell
$env:PATH = "C:\msys64\ucrt64\bin;$env:PATH"
cd C:\path\to\silk
go build -o silkide.exe .\cmd\silkide\
.\silkide.exe
go test .\gui\ .\ged\ .\paint\ .\graph\ .\geom\
```

**Command Prompt (cmd.exe)**:
```cmd
set PATH=C:\msys64\ucrt64\bin;%PATH%
cd C:\path\to\silk
go build -o silkide.exe .\cmd\silkide\
silkide.exe
go test .\gui\ .\ged\ .\paint\ .\graph\ .\geom\
```

#### Verify

```bash
go env CGO_ENABLED   # 1
gcc --version        # ucrt64 gcc, 13.x or newer
pkg-config --modversion cairo  # 1.18.x
```

If `go build` errors with `'cairo.h' file not found`, the UCRT64 `bin/`
isn't on `PATH` — re-run the env-var line for your shell. The `win32`
package is compile-tagged Windows-only so it only enters the build on
this platform; on macOS/Linux it's a no-op.

### Linux (Ubuntu/Debian)

```bash
sudo apt update
sudo apt install -y \
    build-essential \
    pkg-config \
    libcairo2-dev \
    libx11-dev \
    libxcursor-dev \
    libxi-dev \
    libxinerama-dev \
    libxrandr-dev \
    libxxf86vm-dev \
    libgl1-mesa-dev

# Verify
pkg-config --libs cairo
```

### Linux (Fedora/RHEL)

```bash
sudo dnf install -y \
    gcc pkgconfig \
    cairo-devel \
    libX11-devel libXcursor-devel libXi-devel \
    libXinerama-devel libXrandr-devel libXxf86vm-devel \
    mesa-libGL-devel
```

---

## Project Structure

```
silk/
├── core/             Foundation layer
│   ├── factory.go        Object creation from registered types
│   ├── signal-slot.go    Event binding mechanism
│   ├── tdoc.go           Tree-structured persistence
│   └── shell.go          Platform file paths
│
├── gui/              Widgets + theming + layout engine
│   ├── widget.go         Base widget class
│   ├── button.go, label.go, edit.go, ...
│   ├── hbox.go, vbox.go, gridlayout.go   Layout containers
│   ├── theme.go          Color schemes
│   ├── animation.go      12 easing functions
│   ├── codeeditor.go     Full-featured code editor (~3,300 lines)
│   ├── formloader.go     SDK-level .silkui loader
│   ├── window_glfw.go    macOS/Linux window backend
│   └── window_windows.go Windows window backend
│
├── graph/            Scene graph for design canvas
│   ├── view.go           Graph view with zoom/pan
│   ├── tool.go           Interaction tools
│   └── resize-decor.go   Resize handles
│
├── ged/              Visual designer (GUI Editor)
│   ├── ged-view.go       Design canvas
│   ├── codegen.go        Go code generation
│   ├── code-panel.go     Event handler editor
│   ├── file-explorer.go  Project file tree
│   ├── editor-tabs.go    Multi-tab editor
│   ├── build-output.go   Compile error navigation
│   └── ... (25+ designer panels)
│
├── paint/            Cairo rendering abstraction
├── cairo/            Cairo C bindings (CGO)
├── geom/             2D vectors, matrices, rectangles
├── prop/             Property system
│
├── examples/         Runnable examples
│   ├── calculator/       Calculator app
│   ├── dashboard/        Charts & data binding
│   ├── todoapp/          Todo list
│   ├── texteditor/       Basic text editor
│   ├── showcase/         Widget gallery demo
│   └── load_silkui/      SDK loader example
│
├── icon/             PNG icons (4 sizes × 64 icons)
│
├── design.go         Visual designer entry point
├── demo.go           Widget gallery demo
└── sandbox.go        Test sandbox
```

### Module Layout

```go
// go.mod
module github.com/uk0/silk
```

Internal imports use: `github.com/uk0/silk/core`, `github.com/uk0/silk/gui`, `github.com/uk0/silk/ged`, etc.

---

## 组态 / SCADA & Industrial Platform

Beyond the widget toolkit, Silk ships a full industrial-automation (组态) stack
built on a real-time tag database:

- **Field-bus drivers** — Modbus TCP, Siemens S7, OPC-UA and MQTT, covering every
  PLC data type and all four register/byte orders (ABCD/DCBA/BADC/CDAB), read-only
  or read-write. Wrap two in a **redundant** driver for primary/backup failover, or
  bridge protocols with the **gateway**. A **simulator** driver runs screens without
  hardware.
- **Tags & bindings** — `core.TagDB` streams device values into widgets via
  value-driven bindings, animation, alarms and rolling trends. Configure a device and
  its tag points visually with `DeviceComponent`, or stamp many devices from a
  structured **template**.
- **Data & logic** — **historian** (SQLite history), **reports** (interval
  aggregation → CSV/HTML), **trend playback**, **recipes**, **calc/formula** tags,
  **event log**, live **statistics**, runtime **Go scripting**, and **user auth** with
  login sessions.

```go
tags := core.NewTagDB()
dev := device.NewDeviceComponent()          // Modbus / S7 / OPC-UA / MQTT
dev.SetProtocol("modbus")
dev.SetHost("192.168.0.10")
dev.SetPoints("level, hr:0, Float32, ABCD, RO\npump, coil:0, Bool, ABCD, RW")
dev.Start(tags)                             // poll device -> tags -> screen
```

The `silkide` designer/IDE adds LSP (gopls) code intelligence, a Delve debugger, and
Qt Creator-style **locator**, **find-in-files**, **snippets** and **build-issue**
navigation.

---

## Your First App

### Option 1: Pure SDK (No Designer)

```go
package main

import (
    "github.com/uk0/silk/core"
    "github.com/uk0/silk/gui"
)

func main() {
    // Create main frame
    f := gui.NewFrameWindow()
    f.SetTitle("My First Silk App")
    gui.SetDefaultFrame(f)

    // Create a form with widgets
    form := gui.NewForm()
    form.SetTitle("Hello")

    btn := gui.NewButton1("Click Me", nil)
    btn.SetParent(form)
    btn.SetBounds(20, 20, 100, 30)
    btn.Action().BindFunc0(func() {
        gui.ShowMessageDialog(f, "Hi", "Hello, World!")
    })

    // Attach and show
    f.SuggestDocDock().AddView(form)
    f.SetClosedCallback(func(*gui.Frame) { core.Quit() })
    if w := f.Window(); w != nil {
        w.SetSize(400, 300)
        w.MoveToCenter()
    }
    f.Show()
    core.EventLoop()
}
```

Save as `hello.go` and run:

```bash
CGO_CFLAGS="-I/opt/homebrew/include" go run hello.go
```

### Option 2: Load from Designer File

Design your form visually in the designer, save as `main.silkui`, then:

```go
package main

import (
    "github.com/uk0/silk/core"
    "github.com/uk0/silk/gui"
    "log"
)

func main() {
    // Load design file — produced by the visual designer
    form, err := gui.LoadForm("main.silkui")
    if err != nil {
        log.Fatal(err)
    }

    f := gui.NewFrameWindow()
    gui.SetDefaultFrame(f)
    f.SuggestDocDock().AddView(form)
    f.SetClosedCallback(func(*gui.Frame) { core.Quit() })
    f.Show()
    core.EventLoop()
}
```

No designer code needed at runtime — just `github.com/uk0/silk/core` + `github.com/uk0/silk/gui`.

---

## Using the Designer

### Launch

```bash
CGO_CFLAGS="-I/opt/homebrew/include" go run design.go
```

### Two Modes

| Mode | Shortcut | Purpose |
|------|----------|---------|
| **Design Mode** | `Ctrl+1` | Drag widgets, edit properties, visual layout |
| **Code Mode** | `Ctrl+2` | File explorer, multi-tab code editor |

### Design Workflow

1. **Drag** a widget from the left palette onto the canvas
2. **Click** the widget to see/edit properties on the right
3. **Double-click** to open the event handler code editor
4. Press **F5** to compile and run your app
5. Press **Ctrl+R** for quick preview (no compile)

### Code Workflow

1. **Ctrl+2** to enter Code Mode
2. **Ctrl+P** to quick-open any file
3. **Cmd/Ctrl+Click** on a function → go to definition
4. **Ctrl+Shift+O** → symbol navigation
5. **Ctrl+Shift+F** → format code (gofmt)
6. **F5** → compile, errors shown with clickable navigation

### Full Shortcut Reference

See **Help → Keyboard Shortcuts** in the designer for all 40+ shortcuts.

---

## Building & Running

### Running Examples

```bash
# Set CGO flags once per shell session
export CGO_CFLAGS="-I/opt/homebrew/include"
export CGO_LDFLAGS="-L/opt/homebrew/lib -lcairo"

# Then run any example
go run examples/calculator/main.go
go run examples/dashboard/main.go
go run examples/showcase/main.go
```

Note: Most examples use `//go:build ignore` — they're standalone programs, not part of the package build.

### Building a Release Binary

```bash
go build -v -o myapp hello.go

# Smaller binary (strip debug info)
go build -ldflags="-s -w" -o myapp hello.go

# Cross-compile (static linking may require extra setup)
GOOS=linux GOARCH=amd64 go build -o myapp hello.go
```

### Running Tests

```bash
# All tests
go test -short ./...

# Specific package with verbose output
go test -v ./gui/

# Benchmarks
go test -bench=. -benchmem ./gui/
```

Current test suite: **398+ tests**, 100% pass rate.

### Development Cycle

```bash
# 1. Edit source files
vim gui/button.go

# 2. Build to check
go build ./gui/

# 3. Run relevant tests
go test ./gui/

# 4. Launch designer to verify visually
go run design.go
```

---

## Features

### Built-in Widget Families

- **Input** (15): Button, Edit, CheckBox, RadioButton, ComboBox, SpinBox, Slider, ToggleSwitch, SearchBox, NumberInput, DatePicker, ColorPicker, Rating, DropdownButton, SwitchGroup
- **Display** (12): Label, ProgressBar, GroupBox, ImageView, Tag, Badge, Avatar, Breadcrumb, Link, LabelSeparator, Placeholder, Timeline
- **Layout** (10): VBox, HBox, GridLayout, FormLayout, Splitter, StackedWidget, TabWidget, Card, Accordion, ScrollArea
- **Data** (4): ListWidget, TreeView, Table, NotificationPanel
- **Charts** (5): LineChart, BarChart, PieChart, Gauge, ScatterPlot
- **Window** (6): Form, Dialog, Menu, ToolBar, StatusBar, CodeEditor

### Designer Features

- Smart alignment guides (blue snap lines)
- Ctrl+Scroll zoom, Space+drag pan
- Object inspector, property editor with categories
- Undo/redo with visual history panel
- Code generation for 23+ event types
- Tab order editor, widget locking
- Form size presets (Desktop/Tablet/Phone)
- Theme preview, custom template saving

### Code Editor Features

- Multi-cursor editing (Cmd+Alt+Up/Down)
- Cmd/Ctrl+Click cross-file go-to-definition
- Auto-completion (keywords, types, gui.* API)
- Find/Replace (Ctrl+F), Go to line (Ctrl+G)
- Symbol navigation (Ctrl+Shift+O)
- 14 Go code snippets, bracket matching
- Minimap, bookmarks (Ctrl+B)
- Rename refactoring (F2 in Silk IDE; requires gopls)
- Code formatting via gofmt (Ctrl+Shift+F)
- Error markers with squiggly underlines
- Split editor view (Ctrl+\\)
- Git gutter markers

### IDE Features (silkide)

- **Language server (gopls)** — completion, hover, go-to-definition,
  find-references, rename, format, code actions, signature help, diagnostics,
  plus call hierarchy, type hierarchy, implementations, inlay hints, semantic
  tokens and code lens
- **Debugger (Delve)** — breakpoints with conditions/hit counts/logpoints,
  goroutine- and frame-scoped locals, arguments and watches, lazily expanded
  variables, and a debug console
- **Build & run** — kits (toolchain, GOOS/GOARCH, tags, race/coverage, deploy
  profile), multiple named run/debug configurations, and a cancellable task
  runner with streaming output
- **Testing** — `go test -json` driven results in a package → test → subtest
  explorer with run/debug/rerun-failed and gutter actions
- **Analyzers** — vet, race, coverage, pprof, trace, govulncheck, staticcheck
- **Version control** — branches, remotes, fetch/pull/push, stash, rebase,
  cherry-pick, staged/unstaged/conflict grouping, hunk-level diff staging and a
  three-way merge editor
- **Navigation & search** — fuzzy quick-open, project-wide find/replace with
  regex and transactional preview, grouped references, outline, bookmarks and a
  live TODO index
- **Terminal** — PTY-backed shell session with a full ANSI screen (Unix; ConPTY
  pending on Windows)

---

## Troubleshooting

### `cairo/cairo.h` not found

Set the include path:

```bash
# macOS
export CGO_CFLAGS="-I/opt/homebrew/include"
export CGO_LDFLAGS="-L/opt/homebrew/lib -lcairo"

# Linux
export CGO_CFLAGS="-I/usr/include/cairo"
```

### Icons show as red X

The designer loads icons from the `./icon/` directory. Run from the project root:

```bash
cd /path/to/silk
go run design.go   # from the directory containing icon/
```

### "Duplicate libraries -lcairo" warning

Safe to ignore — it's a linker hint, not an error.

### Windows: "The application was unable to start correctly (0xc000007b)"

Ensure you're using the MSYS2 UCRT64 shell or have `C:\msys64\ucrt64\bin` in PATH so Cairo DLLs are found at runtime.

### F5 compile fails with "gofmt not found"

Install Go's standard tools (usually included with Go, but verify):

```bash
which gofmt
# Should print the gofmt path
```

---

## Cross-Platform Support

| Platform | Window Backend | Rendering | Status |
|----------|---------------|-----------|--------|
| macOS    | GLFW + OpenGL | Cairo     | ✅ Primary |
| Windows  | Win32 native  | Cairo     | ✅ Supported |
| Linux    | GLFW + OpenGL | Cairo     | ✅ Supported |

---

## Tech Stack

- **Go 1.21+** — compiler
- **Cairo 2D** — 2D rendering
- **GLFW 3.3** — macOS/Linux window management
- **Win32 API** — Windows window management
- **OpenGL 2.1** — texture upload for back-buffer composition
- **Zero external Go GUI dependencies** — everything built from scratch

---

## Contributing

```bash
# 1. Fork on GitHub
# 2. Clone your fork
git clone https://github.com/YOUR_USERNAME/silk.git
cd silk

# 3. Create a branch
git checkout -b feature/my-feature

# 4. Make changes and test
go test -short ./...

# 5. Commit (no signatures in messages)
git commit -m "Add feature X"

# 6. Push and open a Pull Request
git push origin feature/my-feature
```

---

## File Format

Design files use the `.silkui` extension (TDoc-based tree format). Legacy `.cml`, `.silk`, `.form` files are still accepted on load for backwards compatibility.

Gantt chart regression commands, a reproducible `.silkui` fixture and desktop
verification steps are documented in [GANTT-TESTING.md](GANTT-TESTING.md).

---

## License

AGPL-3.0

---

*Silk — making Go desktop development silky smooth.*
