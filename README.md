# Silk — Cross-Platform Go UI Framework & Visual Designer

A complete Go-native cross-platform GUI framework with **62+ widgets**, **visual form designer**, and **integrated IDE**. Design your UI visually, generate Go code, compile, and run — all in one tool.

---

## Table of Contents

- [Quick Start](#quick-start) — get running in 5 minutes
- [Development Setup](#development-setup) — prerequisites by OS
- [Project Structure](#project-structure) — what lives where
- [Your First App](#your-first-app) — hello world with the SDK
- [Using the Designer](#using-the-designer) — visual form design
- [Building & Running](#building--running) — all the common commands
- [Features](#features) — what's included
- [Troubleshooting](#troubleshooting) — common issues

---

## Quick Start

```bash
# 1. Clone
git clone https://github.com/uk0/silk.git
cd silk

# 2. Install system dependencies (macOS)
brew install cairo pkg-config

# 3. Run the visual designer
CGO_CFLAGS="-I/opt/homebrew/include" go run design.go

# 4. Or run the widget gallery demo
CGO_CFLAGS="-I/opt/homebrew/include" go run demo.go
```

Drag widgets from the left panel onto the canvas, press **F5** to compile & run.

---

## Renderer Backends

Silk supports two rendering backends:

- **Cairo** (default, `main` branch): mature CPU-rasterized 2D rendering uploaded to GPU once per frame. Battle-tested.
- **glui** (experimental, `opengl` branch): pure OpenGL 2D pipeline with shader-based SDF, glyph atlas, and zero-allocation hot paths. Activate with `SILK_GLUI=1`. See [glui/README.md](glui/README.md).

The Cairo path remains production. glui is in active development; the same widget code runs on both backends without modification.

---

## Development Setup

### Prerequisites

| Dependency | Minimum Version | Purpose |
|------------|----------------|---------|
| **Go** | 1.21+ | Compiler |
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

```powershell
# Install MSYS2 from https://www.msys2.org/
# In MSYS2 MinGW64 shell:
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-pkg-config mingw-w64-x86_64-cairo

# Add MinGW64 to PATH:
set PATH=C:\msys64\mingw64\bin;%PATH%

# Verify CGO works:
go env CGO_ENABLED
# Should output: 1
```

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
├── gui/              62+ widgets + theming + layout engine
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
│   ├── showcase/         All 62 widgets demo
│   └── load_silkui/      SDK loader example
│
├── icon/             PNG icons (4 sizes × 46 icons)
│
├── design.go         Visual designer entry point
├── demo.go           Widget gallery demo
└── sandbox.go        Test sandbox
```

### Module Layout

```go
// go.mod
module silk
```

Internal imports use: `silk/core`, `silk/gui`, `silk/ged`, etc.

---

## Your First App

### Option 1: Pure SDK (No Designer)

```go
package main

import (
    "silk/core"
    "silk/gui"
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
    "silk/core"
    "silk/gui"
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

No designer code needed at runtime — just `silk/core` + `silk/gui`.

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

### 62 Built-in Widgets

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
- Rename refactoring (Ctrl+Shift+R)
- Code formatting via gofmt (Ctrl+Shift+F)
- Error markers with squiggly underlines
- Split editor view (Ctrl+\\)
- Git gutter markers

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

Ensure you're using the MSYS2 MinGW64 shell or have `C:\msys64\mingw64\bin` in PATH so Cairo DLLs are found at runtime.

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

---

## License

AGPL-3.0

---

*Silk — making Go desktop development silky smooth.*
